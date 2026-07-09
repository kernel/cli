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

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type exportChunk struct {
	body       []byte
	rowCount   int
	hasMore    bool
	nextCursor string
	sha        string // overrides the computed body sha when set
}

func exportChunkResponse(c exportChunk) *http.Response {
	sha := c.sha
	if sha == "" {
		sum := sha256.Sum256(c.body)
		sha = hex.EncodeToString(sum[:])
	}
	header := http.Header{}
	header.Set("X-Has-More", strconv.FormatBool(c.hasMore))
	header.Set("X-Row-Count", strconv.Itoa(c.rowCount))
	header.Set("X-Content-Sha256", sha)
	if c.nextCursor != "" {
		header.Set("X-Next-Cursor", c.nextCursor)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(bytes.NewReader(c.body))}
}

func gzipMember(t *testing.T, lines string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte(lines))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func gunzipAll(t *testing.T, data []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

// chunkServer serves a scripted sequence of responses keyed by call order and
// records the cursor each call carried.
func chunkServer(t *testing.T, responses []func() (*http.Response, error)) (*FakeAuditLogsService, *[]string) {
	t.Helper()
	var cursors []string
	calls := 0
	fake := &FakeAuditLogsService{
		ExportChunkFunc: func(ctx context.Context, query kernel.AuditLogExportChunkParams, opts ...option.RequestOption) (*http.Response, error) {
			cursors = append(cursors, query.Cursor.Value)
			require.Less(t, calls, len(responses), "more ExportChunk calls than scripted responses")
			res, err := responses[calls]()
			calls++
			return res, err
		},
	}
	return fake, &cursors
}

func downloadInput(outPath string) AuditLogsDownloadInput {
	return AuditLogsDownloadInput{
		Start:  "2026-06-01",
		End:    "2026-06-28",
		To:     outPath,
		Format: "jsonl.gz",
	}
}

func TestAuditLogsDownloadMultiChunk(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk1 := gzipMember(t, "{\"n\":1}\n{\"n\":2}\n")
	chunk2 := gzipMember(t, "{\"n\":3}\n")
	chunk3 := gzipMember(t, "{\"n\":4}\n{\"n\":5}\n")
	fake, cursors := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk1, rowCount: 2, hasMore: true, nextCursor: "c1"}), nil
		},
		// A short chunk with hasMore=true must not end the download.
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk2, rowCount: 1, hasMore: true, nextCursor: "c2"}), nil
		},
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk3, rowCount: 2, hasMore: false}), nil
		},
	})

	c := AuditLogsCmd{auditLogs: fake}
	require.NoError(t, c.Download(context.Background(), downloadInput(outPath)))

	assert.Equal(t, []string{"", "c1", "c2"}, *cursors)
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":1}\n{\"n\":2}\n{\"n\":3}\n{\"n\":4}\n{\"n\":5}\n", gunzipAll(t, data))
	_, err = os.Stat(outPath + ".state.json")
	assert.True(t, os.IsNotExist(err), "state file should be removed on completion")
}

func TestAuditLogsDownloadChecksumMismatchRetriesThenSucceeds(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk := gzipMember(t, "{\"n\":1}\n")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, sha: "deadbeef"}), nil
		},
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1}), nil
		},
	})

	c := AuditLogsCmd{auditLogs: fake}
	require.NoError(t, c.Download(context.Background(), downloadInput(outPath)))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":1}\n", gunzipAll(t, data), "corrupt chunk must not be written")
}

func TestAuditLogsDownloadChecksumMismatchExhaustsRetries(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk := gzipMember(t, "{\"n\":1}\n")
	bad := func() (*http.Response, error) {
		return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, sha: "deadbeef"}), nil
	}
	fake, _ := chunkServer(t, []func() (*http.Response, error){bad, bad, bad})

	c := AuditLogsCmd{auditLogs: fake}
	err := c.Download(context.Background(), downloadInput(outPath))
	require.ErrorContains(t, err, "checksum mismatch")

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Empty(t, data, "no bytes may be written for a chunk that never verified")
}

