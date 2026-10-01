package browserimport

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCookieBundleMatchesServerContract(t *testing.T) {
	profile := Profile{ID: "chrome-default-1234", Name: "Personal", Browser: Browser{ID: "chrome", Name: "Google Chrome"}}
	bundle, err := BuildCookieBundle(context.Background(), profile, "my-browser", "test", []Cookie{{Domain: ".example.com", Path: "/", Name: "session", Value: "secret", Secure: true}})
	require.NoError(t, err)

	decoder, err := zstd.NewReader(bytes.NewReader(bundle))
	require.NoError(t, err)
	defer decoder.Close()
	reader := tar.NewReader(decoder)
	header, err := reader.Next()
	require.NoError(t, err)
	assert.Equal(t, "manifest.json", header.Name)
	manifestData, err := io.ReadAll(reader)
	require.NoError(t, err)
	var manifest Manifest
	require.NoError(t, json.Unmarshal(manifestData, &manifest))
	assert.Equal(t, BundleVersion, manifest.Version)
	assert.Equal(t, "my-browser", manifest.Profiles[0].TargetName)

	header, err = reader.Next()
	require.NoError(t, err)
	assert.Equal(t, "profiles/chrome-default-1234/cookies.jsonl", header.Name)
	var cookie Cookie
	require.NoError(t, json.NewDecoder(reader).Decode(&cookie))
	assert.Equal(t, "secret", cookie.Value)
}

func TestBuildCookieBundleRequiresCookies(t *testing.T) {
	_, err := BuildCookieBundle(context.Background(), Profile{}, "profile", "test", nil)
	assert.EqualError(t, err, "no cookies were selected")
}
