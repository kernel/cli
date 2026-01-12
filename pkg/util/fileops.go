package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyFile copies a single file from src to dst
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Copy file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}

// CopyDir recursively copies a directory from src to dst
func CopyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read source directory
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// ModifyFile replaces all occurrences of oldStr with newStr in the file.
// Returns an error if the pattern is not found in the file
func ModifyFile(path, oldStr, newStr string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	original := string(content)

	// Check if pattern exists in the file
	if !strings.Contains(original, oldStr) {
		return fmt.Errorf("pattern %q not found in file %s", oldStr, path)
	}

	modified := strings.ReplaceAll(original, oldStr, newStr)

	// Skip writing if the replacement is a no-op (oldStr == newStr or pattern doesn't change content)
	if modified == original {
		return nil
	}

	return os.WriteFile(path, []byte(modified), 0644)
}

