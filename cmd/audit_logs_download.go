package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"net/http"
	"net/url"
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
	Format        string
	Force         bool
}

const (
	// auditLogsDownloadMaxRange mirrors the API's max export window.
	auditLogsDownloadMaxRange = 30 * 24 * time.Hour
	// auditLogsDownloadShaRetries is the number of extra attempts for a chunk
	// whose body fails checksum verification.
	auditLogsDownloadShaRetries = 2
	// auditLogsDownloadMaxRowsPerChunk mirrors the export endpoint's limit.
	auditLogsDownloadMaxRowsPerChunk = 50_000
	// auditLogsDownloadMaxStateBytes bounds the local checkpoint before it is
	// decoded. Valid state files are under a few kilobytes.
	auditLogsDownloadMaxStateBytes = 64 << 10
)

// auditLogsDownloadMaxChunkBytes bounds how much of a chunk response is
// buffered. The server caps chunks at 50k rows, so a legitimate response is
// far smaller; this guards against a broken server streaming without end.
// Variable so tests can lower it.
var auditLogsDownloadMaxChunkBytes int64 = 256 << 20

// auditLogsDownloadState is the sidecar file that makes a download resumable.
// It is written atomically after every committed chunk and removed on
// completion. Version guards future format changes (e.g. multi-window splits).
type auditLogsDownloadState struct {
	Version         int    `json:"version"`
	Params          string `json:"params"`
	Identity        string `json:"identity"`
	Cursor          string `json:"cursor"`
	BytesWritten    int64  `json:"bytes_written"`
	CommittedSHA256 string `json:"committed_sha256"`
	Chunks          int    `json:"chunks"`
	Rows            int64  `json:"rows"`
}

const auditLogsDownloadStateVersion = 2

const auditLogsDownloadEmptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func (c AuditLogsCmd) Download(ctx context.Context, in AuditLogsDownloadInput) error {
	if c.identity == "" {
		return fmt.Errorf("cannot safely resume audit-log download without an API origin and credential identity")
	}

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

	out, created, err := openAndLockAuditLogsDownloadOutput(outPath)
	if err != nil {
		return err
	}
	// Keep a newly created output only after it has a valid matching state.
	removeCreatedOutput := created
	defer func() {
		_ = out.close(removeCreatedOutput)
	}()

	state, stateExists, err := loadAuditLogsDownloadState(statePath, fingerprint, c.identity, in.Force)
	if err != nil {
		return err
	}
	if !stateExists && !in.Force && !created {
		return fmt.Errorf("%s already exists; pass --force to overwrite or --to to pick another path", outPath)
	}
	if stateExists && state.Chunks > 0 && created {
		return fmt.Errorf("state file records progress but %s is missing; pass --force to start over", outPath)
	}
	if !stateExists {
		if err := saveAuditLogsDownloadState(statePath, state); err != nil {
			return err
		}
	}

	completed := state.Chunks > 0 && state.Cursor == ""
	committedHash, err := prepareAuditLogsDownloadOutput(out.file, outPath, state, completed)
	if err != nil {
		return err
	}
	removeCreatedOutput = false

	// A state file with chunks but no cursor means the download finished and
	// only the cleanup was interrupted.
	if completed {
		if err := validateAuditLogsDownloadOutputPath(out.file, outPath); err != nil {
			return err
		}
		if err := removeAuditLogsDownloadState(statePath); err != nil {
			return err
		}
		pterm.Success.Printf("Download already complete: %d rows (%s) in %s\n", state.Rows, util.FormatBytes(state.BytesWritten), outPath)
		return nil
	}

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

		// A missing pagination header must not end the download as a success:
		// a stripped or renamed header would silently truncate the export.
		// Validated before the write so a bad response never touches the file.
		hasMore, err := strconv.ParseBool(header.Get("X-Has-More"))
		if err != nil {
			return fmt.Errorf("response missing or invalid X-Has-More header %q", header.Get("X-Has-More"))
		}
		rowsHeader := header.Get("X-Row-Count")
		rows, err := strconv.ParseInt(rowsHeader, 10, 64)
		if err != nil || rows < 0 || rows > auditLogsDownloadMaxRowsPerChunk {
			return fmt.Errorf("response missing or invalid X-Row-Count header %q", rowsHeader)
		}
		nextCursor := header.Get("X-Next-Cursor")
		if hasMore {
			// Guard against a server bug looping the download forever.
			if nextCursor == "" {
				return fmt.Errorf("server reported more records but returned no cursor; retry, and report this if it persists")
			}
			if nextCursor == state.Cursor {
				return fmt.Errorf("server returned an unchanged cursor; retry, and report this if it persists")
			}
		} else if nextCursor != "" {
			return fmt.Errorf("server returned a cursor after reporting no more records; retry, and report this if it persists")
		}
		if state.Rows > math.MaxInt64-rows {
			return fmt.Errorf("row count exceeds the supported range")
		}
		bodyBytes := int64(len(body))
		if state.BytesWritten > math.MaxInt64-bodyBytes {
			return fmt.Errorf("download size exceeds the supported range")
		}
		if state.Chunks == math.MaxInt {
			return fmt.Errorf("chunk count exceeds the supported range")
		}

		if err := validateAuditLogsDownloadOutputPath(out.file, outPath); err != nil {
			return err
		}
		if _, err := out.file.Write(body); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		if err := out.file.Sync(); err != nil {
			return fmt.Errorf("sync %s: %w", outPath, err)
		}
		if err := validateAuditLogsDownloadOutputPath(out.file, outPath); err != nil {
			return err
		}
		_, _ = committedHash.Write(body)

		state.Chunks++
		state.Rows += rows
		state.BytesWritten += bodyBytes
		state.CommittedSHA256 = hex.EncodeToString(committedHash.Sum(nil))
		state.Cursor = nextCursor
		if err := saveAuditLogsDownloadState(statePath, state); err != nil {
			return err
		}

		pterm.Info.Printf("Chunk %d: %d rows (%d total, %s)\n", state.Chunks, rows, state.Rows, util.FormatBytes(state.BytesWritten))
		if !hasMore {
			break
		}
	}

	if err := validateAuditLogsDownloadOutputPath(out.file, outPath); err != nil {
		return err
	}
	if err := removeAuditLogsDownloadState(statePath); err != nil {
		return err
	}
	pterm.Success.Printf("Downloaded %d rows (%s) to %s\n", state.Rows, util.FormatBytes(state.BytesWritten), outPath)
	return nil
}

