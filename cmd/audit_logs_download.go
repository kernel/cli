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
	Format        string
	Force         bool
}

const (
	// auditLogsDownloadMaxRange mirrors the API's max export window.
	auditLogsDownloadMaxRange = 30 * 24 * time.Hour
	// auditLogsDownloadShaRetries is the number of extra attempts for a chunk
	// whose body fails checksum verification.
	auditLogsDownloadShaRetries = 2
)

// auditLogsDownloadState is the sidecar file that makes a download resumable.
// It is written atomically after every committed chunk and removed on
// completion. Version guards future format changes (e.g. multi-window splits).
type auditLogsDownloadState struct {
	Version      int    `json:"version"`
	Params       string `json:"params"`
	Cursor       string `json:"cursor"`
	BytesWritten int64  `json:"bytes_written"`
	Chunks       int    `json:"chunks"`
	Rows         int64  `json:"rows"`
}

const auditLogsDownloadStateVersion = 1

func (c AuditLogsCmd) Download(ctx context.Context, in AuditLogsDownloadInput) error {
	params, err := buildAuditLogsDownloadParams(in)
	if err != nil {
		return err
	}

	format := in.Format
	if format == "" {
		format = string(kernel.AuditLogExportChunkParamsFormatJSONLGz)
	}
	switch format {
	case string(kernel.AuditLogExportChunkParamsFormatJSONL), string(kernel.AuditLogExportChunkParamsFormatJSONLGz):
		params.Format = kernel.AuditLogExportChunkParamsFormat(format)
	default:
		return fmt.Errorf("--format must be jsonl or jsonl.gz")
	}

	outPath := in.To
	if outPath == "" {
		outPath = defaultAuditLogsDownloadPath(params.Start, params.End, format)
	}
	statePath := outPath + ".state.json"
	fingerprint, err := auditLogsDownloadFingerprint(params)
	if err != nil {
		return err
	}

	state, err := loadAuditLogsDownloadState(statePath, outPath, fingerprint, in.Force)
	if err != nil {
		return err
	}
	// A state file with chunks but no cursor means the download finished and
	// only the cleanup was interrupted.
	if state.Chunks > 0 && state.Cursor == "" {
		if err := os.Remove(statePath); err != nil {
			return fmt.Errorf("remove state file: %w", err)
		}
		pterm.Success.Printf("Download already complete: %d rows (%s) in %s\n", state.Rows, util.FormatBytes(state.BytesWritten), outPath)
		return nil
	}

	out, err := openAuditLogsDownloadOutput(outPath, state.BytesWritten)
	if err != nil {
		return err
	}
	defer out.Close()

	if state.Chunks > 0 {
		pterm.Info.Printf("Resuming download at chunk %d (%d rows so far)\n", state.Chunks+1, state.Rows)
	}

	for {
		if state.Cursor != "" {
			params.Cursor = kernel.String(state.Cursor)
		}
		body, header, err := c.fetchAuditLogsChunk(ctx, params)
		if err != nil {
			if state.Chunks > 0 {
				pterm.Info.Println("Progress saved; rerun the same command to resume")
			}
			return err
		}

		if _, err := out.Write(body); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		if err := out.Sync(); err != nil {
			return fmt.Errorf("sync %s: %w", outPath, err)
		}

		rows, _ := strconv.ParseInt(header.Get("X-Row-Count"), 10, 64)
		state.Chunks++
		state.Rows += rows
		state.BytesWritten += int64(len(body))

		hasMore, _ := strconv.ParseBool(header.Get("X-Has-More"))
		nextCursor := header.Get("X-Next-Cursor")
		if hasMore {
			// Guard against a server bug looping the download forever.
			if nextCursor == "" {
				return fmt.Errorf("server reported more records but returned no cursor; retry, and report this if it persists")
			}
			if nextCursor == state.Cursor {
				return fmt.Errorf("server returned an unchanged cursor; retry, and report this if it persists")
			}
		}
		state.Cursor = nextCursor
		state.Params = fingerprint
		if err := saveAuditLogsDownloadState(statePath, state); err != nil {
			return err
		}

		pterm.Info.Printf("Chunk %d: %d rows (%d total, %s)\n", state.Chunks, rows, state.Rows, util.FormatBytes(state.BytesWritten))
		if !hasMore {
			break
		}
	}

	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove state file: %w", err)
	}
	pterm.Success.Printf("Downloaded %d rows (%s) to %s\n", state.Rows, util.FormatBytes(state.BytesWritten), outPath)
	return nil
}

