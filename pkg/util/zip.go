package util

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/boyter/gocodewalker"
)

// ZipOptions which directories and files to exclude from the zip
type ZipOptions struct {
	// ExcludeDirectories: exact directory names to exclude (case-sensitive)
	ExcludeDirectories []string
	// ExcludeFilenamePatterns: glob patterns for filename exclusion (e.g., "*.test.js")
	ExcludeFilenamePatterns []string
}

// ZipDirectory compresses the given source directory into the destination file path.
func ZipDirectory(srcDir, destZip string, opts *ZipOptions) error {
	zipFile, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Use gocodewalker to respect .gitignore/.ignore files while collecting paths
	fileQueue := make(chan *gocodewalker.File, 256)
	walker := gocodewalker.NewFileWalker(srcDir, fileQueue)
	// Include hidden files (to match previous behaviour) but still respect .gitignore rules
	walker.IncludeHidden = true

	// Apply directory exclusions to walker
	if opts != nil {
		walker.ExcludeDirectory = append(walker.ExcludeDirectory, opts.ExcludeDirectories...)
	}

	defer walker.Terminate()

	errChan := make(chan error, 1)
	go func() {
		errChan <- walker.Start()
	}()

	// Track directories we've already added to the zip archive so we don't duplicate entries
	dirsAdded := make(map[string]struct{})

	for f := range fileQueue {
		// Compute path in archive using forward slashes
		relPath, err := filepath.Rel(srcDir, f.Location)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		// Check against pattern-based exclusions if provided
		if opts != nil && len(opts.ExcludeFilenamePatterns) > 0 {
			filename := filepath.Base(f.Location)
			shouldExclude := false
			for _, pattern := range opts.ExcludeFilenamePatterns {
				matched, err := filepath.Match(pattern, filename)
				if err == nil && matched {
					shouldExclude = true
					break
				}
			}
			if shouldExclude {
				continue
			}
		}

		// Ensure parent directories exist in the archive
		if dir := filepath.Dir(relPath); dir != "." && dir != "" {
			// Walk up the directory tree ensuring each level exists
			segments := strings.Split(dir, "/")
			var current string
			for _, segment := range segments {
				if current == "" {
					current = segment
				} else {
					current = current + "/" + segment
				}
				if _, exists := dirsAdded[current+"/"]; !exists {
					if _, err := zipWriter.Create(current + "/"); err != nil {
						return err
					}
					dirsAdded[current+"/"] = struct{}{}
				}
			}
		}

		// Determine if the current path is a symbolic link so we can handle it properly
		fileInfo, err := os.Lstat(f.Location)
		if err != nil {
			return err
		}
		isSymlink := fileInfo.Mode()&os.ModeSymlink != 0

		// Create the file inside the zip archive
		if isSymlink {
			// Read the link target to store inside the archive
			linkTarget, err := os.Readlink(f.Location)
			if err != nil {
				return err
			}

			// Prepare a custom header marking this entry as a symlink.
			hdr := &zip.FileHeader{
				Name:   relPath,
				Method: zip.Store, // No compression; matches behaviour of most zip tools for symlinks
			}
			// Mark as symlink with 0777 permissions (lrwxrwxrwx)
			hdr.SetMode(os.ModeSymlink | 0777)

			zipFileWriter, err := zipWriter.CreateHeader(hdr)
			if err != nil {
				return err
			}
			if _, err := zipFileWriter.Write([]byte(linkTarget)); err != nil {
				return err
			}
		} else {
			zipFileWriter, err := zipWriter.Create(relPath)
			if err != nil {
				return err
			}

			file, err := os.Open(f.Location)
			if err != nil {
				return err
			}
			// Avoid deferring to reduce open FDs on huge trees
			_, err = io.Copy(zipFileWriter, file)
			if closeErr := file.Close(); closeErr != nil {
				return closeErr
			}
			if err != nil {
				return err
			}
		}
	}

	if err := <-errChan; err != nil {
		return fmt.Errorf("directory walk failed: %w", err)
	}

	return nil
}

// Unzip extracts a zip file to the specified directory
func Unzip(zipFilePath, destDir string) error {
	// Open the zip file
	reader, err := zip.OpenReader(zipFilePath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	// Create the destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	// Extract each file
	for _, file := range reader.File {
		// Create the full destination path
		destPath := filepath.Join(destDir, file.Name)

		// Check for directory traversal vulnerabilities
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", file.Name)
		}

		// Handle directories
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Create the containing directory if it doesn't exist
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory path: %w", err)
		}

		// Open the file from the zip
		fileReader, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}
		defer fileReader.Close()

		// Create the destination file
		destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return fmt.Errorf("failed to create destination file (file mode %s): %w", file.Mode().String(), err)
		}
		defer destFile.Close()

		// Copy the contents
		if _, err := io.Copy(destFile, fileReader); err != nil {
			return fmt.Errorf("failed to extract file: %w", err)
		}
	}

	return nil
}