// fetchAuditLogsChunk downloads one chunk and verifies it against the
// X-Content-Sha256 header before returning it, retrying on mismatch. Nothing
// is written to disk until a chunk verifies; a response without a checksum is
// rejected rather than trusted.
func (c AuditLogsCmd) fetchAuditLogsChunk(ctx context.Context, params kernel.AuditLogExportChunkParams) ([]byte, http.Header, error) {
	var lastErr error
	for attempt := 0; attempt <= auditLogsDownloadShaRetries; attempt++ {
		res, err := c.auditLogs.ExportChunk(ctx, params)
		if err != nil {
			return nil, nil, util.CleanedUpSdkError{Err: err}
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, auditLogsDownloadMaxChunkBytes+1))
		res.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read chunk body: %w", err)
			continue
		}
		if int64(len(body)) > auditLogsDownloadMaxChunkBytes {
			return nil, nil, fmt.Errorf("chunk response exceeds %s; refusing to buffer it", util.FormatBytes(auditLogsDownloadMaxChunkBytes))
		}
		want := res.Header.Get("X-Content-Sha256")
		if want == "" {
			return nil, nil, fmt.Errorf("response missing X-Content-Sha256 header; refusing to write unverified data")
		}
		sum := sha256.Sum256(body)
		if got := hex.EncodeToString(sum[:]); got != want {
			lastErr = fmt.Errorf("chunk checksum mismatch (got %s, want %s)", got, want)
			continue
		}
		return body, res.Header, nil
	}
	return nil, nil, lastErr
}

