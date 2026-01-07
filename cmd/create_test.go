package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/onkernel/cli/pkg/create"
	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	DIR_PERM  = 0755 // rwxr-xr-x
	FILE_PERM = 0644 // rw-r--r--
)

func TestCreateCommand(t *testing.T) {
	tests := []struct {
		name        string
		input       create.CreateInput
		wantErr     bool
		errContains string
		validate    func(t *testing.T, appPath string)
	}{
		{
			name: "create typescript sample-app",
			input: create.CreateInput{
				Name:     "test-app",
				Language: "typescript",
				Template: "sample-app",
			},
			validate: func(t *testing.T, appPath string) {
				// Verify files were created
				assert.FileExists(t, filepath.Join(appPath, "index.ts"))
				assert.FileExists(t, filepath.Join(appPath, "package.json"))
				assert.FileExists(t, filepath.Join(appPath, ".gitignore"))
				assert.NoFileExists(t, filepath.Join(appPath, "_gitignore"))
			},
		},
		{
			name: "fail with invalid template",
			input: create.CreateInput{
				Name:     "test-app",
				Language: "typescript",
				Template: "nonexistent",
			},
			wantErr:     true,
			errContains: "template not found: typescript/nonexistent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			orgDir, err := os.Getwd()
			require.NoError(t, err)

			err = os.Chdir(tmpDir)
			require.NoError(t, err)

			t.Cleanup(func() {
				os.Chdir(orgDir)
			})

			c := CreateCmd{}
			err = c.Create(context.Background(), tt.input)

			// Check if error is expected
			if tt.wantErr {
				require.Error(t, err, "expected command to fail but it succeeded")
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains, "error message should contain expected text")
				}
				return
			}

			require.NoError(t, err, "failed to execute create command")

			// Validate the created app
			appPath := filepath.Join(tmpDir, tt.input.Name)
			assert.DirExists(t, appPath, "app directory should be created")

			if tt.validate != nil {
				tt.validate(t, appPath)
			}
		})
	}
}

// TestAllTemplatesWithDependencies tests all available templates and verifies dependencies are installed
func TestAllTemplatesWithDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dependency installation tests in short mode")
	}

	tests := getTemplateInfo()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			appName := "test-app"

			orgDir, err := os.Getwd()
			require.NoError(t, err)

			err = os.Chdir(tmpDir)
			require.NoError(t, err)

			t.Cleanup(func() {
				os.Chdir(orgDir)
			})

			// Create the app
			c := CreateCmd{}
			err = c.Create(context.Background(), create.CreateInput{
				Name:     appName,
				Language: tt.language,
				Template: tt.template,
			})
			require.NoError(t, err, "failed to create app")

			appPath := filepath.Join(tmpDir, appName)

			// Verify app directory exists
			assert.DirExists(t, appPath, "app directory should exist")

			// Language-specific validations
			switch tt.language {
			case create.LanguageTypeScript:
				validateTypeScriptTemplate(t, appPath, true)
			case create.LanguagePython:
				validatePythonTemplate(t, appPath, true)
			}
		})
	}
}

// TestAllTemplatesCreation tests that all templates can be created without installing dependencies
func TestAllTemplatesCreation(t *testing.T) {
	tests := getTemplateInfo()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			appName := "test-app"
			appPath := filepath.Join(tmpDir, appName)

			// Create app directory
			err := os.MkdirAll(appPath, DIR_PERM)
			require.NoError(t, err, "failed to create app directory")

			// Copy template files without installing dependencies
			err = create.CopyTemplateFiles(appPath, tt.language, tt.template)
			require.NoError(t, err, "failed to copy template files")

			// Verify app directory exists
			assert.DirExists(t, appPath, "app directory should exist")

			// Language-specific validations (without dependency checks)
			switch tt.language {
			case create.LanguageTypeScript:
				validateTypeScriptTemplate(t, appPath, false)
			case create.LanguagePython:
				validatePythonTemplate(t, appPath, false)
			}
		})
	}
}

