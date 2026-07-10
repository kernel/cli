package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Force         bool
}

const auditLogsDownloadMaxRange = 30 * 24 * time.Hour

func (c AuditLogsCmd) Download(ctx context.Context, in AuditLogsDownloadInput) error {
	params, err := buildAuditLogsDownloadParams(in)
	if err != nil {
		return err
	}

	outPath := in.To
	if outPath == "" {
		outPath = defaultAuditLogsDownloadPath(params.Start, params.End)
	}
	partialPath := outPath + ".partial"
	out, err := openAuditLogsDownloadOutput(partialPath, outPath, in.Force)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if out != nil {
			out.Close()
		}
		if !complete {
			os.Remove(partialPath)
		}
	}()

	var cursor string
	var bytesWritten, rows int64
	chunks := 0
	for {
		if cursor != "" {
			params.Cursor = kernel.String(cursor)
		}
		body, header, err := c.fetchAuditLogsChunk(ctx, params)
		if err != nil {
			return err
		}
		chunkRows, nextCursor, hasMore, err := parseAuditLogsChunkHeaders(header, cursor)
		if err != nil {
			return err
		}

		if _, err := out.Write(body); err != nil {
			return fmt.Errorf("write %s: %w", partialPath, err)
		}
		cursor = nextCursor
		bytesWritten += int64(len(body))
		chunks++
		rows += chunkRows
		pterm.Info.Printf("Chunk %d: %d rows (%d total, %s)\n", chunks, chunkRows, rows, util.FormatBytes(bytesWritten))
		if !hasMore {
			break
		}
	}

	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", partialPath, err)
	}
	closeErr := out.Close()
	out = nil
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", partialPath, closeErr)
	}
	if err := commitAuditLogsDownloadOutput(partialPath, outPath, in.Force); err != nil {
		return err
	}
	complete = true
	pterm.Success.Printf("Downloaded %d rows (%s) to %s\n", rows, util.FormatBytes(bytesWritten), outPath)
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

func defaultAuditLogsDownloadPath(start, end time.Time) string {
	const stamp = "20060102"
	return fmt.Sprintf("audit-logs-%s-%s.jsonl.gz", start.UTC().Format(stamp), end.UTC().Format(stamp))
}

func openAuditLogsDownloadOutput(partialPath, outPath string, force bool) (*os.File, error) {
	if info, err := os.Lstat(outPath); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", outPath)
		}
		if !force {
			return nil, fmt.Errorf("%s already exists; pass --force to overwrite", outPath)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect %s: %w", outPath, err)
	}
	if info, err := os.Lstat(partialPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", partialPath)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect %s: %w", partialPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(partialPath), 0o700); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	out, err := os.OpenFile(partialPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", partialPath, err)
	}
	if err := out.Chmod(0o600); err != nil {
		out.Close()
		return nil, fmt.Errorf("secure %s: %w", partialPath, err)
	}
	return out, nil
}

func commitAuditLogsDownloadOutput(partialPath, outPath string, force bool) error {
	if info, err := os.Lstat(outPath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", outPath)
		}
		if !force {
			return fmt.Errorf("%s already exists; pass --force to overwrite", outPath)
		}
		if err := os.Remove(outPath); err != nil {
			return fmt.Errorf("remove %s: %w", outPath, err)
		}
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
	Long: "Download audit logs as gzip-compressed JSONL in verified chunks. The time range is [start, end).\n\n" +
		"GET requests are excluded by default; pass --include-get to include them.\n\n" +
		"The output file is published only after every chunk is downloaded.",
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
	auditLogsDownloadCmd.Flags().Bool("force", false, "Overwrite the output file")
	_ = auditLogsDownloadCmd.MarkFlagRequired("start")
	_ = auditLogsDownloadCmd.MarkFlagRequired("end")
	auditLogsCmd.AddCommand(auditLogsDownloadCmd)
}
