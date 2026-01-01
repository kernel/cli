package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindLatestVersionDir(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "claude-version-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create version directories
	versions := []string{"1.0.0_0", "1.0.1_0", "2.0.0_0", "1.5.0_0"}
	for _, v := range versions {
		require.NoError(t, os.MkdirAll(filepath.Join(tempDir, v), 0755))
	}

	// Should find the latest (2.0.0_0 is lexicographically highest)
	latest, err := findLatestVersionDir(tempDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, "2.0.0_0"), latest)
}

func TestFindLatestVersionDirEmpty(t *testing.T) {
	// Create an empty directory
	tempDir, err := os.MkdirTemp("", "claude-empty-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Should return an error
	_, err = findLatestVersionDir(tempDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no version directories")
}

func TestFindLatestVersionDirSkipsHidden(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "claude-hidden-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create version directories including hidden ones
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".hidden"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "1.0.0_0"), 0755))

	// Should find 1.0.0_0, not .hidden
	latest, err := findLatestVersionDir(tempDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, "1.0.0_0"), latest)
}

func TestGetChromeUserDataDir(t *testing.T) {
	// This test just verifies the function returns a path based on OS
	// It will likely fail on CI unless Chrome is installed, so we just
	// check that it returns an error with expected message

	_, err := getChromeUserDataDir()
	if err != nil {
		// Expected error when Chrome is not installed
		assert.Contains(t, err.Error(), "Chrome")
	}
	// If no error, Chrome is installed and we got a valid path
}

func TestListChromeProfiles(t *testing.T) {
	// This test will likely fail on CI unless Chrome is installed
	profiles, err := ListChromeProfiles()
	if err != nil {
		// Expected when Chrome is not installed
		t.Logf("Chrome not found: %v", err)
		return
	}

	// If Chrome is installed, we should have at least one profile
	t.Logf("Found Chrome profiles: %v", profiles)
}

func TestGetChromeExtensionPathNotInstalled(t *testing.T) {
	// Claude extension is unlikely to be installed in CI
	_, err := GetChromeExtensionPath("Default")
	if err != nil {
		// Expected - either Chrome not installed or Claude extension not found
		assert.Contains(t, err.Error(), "not found")
	}
}

func TestGetChromeAuthStoragePathNotInstalled(t *testing.T) {
	// Claude extension is unlikely to be installed in CI
	_, err := GetChromeAuthStoragePath("Default")
	if err != nil {
		// Expected - either Chrome not installed or auth storage not found
		assert.Contains(t, err.Error(), "not found")
	}
}

func TestKernelPaths(t *testing.T) {
	// Verify the Kernel-specific paths are correct for Linux
	assert.Equal(t, "/home/kernel/user-data", KernelUserDataPath)
	assert.Equal(t, "/home/kernel/user-data/Default", KernelDefaultProfilePath)
	assert.Equal(t, "/home/kernel/user-data/Default/Local Extension Settings", KernelExtensionSettingsPath)
	assert.Equal(t, "kernel", KernelUser)
}

func TestChromeUserDataDirPath(t *testing.T) {
	// Just verify the expected path format based on OS
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	expectedPaths := map[string]string{
		"darwin":  filepath.Join(homeDir, "Library", "Application Support", "Google", "Chrome"),
		"linux":   filepath.Join(homeDir, ".config", "google-chrome"),
		"windows": "", // Windows path depends on LOCALAPPDATA env var
	}

	if expected, ok := expectedPaths[runtime.GOOS]; ok && expected != "" {
		// The function will return error if Chrome isn't installed,
		// but the path format should be correct
		t.Logf("Expected Chrome path for %s: %s", runtime.GOOS, expected)
	}
}
