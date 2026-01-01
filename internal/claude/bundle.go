package claude

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Bundle represents an extracted Claude extension bundle.
type Bundle struct {
	// ExtensionPath is the path to the extracted extension directory
	ExtensionPath string

	// AuthStoragePath is the path to the extracted auth storage directory (may be empty if no auth)
	AuthStoragePath string

	// TempDir is the temporary directory containing the extracted bundle (for cleanup)
	TempDir string
}

// Cleanup removes the temporary directory containing the extracted bundle.
func (b *Bundle) Cleanup() {
	if b.TempDir != "" {
		os.RemoveAll(b.TempDir)
	}
}

// CreateBundle creates a zip bundle from Chrome's Claude extension and optionally its auth storage.
func CreateBundle(outputPath string, chromeProfile string, includeAuth bool) error {
	// Get extension path
	extPath, err := GetChromeExtensionPath(chromeProfile)
	if err != nil {
		return fmt.Errorf("failed to locate Claude extension: %w", err)
	}

	// Get auth storage path (optional)
	var authPath string
	if includeAuth {
		authPath, err = GetChromeAuthStoragePath(chromeProfile)
		if err != nil {
			// Auth storage is optional - warn but continue
			fmt.Printf("Warning: Could not locate auth storage: %v\n", err)
			authPath = ""
		}
	}

	// Create the output zip file
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create bundle file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add extension files under "extension/" prefix
	if err := addDirectoryToZip(zipWriter, extPath, BundleExtensionDir); err != nil {
		return fmt.Errorf("failed to add extension to bundle: %w", err)
	}

	// Add auth storage files under "auth-storage/" prefix (if available)
	if authPath != "" {
		if err := addDirectoryToZip(zipWriter, authPath, BundleAuthStorageDir); err != nil {
			return fmt.Errorf("failed to add auth storage to bundle: %w", err)
		}
	}

	return nil
}

// ExtractBundle extracts a bundle zip file to a temporary directory.
// Returns a Bundle struct with paths to the extracted directories.
// The caller is responsible for calling Bundle.Cleanup() when done.
func ExtractBundle(bundlePath string) (*Bundle, error) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "claude-bundle-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Extract the zip
	if err := unzip(bundlePath, tempDir); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to extract bundle: %w", err)
	}

	bundle := &Bundle{
		TempDir: tempDir,
	}

	// Check for extension directory
	extDir := filepath.Join(tempDir, BundleExtensionDir)
	if _, err := os.Stat(extDir); err == nil {
		bundle.ExtensionPath = extDir
	} else {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("bundle does not contain extension directory")
	}

	// Check for auth storage directory (optional)
	authDir := filepath.Join(tempDir, BundleAuthStorageDir)
	if _, err := os.Stat(authDir); err == nil {
		bundle.AuthStoragePath = authDir
	}

	return bundle, nil
}

// HasAuthStorage returns true if the bundle contains auth storage.
func (b *Bundle) HasAuthStorage() bool {
	return b.AuthStoragePath != ""
}

// addDirectoryToZip adds all files from a directory to a zip archive under the given prefix.
func addDirectoryToZip(zipWriter *zip.Writer, srcDir, prefix string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == srcDir {
			return nil
		}

		// Compute relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes and add prefix
		zipPath := filepath.ToSlash(filepath.Join(prefix, relPath))

		if info.IsDir() {
			// Add directory entry
			_, err := zipWriter.Create(zipPath + "/")
			return err
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}

			header := &zip.FileHeader{
				Name:   zipPath,
				Method: zip.Store,
			}
			header.SetMode(os.ModeSymlink | 0777)

			writer, err := zipWriter.CreateHeader(header)
			if err != nil {
				return err
			}
			_, err = writer.Write([]byte(linkTarget))
			return err
		}

		// Regular file
		writer, err := zipWriter.Create(zipPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// unzip extracts a zip file to the destination directory.
func unzip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		destPath := filepath.Join(destDir, file.Name)

		// Security check: prevent zip slip
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Handle symlinks
		if file.Mode()&os.ModeSymlink != 0 {
			fileReader, err := file.Open()
			if err != nil {
				return err
			}
			linkTarget, err := io.ReadAll(fileReader)
			fileReader.Close()
			if err != nil {
				return err
			}
			if err := os.Symlink(string(linkTarget), destPath); err != nil {
				return err
			}
			continue
		}

		// Regular file
		destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		fileReader, err := file.Open()
		if err != nil {
			destFile.Close()
			return err
		}

		_, err = io.Copy(destFile, fileReader)
		fileReader.Close()
		destFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
