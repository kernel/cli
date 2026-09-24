// Package interactive owns every interactive prompt in the CLI: it detects
// whether stdin is attached to a terminal, executes pterm confirm/select/text
// prompts when it is, and fails fast with actionable errors when it is not.
//
// Interactive prompts read keystrokes from the terminal. In a non-interactive
// shell — an AI agent's bash tool, CI, or any piped stdin — they never return
// (the underlying keyboard listener spins forever), so prompt execution is
// centralized here behind the TTY gate. Command code must prompt through a
// Prompter (or the package-level helpers backed by the default Prompter)
// instead of pterm's interactive printers; no direct pterm interactive calls
// should exist outside this package.
package interactive

import (
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// isTerminal reports whether the given file is attached to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// Prompter executes interactive prompts against a terminal capability. Each
// value owns its capability — there is no package-level mutable state — so
// tests can construct a Prompter with a fixed capability without affecting
// other goroutines or invocations.
//
// The zero value is equivalent to NewPrompter(): it detects the terminal
// from stdin at prompt time.
type Prompter struct {
	// isTerminal overrides terminal detection when non-nil.
	isTerminal func() bool
}

// NewPrompter returns a Prompter that detects terminal capability from
// stdin at prompt time.
func NewPrompter() Prompter {
	return Prompter{}
}

// NewPrompterWithTerminal returns a Prompter with a fixed terminal
// capability. Tests use NewPrompterWithTerminal(false) to exercise the
// fail-fast paths deterministically, regardless of the harness's stdin.
func NewPrompterWithTerminal(isTTY bool) Prompter {
	return Prompter{isTerminal: func() bool { return isTTY }}
}

// CanPrompt reports whether this Prompter can show interactive prompts.
func (p Prompter) CanPrompt() bool {
	if p.isTerminal != nil {
		return p.isTerminal()
	}
	return isTerminal(os.Stdin)
}

// IsInteractive reports whether stdin is attached to a terminal, i.e.
// whether interactive prompts can be shown. Equivalent to
// NewPrompter().CanPrompt().
func IsInteractive() bool {
	return NewPrompter().CanPrompt()
}

// PromptError is returned instead of showing an interactive prompt when
// stdin is not a terminal. It carries every problem the caller must fix so a
// single retry can succeed. The CLI's top-level error handler renders it via
// Display without width-based re-wrapping, so flag tokens such as --yes or
// --template are never split across lines.
type PromptError struct {
	// What names what would have been prompted for, e.g. "input" or
	// "confirmation to delete profile 'foo'".
	What string
	// Problems lists the fixes, e.g. "--name is required" or "re-run with
	// --yes to skip the confirmation prompt". Always at least one entry.
	Problems []string
}

// Error renders the problems inline on a single line, following the Go
// convention that error strings do not contain newlines.
func (e *PromptError) Error() string {
	header := fmt.Sprintf("cannot prompt for %s: stdin is not an interactive terminal", e.What)
	if len(e.Problems) == 1 {
		return header + "; " + e.Problems[0]
	}
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("; fix all of the following and re-run:")
	for i, p := range e.Problems {
		if i > 0 {
			b.WriteString(";")
		}
		fmt.Fprintf(&b, " (%d) %s", i+1, p)
	}
	return b.String()
}

// Display renders the problems for terminal output, one per line, so each
// fix instruction stays an intact, greppable token sequence regardless of
// terminal width.
func (e *PromptError) Display() string {
	if len(e.Problems) == 1 {
		return e.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "cannot prompt for %s: stdin is not an interactive terminal; fix all of the following and re-run:", e.What)
	for _, p := range e.Problems {
		b.WriteString("\n  - ")
		b.WriteString(p)
	}
	return b.String()
}

// ErrConfirmationRequired builds the fail-fast error for confirmation
// prompts. action describes what would have been confirmed, e.g.
// "delete profile 'foo'". The resulting error tells the caller (often an AI
// agent) to re-run with --yes.
func ErrConfirmationRequired(action string) error {
	return &PromptError{
		What:     "confirmation to " + action,
		Problems: []string{"re-run with --yes to skip the confirmation prompt"},
	}
}

// ErrInputRequired builds the fail-fast error for a single text/select
// prompt. what describes the input that would have been prompted for, e.g.
// "app name"; hint names the flag(s) to pass instead, e.g. "pass --name to
// set the app name".
func ErrInputRequired(what, hint string) error {
	return &PromptError{What: what, Problems: []string{hint}}
}

// ErrInputsRequired builds the fail-fast error for one or more missing or
// invalid inputs. Commands with several promptable inputs should validate
// them all up front and report every problem in a single error, so a
// non-interactive caller can fix everything in one retry instead of
// discovering problems one invocation at a time.
func ErrInputsRequired(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return &PromptError{What: "input", Problems: problems}
}

// Confirm shows a yes/no confirmation prompt with the given prompt text and
// reports the choice. When the Prompter cannot prompt it fails fast with
// ErrConfirmationRequired(action) instead.
func (p Prompter) Confirm(action, promptText string) (bool, error) {
	if !p.CanPrompt() {
		return false, ErrConfirmationRequired(action)
	}
	return pterm.DefaultInteractiveConfirm.
		WithDefaultText(promptText).
		WithDefaultValue(false).
		Show()
}

// Select shows a select prompt over options and returns the chosen option.
// When the Prompter cannot prompt it fails fast with
// ErrInputRequired(what, hint) instead.
func (p Prompter) Select(what, hint, promptText string, options []string) (string, error) {
	if !p.CanPrompt() {
		return "", ErrInputRequired(what, hint)
	}
	return pterm.DefaultInteractiveSelect.
		WithOptions(options).
		WithDefaultText(promptText).
		WithMaxHeight(len(options)).
		Show()
}

// MultiSelect shows a checkbox menu and returns the selected options.
func (p Prompter) MultiSelect(what, hint, promptText string, options, defaults []string) ([]string, error) {
	if !p.CanPrompt() {
		return nil, ErrInputRequired(what, hint)
	}
	return pterm.DefaultInteractiveMultiselect.
		WithOptions(options).
		WithDefaultOptions(defaults).
		WithDefaultText(promptText).
		WithFilter(true).
		WithMaxHeight(min(len(options), 12)).
		Show()
}

// TextInput shows a free-text prompt and returns the entered text. When the
// Prompter cannot prompt it fails fast with ErrInputRequired(what, hint)
// instead.
func (p Prompter) TextInput(what, hint, promptText string) (string, error) {
	if !p.CanPrompt() {
		return "", ErrInputRequired(what, hint)
	}
	return pterm.DefaultInteractiveTextInput.
		WithDefaultText(promptText).
		Show()
}

// Confirm is Prompter.Confirm on the default (ambient-stdin) Prompter, for
// command code without an injected Prompter.
func Confirm(action, promptText string) (bool, error) {
	return NewPrompter().Confirm(action, promptText)
}

// Select is Prompter.Select on the default (ambient-stdin) Prompter, for
// command code without an injected Prompter.
func Select(what, hint, promptText string, options []string) (string, error) {
	return NewPrompter().Select(what, hint, promptText, options)
}

// TextInput is Prompter.TextInput on the default (ambient-stdin) Prompter,
// for command code without an injected Prompter.
func TextInput(what, hint, promptText string) (string, error) {
	return NewPrompter().TextInput(what, hint, promptText)
}
