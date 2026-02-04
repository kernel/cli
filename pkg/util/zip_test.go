package util

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestZipDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zip-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	tmpZip, err := os.CreateTemp("", "test-zip-*.zip")
	if err != nil {
		t.Fatalf("Failed to create temp zip: %v", err)
	}
	tmpZip.Close()
	defer os.Remove(tmpZip.Name())

	// Test with exclusions
	t.Run("with exclusions", func(t *testing.T) {
		opts := &ZipOptions{
			ExcludeDirectories:      []string{"node_modules"},
			ExcludeFilenamePatterns: []string{"*.test.js"},
		}
		if err := ZipDirectory(tmpDir, tmpZip.Name(), opts); err != nil {
			t.Fatalf("ZipDirectory failed: %v", err)
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
					t.Errorf("Unexpected file found in zip: %s", f.Name)
				}
			}
		}

		for name, found := range expectedFiles {
			if !found && name != "icons/" {
				t.Errorf("Expected file not found in zip: %s", name)
			}
		}
	})

	// Test without exclusions (nil opts)
	t.Run("without exclusions", func(t *testing.T) {
		tmpZip2, err := os.CreateTemp("", "test-zip-no-exclude-*.zip")
		if err != nil {
			t.Fatalf("Failed to create temp zip: %v", err)
		}
		tmpZip2.Close()
		defer os.Remove(tmpZip2.Name())

		if err := ZipDirectory(tmpDir, tmpZip2.Name(), nil); err != nil {
			t.Fatalf("ZipDirectory failed: %v", err)
		}

		// Verify all files are included (no exclusions)
		r, err := zip.OpenReader(tmpZip2.Name())
		if err != nil {
			t.Fatalf("Failed to open zip: %v", err)
		}
		defer r.Close()

		fileCount := 0
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				fileCount++
			}
		}
		if fileCount <= 4 {
			t.Errorf("Expected more than 4 files when exclusions are disabled, got %d", fileCount)
		}
	})
}
