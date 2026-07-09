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
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type exportChunk struct {
	body         []byte
	rowCount     int
	rowCountRaw  string
	hasMore      bool
	nextCursor   string
	sha          string // overrides the computed body sha when set
	omitSha      bool
	omitHasMore  bool
	omitRowCount bool
}

func exportChunkResponse(c exportChunk) *http.Response {
	sha := c.sha
	if sha == "" {
		sum := sha256.Sum256(c.body)
		sha = hex.EncodeToString(sum[:])
	}
	header := http.Header{}
	if !c.omitHasMore {
		header.Set("X-Has-More", strconv.FormatBool(c.hasMore))
	}
	if !c.omitRowCount {
		rowCount := c.rowCountRaw
		if rowCount == "" {
			rowCount = strconv.Itoa(c.rowCount)
		}
		header.Set("X-Row-Count", rowCount)
	}
	if !c.omitSha {
		header.Set("X-Content-Sha256", sha)
	}
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

const auditLogsDownloadTestIdentity = "https://api.test|org:org-test"

func auditLogsDownloadTestCmd(service AuditLogsService) AuditLogsCmd {
	return AuditLogsCmd{auditLogs: service, identity: auditLogsDownloadTestIdentity}
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

	c := auditLogsDownloadTestCmd(fake)
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

	c := auditLogsDownloadTestCmd(fake)
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

	c := auditLogsDownloadTestCmd(fake)
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
	c := auditLogsDownloadTestCmd(fake)
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	fake2, cursors := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk2, rowCount: 1, hasMore: false}), nil
		},
	})
	c2 := auditLogsDownloadTestCmd(fake2)
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
	c := auditLogsDownloadTestCmd(fake)
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
	c2 := auditLogsDownloadTestCmd(fake2)
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
	c := auditLogsDownloadTestCmd(fake)
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
	c := auditLogsDownloadTestCmd(fake)
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	chunk := gzipMember(t, "{\"n\":9}\n")
	fake2, cursors := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: false}), nil
		},
	})
	in := downloadInput(outPath)
	in.Force = true
	c2 := auditLogsDownloadTestCmd(fake2)
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

	c := auditLogsDownloadTestCmd(&FakeAuditLogsService{})
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
	c := auditLogsDownloadTestCmd(fake)
	require.NoError(t, c.Download(context.Background(), downloadInput(outPath)))

	// Simulate a crash after the final state save but before cleanup by
	// re-creating the completed state file.
	statePath := outPath + ".state.json"
	sum := sha256.Sum256(chunk1)
	require.NoError(t, saveAuditLogsDownloadState(statePath, auditLogsDownloadState{
		Version:         auditLogsDownloadStateVersion,
		Params:          mustFingerprint(t),
		Identity:        auditLogsDownloadTestIdentity,
		BytesWritten:    int64(len(chunk1)),
		CommittedSHA256: hex.EncodeToString(sum[:]),
		Chunks:          1,
		Rows:            1,
	}))

	c2 := auditLogsDownloadTestCmd(&FakeAuditLogsService{})
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
	c := auditLogsDownloadTestCmd(&FakeAuditLogsService{})
	err := c.Download(context.Background(), AuditLogsDownloadInput{Start: "2026-04-01", End: "2026-06-30"})
	require.ErrorContains(t, err, "at most 30 days")
	assert.Contains(t, buf.String(), "--start 2026-05-31T00:00:00Z --end 2026-06-30T00:00:00Z")
	assert.Contains(t, buf.String(), "--start 2026-04-01T00:00:00Z --end 2026-05-01T00:00:00Z")
}

func TestAuditLogsDownloadDateEndIsExclusive(t *testing.T) {
	in := downloadInput("")
	params, err := buildAuditLogsDownloadParams(in)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC), params.End)
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
		c := auditLogsDownloadTestCmd(fake)
		err := c.Download(context.Background(), downloadInput(outPath))
		require.ErrorContains(t, err, "no cursor")
	})

	t.Run("unchanged cursor", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
		repeat := func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		}
		fake, _ := chunkServer(t, []func() (*http.Response, error){repeat, repeat})
		c := auditLogsDownloadTestCmd(fake)
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
	c := auditLogsDownloadTestCmd(fake)
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

func TestAuditLogsDownloadRejectsDifferentCredentials(t *testing.T) {
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
	c := AuditLogsCmd{auditLogs: fake, identity: "org:org-a"}
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	c2 := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}, identity: "org:org-b"}
	err := c2.Download(context.Background(), downloadInput(outPath))
	require.ErrorContains(t, err, "different API origin or credential")
}

