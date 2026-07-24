package create

import (
	"testing"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every prompt wrapper must fail fast with a flag-usage hint when the shell
// is non-interactive. The terminal capability is injected explicitly so the
// tests do not depend on the harness's ambient stdin.
func TestPromptsFailFastWhenNonInteractive(t *testing.T) {
	t.Cleanup(interactive.ForceTerminal(false))

	_, err := PromptName()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name")
	assert.Contains(t, err.Error(), "not an interactive terminal")

	_, err = PromptLanguage()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--language")
	assert.Contains(t, err.Error(), LanguageTypeScript)

	_, err = PromptTemplate(LanguageTypeScript)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--template")
	assert.Contains(t, err.Error(), TemplateSampleApp)

	ok, err := PromptOverwrite("existing-dir")
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "overwrite existing directory 'existing-dir'")
	assert.Contains(t, err.Error(), "--yes")
}
