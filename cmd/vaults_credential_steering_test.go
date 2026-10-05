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
	assert.Contains(t, cmd.Long, "optional non-secret human-readable label")
	assert.Contains(t, cmd.Long, "browser fills always use name")
	assert.Contains(t, cmd.Long, "sensitive:false explicitly for ordinary usernames and email addresses")
	assert.Contains(t, cmd.Example, `"description":"Hacker News"`)
	assert.Contains(t, cmd.Example, `"fields":[{"name":"username"`)
	assert.Contains(t, cmd.Example, `"label":"Username"`)
	assert.Contains(t, cmd.Example, `"sensitive":false`)
}

func TestCredentialHelpReadyOperations(t *testing.T) {
	create, _, err := newVaultsCommand().Find([]string{"credentials", "create"})
	require.NoError(t, err)
	group, _, err := newVaultsCommand().Find([]string{"credentials"})
	require.NoError(t, err)
	items, _, err := newVaultsCommand().Find([]string{"items"})
	require.NoError(t, err)
	flat := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	for name, long := range map[string]string{"vaults": newVaultsCommand().Long, "credentials": group.Long, "credentials create": create.Long} {
		t.Run(name, func(t *testing.T) {
			text := flat(long)
			paths := flat(vaultCredentialPathsHelp)
			assert.Contains(t, text, paths)
			assert.Contains(t, paths, "Once ready, items invoke fill writes them into ordinary web form fields in a vault-bound browser without submitting")
			assert.Contains(t, paths, "When the item advertises webmcp_invoke, items webmcp invoke instead binds credential fields to existing null inputs of a live WebMCP tool; the tool may submit or have other side effects")
			assert.Contains(t, paths, "1pw_fill fills and submits through the 1Password extension")
			assert.Contains(t, text, "Never automatically retry")
		})
	}
	text := flat(create.Long)
	assert.Contains(t, text, "Ordinary web forms: items invoke fill writes field values without submitting")
	assert.Contains(t, text, "Live WebMCP tool: when webmcp_invoke is advertised, run browsers webmcp list, then items webmcp invoke")
	assert.Contains(t, text, "Never automatically retry fill or webmcp_invoke")
	assert.Contains(t, text, "approves each access request in the 1Password app")

	vaults := flat(newVaultsCommand().Long)
	assert.Contains(t, vaults, "for ordinary web forms use items invoke <vault> <key> fill --spec-file")
	assert.Contains(t, vaults, "When the item advertises webmcp_invoke, items webmcp invoke instead binds credential fields to existing null inputs of a live WebMCP tool (the tool may submit or have side effects)")
	assert.Contains(t, vaults, "items webmcp invoke --help")
	assert.Contains(t, items.Long, "1Password credentials use the advertised 1pw_* operations instead of collect/fill/webmcp_invoke")

	// The 1Password path keeps its own operations and does not advertise webmcp_invoke.
	onePassword := flat(vaultOnePasswordCredentialHelp)
	assert.NotContains(t, onePassword, "webmcp")
	assert.Contains(t, onePassword, "Never automatically retry an access request, fill, or recovery")
}
