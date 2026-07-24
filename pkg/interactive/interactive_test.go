package interactive

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default detector must report false for a file descriptor that is
// explicitly not a terminal (a pipe), independent of the test harness's
// ambient stdin.
func TestTerminalCheckReportsFalseForPipe(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	assert.False(t, terminalCheck(r))
	assert.False(t, terminalCheck(w))
}

func TestForceTerminalOverridesDetection(t *testing.T) {
	restore := ForceTerminal(true)
	assert.True(t, IsInteractive())
	restore()

	t.Cleanup(ForceTerminal(false))
	assert.False(t, IsInteractive())
}

func TestPromptErrorSingleProblem(t *testing.T) {
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
	assert.NoError(t, ErrInputsRequired(nil))
}

func TestErrInputRequired(t *testing.T) {
	err := ErrInputRequired("app name", "pass --name to set the app name")
	assert.Contains(t, err.Error(), "app name")
	assert.Contains(t, err.Error(), "pass --name")
	assert.Contains(t, err.Error(), "not an interactive terminal")
}

// The prompt primitives must fail fast (never touch pterm) when the shell is
// non-interactive.
func TestPromptPrimitivesFailFastWhenNonInteractive(t *testing.T) {
	t.Cleanup(ForceTerminal(false))

	ok, err := Confirm("delete widget 'w'", "Are you sure?")
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "delete widget 'w'")
	assert.Contains(t, err.Error(), "--yes")

	_, err = Select("widget selection", "pass --widget", "Pick a widget:", []string{"a", "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "widget selection")
	assert.Contains(t, err.Error(), "pass --widget")

	_, err = TextInput("widget name", "pass --name", "Name?")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "widget name")
	assert.Contains(t, err.Error(), "pass --name")
}
