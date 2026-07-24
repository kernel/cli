package interactive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Under `go test` stdin is not attached to a terminal, so IsInteractive must
// report false. This is the same environment as CI pipelines and AI-agent
// shells — exactly the case the fail-fast gating exists for.
func TestIsInteractiveFalseWithoutTerminal(t *testing.T) {
	assert.False(t, IsInteractive())
}

func TestErrConfirmationRequired(t *testing.T) {
	err := ErrConfirmationRequired("delete profile 'foo'")
	assert.Contains(t, err.Error(), "delete profile 'foo'")
	assert.Contains(t, err.Error(), "--yes")
	assert.Contains(t, err.Error(), "not an interactive terminal")
}

func TestErrInputRequired(t *testing.T) {
	err := ErrInputRequired("app name", "pass --name to set the app name")
	assert.Contains(t, err.Error(), "app name")
	assert.Contains(t, err.Error(), "pass --name")
	assert.Contains(t, err.Error(), "not an interactive terminal")
}
