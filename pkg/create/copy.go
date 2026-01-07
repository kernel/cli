package create

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/onkernel/cli/pkg/templates"
)

const (
	DIR_PERM  = 0755 // rwxr-xr-x
	FILE_PERM = 0644 // rw-r--r--
)

// CopyTemplateFiles copies all files and directories from the specified embedded template
// into the target application path. It uses the given language and template names
// to locate the template inside the embedded filesystem.
//
//   - appPath: filesystem path where the files should be written (the project directory)
//   - language: language subdirectory (e.g., "typescript")
//   - template: template subdirectory (e.g., "sample-app")
//
// The function will recursively walk through the embedded template directory and
// replicate all files and folders in appPath. If a file named "_gitignore" is encountered,
// it is renamed to ".gitignore" in the output, to work around file embedding limitations.
//
// Returns an error if the template path is invalid, empty, or if any file operations fail.
func CopyTemplateFiles(appPath, language, template string) error {
	// Build the template path within the embedded FS (e.g., "typescript/sample-app")
	templatePath := filepath.Join(language, template)

	// Check if the template exists and is non-empty
	entries, err := fs.ReadDir(templates.FS, templatePath)
	if err != nil {
		return fmt.Errorf("template not found: %s/%s", language, template)
	}
	if len(entries) == 0 {
		return fmt.Errorf("template directory is empty: %s/%s", language, template)
	}

	// Walk through the embedded template directory and copy contents
	return fs.WalkDir(templates.FS, templatePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Determine the path relative to the root of the template
		relPath, err := filepath.Rel(templatePath, path)
		if err != nil {
			return err
		}

		// Skip the template root directory itself
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(appPath, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, DIR_PERM)
		}

		// Read the file content from the embedded filesystem
		content, err := fs.ReadFile(templates.FS, path)
		if err != nil {
			return fmt.Errorf("failed to read template file %s: %w", path, err)
		}

		// Rename _gitignore to .gitignore in the destination
		if filepath.Base(destPath) == "_gitignore" {
			destPath = filepath.Join(filepath.Dir(destPath), ".gitignore")
		}

		// Write the file to disk in the target project directory
		if err := os.WriteFile(destPath, content, FILE_PERM); err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		return nil
	})
}
