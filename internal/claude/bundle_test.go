package claude

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractBundle(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "claude-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a zip of the bundle manually since CreateBundle requires Chrome
	zipPath := filepath.Join(tempDir, "bundle.zip")
	zipFile, err := os.Create(zipPath)
	require.NoError(t, err)

	zipWriter := zip.NewWriter(zipFile)

	// Add extension directory
	_, err = zipWriter.Create(BundleExtensionDir + "/")
	require.NoError(t, err)

	w, err := zipWriter.Create(BundleExtensionDir + "/manifest.json")
	require.NoError(t, err)
	_, err = w.Write([]byte(`{"name": "Claude"}`))
	require.NoError(t, err)

	// Add auth directory
	_, err = zipWriter.Create(BundleAuthStorageDir + "/")
	require.NoError(t, err)

	w, err = zipWriter.Create(BundleAuthStorageDir + "/CURRENT")
	require.NoError(t, err)
	_, err = w.Write([]byte("test"))
	require.NoError(t, err)

	require.NoError(t, zipWriter.Close())
	require.NoError(t, zipFile.Close())

	// Test extraction
	bundle, err := ExtractBundle(zipPath)
	require.NoError(t, err)
	defer bundle.Cleanup()

	assert.NotEmpty(t, bundle.ExtensionPath)
	assert.NotEmpty(t, bundle.AuthStoragePath)
	assert.True(t, bundle.HasAuthStorage())

	// Verify files exist
	manifestPath := filepath.Join(bundle.ExtensionPath, "manifest.json")
	_, err = os.Stat(manifestPath)
	assert.NoError(t, err)

	currentPath := filepath.Join(bundle.AuthStoragePath, "CURRENT")
	_, err = os.Stat(currentPath)
	assert.NoError(t, err)
}

func TestExtractBundleWithoutAuth(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "claude-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a zip without auth storage
	zipPath := filepath.Join(tempDir, "bundle-no-auth.zip")
	zipFile, err := os.Create(zipPath)
	require.NoError(t, err)

	zipWriter := zip.NewWriter(zipFile)

	// Add extension directory only
	_, err = zipWriter.Create(BundleExtensionDir + "/")
	require.NoError(t, err)

	w, err := zipWriter.Create(BundleExtensionDir + "/manifest.json")
	require.NoError(t, err)
	_, err = w.Write([]byte(`{"name": "Claude"}`))
	require.NoError(t, err)

	require.NoError(t, zipWriter.Close())
	require.NoError(t, zipFile.Close())

	// Test extraction
	bundle, err := ExtractBundle(zipPath)
	require.NoError(t, err)
	defer bundle.Cleanup()

	assert.NotEmpty(t, bundle.ExtensionPath)
	assert.Empty(t, bundle.AuthStoragePath)
	assert.False(t, bundle.HasAuthStorage())
}

func TestExtractBundleMissingExtension(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "claude-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create an empty zip (just the zip structure, no directories)
	zipPath := filepath.Join(tempDir, "empty.zip")
	zipFile, err := os.Create(zipPath)
	require.NoError(t, err)

	zipWriter := zip.NewWriter(zipFile)
	require.NoError(t, zipWriter.Close())
	require.NoError(t, zipFile.Close())

	// Test extraction should fail
	_, err = ExtractBundle(zipPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "extension directory")
}

func TestBundleCleanup(t *testing.T) {
	// Create a temporary directory manually
	tempDir, err := os.MkdirTemp("", "claude-cleanup-test-*")
	require.NoError(t, err)

	bundle := &Bundle{
		TempDir:       tempDir,
		ExtensionPath: filepath.Join(tempDir, BundleExtensionDir),
	}

	// Create the extension path
	require.NoError(t, os.MkdirAll(bundle.ExtensionPath, 0755))

	// Verify it exists
	_, err = os.Stat(tempDir)
	require.NoError(t, err)

	// Cleanup
	bundle.Cleanup()

	// Verify it's gone
	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestUnzipSecurityCheck(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "claude-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a malicious zip with path traversal attempt
	zipPath := filepath.Join(tempDir, "malicious.zip")
	zipFile, err := os.Create(zipPath)
	require.NoError(t, err)

	zipWriter := zip.NewWriter(zipFile)

	// Try to add a file with path traversal
	_, err = zipWriter.Create("../../../etc/passwd")
	require.NoError(t, err)

	require.NoError(t, zipWriter.Close())
	require.NoError(t, zipFile.Close())

	// Extraction should fail due to security check
	extractDir := filepath.Join(tempDir, "extracted")
	err = unzip(zipPath, extractDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "illegal file path")
}

func TestConstants(t *testing.T) {
	// Verify constants are set correctly
	assert.Equal(t, "fcoeoabgfenejglbffodgkkbkcdhcgfn", ExtensionID)
	assert.Equal(t, "Claude for Chrome", ExtensionName)
	assert.Equal(t, "extension", BundleExtensionDir)
	assert.Equal(t, "auth-storage", BundleAuthStorageDir)
	assert.Contains(t, SidePanelURL, ExtensionID)
}