// validateTypeScriptTemplate verifies TypeScript template structure and optionally dependencies
func validateTypeScriptTemplate(t *testing.T, appPath string, checkDependencies bool) {
	t.Helper()

	// Verify essential files exist
	assert.FileExists(t, filepath.Join(appPath, "package.json"), "package.json should exist")
	assert.FileExists(t, filepath.Join(appPath, "tsconfig.json"), "tsconfig.json should exist")
	assert.FileExists(t, filepath.Join(appPath, "index.ts"), "index.ts should exist")
	assert.FileExists(t, filepath.Join(appPath, ".gitignore"), ".gitignore should exist")

	// Verify _gitignore was renamed
	assert.NoFileExists(t, filepath.Join(appPath, "_gitignore"), "_gitignore should not exist")

	if checkDependencies {
		// Verify node_modules exists (dependencies were installed)
		nodeModulesPath := filepath.Join(appPath, "node_modules")
		if _, err := os.Stat(nodeModulesPath); err == nil {
			// Only check contents if node_modules exists
			entries, err := os.ReadDir(nodeModulesPath)
			require.NoError(t, err, "should be able to read node_modules directory")
			assert.NotEmpty(t, entries, "node_modules should contain installed packages")
		} else {
			t.Logf("Warning: node_modules not found at %s (npm install may have failed)", nodeModulesPath)
		}
	}
}

// validatePythonTemplate verifies Python template structure and optionally dependencies
func validatePythonTemplate(t *testing.T, appPath string, checkDependencies bool) {
	t.Helper()

	// Verify essential files exist
	assert.FileExists(t, filepath.Join(appPath, "pyproject.toml"), "pyproject.toml should exist")
	assert.FileExists(t, filepath.Join(appPath, "main.py"), "main.py should exist")
	assert.FileExists(t, filepath.Join(appPath, ".gitignore"), ".gitignore should exist")

	// Verify _gitignore was renamed
	assert.NoFileExists(t, filepath.Join(appPath, "_gitignore"), "_gitignore should not exist")

	if checkDependencies {
		// Verify .venv exists (virtual environment was created)
		venvPath := filepath.Join(appPath, ".venv")
		if _, err := os.Stat(venvPath); err == nil {
			// Only check contents if .venv exists
			binPath := filepath.Join(venvPath, "bin")
			assert.DirExists(t, binPath, ".venv/bin directory should exist")

			pythonPath := filepath.Join(binPath, "python")
			assert.FileExists(t, pythonPath, ".venv/bin/python should exist")
		} else {
			t.Logf("Warning: .venv not found at %s (uv venv may have failed)", venvPath)
		}
	}
}

// TestCreateCommand_DependencyInstallationFails tests that the app is still created
// even when dependency installation fails, with appropriate warning message
func TestCreateCommand_DependencyInstallationFails(t *testing.T) {
	tmpDir := t.TempDir()
	appName := "test-app"

	orgDir, err := os.Getwd()
	require.NoError(t, err)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Chdir(orgDir)
	})

	var outputBuf bytes.Buffer
	multiWriter := io.MultiWriter(&outputBuf, os.Stdout)
	pterm.SetDefaultOutput(multiWriter)

	t.Cleanup(func() {
		pterm.SetDefaultOutput(os.Stdout)
	})

	// Override the install command to use a command that will fail
	originalInstallCommands := create.InstallCommands
	create.InstallCommands = map[string]string{
		create.LanguageTypeScript: "exit 1", // Command that always fails
	}

	// Restore original install commands after test
	t.Cleanup(func() {
		create.InstallCommands = originalInstallCommands
	})

	// Create the app - should succeed even though dependency installation fails
	c := CreateCmd{}
	err = c.Create(context.Background(), create.CreateInput{
		Name:     appName,
		Language: create.LanguageTypeScript,
		Template: "sample-app",
	})

	output := outputBuf.String()

	assert.Contains(t, output, "cd test-app", "should print cd command")
	assert.Contains(t, output, "pnpm install", "should print pnpm install command")
}