func TestAuditLogsDownloadRejectsMissingOrShortenedOutput(t *testing.T) {
	capturePtermOutput(t)
	interrupted := func(t *testing.T) (string, AuditLogsCmd) {
		t.Helper()
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
		c := auditLogsDownloadTestCmd(fake)
		require.Error(t, c.Download(context.Background(), downloadInput(outPath)))
		return outPath, auditLogsDownloadTestCmd(&FakeAuditLogsService{})
	}

	t.Run("missing output", func(t *testing.T) {
		outPath, c := interrupted(t)
		require.NoError(t, os.Remove(outPath))
		err := c.Download(context.Background(), downloadInput(outPath))
		require.ErrorContains(t, err, "is missing")
	})

	t.Run("shortened output", func(t *testing.T) {
		outPath, c := interrupted(t)
		require.NoError(t, os.Truncate(outPath, 1))
		err := c.Download(context.Background(), downloadInput(outPath))
		require.ErrorContains(t, err, "shorter than the recorded progress")
	})
}

func TestAuditLogsDownloadFirstChunkFailureRetriesWithoutForce(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	failing, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	c := auditLogsDownloadTestCmd(failing)
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	chunk := gzipMember(t, "{\"n\":1}\n")
	fake, cursors := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: false}), nil
		},
	})
	c2 := auditLogsDownloadTestCmd(fake)
	require.NoError(t, c2.Download(context.Background(), downloadInput(outPath)), "a failed first chunk must not require --force to retry")

	assert.Equal(t, []string{""}, *cursors)
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":1}\n", gunzipAll(t, data))
}

func TestAuditLogsDownloadFailedForceLeavesNoStaleProgress(t *testing.T) {
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
	c := auditLogsDownloadTestCmd(fake)
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	// --force run that dies before its first chunk.
	failing, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	in := downloadInput(outPath)
	in.Force = true
	c2 := auditLogsDownloadTestCmd(failing)
	require.Error(t, c2.Download(context.Background(), in))

	// The plain rerun must start from scratch, not resume chunk 1's stale cursor.
	chunk := gzipMember(t, "{\"n\":9}\n")
	fake3, cursors := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: false}), nil
		},
	})
	c3 := auditLogsDownloadTestCmd(fake3)
	require.NoError(t, c3.Download(context.Background(), downloadInput(outPath)))

	assert.Equal(t, []string{""}, *cursors)
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":9}\n", gunzipAll(t, data))
}

func TestAuditLogsDownloadFailsClosedOnMissingHeaders(t *testing.T) {
	capturePtermOutput(t)
	chunk := gzipMember(t, "{\"n\":1}\n")

	t.Run("missing X-Has-More", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
		fake, _ := chunkServer(t, []func() (*http.Response, error){
			func() (*http.Response, error) {
				return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, omitHasMore: true}), nil
			},
		})
		c := auditLogsDownloadTestCmd(fake)
		err := c.Download(context.Background(), downloadInput(outPath))
		require.ErrorContains(t, err, "X-Has-More")
		data, readErr := os.ReadFile(outPath)
		require.NoError(t, readErr)
		assert.Empty(t, data, "an unvalidated response must not be written")
	})

	t.Run("missing X-Content-Sha256", func(t *testing.T) {
		outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
		fake, _ := chunkServer(t, []func() (*http.Response, error){
			func() (*http.Response, error) {
				return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, omitSha: true}), nil
			},
		})
		c := auditLogsDownloadTestCmd(fake)
		err := c.Download(context.Background(), downloadInput(outPath))
		require.ErrorContains(t, err, "X-Content-Sha256")
	})
}

func TestAuditLogsDownloadRejectsOversizedChunk(t *testing.T) {
	capturePtermOutput(t)
	prev := auditLogsDownloadMaxChunkBytes
	auditLogsDownloadMaxChunkBytes = 8
	t.Cleanup(func() { auditLogsDownloadMaxChunkBytes = prev })

	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: []byte("well over eight bytes"), rowCount: 1}), nil
		},
	})
	c := auditLogsDownloadTestCmd(fake)
	err := c.Download(context.Background(), downloadInput(outPath))
	require.ErrorContains(t, err, "refusing to buffer")
}

