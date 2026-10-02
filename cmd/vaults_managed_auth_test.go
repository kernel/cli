package cmd

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const managedAuthCredentialFixture = `{"id":"credential-3","key":"amazon","type":"credential","version":1,"spec":{"provider":"managed_auth","connection_id":"ma_abc123xyz","description":"Amazon"},"state":{"provider":"managed_auth","status":"ready","fields":{"username":{"type":"email"},"password":{"type":"password"}}},"available_operations":[{"type":"fill","description":"Fill the login form."}],"available_expansions":[],"created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`

func TestCredentialSpecInputManagedAuth(t *testing.T) {
	spec, err := credentialSpecInput([]byte(`{"provider":"managed_auth","connection_id":"ma_abc123xyz","description":"Amazon"}`))
	require.NoError(t, err)
	require.NotNil(t, spec.OfManagedAuth)
	assert.Equal(t, "ma_abc123xyz", spec.OfManagedAuth.ConnectionID)
	assert.Equal(t, "Amazon", spec.OfManagedAuth.Description.Value)

	_, err = credentialSpecInput([]byte(`{"provider":"managed_auth"}`))
	assert.EqualError(t, err, "managed auth credential spec requires connection_id")
}

func TestCredentialCreateManagedAuth(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/vaults/user/items/amazon", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"credential","spec":{"provider":"managed_auth","connection_id":"ma_abc123xyz","description":"Amazon"}}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, managedAuthCredentialFixture)
	})
	spec := `{"provider":"managed_auth","connection_id":"ma_abc123xyz","description":"Amazon"}`
	out, _, err := executeVaultInputCommand(t, client, spec, "vaults", "credentials", "create", "user", "amazon", "--spec-file", "-", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"connection_id": "ma_abc123xyz"`)
	assert.Contains(t, out, `"type": "password"`)
	assert.NotContains(t, out, `"has_value"`)

	out, text, err := executeVaultInputCommand(t, client, spec, "vaults", "credentials", "create", "user", "amazon", "--spec-file", "-")
	require.NoError(t, err)
	assert.Contains(t, out+text, "Managed auth connection (immutable)")
	assert.Contains(t, out+text, "ma_abc123xyz")
}
