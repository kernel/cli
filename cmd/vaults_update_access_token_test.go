package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const customerRefreshedLinkWalletFixture = `{"id":"wallet-id","key":"wallet-1","type":"wallet","spec":{"provider":"link","authorization":{"method":"oauth","client":{"type":"customer_managed","provider_config":{"id":"config-1"}},"token_refresh":"customer"}},"state":{"provider":"link","status":"connected"},"available_operations":[{"type":"update_access_token","description":"Replace the stored access token"}],"available_expansions":[]}`

func TestVaultUpdateAccessTokenOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	accessToken := vaultTestSecret(t)
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			assert.Equal(t, http.MethodGet, r.Method)
			_, _ = io.WriteString(w, customerRefreshedLinkWalletFixture)
		case 2:
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/vaults/checkout/items/wallet-1/operations", r.URL.Path)
			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.JSONEq(t, `"update_access_token"`, string(body["type"]))
			assert.JSONEq(t, fmt.Sprintf(`%q`, accessToken), string(body["access_token"]))
			assert.Len(t, body, 2)
			_, _ = io.WriteString(w, customerRefreshedLinkWalletFixture)
		default:
			t.Fatalf("unexpected API call %d", calls)
		}
	})
	out, human, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"access_token":%q}`, accessToken),
		"vaults", "items", "invoke", "checkout", "wallet-1", "update_access_token", "--spec-file", "-", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.NotContains(t, out+human, accessToken)
	assert.Contains(t, out, `"token_refresh": "customer"`)
}

func TestVaultUpdateAccessTokenRequiresProtectedFile(t *testing.T) {
	secret := vaultTestSecret(t)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid update_access_token reached API") })
	for _, tc := range []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{name: "inline", args: []string{"--params", fmt.Sprintf(`{"access_token":%q}`, secret)}, want: "only through --spec-file"},
		{name: "missing", want: "requires --spec-file"},
		{name: "empty", args: []string{"--spec-file", "-"}, stdin: `{"access_token":""}`, want: "access_token"},
		{name: "unknown", args: []string{"--spec-file", "-"}, stdin: fmt.Sprintf(`{"access_token":%q,"refresh_token":%q}`, secret, secret), want: "only supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"vaults", "items", "invoke", "checkout", "wallet-1", "update_access_token"}, tc.args...)
			out, human, err := executeVaultInputCommand(t, client, tc.stdin, args...)
			require.ErrorContains(t, err, tc.want)
			assert.False(t, strings.Contains(out+human+err.Error(), secret))
		})
	}
}

func TestVaultUpdateAccessTokenErrorDoesNotEchoToken(t *testing.T) {
	secret := vaultTestSecret(t)
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = io.WriteString(w, customerRefreshedLinkWalletFixture)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"message":%q}`, secret)
	})
	out, human, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"access_token":%q}`, secret),
		"vaults", "items", "invoke", "checkout", "wallet-1", "update_access_token", "--spec-file", "-")
	require.Error(t, err)
	assert.Equal(t, 2, calls)
	assert.False(t, strings.Contains(out+human+util.CleanedUpSdkError{Err: err}.Error(), secret))
	assert.Contains(t, err.Error(), "vault request rejected")
}
