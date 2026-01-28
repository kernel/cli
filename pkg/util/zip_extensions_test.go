package util

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestZipExtensionDirectory(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "zip-extension-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file structure
	files := map[string]string{
		"manifest.json":           `{"name": "test", "version": "1.0"}`,
		"background.js":           "console.log('background');",
		"content.js":              "console.log('content');",
		"icons/icon.png":          "fake-png-data",
		"node_modules/dep/foo.js": "should be excluded",
		"test.test.js":            "should be excluded",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", fullPath, err)
		}
	}

	// Create output zip file
	tmpZip, err := os.CreateTemp("", "test-extension-*.zip")
	if err != nil {
		t.Fatalf("Failed to create temp zip: %v", err)
	}
	tmpZip.Close()
	defer os.Remove(tmpZip.Name())

	// Test with default exclusions
	t.Run("with default exclusions", func(t *testing.T) {
		_, err := ZipExtensionDirectory(tmpDir, tmpZip.Name(), &ExtensionZipOptions{
			ExcludeDefaults: false,
		})
		if err != nil {
			t.Fatalf("ZipExtensionDirectory failed: %v", err)
		}

		// Verify the zip contents
		r, err := zip.OpenReader(tmpZip.Name())
		if err != nil {
			t.Fatalf("Failed to open zip: %v", err)
		}
		defer r.Close()

		expectedFiles := map[string]bool{
			"manifest.json":  false,
			"background.js":  false,
			"content.js":     false,
			"icons/":         false,
			"icons/icon.png": false,
		}

		for _, f := range r.File {
			if f.FileInfo().IsDir() {
				expectedFiles[f.Name] = true
			} else {
				if _, ok := expectedFiles[f.Name]; ok {
					expectedFiles[f.Name] = true
				} else {
					// Check for files that should be excluded
					if contains([]string{"node_modules", ".git", "test.test.js", "package-lock.json", "__tests__"}, f.Name) {
						t.Errorf("Excluded file found in zip: %s", f.Name)
					}
				}
			}
		}

		for name, found := range expectedFiles {
			if !found && name != "icons/" {
				t.Errorf("Expected file not found in zip: %s", name)
			}
		}
	})

	// Test without exclusions
	t.Run("without default exclusions", func(t *testing.T) {
		tmpZip2, err := os.CreateTemp("", "test-extension-no-exclude-*.zip")
		if err != nil {
			t.Fatalf("Failed to create temp zip: %v", err)
		}
		tmpZip2.Close()
		defer os.Remove(tmpZip2.Name())

		stats, err := ZipExtensionDirectory(tmpDir, tmpZip2.Name(), &ExtensionZipOptions{
			ExcludeDefaults: true, // Disable default exclusions
		})
		if err != nil {
			t.Fatalf("ZipExtensionDirectory failed: %v", err)
		}

		// Should include all files when exclusions are disabled
		if stats.FilesIncluded <= 4 {
			t.Errorf("Expected more than 4 files when exclusions are disabled, got %d", stats.FilesIncluded)
		}
	})
}

func TestDefaultExtensionExclusions(t *testing.T) {
	// Verify the exclusion lists are not empty
	if len(DefaultExtensionExclusions.ExcludeDirectory) == 0 {
		t.Error("ExcludeDirectory should not be empty")
	}
	if len(DefaultExtensionExclusions.ExcludeFilenamePatterns) == 0 {
		t.Error("ExcludeFilenamePatterns should not be empty")
	}

	// Verify specific important exclusions are present
	expectedDirs := []string{"node_modules", ".git", "__tests__", "coverage"}
	for _, path := range expectedDirs {
		if !contains(DefaultExtensionExclusions.ExcludeDirectory, path) {
			t.Errorf("Expected directory exclusion not found: %s", path)
		}
	}

	expectedPatterns := []string{"*.test.js", "*.test.ts", "*.spec.js", "*.spec.ts", "*.log", "*.swp"}
	for _, pattern := range expectedPatterns {
		if !contains(DefaultExtensionExclusions.ExcludeFilenamePatterns, pattern) {
			t.Errorf("Expected pattern exclusion not found: %s", pattern)
		}
	}
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
