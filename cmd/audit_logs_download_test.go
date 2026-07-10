package cmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type auditLogTestChunk struct {
	body       []byte
	rows       int
	hasMore    bool
	nextCursor string
	checksum   string
}

func auditLogChunkResponse(chunk auditLogTestChunk) *http.Response {
	checksum := chunk.checksum
	if checksum == "" {
		sum := sha256.Sum256(chunk.body)
		checksum = hex.EncodeToString(sum[:])
	}
	header := http.Header{}
	header.Set("X-Content-Sha256", checksum)
	header.Set("X-Row-Count", strconv.Itoa(chunk.rows))
	header.Set("X-Has-More", strconv.FormatBool(chunk.hasMore))
	if chunk.nextCursor != "" {
		header.Set("X-Next-Cursor", chunk.nextCursor)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(bytes.NewReader(chunk.body))}
}

func auditLogChunkService(t *testing.T, responses ...func() (*http.Response, error)) (*FakeAuditLogsService, *[]string) {
	t.Helper()
	call := 0
	cursors := make([]string, 0, len(responses))
	service := &FakeAuditLogsService{
		ExportChunkFunc: func(ctx context.Context, query kernel.AuditLogExportChunkParams, opts ...option.RequestOption) (*http.Response, error) {
			cursors = append(cursors, query.Cursor.Value)
			require.Less(t, call, len(responses))
			response, err := responses[call]()
			call++
			return response, err
		},
	}
	return service, &cursors
}

func auditLogGzip(t *testing.T, body string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	_, err := writer.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return out.Bytes()
}

func readAuditLogGzip(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	reader, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(body)
}

func auditLogsDownloadInput(path string) AuditLogsDownloadInput {
	return AuditLogsDownloadInput{Start: "2026-06-01", End: "2026-06-28", To: path}
}

func TestAuditLogsDownloadWritesAllChunks(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	first := auditLogGzip(t, "{\"n\":1}\n")
	second := auditLogGzip(t, "{\"n\":2}\n")
	service, cursors := auditLogChunkService(t,
		func() (*http.Response, error) {
			return auditLogChunkResponse(auditLogTestChunk{body: first, rows: 1, hasMore: true, nextCursor: "next"}), nil
		},
		func() (*http.Response, error) {
			return auditLogChunkResponse(auditLogTestChunk{body: second, rows: 1}), nil
		},
	)

	err := (AuditLogsCmd{auditLogs: service}).Download(context.Background(), auditLogsDownloadInput(outPath))
	require.NoError(t, err)
	assert.Equal(t, []string{"", "next"}, *cursors)
	assert.Equal(t, "{\"n\":1}\n{\"n\":2}\n", readAuditLogGzip(t, outPath))
	_, err = os.Stat(outPath + ".partial")
	assert.True(t, os.IsNotExist(err))
}

func TestAuditLogsDownloadExcludeMethodStacksWithDefaultGetExclusion(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	body := auditLogGzip(t, "{\"method\":\"POST\",\"n\":1}\n{\"method\":\"DELETE\",\"n\":2}\n")
	service := &FakeAuditLogsService{
		ExportChunkFunc: func(ctx context.Context, query kernel.AuditLogExportChunkParams, opts ...option.RequestOption) (*http.Response, error) {
			assert.Equal(t, "GET", query.ExcludeMethod.Value)
			return auditLogChunkResponse(auditLogTestChunk{body: body, rows: 2}), nil
		},
	}
	in := auditLogsDownloadInput(outPath)
	in.ExcludeMethod = "post"

	require.NoError(t, (AuditLogsCmd{auditLogs: service}).Download(context.Background(), in))
	assert.Equal(t, "{\"method\":\"DELETE\",\"n\":2}\n", readAuditLogGzip(t, outPath))
}

