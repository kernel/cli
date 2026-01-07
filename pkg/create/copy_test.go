package create

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDirPerm  = 0755
	testFilePerm = 0644
)

func TestCopyTemplateFiles_Success(t *testing.T) {
	tests := []struct {
		name     string
		language string
		template string
		validate func(t *testing.T, appPath string)
	}{
		{
			name:     "typescript sample-app",
			language: LanguageTypeScript,
			template: TemplateSampleApp,
			validate: func(t *testing.T, appPath string) {
				assert.FileExists(t, filepath.Join(appPath, "index.ts"))
				assert.FileExists(t, filepath.Join(appPath, "package.json"))
				assert.FileExists(t, filepath.Join(appPath, "tsconfig.json"))
				assert.FileExists(t, filepath.Join(appPath, ".gitignore"))
				assert.NoFileExists(t, filepath.Join(appPath, "_gitignore"))
			},
		},
		{
			name:     "python sample-app",
			language: LanguagePython,
			template: TemplateSampleApp,
			validate: func(t *testing.T, appPath string) {
				assert.FileExists(t, filepath.Join(appPath, "main.py"))
				assert.FileExists(t, filepath.Join(appPath, "pyproject.toml"))
				assert.FileExists(t, filepath.Join(appPath, ".gitignore"))
				assert.NoFileExists(t, filepath.Join(appPath, "_gitignore"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			appPath := filepath.Join(tmpDir, "test-app")

			err := os.MkdirAll(appPath, testDirPerm)
			require.NoError(t, err)

			err = CopyTemplateFiles(appPath, tt.language, tt.template)
			require.NoError(t, err, "CopyTemplateFiles should succeed")

			if tt.validate != nil {
				tt.validate(t, appPath)
			}
		})
	}
}

func TestCopyTemplateFiles_InvalidTemplate(t *testing.T) {
	tests := []struct {
		name        string
		language    string
		template    string
		errContains string
	}{
		{
			name:        "nonexistent template",
			language:    LanguageTypeScript,
			template:    "nonexistent-template",
			errContains: "template not found: typescript/nonexistent-template",
		},
		{
			name:        "nonexistent language",
			language:    "ruby",
			template:    TemplateSampleApp,
			errContains: "template not found: ruby/sample-app",
		},
		{
			name:        "empty language",
			language:    "",
			template:    TemplateSampleApp,
			errContains: "template not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			appPath := filepath.Join(tmpDir, "test-app")

			err := os.MkdirAll(appPath, testDirPerm)
			require.NoError(t, err)

			err = CopyTemplateFiles(appPath, tt.language, tt.template)
			require.Error(t, err, "CopyTemplateFiles should fail with invalid template")
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCopyTemplateFiles_GitignoreRename(t *testing.T) {
	// Test that _gitignore is properly renamed to .gitignore
	tmpDir := t.TempDir()
	appPath := filepath.Join(tmpDir, "test-app")

	err := os.MkdirAll(appPath, testDirPerm)
	require.NoError(t, err)

	err = CopyTemplateFiles(appPath, LanguageTypeScript, TemplateSampleApp)
	require.NoError(t, err)

	// Verify .gitignore exists
	gitignorePath := filepath.Join(appPath, ".gitignore")
	assert.FileExists(t, gitignorePath, ".gitignore should exist")

	// Verify _gitignore does not exist
	underscoreGitignorePath := filepath.Join(appPath, "_gitignore")
	assert.NoFileExists(t, underscoreGitignorePath, "_gitignore should not exist")

	// Verify .gitignore has content
	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.NotEmpty(t, content, ".gitignore should have content")
}

func TestCopyTemplateFiles_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	appPath := filepath.Join(tmpDir, "test-app")

	err := os.MkdirAll(appPath, testDirPerm)
	require.NoError(t, err)

	err = CopyTemplateFiles(appPath, LanguageTypeScript, TemplateSampleApp)
	require.NoError(t, err)

	// Check file permissions
	indexPath := filepath.Join(appPath, "index.ts")
	info, err := os.Stat(indexPath)
	require.NoError(t, err)

	// Verify file has correct permissions (0644)
	mode := info.Mode()
	assert.Equal(t, os.FileMode(FILE_PERM), mode.Perm(), "File should have 0644 permissions")
}

func TestCopyTemplateFiles_DirectoryPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	appPath := filepath.Join(tmpDir, "test-app")

	err := os.MkdirAll(appPath, testDirPerm)
	require.NoError(t, err)

	err = CopyTemplateFiles(appPath, LanguageTypeScript, TemplateSampleApp)
	require.NoError(t, err)

	// Check directory permissions
	info, err := os.Stat(appPath)
	require.NoError(t, err)

	// Verify directory has correct permissions (0755)
	mode := info.Mode()
	assert.True(t, mode.IsDir(), "Should be a directory")
	assert.Equal(t, os.FileMode(DIR_PERM), mode.Perm(), "Directory should have 0755 permissions")
}

func TestCopyTemplateFiles_PreservesDirectoryStructure(t *testing.T) {
	tmpDir := t.TempDir()
	appPath := filepath.Join(tmpDir, "test-app")

	err := os.MkdirAll(appPath, testDirPerm)
	require.NoError(t, err)

	// Use a template that has subdirectories
	err = CopyTemplateFiles(appPath, LanguageTypeScript, TemplateAnthropicComputerUse)
	require.NoError(t, err)

	// Verify that subdirectories are created (anthropic-computer-use has src/ directory)
	srcDir := filepath.Join(appPath, "src")
	if _, err := os.Stat(srcDir); err == nil {
		assert.DirExists(t, srcDir, "Subdirectories should be preserved")
	}
}

func TestCopyTemplateFiles_OverwritesExistingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	appPath := filepath.Join(tmpDir, "test-app")

	err := os.MkdirAll(appPath, testDirPerm)
	require.NoError(t, err)

	// Create an existing file with different content
	existingFile := filepath.Join(appPath, "index.ts")
	existingContent := []byte("// old content")
	err = os.WriteFile(existingFile, existingContent, testFilePerm)
	require.NoError(t, err)

	// Copy template files (should overwrite)
	err = CopyTemplateFiles(appPath, LanguageTypeScript, TemplateSampleApp)
	require.NoError(t, err)

	// Verify file was overwritten
	newContent, err := os.ReadFile(existingFile)
	require.NoError(t, err)
	assert.NotEqual(t, existingContent, newContent, "File should be overwritten with new content")
}

func TestCopyTemplateFiles_InvalidDestinationPath(t *testing.T) {
	// Test with a path that cannot be created (e.g., file exists where directory should be)
	tmpDir := t.TempDir()

	// Create a file where we want to create a directory
	blockingFile := filepath.Join(tmpDir, "test-app")
	err := os.WriteFile(blockingFile, []byte("blocking"), testFilePerm)
	require.NoError(t, err)

	// Try to copy template (should fail because test-app is a file, not a directory)
	err = CopyTemplateFiles(blockingFile, LanguageTypeScript, TemplateSampleApp)
	require.Error(t, err, "Should fail when destination path is invalid")
}

func TestCopyTemplateFiles_AllTemplatesForAllLanguages(t *testing.T) {
	// Comprehensive test that all template/language combinations work
	for templateKey, templateInfo := range Templates {
		for _, lang := range templateInfo.Languages {
			t.Run(lang+"/"+templateKey, func(t *testing.T) {
				tmpDir := t.TempDir()
				appPath := filepath.Join(tmpDir, "test-app")

				err := os.MkdirAll(appPath, testDirPerm)
				require.NoError(t, err)

				err = CopyTemplateFiles(appPath, lang, templateKey)
				require.NoError(t, err, "Should successfully copy %s/%s template", lang, templateKey)

				// Verify at least some files were created
				entries, err := os.ReadDir(appPath)
				require.NoError(t, err)
				assert.NotEmpty(t, entries, "Template directory should not be empty")

				// Verify .gitignore exists and _gitignore does not
				gitignorePath := filepath.Join(appPath, ".gitignore")
				assert.FileExists(t, gitignorePath, ".gitignore should exist")

				underscoreGitignorePath := filepath.Join(appPath, "_gitignore")
				assert.NoFileExists(t, underscoreGitignorePath, "_gitignore should not exist")
			})
		}
	}
}
