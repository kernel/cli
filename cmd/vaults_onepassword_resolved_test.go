package cmd

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultOnePasswordResolvedReferencesAreDisplayed(t *testing.T) {
	fixture := `{
      "id":"credential-2","key":"github","type":"credential","version":1,
      "spec":{"provider":"1password","requests":{"version":2,"entries":[{"id":"entry-1","type":"login","parameters":{"website":"https://github.com"}}]}},
      "state":{"provider":"1password","status":"ready","access_request":{"id":"req-1","state":"approved","has_autofill_token":false,"granted_count":1,"resolved":[{"reference":"op://Private/GitHub/login","entryId":"entry-1","secret":"never-print"}]}},
      "available_operations":[{"type":"1pw_fill","description":"Fill the approved login"}],
      "available_expansions":[]
    }`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "github", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"id": "entry-1"`)
	assert.Contains(t, out, `"resolved"`)
	assert.Contains(t, out, `"reference": "op://Private/GitHub/login"`)
	assert.Contains(t, out, `"entryId": "entry-1"`)
	assert.NotContains(t, out, "never-print")

	_, text, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "github")
	require.NoError(t, err)
	assert.Contains(t, text, "resolved 1Password reference")
	assert.Contains(t, text, "op://Private/GitHub/login")
	assert.Contains(t, text, "entry entry-1")
	assert.NotContains(t, text, "never-print")
}