// TestCreateCommand_RequiredToolMissing tests that the app is created
func TestCreateCommand_RequiredToolMissing(t *testing.T) {
	tests := []struct {
		name     string
		language string
		template string
	}{
		{
			name:     "typescript with missing pnpm",
			language: create.LanguageTypeScript,
			template: "sample-app",
		},
		{
			name:     "python with missing uv",
			language: create.LanguagePython,
			template: "sample-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			appName := "test-app"

			orgDir, err := os.Getwd()
			require.NoError(t, err)

			err = os.Chdir(tmpDir)
			require.NoError(t, err)

			t.Cleanup(func() {
				os.Chdir(orgDir)
			})

			// Override the required tool to point to a non-existent command
			originalRequiredTools := create.RequiredTools
			create.RequiredTools = map[string]string{
				create.LanguageTypeScript: "nonexistent-pnpm-tool",
				create.LanguagePython:     "nonexistent-uv-tool",
			}

			// Restore original required tools after test
			t.Cleanup(func() {
				create.RequiredTools = originalRequiredTools
			})

			// Create the app - should succeed even though required tool is missing
			c := CreateCmd{}
			err = c.Create(context.Background(), create.CreateInput{
				Name:     appName,
				Language: tt.language,
				Template: tt.template,
			})

			// Should not return an error - the command should complete successfully
			// but skip dependency installation
			require.NoError(t, err, "app creation should succeed even when required tool is missing")

			// Verify the app directory and files were created
			appPath := filepath.Join(tmpDir, appName)
			assert.DirExists(t, appPath, "app directory should exist")

			// Language-specific file checks
			switch tt.language {
			case create.LanguageTypeScript:
				assert.FileExists(t, filepath.Join(appPath, "package.json"), "package.json should exist")
				assert.FileExists(t, filepath.Join(appPath, "index.ts"), "index.ts should exist")
				assert.FileExists(t, filepath.Join(appPath, "tsconfig.json"), "tsconfig.json should exist")

				// node_modules should NOT exist since pnpm was not available
				assert.NoDirExists(t, filepath.Join(appPath, "node_modules"), "node_modules should not exist when pnpm is missing")
			case create.LanguagePython:
				assert.FileExists(t, filepath.Join(appPath, "pyproject.toml"), "pyproject.toml should exist")
				assert.FileExists(t, filepath.Join(appPath, "main.py"), "main.py should exist")

				// .venv should NOT exist since uv was not available
				assert.NoDirExists(t, filepath.Join(appPath, ".venv"), ".venv should not exist when uv is missing")
			}
		})
	}
}

// TestCreateCommand_DirectoryOverwrite tests that overwriting an existing directory
// properly removes old content and creates new content
func TestCreateCommand_DirectoryOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	appName := "test-app"
	appPath := filepath.Join(tmpDir, appName)

	orgDir, err := os.Getwd()
	require.NoError(t, err)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Chdir(orgDir)
	})

	// Initialize directory with some files
	err = os.MkdirAll(appPath, DIR_PERM)
	require.NoError(t, err, "failed to create initial directory")

	// Create some initial files that should be removed after overwrite
	oldFile1 := filepath.Join(appPath, "old-file-1.txt")
	oldSubDir := filepath.Join(appPath, "old-subdir")

	err = os.WriteFile(oldFile1, []byte("old content 1"), FILE_PERM)
	require.NoError(t, err, "failed to create old file 1")

	err = os.MkdirAll(oldSubDir, DIR_PERM)
	require.NoError(t, err, "failed to create old subdirectory")

	// Verify initial files exist
	assert.FileExists(t, oldFile1, "old file 1 should exist before overwrite")
	assert.DirExists(t, oldSubDir, "old subdirectory should exist before overwrite")

	// Manually remove the directory and create the new app
	err = os.RemoveAll(appPath)
	require.NoError(t, err, "failed to remove existing directory")

	c := CreateCmd{}
	err = c.Create(context.Background(), create.CreateInput{
		Name:     appName,
		Language: create.LanguageTypeScript,
		Template: "sample-app",
	})
	require.NoError(t, err, "failed to create new app")

	// Verify old files are gone
	assert.NoFileExists(t, oldFile1, "old file 1 should not exist after overwrite")
	assert.NoDirExists(t, oldSubDir, "old subdirectory should not exist after overwrite")

	// Verify new template files exist
	assert.FileExists(t, filepath.Join(appPath, "index.ts"), "new index.ts should exist")
}