func TestAuditLogsDownloadResumesAfterFailure(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk1 := gzipMember(t, "{\"n\":1}\n")
	chunk2 := gzipMember(t, "{\"n\":2}\n")

	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk1, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	c := AuditLogsCmd{auditLogs: fake}
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	fake2, cursors := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk2, rowCount: 1, hasMore: false}), nil
		},
	})
	c2 := AuditLogsCmd{auditLogs: fake2}
	require.NoError(t, c2.Download(context.Background(), downloadInput(outPath)))

	assert.Equal(t, []string{"c1"}, *cursors, "resume must continue from the saved cursor")
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":1}\n{\"n\":2}\n", gunzipAll(t, data), "resumed file must have no duplicate or missing rows")
	_, err = os.Stat(outPath + ".state.json")
	assert.True(t, os.IsNotExist(err))
}

func TestAuditLogsDownloadResumeTruncatesTornWrite(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk1 := gzipMember(t, "{\"n\":1}\n")
	chunk2 := gzipMember(t, "{\"n\":2}\n")

	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk1, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	c := AuditLogsCmd{auditLogs: fake}
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	// Simulate a crash mid-append: garbage past the committed offset.
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	_, err = f.Write([]byte("torn partial chunk"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	fake2, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk2, rowCount: 1, hasMore: false}), nil
		},
	})
	c2 := AuditLogsCmd{auditLogs: fake2}
	require.NoError(t, c2.Download(context.Background(), downloadInput(outPath)))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":1}\n{\"n\":2}\n", gunzipAll(t, data))
}

func TestAuditLogsDownloadRejectsMismatchedState(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk1 := gzipMember(t, "{\"n\":1}\n")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk1, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	c := AuditLogsCmd{auditLogs: fake}
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	in := downloadInput(outPath)
	in.Service = "api"
	err := c.Download(context.Background(), in)
	require.ErrorContains(t, err, "different parameters")
}

func TestAuditLogsDownloadForceRestarts(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk1 := gzipMember(t, "{\"n\":1}\n")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk1, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	c := AuditLogsCmd{auditLogs: fake}
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	chunk := gzipMember(t, "{\"n\":9}\n")
	fake2, cursors := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: false}), nil
		},
	})
	in := downloadInput(outPath)
	in.Force = true
	c2 := AuditLogsCmd{auditLogs: fake2}
	require.NoError(t, c2.Download(context.Background(), in))

	assert.Equal(t, []string{""}, *cursors, "--force must restart from the beginning")
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":9}\n", gunzipAll(t, data))
}

func TestAuditLogsDownloadRefusesExistingOutputWithoutState(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	require.NoError(t, os.WriteFile(outPath, []byte("existing"), 0o644))

	c := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}
	err := c.Download(context.Background(), downloadInput(outPath))
	require.ErrorContains(t, err, "already exists")
}

func TestAuditLogsDownloadFinishesCompletedLeftoverState(t *testing.T) {
	buf := capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk1 := gzipMember(t, "{\"n\":1}\n")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk1, rowCount: 1, hasMore: false}), nil
		},
	})
	c := AuditLogsCmd{auditLogs: fake}
	require.NoError(t, c.Download(context.Background(), downloadInput(outPath)))

	// Simulate a crash after the final state save but before cleanup by
	// re-creating the completed state file.
	statePath := outPath + ".state.json"
	require.NoError(t, os.WriteFile(statePath, []byte(`{"version":1,"params":"`+mustFingerprint(t)+`","cursor":"","bytes_written":`+strconv.Itoa(len(chunk1))+`,"chunks":1,"rows":1}`), 0o644))

	c2 := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}
	require.NoError(t, c2.Download(context.Background(), downloadInput(outPath)))
	assert.Contains(t, buf.String(), "already complete")
	_, err := os.Stat(statePath)
	assert.True(t, os.IsNotExist(err))
}

func mustFingerprint(t *testing.T) string {
	t.Helper()
	params, err := buildAuditLogsDownloadParams(downloadInput(""))
	require.NoError(t, err)
	params.Format = kernel.AuditLogExportChunkParamsFormatJSONLGz
	fp, err := auditLogsDownloadFingerprint(params)
	require.NoError(t, err)
	return fp
}

