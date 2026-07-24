// Package interactive reports whether the CLI can show interactive prompts
// and builds the fail-fast errors used when it cannot.
//
// Interactive prompts (pterm confirm/select/text input) read keystrokes from
// the terminal. In a non-interactive shell — an AI agent's bash tool, CI, or
// any piped stdin — they never return (the underlying keyboard listener spins
// forever), so every prompt call site must gate on IsInteractive first and
// fail fast with instructions for avoiding the prompt.
package interactive

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// IsInteractive reports whether stdin is attached to a terminal, i.e. whether
// interactive prompts can be shown.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ErrConfirmationRequired builds the fail-fast error for confirmation
// prompts. action describes what would have been confirmed, e.g.
// "delete profile 'foo'". The resulting error tells the caller (often an AI
// agent) to re-run with --yes.
func ErrConfirmationRequired(action string) error {
	return fmt.Errorf("cannot prompt for confirmation to %s: stdin is not an interactive terminal; re-run with --yes to skip the confirmation prompt", action)
}

// ErrInputRequired builds the fail-fast error for text/select prompts. what
// describes the input that would have been prompted for, e.g. "app name";
// hint names the flag(s) to pass instead, e.g. "pass --name to set the app
// name".
func ErrInputRequired(what, hint string) error {
	return fmt.Errorf("cannot prompt for %s: stdin is not an interactive terminal; %s", what, hint)
}