func buildAuditLogsDownloadParams(in AuditLogsDownloadInput) (kernel.AuditLogExportChunkParams, error) {
	var params kernel.AuditLogExportChunkParams
	if in.Start == "" || in.End == "" {
		return params, fmt.Errorf("--start and --end are required so interrupted downloads can resume with the same time range")
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

// loadAuditLogsDownloadState decides whether a download starts fresh or resumes
// from a checkpoint bound to the same query, API origin, and credential.
func loadAuditLogsDownloadState(statePath, fingerprint, identity string, force bool) (auditLogsDownloadState, bool, error) {
	fresh := auditLogsDownloadState{
		Version:         auditLogsDownloadStateVersion,
		Params:          fingerprint,
		Identity:        identity,
		CommittedSHA256: auditLogsDownloadEmptySHA256,
	}
	if force {
		if err := removeAuditLogsDownloadState(statePath); err != nil {
			return fresh, false, err
		}
		return fresh, false, nil
	}

	raw, exists, err := readAuditLogsDownloadState(statePath)
	if err != nil {
		return fresh, false, err
	}
	if !exists {
		return fresh, false, nil
	}

	var state auditLogsDownloadState
	if err := json.Unmarshal(raw, &state); err != nil {
		return fresh, false, fmt.Errorf("state file %s is corrupt; pass --force to start over", statePath)
	}
	if state.Version != auditLogsDownloadStateVersion {
		return fresh, false, fmt.Errorf("state file %s was written by an incompatible CLI version; pass --force to start over", statePath)
	}
	if state.Params != fingerprint {
		return fresh, false, fmt.Errorf("state file %s belongs to a download with different parameters; pass --force to start over or --to to pick another path", statePath)
	}
	if state.Identity != identity {
		return fresh, false, fmt.Errorf("state file %s was written for a different API origin or credential; pass --force to start over", statePath)
	}
	if err := validateAuditLogsDownloadState(state); err != nil {
		return fresh, false, fmt.Errorf("state file %s is inconsistent (%v); pass --force to start over", statePath, err)
	}
	return state, true, nil
}

func readAuditLogsDownloadState(statePath string) ([]byte, bool, error) {
	info, err := os.Lstat(statePath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read state file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("state file %s is not a regular file; pass --force to start over", statePath)
	}
	if info.Size() > auditLogsDownloadMaxStateBytes {
		return nil, false, fmt.Errorf("state file %s is too large; pass --force to start over", statePath)
	}

	f, err := os.Open(statePath)
	if err != nil {
		return nil, false, fmt.Errorf("read state file: %w", err)
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("read state file: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, false, fmt.Errorf("state file %s changed while it was being opened; retry", statePath)
	}
	raw, err := io.ReadAll(io.LimitReader(f, auditLogsDownloadMaxStateBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("read state file: %w", err)
	}
	if len(raw) > auditLogsDownloadMaxStateBytes {
		return nil, false, fmt.Errorf("state file %s is too large; pass --force to start over", statePath)
	}
	return raw, true, nil
}

func removeAuditLogsDownloadState(statePath string) error {
	info, err := os.Lstat(statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove state file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("state path %s is a directory; remove it before using --force", statePath)
	}
	if err := os.Remove(statePath); err != nil {
		return fmt.Errorf("remove state file: %w", err)
	}
	return nil
}

func validateAuditLogsDownloadState(state auditLogsDownloadState) error {
	if state.BytesWritten < 0 || state.Chunks < 0 || state.Rows < 0 {
		return fmt.Errorf("negative progress counters")
	}
	digest, err := hex.DecodeString(state.CommittedSHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("invalid committed checksum")
	}
	if state.Chunks == 0 {
		if state.Cursor != "" || state.BytesWritten != 0 || state.Rows != 0 || state.CommittedSHA256 != auditLogsDownloadEmptySHA256 {
			return fmt.Errorf("zero-chunk state contains committed progress")
		}
	}
	return nil
}

func saveAuditLogsDownloadState(statePath string, state auditLogsDownloadState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if len(raw) > auditLogsDownloadMaxStateBytes {
		return fmt.Errorf("state file would exceed %s; narrow the download filters", util.FormatBytes(auditLogsDownloadMaxStateBytes))
	}
	// CreateTemp gives an unpredictable 0600 file, so a symlink planted at a
	// guessable name in a shared directory can't redirect the write.
	tmp, err := os.CreateTemp(filepath.Dir(statePath), filepath.Base(statePath)+".*")
	if err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write state file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("sync state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write state file: %w", err)
	}
	if err := commitAuditLogsDownloadStateFile(tmp.Name(), statePath); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("commit state file: %w", err)
	}
	return nil
}

type lockedAuditLogsDownloadOutput struct {
	file     *os.File
	pathLock *os.File
	outPath  string
}

func (o *lockedAuditLogsDownloadOutput) Close() error {
	return o.close(false)
}

func (o *lockedAuditLogsDownloadOutput) close(removeOutput bool) error {
	outputErr := o.file.Close()
	var removeErr error
	if removeOutput {
		removeErr = os.Remove(o.outPath)
		if removeErr == nil {
			removeErr = syncAuditLogsDownloadDir(filepath.Dir(o.outPath))
		}
	}
	lockErr := o.pathLock.Close()
	return errors.Join(outputErr, removeErr, lockErr)
}

// openAndLockAuditLogsDownloadOutput securely opens a regular output file and
// holds a stable path lock before any state is read or output is modified.
func openAndLockAuditLogsDownloadOutput(outPath string) (*lockedAuditLogsDownloadOutput, bool, error) {
	if dir := filepath.Dir(outPath); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, false, fmt.Errorf("create output directory: %w", err)
		}
	}

	pathLock, err := openAndLockAuditLogsDownloadPath(outPath + ".lock")
	if err != nil {
		return nil, false, err
	}
	closePathLock := true
	defer func() {
		if closePathLock {
			_ = pathLock.Close()
		}
	}()

	out, created, err := openAuditLogsDownloadRegularFile(outPath)
	if err != nil {
		return nil, false, err
	}
	locked, err := tryLockAuditLogsDownloadFile(out)
	if err != nil {
		out.Close()
		return nil, false, fmt.Errorf("lock %s: %w", outPath, err)
	}
	if !locked {
		out.Close()
		return nil, false, fmt.Errorf("another audit-log download is already using %s", outPath)
	}
	if created {
		if err := out.Sync(); err != nil {
			out.Close()
			_ = os.Remove(outPath)
			return nil, false, fmt.Errorf("sync %s: %w", outPath, err)
		}
		if err := syncAuditLogsDownloadDir(filepath.Dir(outPath)); err != nil {
			out.Close()
			_ = os.Remove(outPath)
			return nil, false, fmt.Errorf("sync output directory: %w", err)
		}
	}
	closePathLock = false
	return &lockedAuditLogsDownloadOutput{file: out, pathLock: pathLock, outPath: outPath}, created, nil
}

