package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/auth"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type AuditLogsDownloadInput struct {
	Start         string
	End           string
	Search        string
	Method        string
	ExcludeMethod string
	IncludeGet    bool
	Service       string
	AuthStrategy  string
	UserIDs       []string
	To            string
	Force         bool
}

const auditLogsDownloadMaxRange = 30 * 24 * time.Hour

type auditLogsDownloadState struct {
	Params       string `json:"params"`
	Cursor       string `json:"cursor"`
	BytesWritten int64  `json:"bytes_written"`
	Chunks       int    `json:"chunks"`
	Rows         int64  `json:"rows"`
}

func (c AuditLogsCmd) Download(ctx context.Context, in AuditLogsDownloadInput) error {
	params, err := buildAuditLogsDownloadParams(in)
	if err != nil {
		return err
	}

	fingerprint, err := auditLogsDownloadFingerprint(params, c.downloadIdentity)
	if err != nil {
		return err
	}
	outPath := in.To
	if outPath == "" {
		outPath = defaultAuditLogsDownloadPath(params.Start, params.End, fingerprint)
	}
	partialPath := outPath + ".partial"
	statePath := outPath + ".state.json"
	state, exists, err := loadAuditLogsDownloadState(statePath, partialPath, outPath, fingerprint, in.Force)
	if err != nil {
		return err
	}
	if !exists {
		if err := saveAuditLogsDownloadState(statePath, state); err != nil {
			return err
		}
	}

	if state.Chunks > 0 && state.Cursor == "" {
		if err := commitAuditLogsDownloadOutput(partialPath, outPath, state.BytesWritten); err != nil {
			return err
		}
		if err := removeAuditLogsDownloadState(statePath); err != nil {
			return err
		}
		pterm.Success.Printf("Download already complete: %d rows (%s) in %s\n", state.Rows, util.FormatBytes(state.BytesWritten), outPath)
		return nil
	}

	out, err := openAuditLogsDownloadOutput(partialPath, state.BytesWritten)
	if err != nil {
		return err
	}
	defer func() {
		if out != nil {
			out.Close()
		}
	}()

	if state.Chunks > 0 {
		pterm.Info.Printf("Resuming download at chunk %d (%d rows so far)\n", state.Chunks+1, state.Rows)
	}

	for {
		if state.Cursor != "" {
			params.Cursor = kernel.String(state.Cursor)
		}
		body, header, err := c.fetchAuditLogsChunk(ctx, params)
		if err != nil {
			return err
		}
		rows, nextCursor, hasMore, err := parseAuditLogsChunkHeaders(header, state.Cursor)
		if err != nil {
			return err
		}

		if _, err := out.Write(body); err != nil {
			return fmt.Errorf("write %s: %w", partialPath, err)
		}
		if err := out.Sync(); err != nil {
			return fmt.Errorf("sync %s: %w", partialPath, err)
		}

		state.Cursor = nextCursor
		state.BytesWritten += int64(len(body))
		state.Chunks++
		state.Rows += rows
		if err := saveAuditLogsDownloadState(statePath, state); err != nil {
			return err
		}
		pterm.Info.Printf("Chunk %d: %d rows (%d total, %s)\n", state.Chunks, rows, state.Rows, util.FormatBytes(state.BytesWritten))
		if !hasMore {
			break
		}
	}

	closeErr := out.Close()
	out = nil
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", partialPath, closeErr)
	}
	if err := commitAuditLogsDownloadOutput(partialPath, outPath, state.BytesWritten); err != nil {
		return err
	}
	if err := removeAuditLogsDownloadState(statePath); err != nil {
		return err
	}
	pterm.Success.Printf("Downloaded %d rows (%s) to %s\n", state.Rows, util.FormatBytes(state.BytesWritten), outPath)
	return nil
}

func (c AuditLogsCmd) fetchAuditLogsChunk(ctx context.Context, params kernel.AuditLogExportChunkParams) ([]byte, http.Header, error) {
	res, err := c.auditLogs.ExportChunk(ctx, params)
	if err != nil {
		return nil, nil, util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read chunk body: %w", err)
	}
	want := res.Header.Get("X-Content-Sha256")
	if want == "" {
		return nil, nil, fmt.Errorf("response missing X-Content-Sha256 header")
	}
	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, nil, fmt.Errorf("chunk checksum mismatch (got %s, want %s)", got, want)
	}
	return body, res.Header, nil
}

func parseAuditLogsChunkHeaders(header http.Header, currentCursor string) (int64, string, bool, error) {
	hasMore, err := strconv.ParseBool(header.Get("X-Has-More"))
	if err != nil {
		return 0, "", false, fmt.Errorf("response missing or invalid X-Has-More header")
	}
	rows, err := strconv.ParseInt(header.Get("X-Row-Count"), 10, 64)
	if err != nil || rows < 0 {
		return 0, "", false, fmt.Errorf("response missing or invalid X-Row-Count header")
	}
	nextCursor := header.Get("X-Next-Cursor")
	if hasMore && (nextCursor == "" || nextCursor == currentCursor) {
		return 0, "", false, fmt.Errorf("response has invalid X-Next-Cursor header")
	}
	if !hasMore && nextCursor != "" {
		return 0, "", false, fmt.Errorf("response returned a cursor after the final chunk")
	}
	return rows, nextCursor, hasMore, nil
}

