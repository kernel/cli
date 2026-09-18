package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialEmptyStringUpdate(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"credential","version":2,"spec":{"fields":{"password":{"value":""}}}}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, credentialFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "update", "user", "login", "--version", "2", "--spec-file", credentialSpecFile(t, `{"fields":{"password":{"value":""}}}`), "-o", "json")
	require.NoError(t, err)
}

func TestCredentialHelpSteering(t *testing.T) {
	cmd, _, err := newVaultsCommand().Find([]string{"credentials", "create"})
	require.NoError(t, err)
	assert.Contains(t, cmd.Long, "Do not use credential items to store, collect, or fill credit card data")
	assert.Contains(t, strings.Join(strings.Fields(cmd.Long), " "), "Use wallet and card item types for credit cards and payment checkout instead")
	assert.Contains(t, newVaultsCommand().Long, "Use wallet and card item types")
	assert.Contains(t, cmd.Long, "recognizable site name only")
	assert.Contains(t, cmd.Long, "natural top-to-bottom order")
	assert.Contains(t, cmd.Long, "collection form renders that order unchanged")
	assert.Contains(t, cmd.Long, "sensitive:false explicitly for ordinary usernames and email addresses")
	assert.Contains(t, cmd.Example, `"description":"Hacker News"`)
	assert.Contains(t, cmd.Example, `"fields":[{"name":"username"`)
	assert.Contains(t, cmd.Example, `"sensitive":false`)
}