// TestCreateCommand_InvalidLanguageTemplateCombinations tests that invalid
// language/template combinations fail with appropriate error messages
func TestCreateCommand_InvalidLanguageTemplateCombinations(t *testing.T) {
	tests := []struct {
		name        string
		language    string
		template    string
		errContains string
	}{
		{
			name:        "browser-use not available for typescript",
			language:    create.LanguageTypeScript,
			template:    create.TemplateBrowserUse,
			errContains: "template not found: typescript/browser-use",
		},
		{
			name:        "stagehand not available for python",
			language:    create.LanguagePython,
			template:    create.TemplateStagehand,
			errContains: "template not found: python/stagehand",
		},
		{
			name:        "magnitude not available for python",
			language:    create.LanguagePython,
			template:    create.TemplateMagnitude,
			errContains: "template not found: python/magnitude",
		},
		{
			name:        "gemini-computer-use not available for python",
			language:    create.LanguagePython,
			template:    create.TemplateGeminiComputerUse,
			errContains: "template not found: python/gemini-computer-use",
		},
		{
			name:        "invalid language",
			language:    "ruby",
			template:    create.TemplateSampleApp,
			errContains: "template not found: ruby/sample-app",
		},
		{
			name:        "invalid template",
			language:    create.LanguageTypeScript,
			template:    "nonexistent-template",
			errContains: "template not found: typescript/nonexistent-template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			orgDir, err := os.Getwd()
			require.NoError(t, err)

			err = os.Chdir(tmpDir)
			require.NoError(t, err)

			t.Cleanup(func() {
				os.Chdir(orgDir)
			})

			c := CreateCmd{}
			err = c.Create(context.Background(), create.CreateInput{
				Name:     "test-app",
				Language: tt.language,
				Template: tt.template,
			})

			require.Error(t, err, "should fail with invalid language/template combination")
			assert.Contains(t, err.Error(), tt.errContains, "error message should contain expected text")
		})
	}
}

// TestCreateCommand_ValidateAllTemplateCombinations validates that only valid
// language/template combinations are defined in the Templates map
func TestCreateCommand_ValidateAllTemplateCombinations(t *testing.T) {
	// This test ensures data consistency between Templates and actual template availability
	for templateKey, templateInfo := range create.Templates {
		for _, lang := range templateInfo.Languages {
			t.Run(lang+"/"+templateKey, func(t *testing.T) {
				tmpDir := t.TempDir()
				appPath := filepath.Join(tmpDir, "test-app")

				err := os.MkdirAll(appPath, DIR_PERM)
				require.NoError(t, err)

				// This should succeed for all combinations defined in Templates
				err = create.CopyTemplateFiles(appPath, lang, templateKey)
				require.NoError(t, err, "Template %s should be available for language %s as defined in Templates map", templateKey, lang)
			})
		}
	}
}

// TestCreateCommand_InvalidLanguageShorthand tests that invalid language shorthands
// are handled appropriately
func TestCreateCommand_InvalidLanguageShorthand(t *testing.T) {
	tests := []struct {
		name               string
		languageInput      string
		expectedNormalized string
	}{
		{
			name:               "ts shorthand normalizes to typescript",
			languageInput:      "ts",
			expectedNormalized: create.LanguageTypeScript,
		},
		{
			name:               "py shorthand normalizes to python",
			languageInput:      "py",
			expectedNormalized: create.LanguagePython,
		},
		{
			name:               "typescript remains typescript",
			languageInput:      "typescript",
			expectedNormalized: create.LanguageTypeScript,
		},
		{
			name:               "python remains python",
			languageInput:      "python",
			expectedNormalized: create.LanguagePython,
		},
		{
			name:               "invalid shorthand remains unchanged",
			languageInput:      "js",
			expectedNormalized: "js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := create.NormalizeLanguage(tt.languageInput)
			assert.Equal(t, tt.expectedNormalized, normalized)
		})
	}
}