func buildAuditLogsDownloadParams(in AuditLogsDownloadInput) (kernel.AuditLogExportChunkParams, error) {
	var params kernel.AuditLogExportChunkParams
	if in.Start == "" || in.End == "" {
		return params, fmt.Errorf("--start and --end are required")
	}
	start, _, err := parseAuditLogTime(in.Start)
	if err != nil {
		return params, fmt.Errorf("--start: %w", err)
	}
	end, _, err := parseAuditLogTime(in.End)
	if err != nil {
		return params, fmt.Errorf("--end: %w", err)
	}
	if !start.Before(end) {
		return params, fmt.Errorf("--start must be before --end")
	}
	if end.Sub(start) > auditLogsDownloadMaxRange {
		return params, fmt.Errorf("the API allows at most 30 days per download")
	}

	params.Start = start
	params.End = end
	params.Format = kernel.AuditLogExportChunkParamsFormatJSONLGz
	if in.Search != "" {
		params.Search = kernel.String(in.Search)
	}
	if in.Method != "" {
		params.Method = kernel.String(in.Method)
	}
	excludeMethod := in.ExcludeMethod
	if in.Method == "" && !in.IncludeGet {
		if excludeMethod != "" && !strings.EqualFold(excludeMethod, "GET") {
			return params, fmt.Errorf("add --include-get when excluding a method other than GET")
		}
		excludeMethod = "GET"
	}
	if excludeMethod != "" {
		params.ExcludeMethod = kernel.String(excludeMethod)
	}
	if in.Service != "" {
		params.Service = kernel.String(in.Service)
	}
	if in.AuthStrategy != "" {
		params.AuthStrategy = kernel.String(in.AuthStrategy)
	}
	if len(in.UserIDs) > 0 {
		params.SearchUserID = in.UserIDs
	}
	return params, nil
}

func auditLogsDownloadFingerprint(params kernel.AuditLogExportChunkParams, identity string) (string, error) {
	query, err := params.URLQuery()
	if err != nil {
		return "", fmt.Errorf("fingerprint params: %w", err)
	}
	sum := sha256.Sum256([]byte(query.Encode() + "\n" + identity))
	return hex.EncodeToString(sum[:]), nil
}

func defaultAuditLogsDownloadPath(start, end time.Time, fingerprint string) string {
	const stamp = "20060102"
	return fmt.Sprintf("audit-logs-%s-%s-%s.jsonl.gz", start.UTC().Format(stamp), end.UTC().Format(stamp), fingerprint[:8])
}

func currentAuditLogsDownloadIdentity() string {
	identity := strings.TrimRight(strings.TrimSpace(util.GetBaseURL()), "/")
	if apiKey := os.Getenv("KERNEL_API_KEY"); apiKey != "" {
		return identity + "\napi-key:" + apiKey
	}
	if tokens, err := auth.LoadTokens(); err == nil {
		return identity + "\norg:" + tokens.OrgID
	}
	return identity
}

func loadAuditLogsDownloadState(statePath, partialPath, outPath, fingerprint string, force bool) (auditLogsDownloadState, bool, error) {
	fresh := auditLogsDownloadState{Params: fingerprint}
	if force {
		info, err := os.Lstat(outPath)
		outExists := err == nil
		if outExists {
			if !info.Mode().IsRegular() {
				return fresh, false, fmt.Errorf("%s is not a regular file", outPath)
			}
		} else if !os.IsNotExist(err) {
			return fresh, false, fmt.Errorf("inspect %s: %w", outPath, err)
		}
		if err := resetAuditLogsDownload(partialPath, statePath); err != nil {
			return fresh, false, err
		}
		if outExists {
			if err := os.Rename(outPath, partialPath); err != nil {
				return fresh, false, fmt.Errorf("prepare %s for overwrite: %w", outPath, err)
			}
		}
		return fresh, false, nil
	}
	raw, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		for _, path := range []string{outPath, partialPath} {
			if _, statErr := os.Stat(path); statErr == nil {
				return fresh, false, fmt.Errorf("%s already exists; pass --force to overwrite", path)
			} else if !os.IsNotExist(statErr) {
				return fresh, false, fmt.Errorf("inspect %s: %w", path, statErr)
			}
		}
		return fresh, false, nil
	}
	if err != nil {
		return fresh, false, fmt.Errorf("read state file: %w", err)
	}
	var state auditLogsDownloadState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fresh, false, fmt.Errorf("state file %s is corrupt; pass --force to start over", statePath)
	}
	if state.Params != fingerprint || state.BytesWritten < 0 {
		return fresh, false, fmt.Errorf("state file %s does not match this download; pass --force to start over", statePath)
	}
	return state, true, nil
}

func saveAuditLogsDownloadState(statePath string, state auditLogsDownloadState) error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(statePath), filepath.Base(statePath)+".*")
	if err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write state file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		return fmt.Errorf("commit state file: %w", err)
	}
	return nil
}

