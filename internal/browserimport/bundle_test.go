package browserimport

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestBuildProfileBundleIncludesOnlySelectedCategories(t *testing.T) {
	profile := Profile{ID: "helium-default-1234", Name: "Personal", Browser: Browser{ID: "helium", Name: "Helium"}}
	bundle, err := BuildProfileBundle(t.Context(), profile, "my-browser", "test", ProfileData{
		Cookies:    []Cookie{{Domain: ".example.com", Path: "/", Name: "session", Value: "secret"}},
		Storage:    []StorageRecord{{Origin: "https://example.com", Kind: StorageKindLocal, Key: "theme", Value: "dark"}},
		Bookmarks:  &BookmarkDocument{Roots: []BookmarkRoot{{Name: "bookmark_bar", Children: []BookmarkNode{{Title: "Kernel", URL: "https://onkernel.com"}}}}},
		Extensions: []Extension{{ID: "abcdefghijklmnopabcdefghijklmnop", Source: "chrome_web_store"}},
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
	require.NotEmpty(t, manifest.Profiles[0].Files.Extensions)
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