func TestAuditLogsDownloadFilePermissions(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk := gzipMember(t, "{\"n\":1}\n")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	c := auditLogsDownloadTestCmd(fake)
	require.Error(t, c.Download(context.Background(), downloadInput(outPath)))

	outInfo, err := os.Stat(outPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), outInfo.Mode().Perm())
	stateInfo, err := os.Stat(outPath + ".state.json")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), stateInfo.Mode().Perm())
	lockInfo, err := os.Stat(outPath + ".lock")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), lockInfo.Mode().Perm())
}

func TestAuditLogsDownloadRequiresStableTimeBounds(t *testing.T) {
	c := auditLogsDownloadTestCmd(&FakeAuditLogsService{})
	for _, in := range []AuditLogsDownloadInput{
		{End: "2026-06-28"},
		{Start: "2026-06-01"},
		{},
	} {
		err := c.Download(context.Background(), in)
		require.ErrorContains(t, err, "--start and --end are required")
	}
}

func TestAuditLogsCredentialIdentityIncludesAPIOrigin(t *testing.T) {
	t.Setenv("KERNEL_API_KEY", "test-key")
	t.Setenv("KERNEL_BASE_URL", "HTTPS://API.EXAMPLE.COM/")
	first, err := auditLogsCredentialIdentity()
	require.NoError(t, err)
	assert.Contains(t, first, "https://api.example.com|key:")

	t.Setenv("KERNEL_BASE_URL", "https://api.other.example.com")
	second, err := auditLogsCredentialIdentity()
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
}

func TestAuditLogsDownloadRejectsEmptyIdentity(t *testing.T) {
	c := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}
	err := c.Download(context.Background(), downloadInput(filepath.Join(t.TempDir(), "out.jsonl.gz")))
	require.ErrorContains(t, err, "without an API origin and credential identity")
}

func TestAuditLogsDownloadRejectsModifiedCommittedOutput(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	chunk := gzipMember(t, "{\"n\":1}\n")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1, hasMore: true, nextCursor: "c1"}), nil
		},
		func() (*http.Response, error) { return nil, errors.New("boom") },
	})
	require.Error(t, auditLogsDownloadTestCmd(fake).Download(context.Background(), downloadInput(outPath)))

	require.NoError(t, os.WriteFile(outPath, bytes.Repeat([]byte{'x'}, len(chunk)), 0o600))
	called := false
	resume := &FakeAuditLogsService{ExportChunkFunc: func(ctx context.Context, query kernel.AuditLogExportChunkParams, opts ...option.RequestOption) (*http.Response, error) {
		called = true
		return nil, errors.New("should not fetch")
	}}
	err := auditLogsDownloadTestCmd(resume).Download(context.Background(), downloadInput(outPath))
	require.ErrorContains(t, err, "does not match the recorded checksum")
	assert.False(t, called)
}

func TestAuditLogsDownloadOutputLock(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	first, _, err := openAndLockAuditLogsDownloadOutput(outPath)
	require.NoError(t, err)

	_, _, err = openAndLockAuditLogsDownloadOutput(outPath)
	require.ErrorContains(t, err, "already using")
	require.NoError(t, first.Close())

	third, _, err := openAndLockAuditLogsDownloadOutput(outPath)
	require.NoError(t, err)
	require.NoError(t, third.Close())
}

func TestAuditLogsDownloadPathLockSurvivesOutputRename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows prevents renaming an open output file")
	}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.jsonl.gz")
	movedPath := filepath.Join(dir, "moved.jsonl.gz")
	first, _, err := openAndLockAuditLogsDownloadOutput(outPath)
	require.NoError(t, err)
	require.NoError(t, os.Rename(outPath, movedPath))

	_, _, err = openAndLockAuditLogsDownloadOutput(outPath)
	require.ErrorContains(t, err, "already using")
	require.NoError(t, first.Close())

	third, created, err := openAndLockAuditLogsDownloadOutput(outPath)
	require.NoError(t, err)
	assert.True(t, created)
	require.NoError(t, third.Close())
}

func TestAuditLogsDownloadDetectsOutputRename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows prevents renaming an open output file")
	}
	capturePtermOutput(t)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.jsonl.gz")
	movedPath := filepath.Join(dir, "moved.jsonl.gz")
	chunk := gzipMember(t, "{\"n\":1}\n")
	fake := &FakeAuditLogsService{
		ExportChunkFunc: func(ctx context.Context, query kernel.AuditLogExportChunkParams, opts ...option.RequestOption) (*http.Response, error) {
			require.NoError(t, os.Rename(outPath, movedPath))
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1}), nil
		},
	}

	err := auditLogsDownloadTestCmd(fake).Download(context.Background(), downloadInput(outPath))
	require.ErrorContains(t, err, "changed while the download was running")
}

