package browserimport

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestBundleLimitMatchesUploadContract(t *testing.T) {
	require.Equal(t, 64<<20, maxBundleBytes)
}

func TestEncodeBundleReportsActualCompressedSize(t *testing.T) {
	_, err := encodeBundleWithLimit(t.Context(), []byte(`{"version":1}`), nil, 1)
	require.Error(t, err)

	var tooLarge *BundleTooLargeError
	require.ErrorAs(t, err, &tooLarge)
	require.Equal(t, int64(1), tooLarge.Limit)
	require.Greater(t, tooLarge.Size, tooLarge.Limit)
	require.True(t, errors.Is(err, ErrBundleTooLarge))
}

func TestBundleTooLargeErrorReportsMiBWithoutLosingCause(t *testing.T) {
	err := &BundleTooLargeError{Size: 96 << 20, Limit: 64 << 20}
	require.Equal(t, "selected browser data is 96.0 MiB; Kernel supports 64 MiB: browser import bundle is too large", err.Error())
	require.ErrorIs(t, err, ErrBundleTooLarge)
}

func TestBuildProfileBundleIncludesOnlySelectedCategories(t *testing.T) {
	profile := Profile{ID: "helium-default-1234", Name: "Personal", Browser: Browser{ID: "helium", Name: "Helium"}}
	bundle, err := BuildProfileBundle(t.Context(), profile, "my-browser", "test", ProfileData{
		Cookies:   []Cookie{{Domain: ".example.com", Path: "/", Name: "session", Value: "secret"}},
		Storage:   []StorageRecord{{Origin: "https://example.com", Kind: StorageKindLocal, Key: "theme", Value: "dark"}},
		Bookmarks: &BookmarkDocument{Roots: []BookmarkRoot{{Name: "bookmark_bar", Children: []BookmarkNode{{Title: "Kernel", URL: "https://onkernel.com"}}}}},
	})
	require.NoError(t, err)

	decoder, err := zstd.NewReader(bytes.NewReader(bundle))
	require.NoError(t, err)
	defer decoder.Close()
	reader := tar.NewReader(decoder)
	files := make(map[string][]byte)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		files[header.Name], err = io.ReadAll(reader)
		require.NoError(t, err)
	}
	var manifest Manifest
	require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
	require.NotEmpty(t, manifest.Profiles[0].Files.Cookies)
	require.NotEmpty(t, manifest.Profiles[0].Files.Storage)
	require.NotEmpty(t, manifest.Profiles[0].Files.Bookmarks)
	require.Empty(t, manifest.Profiles[0].Files.History)
}

func TestEncodeJSONLEnforcesPortableRecordLimits(t *testing.T) {
	_, err := encodeJSONL("history", make([]HistoryRecord, maxPortableRecords+1))
	require.ErrorContains(t, err, "100000 record import limit")

	_, err = encodeJSONL("local storage", []StorageRecord{{
		Origin: "https://example.com",
		Kind:   StorageKindLocal,
		Key:    "large",
		Value:  strings.Repeat("x", maxPortableRecordBytes),
	}})
	require.ErrorContains(t, err, "1 MiB import limit")
}