func openAndLockAuditLogsDownloadPath(lockPath string) (*os.File, error) {
	lock, _, err := openAuditLogsDownloadRegularFile(lockPath)
	if err != nil {
		return nil, err
	}
	if err := lock.Chmod(0o600); err != nil {
		lock.Close()
		return nil, fmt.Errorf("secure lock file permissions: %w", err)
	}
	locked, err := tryLockAuditLogsDownloadFile(lock)
	if err != nil {
		lock.Close()
		return nil, fmt.Errorf("lock %s: %w", lockPath, err)
	}
	if !locked {
		lock.Close()
		return nil, fmt.Errorf("another audit-log download is already using %s", strings.TrimSuffix(lockPath, ".lock"))
	}
	currentInfo, err := os.Lstat(lockPath)
	if err != nil {
		lock.Close()
		return nil, fmt.Errorf("inspect lock file: %w", err)
	}
	openedInfo, err := lock.Stat()
	if err != nil {
		lock.Close()
		return nil, fmt.Errorf("inspect lock file: %w", err)
	}
	if !currentInfo.Mode().IsRegular() || !os.SameFile(currentInfo, openedInfo) {
		lock.Close()
		return nil, fmt.Errorf("lock file %s changed while it was being locked; retry", lockPath)
	}
	return lock, nil
}

func openAuditLogsDownloadRegularFile(path string) (*os.File, bool, error) {
	for attempt := 0; attempt < 2; attempt++ {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			f, createErr := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
			if os.IsExist(createErr) {
				continue
			}
			if createErr != nil {
				return nil, false, fmt.Errorf("open %s: %w", path, createErr)
			}
			return f, true, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("inspect %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, false, fmt.Errorf("%s must be a regular file; refusing to follow a symlink or special file", path)
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return nil, false, fmt.Errorf("open %s: %w", path, err)
		}
		openedInfo, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, false, fmt.Errorf("inspect %s: %w", path, err)
		}
		if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
			f.Close()
			return nil, false, fmt.Errorf("%s changed while it was being opened; retry", path)
		}
		return f, false, nil
	}
	return nil, false, fmt.Errorf("%s changed while it was being opened; retry", path)
}

