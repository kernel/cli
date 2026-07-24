package create

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests run under `go test`, where stdin is not a terminal, so every
// prompt path must fail fast with a flag-usage hint instead of prompting.

func TestPromptForAppNameNonInteractive(t *testing.T) {
	t.Run("missing name fails fast with --name hint", func(t *testing.T) {
		_, err := PromptForAppName("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--name")
		assert.Contains(t, err.Error(), "not an interactive terminal")
	})

	t.Run("invalid name fails fast with validation error", func(t *testing.T) {
		_, err := PromptForAppName("bad name!")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --name 'bad name!'")
	})

	t.Run("valid name passes through", func(t *testing.T) {
		name, err := PromptForAppName("my-app")
		require.NoError(t, err)
		assert.Equal(t, "my-app", name)
	})
}

func TestPromptForLanguageNonInteractive(t *testing.T) {
	t.Run("missing language fails fast with --language hint", func(t *testing.T) {
		_, err := PromptForLanguage("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--language")
		assert.Contains(t, err.Error(), LanguageTypeScript)
		assert.Contains(t, err.Error(), LanguagePython)
	})

	t.Run("invalid language fails fast listing options", func(t *testing.T) {
		_, err := PromptForLanguage("ruby")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --language 'ruby'")
		assert.Contains(t, err.Error(), LanguageTypeScript)
	})

	t.Run("valid language passes through", func(t *testing.T) {
		l, err := PromptForLanguage("ts")
		require.NoError(t, err)
		assert.Equal(t, LanguageTypeScript, l)
	})
}

func TestPromptForTemplateNonInteractive(t *testing.T) {
	t.Run("missing template fails fast with --template hint", func(t *testing.T) {
		_, err := PromptForTemplate("", LanguageTypeScript)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--template")
		assert.Contains(t, err.Error(), TemplateSampleApp)
	})

	t.Run("invalid template fails fast listing options", func(t *testing.T) {
		_, err := PromptForTemplate("nope", LanguageTypeScript)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --template 'nope'")
		assert.Contains(t, err.Error(), TemplateSampleApp)
	})

	t.Run("valid template passes through", func(t *testing.T) {
		tmpl, err := PromptForTemplate(TemplateSampleApp, LanguageTypeScript)
		require.NoError(t, err)
		assert.Equal(t, TemplateSampleApp, tmpl)
	})
}

func TestPromptForOverwriteNonInteractive(t *testing.T) {
	overwrite, err := PromptForOverwrite("existing-dir")
	require.Error(t, err)
	assert.False(t, overwrite)
	assert.Contains(t, err.Error(), "overwrite existing directory 'existing-dir'")
	assert.Contains(t, err.Error(), "--yes")
}