// fetchAuditLogsChunk downloads one chunk and verifies it against the
// X-Content-Sha256 trailer before returning it, retrying on mismatch. Nothing
// is written to disk until a chunk verifies.
func (c AuditLogsCmd) fetchAuditLogsChunk(ctx context.Context, params kernel.AuditLogExportChunkParams) ([]byte, http.Header, error) {
	var lastErr error
	for attempt := 0; attempt <= auditLogsDownloadShaRetries; attempt++ {
		res, err := c.auditLogs.ExportChunk(ctx, params)
		if err != nil {
			return nil, nil, util.CleanedUpSdkError{Err: err}
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read chunk body: %w", err)
			continue
		}
		if want := res.Header.Get("X-Content-Sha256"); want != "" {
			sum := sha256.Sum256(body)
			if got := hex.EncodeToString(sum[:]); got != want {
				lastErr = fmt.Errorf("chunk checksum mismatch (got %s, want %s)", got, want)
				continue
			}
		}
		return body, res.Header, nil
	}
	return nil, nil, lastErr
}

func buildAuditLogsDownloadParams(in AuditLogsDownloadInput) (kernel.AuditLogExportChunkParams, error) {
	var params kernel.AuditLogExportChunkParams
	var err error
	start := time.Now().UTC().Add(-24 * time.Hour)
	if in.Start != "" {
		start, _, err = parseAuditLogTime(in.Start)
		if err != nil {
			return params, fmt.Errorf("--start: %w", err)
		}
	}
	end := time.Now().UTC()
	if in.End != "" {
		var dateOnly bool
		end, dateOnly, err = parseAuditLogTime(in.End)
		if err != nil {
			return params, fmt.Errorf("--end: %w", err)
		}
		if dateOnly {
			end = end.Add(24 * time.Hour)
		}
	}
	if !start.Before(end) {
		return params, fmt.Errorf("--start must be before --end")
	}
	if end.Sub(start) > auditLogsDownloadMaxRange {
		// The window list goes through pterm because the error renderer
		// collapses newlines, which would mangle the copy-pasteable flags.
		pterm.Info.Printf("Run one download per window:\n%s", suggestAuditLogsDownloadWindows(start, end))
		return params, fmt.Errorf("range is %d days; the API allows at most 30 days per download", int(end.Sub(start).Hours()/24))
	}

	params.Start = start
	params.End = end
	if in.Search != "" {
		params.Search = kernel.String(in.Search)
	}
	if in.Method != "" {
		params.Method = kernel.String(in.Method)
	}
	// GETs are excluded by default, like search. The API accepts a single
	// exclude_method, and chunks are appended verbatim, so a second exclusion
	// can't be filtered client-side the way search does; require --include-get
	// so the default is never dropped silently.
	excludeMethod := in.ExcludeMethod
	if in.Method == "" && !in.IncludeGet {
		if excludeMethod == "" {
			excludeMethod = "GET"
		} else if !strings.EqualFold(excludeMethod, "GET") {
			return params, fmt.Errorf("--exclude-method %s would replace the default GET exclusion; add --include-get to confirm GET requests should be included", in.ExcludeMethod)
		}
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

// suggestAuditLogsDownloadWindows renders ready-to-run 30-day windows covering
// [start, end), newest first to match the export's record order.
func suggestAuditLogsDownloadWindows(start, end time.Time) string {
	var out string
	for windowEnd := end; windowEnd.After(start); {
		windowStart := windowEnd.Add(-auditLogsDownloadMaxRange)
		if windowStart.Before(start) {
			windowStart = start
		}
		out += fmt.Sprintf("  --start %s --end %s\n", windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339))
		windowEnd = windowStart
	}
	return out
}

// auditLogsDownloadFingerprint identifies the query a state file belongs to,
// so a resume never mixes records from different queries in one output file.
// It must be computed before the cursor is set on params.
func auditLogsDownloadFingerprint(params kernel.AuditLogExportChunkParams) (string, error) {
	q, err := params.URLQuery()
	if err != nil {
		return "", fmt.Errorf("fingerprint params: %w", err)
	}
	return q.Encode(), nil
}

func defaultAuditLogsDownloadPath(start, end time.Time, format string) string {
	const stamp = "20060102T150405Z"
	return fmt.Sprintf("audit-logs-%s-%s.%s", start.UTC().Format(stamp), end.UTC().Format(stamp), format)
}

// loadAuditLogsDownloadState decides how a download starts: fresh, resumed
// from a matching state file, or rejected because the output would be
// clobbered or the state belongs to a different query.
func loadAuditLogsDownloadState(statePath, outPath, fingerprint string, force bool) (auditLogsDownloadState, error) {
	fresh := auditLogsDownloadState{Version: auditLogsDownloadStateVersion, Params: fingerprint}
	raw, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		if !force {
			if _, statErr := os.Stat(outPath); statErr == nil {
				return fresh, fmt.Errorf("%s already exists; pass --force to overwrite or -o to pick another path", outPath)
			}
		}
		return fresh, nil
	}
	if err != nil {
		return fresh, fmt.Errorf("read state file: %w", err)
	}
	if force {
		return fresh, nil
	}
	var state auditLogsDownloadState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fresh, fmt.Errorf("state file %s is corrupt; pass --force to start over", statePath)
	}
	if state.Version != auditLogsDownloadStateVersion {
		return fresh, fmt.Errorf("state file %s was written by an incompatible CLI version; pass --force to start over", statePath)
	}
	if state.Params != fingerprint {
		return fresh, fmt.Errorf("state file %s belongs to a download with different parameters; pass --force to start over or -o to pick another path", statePath)
	}
	return state, nil
}

