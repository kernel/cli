package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultImportedLinkCustomerTokenRefresh(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	accessToken := vaultTestSecret(t)
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Spec struct {
				Authorization struct {
					Method       string            `json:"method"`
					Client       json.RawMessage   `json:"client"`
					Tokens       map[string]string `json:"tokens"`
					TokenRefresh string            `json:"token_refresh"`
				} `json:"authorization"`
			} `json:"spec"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		auth := body.Spec.Authorization
		assert.Equal(t, "oauth", auth.Method)
		assert.JSONEq(t, `{"type":"customer_managed","provider_config":{"id":"config-1"}}`, string(auth.Client))
		assert.Equal(t, "customer", auth.TokenRefresh)
		assert.Equal(t, map[string]string{"access_token": accessToken}, auth.Tokens)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"wallet-id","key":"wallet-1","type":"wallet","spec":{"provider":"link","authorization":{"method":"oauth","client":{"type":"customer_managed","provider_config":{"id":"config-1"}},"token_refresh":"customer"}},"state":{"provider":"link","status":"connected"},"available_operations":[{"type":"update_access_token","description":"Replace the stored access token"}],"available_expansions":[]}`)
	})
	out, human, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"access_token":%q}`, accessToken),
		"vaults", "wallets", "create", "checkout", "wallet-1",
		"--provider", "link", "--spec", "{}", "--provider-config-id", "config-1",
		"--token-refresh", "customer", "--tokens-file", "-", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Empty(t, human)
	assert.NotContains(t, out, accessToken)
	assert.Contains(t, out, `"token_refresh": "customer"`)
}

func TestVaultImportedLinkCustomerTokenRefreshFromSpec(t *testing.T) {
	secret := vaultTestSecret(t)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		var spec struct {
			Authorization struct {
				TokenRefresh string            `json:"token_refresh"`
				Tokens       map[string]string `json:"tokens"`
			} `json:"authorization"`
		}
		require.NoError(t, json.Unmarshal(body["spec"], &spec))
		assert.Equal(t, "customer", spec.Authorization.TokenRefresh)
		assert.Equal(t, map[string]string{"access_token": secret}, spec.Authorization.Tokens)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"wallet-id","key":"wallet-1","type":"wallet","spec":{"provider":"link","authorization":{"method":"oauth","client":{"type":"customer_managed","provider_config":{"id":"config-1"}},"token_refresh":"customer"}},"state":{"provider":"link","status":"connected"},"available_operations":[],"available_expansions":[]}`)
	})
	spec := `{"authorization":{"method":"oauth","client":{"type":"customer_managed","provider_config":{"id":"config-1"}},"token_refresh":"customer"}}`
	_, _, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"access_token":%q}`, secret),
		"vaults", "wallets", "create", "checkout", "wallet-1", "--provider", "link", "--spec", spec, "--tokens-file", "-")
	require.NoError(t, err)
}

func TestVaultImportedLinkTokenRefreshValidation(t *testing.T) {
	secret := vaultTestSecret(t)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid token refresh input reached API") })
	tests := []struct {
		name, spec, input, want string
		flags                   []string
	}{
		{name: "customer rejects refresh token", spec: `{}`, input: fmt.Sprintf(`{"access_token":%q,"refresh_token":%q}`, secret, secret), flags: []string{"--provider-config-id", "config-1", "--token-refresh", "customer"}, want: "only the documented"},
		{name: "kernel requires refresh token", spec: `{}`, input: fmt.Sprintf(`{"access_token":%q}`, secret), flags: []string{"--provider-config-id", "config-1", "--token-refresh", "kernel"}, want: "only the documented"},
		{name: "invalid value", spec: `{}`, input: `{}`, flags: []string{"--provider-config-id", "config-1", "--token-refresh", "other"}, want: "kernel or customer"},
		{name: "empty value", spec: `{}`, input: `{}`, flags: []string{"--provider-config-id", "config-1", "--token-refresh", ""}, want: "kernel or customer"},
		{name: "kernel managed", spec: `{"authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`, input: `{}`, flags: []string{"--token-refresh", "customer"}, want: "imported Link grants"},
		{name: "mismatch", spec: `{"authorization":{"method":"oauth","client":{"type":"customer_managed","provider_config":{"id":"config-1"}},"token_refresh":"kernel"}}`, input: `{}`, flags: []string{"--token-refresh", "customer"}, want: "must match"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"vaults", "wallets", "create", "checkout", "wallet-1", "--provider", "link", "--spec", tc.spec, "--tokens-file", "-"}
			if tc.name == "kernel managed" {
				args = []string{"vaults", "wallets", "create", "checkout", "wallet-1", "--provider", "link", "--spec", tc.spec}
			}
			args = append(args, tc.flags...)
			out, human, err := executeVaultInputCommand(t, client, tc.input, args...)
			require.ErrorContains(t, err, tc.want)
			assert.False(t, strings.Contains(out+human+err.Error(), secret))
		})
	}
}

func TestVaultTokenRefreshFlagRejectsOtherProviders(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid token refresh input reached API") })
	_, _, err := executeVaultInputCommand(t, client, "",
		"vaults", "wallets", "create", "checkout", "wallet-1", "--provider", "kernel", "--spec", "{}", "--token-refresh", "customer")
	require.ErrorContains(t, err, "only for imported Link")
}
