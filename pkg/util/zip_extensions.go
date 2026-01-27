package util

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/boyter/gocodewalker"
)

// DefaultExtensionExclusions contains patterns for files that are not needed
// Focus on large directories and files that provide meaningful size reduction.
// These will be passed to gocodewalker's ExcludeDirectory and ExcludeFilename fields.
var DefaultExtensionExclusions = struct {
	// ExcludeDirectory: exact directory names (case-sensitive)
	ExcludeDirectory []string
	// ExcludeFilenamePatterns: patterns for filename matching (handled manually)
	ExcludeFilenamePatterns []string
}{
	ExcludeDirectory: []string{
		// Dependencies
		"node_modules",

		// Version control
		".git",

		// Test/Coverage
		"__tests__",
		"coverage",
	},

	ExcludeFilenamePatterns: []string{
		// Test files
		"*.test.js",
		"*.test.ts",
		"*.spec.js",
		"*.spec.ts",

		// Log files
		"*.log",

		// Temp files
		"*.swp",
	},
}

// ExtensionZipOptions configures extension-specific zipping behavior
type ExtensionZipOptions struct {
	ExcludeDefaults bool // If true, don't apply default exclusions
	Verbose         bool // Track individual excluded files
}

// ZipStats tracks statistics about the zipping operation
type ZipStats struct {
	mu            sync.Mutex
	FilesIncluded int
	FilesExcluded int
	BytesIncluded int64
	BytesExcluded int64
	ExcludedPaths []string
}

func (s *ZipStats) AddIncluded(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FilesIncluded++
	s.BytesIncluded += bytes
}

func (s *ZipStats) AddExcluded(path string, bytes int64, verbose bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FilesExcluded++
	s.BytesExcluded += bytes
	if verbose {
		s.ExcludedPaths = append(s.ExcludedPaths, path)
	}
}

// ZipExtensionDirectory zips a Chrome extension directory with smart defaults
// that automatically exclude development files (node_modules, .git, etc.)
func ZipExtensionDirectory(srcDir, destZip string, opts *ExtensionZipOptions) (*ZipStats, error) {
	if opts == nil {
		opts = &ExtensionZipOptions{}
	}

	stats := &ZipStats{}

	zipFile, err := os.Create(destZip)
	if err != nil {
		return nil, err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Configure file walker with exclusions
	fileQueue := make(chan *gocodewalker.File, 256)
	walker := gocodewalker.NewFileWalker(srcDir, fileQueue)
	walker.IncludeHidden = true

	// Apply default exclusions unless disabled
	if !opts.ExcludeDefaults {
		walker.ExcludeDirectory = append(walker.ExcludeDirectory, DefaultExtensionExclusions.ExcludeDirectory...)
		walker.ExcludeFilename = append(walker.ExcludeFilename, DefaultExtensionExclusions.ExcludeFilenamePatterns...)
	}

	// Track walker errors
	errChan := make(chan error, 1)
	go func() {
		errChan <- walker.Start()
	}()

	// Track directories we've added to avoid duplicates
	dirsAdded := make(map[string]struct{})

	// Process files from walker
	for f := range fileQueue {
		relPath, err := filepath.Rel(srcDir, f.Location)
		if err != nil {
			return stats, err
		}
		relPath = filepath.ToSlash(relPath)

		// Check against pattern-based exclusions (if defaults are enabled)
		shouldExclude := false
		if !opts.ExcludeDefaults {
			// Check filename against patterns only if defaults are enabled
			filename := filepath.Base(f.Location)
			for _, pattern := range DefaultExtensionExclusions.ExcludeFilenamePatterns {
				matched, err := filepath.Match(pattern, filename)
				if err == nil && matched {
					shouldExclude = true
					break
				}
			}
		}

		if shouldExclude {
			continue
		}

		// Ensure parent directories exist in archive
		if dir := filepath.Dir(relPath); dir != "." && dir != "" {
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
						return stats, err
					}
					dirsAdded[current+"/"] = struct{}{}
				}
			}
		}

		// Get file info
		fileInfo, err := os.Lstat(f.Location)
		if err != nil {
			return stats, err
		}

		// Handle symlinks
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(f.Location)
			if err != nil {
				return stats, err
			}

			hdr := &zip.FileHeader{
				Name:   relPath,
				Method: zip.Store,
			}
			hdr.SetMode(os.ModeSymlink | 0777)

			zipFileWriter, err := zipWriter.CreateHeader(hdr)
			if err != nil {
				return stats, err
			}
			if _, err := zipFileWriter.Write([]byte(linkTarget)); err != nil {
				return stats, err
			}
			stats.AddIncluded(int64(len(linkTarget)))
		} else {
			// Regular file
			zipFileWriter, err := zipWriter.Create(relPath)
			if err != nil {
				return stats, err
			}

			file, err := os.Open(f.Location)
			if err != nil {
				return stats, err
			}

			written, err := io.Copy(zipFileWriter, file)
			closeErr := file.Close()
			if closeErr != nil {
				return stats, closeErr
			}
			if err != nil {
				return stats, err
			}

			stats.AddIncluded(written)
		}
	}

	// Check if walker had an error
	if err := <-errChan; err != nil {
		return stats, fmt.Errorf("directory walk failed: %w", err)
	}

	return stats, nil
}