func TestAuditLogsDownloadRangeOver30DaysSuggestsWindows(t *testing.T) {
	buf := capturePtermOutput(t)
	c := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}
	err := c.Download(context.Background(), AuditLogsDownloadInput{Start: "2026-04-01", End: "2026-06-30"})
	require.ErrorContains(t, err, "at most 30 days")
	assert.Contains(t, buf.String(), "--start 2026-06-01T00:00:00Z --end 2026-07-01T00:00:00Z")
	assert.Contains(t, buf.String(), "--start 2026-04-01T00:00:00Z --end 2026-04-02T00:00:00Z")
}

func TestAuditLogsDownloadServerCursorBugs(t *testing.T) {
	capturePtermOutput(t)
	chunk := gzipMember(t, "{\"n\":1}\n")

	t.Run("has more without cursor", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
		fake, _ := chunkServer(t, []func() (*http.Response, error){
			func() (*http.Response, error) {
				return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: true}), nil
			},
		})
		c := AuditLogsCmd{auditLogs: fake}
		err := c.Download(context.Background(), downloadInput(outPath))
		require.ErrorContains(t, err, "no cursor")
	})

	t.Run("unchanged cursor", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
		repeat := func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		}
		fake, _ := chunkServer(t, []func() (*http.Response, error){repeat, repeat})
		c := AuditLogsCmd{auditLogs: fake}
		err := c.Download(context.Background(), downloadInput(outPath))
		require.ErrorContains(t, err, "unchanged cursor")
	})
}

func TestAuditLogsDownloadJSONLFormat(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl")
	var gotFormat kernel.AuditLogExportChunkParamsFormat
	fake := &FakeAuditLogsService{
		ExportChunkFunc: func(ctx context.Context, query kernel.AuditLogExportChunkParams, opts ...option.RequestOption) (*http.Response, error) {
			gotFormat = query.Format
			return exportChunkResponse(exportChunk{body: []byte("{\"n\":1}\n"), rowCount: 1}), nil
		},
	}
	in := downloadInput(outPath)
	in.Format = "jsonl"
	c := AuditLogsCmd{auditLogs: fake}
	require.NoError(t, c.Download(context.Background(), in))

	assert.Equal(t, kernel.AuditLogExportChunkParamsFormatJSONL, gotFormat)
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":1}\n", string(data))
}

func TestAuditLogsDownloadExcludesGetByDefault(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*AuditLogsDownloadInput)
		wantExclude string
		wantErr     string
	}{
		{name: "default excludes GET", mutate: func(in *AuditLogsDownloadInput) {}, wantExclude: "GET"},
		{name: "include-get lifts the default", mutate: func(in *AuditLogsDownloadInput) { in.IncludeGet = true }, wantExclude: ""},
		{name: "method filter lifts the default", mutate: func(in *AuditLogsDownloadInput) { in.Method = "POST" }, wantExclude: ""},
		{name: "explicit GET exclusion", mutate: func(in *AuditLogsDownloadInput) { in.ExcludeMethod = "get" }, wantExclude: "get"},
		{name: "other exclusion with include-get", mutate: func(in *AuditLogsDownloadInput) {
			in.ExcludeMethod = "OPTIONS"
			in.IncludeGet = true
		}, wantExclude: "OPTIONS"},
		{name: "other exclusion without include-get is rejected", mutate: func(in *AuditLogsDownloadInput) { in.ExcludeMethod = "OPTIONS" },
			wantErr: "add --include-get"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := downloadInput("")
			tc.mutate(&in)
			params, err := buildAuditLogsDownloadParams(in)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantExclude, params.ExcludeMethod.Value)
		})
	}
}

func TestAuditLogsDownloadRejectsInvalidFormat(t *testing.T) {
	c := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}
	in := downloadInput("out.csv")
	in.Format = "csv"
	err := c.Download(context.Background(), in)
	require.ErrorContains(t, err, "--format must be jsonl or jsonl.gz")
}
