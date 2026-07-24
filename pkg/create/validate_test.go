package create

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNonInteractiveAggregatesAllProblems(t *testing.T) {
	_, err := ValidateNonInteractive("", "", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name is required")
	assert.Contains(t, err.Error(), "--language is required")
	assert.Contains(t, err.Error(), "--template is required")
	assert.Contains(t, err.Error(), "not an interactive terminal")
}

func TestValidateNonInteractiveInvalidValues(t *testing.T) {
	_, err := ValidateNonInteractive("bad name!", "ruby", "nope", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name 'bad name!' is invalid")
	assert.Contains(t, err.Error(), "--language 'ruby' is invalid")
	// Language is unknown, but the template exists for no language at all,
	// so it must still be reported in the same error.
	assert.Contains(t, err.Error(), "--template 'nope' is invalid")
}

func TestValidateNonInteractiveTemplateValidatedPerLanguage(t *testing.T) {
	_, err := ValidateNonInteractive("my-app", "ts", "nope", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--template 'nope' is invalid for language 'typescript'")
	assert.Contains(t, err.Error(), TemplateSampleApp)
}

func TestValidateNonInteractiveExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my-app"), 0o755))

	_, err := ValidateNonInteractive("my-app", "ts", TemplateSampleApp, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory 'my-app' already exists")
	assert.Contains(t, err.Error(), "--yes")

	// --yes skips the overwrite confirmation, so it is not a problem.
	in, err := ValidateNonInteractive("my-app", "ts", TemplateSampleApp, true)
	require.NoError(t, err)
	assert.True(t, in.SkipConfirm)
}

func TestValidateNonInteractiveValidInput(t *testing.T) {
	in, err := ValidateNonInteractive("my-app", "py", TemplateSampleApp, false)
	require.NoError(t, err)
	assert.Equal(t, "my-app", in.Name)
	assert.Equal(t, LanguagePython, in.Language)
	assert.Equal(t, TemplateSampleApp, in.Template)
	assert.False(t, in.SkipConfirm)
}