func validateAuditLogsDownloadOutputPath(out *os.File, outPath string) error {
	pathInfo, err := os.Lstat(outPath)
	if err != nil {
		return fmt.Errorf("%s changed while the download was running: %w", outPath, err)
	}
	openedInfo, err := out.Stat()
	if err != nil {
		return fmt.Errorf("inspect %s: %w", outPath, err)
	}
	if !pathInfo.Mode().IsRegular() || !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		return fmt.Errorf("%s changed while the download was running; restore it before resuming", outPath)
	}
	return nil
}

// prepareAuditLogsDownloadOutput verifies the committed prefix before
// truncating an interrupted append and positioning the file for the next chunk.
func prepareAuditLogsDownloadOutput(out *os.File, outPath string, state auditLogsDownloadState, completed bool) (hash.Hash, error) {
	if err := out.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("secure permissions on %s: %w", outPath, err)
	}
	info, err := out.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", outPath, err)
	}
	if info.Size() < state.BytesWritten {
		return nil, fmt.Errorf("%s is shorter than the recorded progress; it was modified or replaced — pass --force to start over", outPath)
	}
	if completed && info.Size() != state.BytesWritten {
		return nil, fmt.Errorf("%s does not match the completed download size; pass --force to start over", outPath)
	}
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek %s: %w", outPath, err)
	}
	committedHash := sha256.New()
	if _, err := io.CopyN(committedHash, out, state.BytesWritten); err != nil {
		return nil, fmt.Errorf("verify %s: %w", outPath, err)
	}
	got := hex.EncodeToString(committedHash.Sum(nil))
	if got != state.CommittedSHA256 {
		return nil, fmt.Errorf("%s does not match the recorded checksum; it was modified or replaced — pass --force to start over", outPath)
	}
	if !completed {
		if err := out.Truncate(state.BytesWritten); err != nil {
			return nil, fmt.Errorf("truncate %s to last committed chunk: %w", outPath, err)
		}
	}
	if _, err := out.Seek(state.BytesWritten, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek %s: %w", outPath, err)
	}
	return committedHash, nil
}

// auditLogsCredentialIdentity binds resume state to both the effective API
// origin and the authenticated organization or API key.
func auditLogsCredentialIdentity() (string, error) {
	baseURL, err := normalizeAuditLogsAPIBaseURL(util.GetBaseURL())
	if err != nil {
		return "", err
	}
	if key := os.Getenv("KERNEL_API_KEY"); key != "" {
		sum := sha256.Sum256([]byte("kernel-cli-audit-logs-download:" + key))
		return baseURL + "|key:" + hex.EncodeToString(sum[:]), nil
	}
	tokens, err := auth.LoadTokens()
	if err != nil {
		return "", fmt.Errorf("determine OAuth organization for resumable download: %w", err)
	}
	if tokens.OrgID == "" {
		return "", fmt.Errorf("cannot determine OAuth organization for resumable download; run 'kernel login --force' and retry")
	}
	return baseURL + "|org:" + tokens.OrgID, nil
}

func normalizeAuditLogsAPIBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid KERNEL_BASE_URL %q", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid KERNEL_BASE_URL scheme %q", u.Scheme)
	}
	if u.User != nil {
		return "", fmt.Errorf("KERNEL_BASE_URL must not contain user information")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("KERNEL_BASE_URL must not contain a query or fragment")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u.String(), nil
}

func runAuditLogsDownload(cmd *cobra.Command, args []string) error {
	c := getAuditLogsHandler(cmd)
	identity, err := auditLogsCredentialIdentity()
	if err != nil {
		return err
	}
	c.identity = identity
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
		"Records in the half-open [start, end) window are fetched in chunks and appended after each chunk is verified, newest first. If a download is interrupted, rerunning the same command resumes where it left off.\n\n" +
		"GET requests are excluded by default; pass --include-get to include them, or --method GET to see only them.\n\n" +
		"The API allows at most 30 days per download; for longer ranges run one download per window.",
	Args: cobra.NoArgs,
	RunE: runAuditLogsDownload,
}

func init() {
	auditLogsDownloadCmd.Flags().String("start", "", "Start of the export window, RFC3339 or YYYY-MM-DD (required)")
	auditLogsDownloadCmd.Flags().String("end", "", "Exclusive end of the export window, RFC3339 or YYYY-MM-DD (required)")
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
	_ = auditLogsDownloadCmd.MarkFlagRequired("start")
	_ = auditLogsDownloadCmd.MarkFlagRequired("end")

	auditLogsCmd.AddCommand(auditLogsDownloadCmd)
}
