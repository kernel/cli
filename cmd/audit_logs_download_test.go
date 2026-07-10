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
	_, err = os.Stat(outPath + ".state.json")
	assert.True(t, os.IsNotExist(err))
}

func TestAuditLogsDownloadResumesFromCommittedChunk(t *testing.T) {
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

	file, err := os.OpenFile(outPath, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteString("partial")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	second := auditLogGzip(t, "{\"n\":2}\n")
	resumed, cursors := auditLogChunkService(t, func() (*http.Response, error) {
		return auditLogChunkResponse(auditLogTestChunk{body: second, rows: 1}), nil
	})
	require.NoError(t, (AuditLogsCmd{auditLogs: resumed}).Download(context.Background(), auditLogsDownloadInput(outPath)))
	assert.Equal(t, []string{"next"}, *cursors)
	assert.Equal(t, "{\"n\":1}\n{\"n\":2}\n", readAuditLogGzip(t, outPath))
}

func TestAuditLogsDownloadRejectsChangedParamsOnResume(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	chunk := auditLogGzip(t, "{\"n\":1}\n")
	service, _ := auditLogChunkService(t,
		func() (*http.Response, error) {
			return auditLogChunkResponse(auditLogTestChunk{body: chunk, rows: 1, hasMore: true, nextCursor: "next"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("network error") },
	)
	in := auditLogsDownloadInput(outPath)
	require.Error(t, (AuditLogsCmd{auditLogs: service}).Download(context.Background(), in))

	in.Service = "api"
	err := (AuditLogsCmd{auditLogs: service}).Download(context.Background(), in)
	require.ErrorContains(t, err, "does not match this download")
}

func TestAuditLogsDownloadFingerprintIncludesIdentity(t *testing.T) {
	params, err := buildAuditLogsDownloadParams(auditLogsDownloadInput(""))
	require.NoError(t, err)

	first, err := auditLogsDownloadFingerprint(params, "https://api.onkernel.com\norg:first")
	require.NoError(t, err)
	second, err := auditLogsDownloadFingerprint(params, "https://api.onkernel.com\norg:second")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Len(t, first, 64)
}

func TestDefaultAuditLogsDownloadPathIncludesFingerprint(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	path := defaultAuditLogsDownloadPath(start, end, "1234567890abcdef")

	assert.Equal(t, "audit-logs-20260601T000000Z-20260628T000000Z-12345678.jsonl.gz", path)
}

func TestAuditLogsDownloadRejectsBadChunkBeforeWriting(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "audit.jsonl.gz")
	service, _ := auditLogChunkService(t, func() (*http.Response, error) {
		return auditLogChunkResponse(auditLogTestChunk{body: []byte("bad"), rows: 1, checksum: "wrong"}), nil
	})

	err := (AuditLogsCmd{auditLogs: service}).Download(context.Background(), auditLogsDownloadInput(outPath))
	require.ErrorContains(t, err, "checksum mismatch")
	data, readErr := os.ReadFile(outPath)
	require.NoError(t, readErr)
	assert.Empty(t, data)
}

func TestAuditLogsDownloadForceRestarts(t *testing.T) {
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
		{name: "conflicting exclusion", in: AuditLogsDownloadInput{Start: "2026-06-01", End: "2026-06-02", ExcludeMethod: "POST"}, want: "--include-get"},
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
