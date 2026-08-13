package interactive

import (
	"os"
	"testing"

	"atomicgo.dev/keyboard/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default detector must report false for a file descriptor that is
// explicitly not a terminal (a pipe), independent of the test harness's
// ambient stdin.
func TestIsTerminalReportsFalseForPipe(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	assert.False(t, isTerminal(r))
	assert.False(t, isTerminal(w))
}

// Each Prompter owns its terminal capability; constructing one never touches
// package state, so opposite capabilities coexist across parallel tests.
func TestPrompterCarriesItsOwnTerminalCapability(t *testing.T) {
	t.Parallel()
	assert.True(t, NewPrompterWithTerminal(true).CanPrompt())
	assert.False(t, NewPrompterWithTerminal(false).CanPrompt())
}

func TestPromptErrorSingleProblem(t *testing.T) {
	t.Parallel()
	err := ErrConfirmationRequired("delete profile 'foo'")

	var promptErr *PromptError
	require.ErrorAs(t, err, &promptErr)
	assert.Contains(t, err.Error(), "delete profile 'foo'")
	assert.Contains(t, err.Error(), "--yes")
	assert.Contains(t, err.Error(), "not an interactive terminal")
	// Single-problem errors render identically in logs and on screen.
	assert.Equal(t, err.Error(), promptErr.Display())
	assert.NotContains(t, err.Error(), "\n")
}

func TestPromptErrorMultiProblemRendering(t *testing.T) {
	t.Parallel()
	err := ErrInputsRequired([]string{
		"--name is required",
		"--language is required: one of: typescript (ts), python (py)",
		"--template is required",
	})

	var promptErr *PromptError
	require.ErrorAs(t, err, &promptErr)

	// Error() follows Go convention: single line, numbered problems.
	msg := err.Error()
	assert.NotContains(t, msg, "\n")
	assert.Contains(t, msg, "(1) --name is required")
	assert.Contains(t, msg, "(2) --language is required")
	assert.Contains(t, msg, "(3) --template is required")

	// Display() puts each problem on its own line so flag tokens are never
	// split by terminal-width re-wrapping.
	display := promptErr.Display()
	assert.Contains(t, display, "\n  - --name is required")
	assert.Contains(t, display, "\n  - --language is required")
	assert.Contains(t, display, "\n  - --template is required")
}

func TestErrInputsRequiredEmptyIsNil(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ErrInputsRequired(nil))
}

func TestErrInputRequired(t *testing.T) {
	t.Parallel()
	err := ErrInputRequired("app name", "pass --name to set the app name")
	assert.Contains(t, err.Error(), "app name")
	assert.Contains(t, err.Error(), "pass --name")
	assert.Contains(t, err.Error(), "not an interactive terminal")
}

// The prompt primitives must fail fast (never touch pterm) when the Prompter
// cannot prompt.
func TestPromptPrimitivesFailFastWhenNonInteractive(t *testing.T) {
	t.Parallel()
	p := NewPrompterWithTerminal(false)

	ok, err := p.Confirm("delete widget 'w'", "Are you sure?")
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "delete widget 'w'")
	assert.Contains(t, err.Error(), "--yes")

	_, err = p.Select("widget selection", "pass --widget", "Pick a widget:", []string{"a", "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "widget selection")
	assert.Contains(t, err.Error(), "pass --widget")

	_, err = p.MultiSelect("websites", "pass --sites", "Choose websites", []string{"example.com"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "websites")
	assert.Contains(t, err.Error(), "pass --sites")

	_, err = p.TextInput("widget name", "pass --name", "Name?")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "widget name")
	assert.Contains(t, err.Error(), "pass --name")
}

func TestMultiSelectUsesConventionalKeys(t *testing.T) {
	t.Parallel()
	printer := newMultiSelectPrinter("Choose websites", []string{"example.com"}, nil)

	assert.False(t, printer.Filter)
	assert.Equal(t, keys.Space, printer.KeySelect)
	assert.Equal(t, keys.Enter, printer.KeyConfirm)
}

func TestSelectDefaultsToFirstOption(t *testing.T) {
	t.Parallel()
	options := make([]string, 2000)
	options[0] = "Done"
	printer := newSelectPrinter("Remove a website, or continue", options, options[0])

	assert.Equal(t, "Done", printer.DefaultOption)
	assert.Equal(t, 12, printer.MaxHeight)
}

func TestSelectCanHighlightARequestedOption(t *testing.T) {
	t.Parallel()
	options := []string{"first", "chosen", "skip"}
	printer := newSelectPrinter("Choose one", options, "chosen")

	assert.Equal(t, "chosen", printer.DefaultOption)
}

func TestConfirmPrinterUsesRequestedDefault(t *testing.T) {
	t.Parallel()
	assert.False(t, newConfirmPrinter("Delete it?", false).DefaultValue)
	assert.True(t, newConfirmPrinter("Unlock it?", true).DefaultValue)
}