func removeAuditLogsDownloadState(statePath string) error {
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove state file: %w", err)
	}
	return nil
}

func resetAuditLogsDownload(paths ...string) error {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}

func openAuditLogsDownloadOutput(outPath string, committed int64) (*os.File, error) {
	if committed > 0 {
		if _, err := os.Stat(outPath); err != nil {
			return nil, fmt.Errorf("state records progress but %s is missing: %w", outPath, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	out, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", outPath, err)
	}
	info, err := out.Stat()
	if err != nil {
		out.Close()
		return nil, fmt.Errorf("inspect %s: %w", outPath, err)
	}
	if !info.Mode().IsRegular() || info.Size() < committed {
		out.Close()
		return nil, fmt.Errorf("%s does not match saved progress", outPath)
	}
	if err := out.Chmod(0o600); err != nil {
		out.Close()
		return nil, fmt.Errorf("secure %s: %w", outPath, err)
	}
	if err := out.Truncate(committed); err != nil {
		out.Close()
		return nil, fmt.Errorf("truncate %s: %w", outPath, err)
	}
	if _, err := out.Seek(committed, io.SeekStart); err != nil {
		out.Close()
		return nil, fmt.Errorf("seek %s: %w", outPath, err)
	}
	return out, nil
}

func commitAuditLogsDownloadOutput(partialPath, outPath string, expected int64) error {
	info, err := os.Stat(partialPath)
	if os.IsNotExist(err) {
		info, err = os.Stat(outPath)
		if err != nil {
			return fmt.Errorf("completed download is missing: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() != expected {
			return fmt.Errorf("%s does not match completed download", outPath)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", partialPath, err)
	}
	if !info.Mode().IsRegular() || info.Size() != expected {
		return fmt.Errorf("%s does not match completed download", partialPath)
	}
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("%s already exists; move it or pass --force to start over", outPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", outPath, err)
	}
	if err := os.Rename(partialPath, outPath); err != nil {
		return fmt.Errorf("finalize %s: %w", outPath, err)
	}
	return nil
}

func runAuditLogsDownload(cmd *cobra.Command, args []string) error {
	c := getAuditLogsHandler(cmd)
	c.downloadIdentity = currentAuditLogsDownloadIdentity()
	start, _ := cmd.Flags().GetString("start")
	end, _ := cmd.Flags().GetString("end")
	search, _ := cmd.Flags().GetString("search")
	method, _ := cmd.Flags().GetString("method")
	excludeMethod, _ := cmd.Flags().GetString("exclude-method")
	includeGet, _ := cmd.Flags().GetBool("include-get")
	service, _ := cmd.Flags().GetString("service")
	authStrategy, _ := cmd.Flags().GetString("auth-strategy")
	userIDs, _ := cmd.Flags().GetStringArray("user-id")
	to, _ := cmd.Flags().GetString("to")
	force, _ := cmd.Flags().GetBool("force")

	return c.Download(cmd.Context(), AuditLogsDownloadInput{
		Start: start, End: end, Search: search, Method: method,
		ExcludeMethod: excludeMethod, IncludeGet: includeGet, Service: service,
		AuthStrategy: authStrategy, UserIDs: userIDs, To: to, Force: force,
	})
}

var auditLogsDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download audit logs as gzip-compressed JSONL",
	Long: "Download audit logs as gzip-compressed JSONL in verified, resumable chunks. The time range is [start, end).\n\n" +
		"GET requests are excluded by default; pass --include-get to include them.\n\n" +
		"Incomplete downloads use an adjacent .partial file until finalization.",
	Args: cobra.NoArgs,
	RunE: runAuditLogsDownload,
}

func init() {
	auditLogsDownloadCmd.Flags().String("start", "", "Start of the export window, RFC3339 or YYYY-MM-DD (required)")
	auditLogsDownloadCmd.Flags().String("end", "", "Exclusive end of the export window, RFC3339 or YYYY-MM-DD (required)")
	auditLogsDownloadCmd.Flags().String("search", "", "Free-text search")
	auditLogsDownloadCmd.Flags().String("method", "", "Filter by HTTP method")
	auditLogsDownloadCmd.Flags().String("exclude-method", "", "Exclude an HTTP method")
	auditLogsDownloadCmd.Flags().Bool("include-get", false, "Include GET requests, which are excluded by default")
	auditLogsDownloadCmd.Flags().String("service", "", "Filter by service")
	auditLogsDownloadCmd.Flags().String("auth-strategy", "", "Filter by authentication strategy")
	auditLogsDownloadCmd.Flags().StringArray("user-id", nil, "Filter by user ID (repeatable)")
	auditLogsDownloadCmd.Flags().String("to", "", "Output .jsonl.gz file path")
	auditLogsDownloadCmd.Flags().Bool("force", false, "Overwrite the output file and ignore saved progress")
	_ = auditLogsDownloadCmd.MarkFlagRequired("start")
	_ = auditLogsDownloadCmd.MarkFlagRequired("end")
	auditLogsCmd.AddCommand(auditLogsDownloadCmd)
}