func TestAuditLogsDownloadRejectsSymlinkOutput(t *testing.T) {
	capturePtermOutput(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	outPath := filepath.Join(dir, "out.jsonl.gz")
	require.NoError(t, os.WriteFile(target, []byte("keep"), 0o600))
	if err := os.Symlink(target, outPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	in := downloadInput(outPath)
	in.Force = true
	err := auditLogsDownloadTestCmd(&FakeAuditLogsService{}).Download(context.Background(), in)
	require.ErrorContains(t, err, "must be a regular file")
	data, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "keep", string(data))
}

func TestAuditLogsDownloadStateReadIsBoundedAndForceBypassesIt(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "out.state.json")
	require.NoError(t, os.WriteFile(statePath, make([]byte, auditLogsDownloadMaxStateBytes+1), 0o600))

	_, _, err := loadAuditLogsDownloadState(statePath, "params", auditLogsDownloadTestIdentity, false)
	require.ErrorContains(t, err, "is too large")
	state, exists, err := loadAuditLogsDownloadState(statePath, "params", auditLogsDownloadTestIdentity, true)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, auditLogsDownloadEmptySHA256, state.CommittedSHA256)
	_, err = os.Lstat(statePath)
	assert.True(t, os.IsNotExist(err))
}

func TestAuditLogsDownloadRejectsInconsistentState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "out.state.json")
	require.NoError(t, saveAuditLogsDownloadState(statePath, auditLogsDownloadState{
		Version:         auditLogsDownloadStateVersion,
		Params:          "params",
		Identity:        auditLogsDownloadTestIdentity,
		BytesWritten:    1,
		CommittedSHA256: auditLogsDownloadEmptySHA256,
	}))

	_, _, err := loadAuditLogsDownloadState(statePath, "params", auditLogsDownloadTestIdentity, false)
	require.ErrorContains(t, err, "zero-chunk state contains committed progress")
}

func TestAuditLogsDownloadForceSecuresExistingOutput(t *testing.T) {
	capturePtermOutput(t)
	outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
	require.NoError(t, os.WriteFile(outPath, []byte("old"), 0o644))
	require.NoError(t, os.Chmod(outPath, 0o644))
	chunk := gzipMember(t, "{\"n\":1}\n")
	fake, _ := chunkServer(t, []func() (*http.Response, error){
		func() (*http.Response, error) {
			return exportChunkResponse(exportChunk{body: chunk, rowCount: 1}), nil
		},
	})
	in := downloadInput(outPath)
	in.Force = true
	require.NoError(t, auditLogsDownloadTestCmd(fake).Download(context.Background(), in))

	info, err := os.Stat(outPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestAuditLogsDownloadRejectsInvalidRowCount(t *testing.T) {
	capturePtermOutput(t)
	chunk := gzipMember(t, "{\"n\":1}\n")
	tests := []struct {
		name  string
		chunk exportChunk
	}{
		{name: "missing", chunk: exportChunk{body: chunk, omitRowCount: true}},
		{name: "malformed", chunk: exportChunk{body: chunk, rowCountRaw: "nope"}},
		{name: "negative", chunk: exportChunk{body: chunk, rowCountRaw: "-1"}},
		{name: "above server limit", chunk: exportChunk{body: chunk, rowCountRaw: "50001"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outPath := filepath.Join(t.TempDir(), "out.jsonl.gz")
			fake, _ := chunkServer(t, []func() (*http.Response, error){
				func() (*http.Response, error) { return exportChunkResponse(tc.chunk), nil },
			})
			err := auditLogsDownloadTestCmd(fake).Download(context.Background(), downloadInput(outPath))
			require.ErrorContains(t, err, "X-Row-Count")
			data, readErr := os.ReadFile(outPath)
			require.NoError(t, readErr)
			assert.Empty(t, data)
		})
	}
}

func TestAuditLogsDownloadRejectsInvalidFormat(t *testing.T) {
	c := auditLogsDownloadTestCmd(&FakeAuditLogsService{})
	in := downloadInput("out.csv")
	in.Format = "csv"
	err := c.Download(context.Background(), in)
	require.ErrorContains(t, err, "--format must be jsonl or jsonl.gz")
}