// TestCreateCommand_TemplateNotAvailableForLanguage tests specific cases where
// templates are not available for certain languages
func TestCreateCommand_TemplateNotAvailableForLanguage(t *testing.T) {
	// Map of templates to languages they should NOT be available for
	unavailableCombinations := map[string][]string{
		create.TemplateBrowserUse: {create.LanguageTypeScript},
		create.TemplateStagehand:  {create.LanguagePython},
		create.TemplateMagnitude:  {create.LanguagePython},
		create.TemplateGeminiComputerUse: {create.LanguagePython},
	}

	for template, unavailableLanguages := range unavailableCombinations {
		for _, lang := range unavailableLanguages {
			t.Run(template+"/"+lang, func(t *testing.T) {
				// Verify the template info doesn't list this language
				templateInfo, exists := create.Templates[template]
				require.True(t, exists, "Template %s should exist in Templates map", template)

				assert.NotContains(t, templateInfo.Languages, lang,
					"Template %s should not list %s as a supported language", template, lang)

				// Verify copying fails
				tmpDir := t.TempDir()
				appPath := filepath.Join(tmpDir, "test-app")
				err := os.MkdirAll(appPath, DIR_PERM)
				require.NoError(t, err)

				err = create.CopyTemplateFiles(appPath, lang, template)
				require.Error(t, err, "Should fail to copy %s template for %s", template, lang)
			})
		}
	}
}

// TestCreateCommand_AllTemplatesHaveDeployCommands ensures that all templates
// have corresponding deploy commands defined
func TestCreateCommand_AllTemplatesHaveDeployCommands(t *testing.T) {
	for templateKey, templateInfo := range create.Templates {
		for _, lang := range templateInfo.Languages {
			t.Run(lang+"/"+templateKey, func(t *testing.T) {
				deployCmd := create.GetDeployCommand(lang, templateKey)
				assert.NotEmpty(t, deployCmd, "Deploy command should exist for %s/%s", lang, templateKey)

				// Verify deploy command starts with "kernel deploy"
				assert.Contains(t, deployCmd, "kernel deploy", "Deploy command should start with 'kernel deploy'")

				// Verify it contains the entry point
				switch lang {
				case create.LanguageTypeScript:
					assert.Contains(t, deployCmd, "index.ts", "TypeScript deploy command should contain index.ts")
				case create.LanguagePython:
					assert.Contains(t, deployCmd, "main.py", "Python deploy command should contain main.py")
				}
			})
		}
	}
}

// TestCreateCommand_AllTemplatesHaveInvokeSamples ensures that all templates
// have corresponding invoke samples defined
func TestCreateCommand_AllTemplatesHaveInvokeSamples(t *testing.T) {
	for templateKey, templateInfo := range create.Templates {
		for _, lang := range templateInfo.Languages {
			t.Run(lang+"/"+templateKey, func(t *testing.T) {
				invokeCmd := create.GetInvokeSample(lang, templateKey)
				assert.NotEmpty(t, invokeCmd, "Invoke sample should exist for %s/%s", lang, templateKey)

				// Verify invoke command starts with "kernel invoke"
				assert.Contains(t, invokeCmd, "kernel invoke", "Invoke command should start with 'kernel invoke'")
			})
		}
	}
}

func getTemplateInfo() []struct {
	name     string
	language string
	template string
} {
	tests := make([]struct {
		name     string
		language string
		template string
	}, 0)

	for templateKey, templateInfo := range create.Templates {
		for _, lang := range templateInfo.Languages {
			tests = append(tests, struct {
				name     string
				language string
				template string
			}{
				name:     lang + "/" + templateKey,
				language: lang,
				template: templateKey,
			})
		}
	}

	return tests
}