func TestAuditLogsDownloadRestartsAfterFailure(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	first := auditLogGzip(t, "{\"n\":1}\n")
	service, _ := auditLogChunkService(t,
		func() (*http.Response, error) {
			return auditLogChunkResponse(auditLogTestChunk{body: first, rows: 1, hasMore: true, nextCursor: "next"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("network error") },
	)
	require.Error(t, (AuditLogsCmd{auditLogs: service}).Download(context.Background(), auditLogsDownloadInput(outPath)))
	_, err := os.Stat(outPath)
	assert.True(t, os.IsNotExist(err))
	partialPath := outPath + ".partial"
	_, err = os.Stat(partialPath)
	assert.True(t, os.IsNotExist(err))
	require.NoError(t, os.WriteFile(partialPath, []byte("stale"), 0o600))

	second := auditLogGzip(t, "{\"n\":2}\n")
	restarted, cursors := auditLogChunkService(t,
		func() (*http.Response, error) {
			return auditLogChunkResponse(auditLogTestChunk{body: first, rows: 1, hasMore: true, nextCursor: "next"}), nil
		},
		func() (*http.Response, error) {
			return auditLogChunkResponse(auditLogTestChunk{body: second, rows: 1}), nil
		},
	)
	require.NoError(t, (AuditLogsCmd{auditLogs: restarted}).Download(context.Background(), auditLogsDownloadInput(outPath)))
	assert.Equal(t, []string{"", "next"}, *cursors)
	assert.Equal(t, "{\"n\":1}\n{\"n\":2}\n", readAuditLogGzip(t, outPath))
	_, err = os.Stat(partialPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDefaultAuditLogsDownloadPath(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	path := defaultAuditLogsDownloadPath(start, end)

	assert.Equal(t, "audit-logs-20260601-20260628.jsonl.gz", path)
}

func TestAuditLogsDownloadRejectsBadChunkBeforeWriting(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	service, _ := auditLogChunkService(t, func() (*http.Response, error) {
		return auditLogChunkResponse(auditLogTestChunk{body: []byte("bad"), rows: 1, checksum: "wrong"}), nil
	})

	err := (AuditLogsCmd{auditLogs: service}).Download(context.Background(), auditLogsDownloadInput(outPath))
	require.ErrorContains(t, err, "checksum mismatch")
	_, statErr := os.Stat(outPath)
	assert.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(outPath + ".partial")
	assert.True(t, os.IsNotExist(statErr))
}

func TestAuditLogsDownloadForceOverwrites(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	require.NoError(t, os.WriteFile(outPath, []byte("old"), 0o600))
	chunk := auditLogGzip(t, "{\"n\":1}\n")
	service, cursors := auditLogChunkService(t, func() (*http.Response, error) {
		return auditLogChunkResponse(auditLogTestChunk{body: chunk, rows: 1}), nil
	})
	in := auditLogsDownloadInput(outPath)
	in.Force = true

	require.NoError(t, (AuditLogsCmd{auditLogs: service}).Download(context.Background(), in))
	assert.Equal(t, []string{""}, *cursors)
	assert.Equal(t, "{\"n\":1}\n", readAuditLogGzip(t, outPath))
}

func TestAuditLogsDownloadPreservesCompletedPartialOnFinalizeFailure(t *testing.T) {
	capturePtermOutput(t)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "audit.jsonl.gz")
	chunk := auditLogGzip(t, "{\"n\":1}\n")
	service, _ := auditLogChunkService(t, func() (*http.Response, error) {
		require.NoError(t, os.Mkdir(outPath, 0o700))
		return auditLogChunkResponse(auditLogTestChunk{body: chunk, rows: 1}), nil
	})

	err := (AuditLogsCmd{auditLogs: service}).Download(context.Background(), auditLogsDownloadInput(outPath))
	require.ErrorContains(t, err, "completed download remains")
	assert.Equal(t, "{\"n\":1}\n", readAuditLogGzip(t, outPath+".partial"))
}

func TestCommitAuditLogsDownloadOutputPreservesExistingFileOnFailure(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	require.NoError(t, os.WriteFile(outPath, []byte("existing"), 0o600))

	err := commitAuditLogsDownloadOutput(outPath+".partial", outPath, true)
	require.Error(t, err)
	data, readErr := os.ReadFile(outPath)
	require.NoError(t, readErr)
	assert.Equal(t, "existing", string(data))
}

func TestBuildAuditLogsDownloadParams(t *testing.T) {
	in := auditLogsDownloadInput("")
	in.Search = "browser"
	in.Service = "api"
	in.UserIDs = []string{"user_1"}
	params, err := buildAuditLogsDownloadParams(in)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC), params.End)
	assert.Equal(t, "browser", params.Search.Value)
	assert.Equal(t, "GET", params.ExcludeMethod.Value)
	assert.Equal(t, "api", params.Service.Value)
	assert.Equal(t, []string{"user_1"}, params.SearchUserID)
}

func TestBuildAuditLogsDownloadParamsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		in   AuditLogsDownloadInput
		want string
	}{
		{name: "missing bounds", in: AuditLogsDownloadInput{}, want: "--start and --end are required"},
		{name: "reversed bounds", in: AuditLogsDownloadInput{Start: "2026-06-02", End: "2026-06-01"}, want: "--start must be before --end"},
		{name: "range too large", in: AuditLogsDownloadInput{Start: "2026-05-01", End: "2026-06-01"}, want: "at most 30 days"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildAuditLogsDownloadParams(test.in)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestParseAuditLogsChunkHeaders(t *testing.T) {
	tests := []struct {
		name    string
		header  http.Header
		current string
		want    string
	}{
		{name: "missing has more", header: http.Header{"X-Row-Count": []string{"1"}}, want: "X-Has-More"},
		{name: "missing row count", header: http.Header{"X-Has-More": []string{"false"}}, want: "X-Row-Count"},
		{name: "missing cursor", header: http.Header{"X-Has-More": []string{"true"}, "X-Row-Count": []string{"1"}}, want: "X-Next-Cursor"},
		{name: "unchanged cursor", current: "next", header: http.Header{"X-Has-More": []string{"true"}, "X-Row-Count": []string{"1"}, "X-Next-Cursor": []string{"next"}}, want: "X-Next-Cursor"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := parseAuditLogsChunkHeaders(test.header, test.current)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestAuditLogsDownloadRejectsExistingOutput(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	require.NoError(t, os.WriteFile(outPath, []byte("keep"), 0o600))
	err := (AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}).Download(context.Background(), auditLogsDownloadInput(outPath))
	require.ErrorContains(t, err, "already exists")
}
