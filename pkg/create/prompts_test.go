package create

import (
	"testing"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every prompt wrapper must fail fast with a flag-usage hint when its
// Prompter cannot prompt. The terminal capability is carried by the injected
// Prompter, so the tests do not depend on the harness's ambient stdin and
// mutate no package state.
func TestPromptsFailFastWhenNonInteractive(t *testing.T) {
	t.Parallel()
	p := interactive.NewPrompterWithTerminal(false)

	_, err := PromptName(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name")
	assert.Contains(t, err.Error(), "not an interactive terminal")

	_, err = PromptLanguage(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--language")
	assert.Contains(t, err.Error(), LanguageTypeScript)

	_, err = PromptTemplate(p, LanguageTypeScript)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--template")
	assert.Contains(t, err.Error(), TemplateSampleApp)

	ok, err := PromptOverwrite(p, "existing-dir")
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "overwrite existing directory 'existing-dir'")
	assert.Contains(t, err.Error(), "--yes")
}
