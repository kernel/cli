package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateJSONOutput(t *testing.T) {
	require.NoError(t, validateJSONOutput(""))
	require.NoError(t, validateJSONOutput("json"))

	err := validateJSONOutput("yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
	assert.Contains(t, err.Error(), `"yaml"`)
	assert.Contains(t, err.Error(), "omit --output")
}