func saveAuditLogsDownloadState(statePath string, state auditLogsDownloadState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	if err := os.Rename(tmp, statePath); err != nil {
		return fmt.Errorf("commit state file: %w", err)
	}
	return nil
}

// openAuditLogsDownloadOutput opens the output for appending, truncated to the
// last committed offset so a chunk interrupted mid-write is dropped rather
// than duplicated on resume.
func openAuditLogsDownloadOutput(outPath string, committed int64) (*os.File, error) {
	if dir := filepath.Dir(outPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create output directory: %w", err)
		}
	}
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", outPath, err)
	}
	if err := out.Truncate(committed); err != nil {
		out.Close()
		return nil, fmt.Errorf("truncate %s to last committed chunk: %w", outPath, err)
	}
	if _, err := out.Seek(committed, io.SeekStart); err != nil {
		out.Close()
		return nil, fmt.Errorf("seek %s: %w", outPath, err)
	}
	return out, nil
}

func runAuditLogsDownload(cmd *cobra.Command, args []string) error {
	c := getAuditLogsHandler(cmd)
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
	format, _ := cmd.Flags().GetString("format")
	force, _ := cmd.Flags().GetBool("force")

	return c.Download(cmd.Context(), AuditLogsDownloadInput{
		Start:         start,
		End:           end,
		Search:        search,
		Method:        method,
		ExcludeMethod: excludeMethod,
		IncludeGet:    includeGet,
		Service:       service,
		AuthStrategy:  authStrategy,
		UserIDs:       userIDs,
		To:            to,
		Format:        format,
		Force:         force,
	})
}

var auditLogsDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download audit logs for a time range to a file",
	Long: "Download an organization's audit log records for a time range as a file, for archival, compliance, or offline analysis.\n\n" +
		"Records are fetched in chunks and appended after each chunk is verified, newest first. If a download is interrupted, rerunning the same command resumes where it left off.\n\n" +
		"GET requests are excluded by default; pass --include-get to include them, or --method GET to see only them.\n\n" +
		"The API allows at most 30 days per download; for longer ranges run one download per window.",
	Args: cobra.NoArgs,
	RunE: runAuditLogsDownload,
}

func init() {
	auditLogsDownloadCmd.Flags().String("start", "", "Start of the export window, RFC3339 or YYYY-MM-DD (default: 24 hours ago)")
	auditLogsDownloadCmd.Flags().String("end", "", "End of the export window, RFC3339 or YYYY-MM-DD inclusive (default: now)")
	auditLogsDownloadCmd.Flags().String("search", "", "Free-text search")
	auditLogsDownloadCmd.Flags().String("method", "", "Filter by HTTP method (e.g. GET)")
	auditLogsDownloadCmd.Flags().String("exclude-method", "", "Exclude an HTTP method")
	auditLogsDownloadCmd.Flags().Bool("include-get", false, "Include GET requests, which are excluded by default")
	auditLogsDownloadCmd.Flags().String("service", "", "Filter by service")
	auditLogsDownloadCmd.Flags().String("auth-strategy", "", "Filter by authentication strategy")
	auditLogsDownloadCmd.Flags().StringArray("user-id", nil, "Filter by user ID (repeatable)")
	auditLogsDownloadCmd.Flags().String("to", "", "Output file path (default: audit-logs-<start>-<end>.<format>)")
	auditLogsDownloadCmd.Flags().String("format", "jsonl.gz", "Output format: jsonl or jsonl.gz")
	auditLogsDownloadCmd.Flags().Bool("force", false, "Overwrite the output file and ignore saved progress")

	auditLogsCmd.AddCommand(auditLogsDownloadCmd)
}
