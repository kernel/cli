package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const onePasswordAccountFixture = `{"id":"account-1","key":"onepassword","type":"credential_account","spec":{"provider":"1password","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}},"state":{"provider":"1password","status":"pending_authorization"},"action":{"name":"1password_oauth","url":"https://my.1password.example/oauth/authorize?client_id=kernel&state=opaque"},"available_operations":[],"available_expansions":[],"created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`

const onePasswordCredentialFixture = `{"id":"credential-2","key":"github","type":"credential","version":1,"spec":{"provider":"1password","account_id":"account-1","requests":{"version":2,"entries":[{"type":"login","parameters":{"website":"https://github.com"}}]}},"state":{"provider":"1password","status":"pending_authorization","access_request_id":"req-1","access_request":{"id":"req-1","state":"pending","identity":"never-print-identity","path":"never-print-path","has_autofill_token":false,"granted_count":0}},"action":{"name":"1password_access_approval","url":"onepassword://grant-brokered-access?access_request_reference=ref-1","instructions":"Present this link to the account owner."},"available_operations":[{"type":"1pw_poll_access","description":"Check the request."}],"available_expansions":[],"created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`

func onePasswordCredentialWithOperation(operation string) string {
	return strings.Replace(onePasswordCredentialFixture, `"1pw_poll_access"`, `"`+operation+`"`, 1)
}

func TestCredentialConnectOnePassword(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/vaults/user/items/onepassword", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"credential_account","spec":{"provider":"1password","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, onePasswordAccountFixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "credentials", "connect", "user", "onepassword", "--provider", "1password", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Contains(t, out, `"name": "1password_oauth"`)
	assert.Contains(t, out, "https://my.1password.example/oauth/authorize")

	_, text, err := executeVaultCommand(t, client, "vaults", "credentials", "connect", "user", "onepassword", "--provider", "1password")
	require.NoError(t, err)
	assert.Contains(t, text, "Share the 1Password authorization URL with the account owner")

	_, _, err = executeVaultCommand(t, client, "vaults", "credentials", "connect", "user", "onepassword", "--provider", "kernel")
	require.ErrorContains(t, err, "--provider must be 1password")
	assert.Equal(t, 2, calls)
}

func TestCredentialCreateOnePassword(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	spec := `{"provider":"1password","account_id":"account-1","requests":{"version":2,"entries":[{"type":"login","parameters":{"website":"https://github.com"},"reason":"Sign in"}]}}`
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"credential","spec":`+spec+`}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, onePasswordCredentialFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "user", "github", "--spec-file", credentialSpecFile(t, spec), "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	for _, invalid := range []string{
		`{"provider":"1password","requests":{"version":2,"entries":[{"type":"login","parameters":{"website":"https://github.com"}}]}}`,
		`{"provider":"1password","account_id":"account-1"}`,
		`{"provider":"lastpass","fields":[{"name":"password","type":"password"}]}`,
	} {
		_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "user", "github", "--spec-file", credentialSpecFile(t, invalid))
		require.Error(t, err, invalid)
	}
	assert.Equal(t, 1, calls)
}

func TestOnePasswordCredentialOutput(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	fixture := onePasswordCredentialFixture
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, fixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "github", "-o", "json")
	require.NoError(t, err)
	for _, want := range []string{`"account_id": "account-1"`, `"website": "https://github.com"`, `"access_request_id": "req-1"`, `"state": "pending"`, "onepassword://grant-brokered-access?access_request_reference=ref-1", `"instructions": "Present this link to the account owner."`} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "never-print")

	out, text, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "github")
	require.NoError(t, err)
	output := out + text
	assert.Contains(t, output, "onepassword://grant-brokered-access?access_request_reference=ref-1")
	assert.Contains(t, output, "Present this link to the account owner.")
	assert.Contains(t, output, "account-1")
	assert.Contains(t, output, "Invoke: kernel vaults items invoke --params '<json>' -- user github 1pw_poll_access")
	assert.NotContains(t, output, "field definitions")
	assert.NotContains(t, output, "never-print")

	for _, forged := range []string{
		"onepassword://grant-brokered-access?access_request_reference=ref-1&code=secret",
		"onepassword://other?access_request_reference=ref-1",
	} {
		fixture = strings.Replace(onePasswordCredentialFixture, "onepassword://grant-brokered-access?access_request_reference=ref-1", forged, 1)
		out, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "github", "-o", "json")
		require.NoError(t, err)
		assert.NotContains(t, out, "onepassword://", forged)
	}
	fixture = strings.Replace(onePasswordCredentialFixture, `"name":"1password_access_approval"`, `"name":"collect"`, 1)
	out, _, err = executeVaultCommand(t, client, "vaults", "items", "get", "user", "github", "-o", "json")
	require.NoError(t, err)
	assert.NotContains(t, out, "onepassword://")
}

func TestOnePasswordOperationRequests(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		operation, params, body, item string
	}{
		{"1pw_request_access", `{"browser_id":"browser-1","goal":"Manage billing","reason":"Sign in","keywords":["personal"]}`, `{"type":"1pw_request_access","browser_id":"browser-1","goal":"Manage billing","reason":"Sign in","keywords":["personal"]}`, onePasswordCredentialFixture},
		{"1pw_poll_access", `{"browser_id":"browser-1","timeout_seconds":60}`, `{"type":"1pw_poll_access","browser_id":"browser-1","timeout_seconds":60}`, onePasswordCredentialFixture},
		{"1pw_reconcile_access", `{"acknowledge_unconfirmed":true}`, `{"type":"1pw_reconcile_access","acknowledge_unconfirmed":true}`, onePasswordCredentialFixture},
		{"1pw_recover", "", `{"type":"1pw_recover"}`, onePasswordAccountFixture},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			item := strings.Replace(tc.item, `"available_operations":[]`, `"available_operations":[{"type":"1pw_poll_access","description":"x"}]`, 1)
			item = strings.Replace(item, `"1pw_poll_access"`, `"`+tc.operation+`"`, 1)
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posts++
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, tc.body, string(body))
				}
				io.WriteString(w, item)
			})
			args := []string{"vaults", "items", "invoke", "user", "github", tc.operation, "-o", "json"}
			if tc.params != "" {
				args = append(args, "--params", tc.params)
			}
			_, _, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 1, posts)
		})
	}
}

func TestOnePasswordOperationValidation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct{ operation, params, err string }{
		{"1pw_request_access", "", "requires --params"},
		{"1pw_request_access", `{"browser_id":""}`, "browser_id"},
		{"1pw_request_access", `{"browser_id":"b","password":"x"}`, "only supported"},
		{"1pw_request_access", `{"type":"1pw_request_access","browser_id":"b"}`, "must not contain type"},
		{"1pw_poll_access", `{"browser_id":"b","timeout_seconds":121}`, "timeout_seconds"},
		{"1pw_reconcile_access", `{"acknowledge_unconfirmed":false}`, "acknowledge_unconfirmed must be true"},
		{"1pw_fill", `{"browser_id":"b"}`, "page_url"},
		{"1pw_fill", `{"browser_id":"b","page_url":"https://github.com/login","timeout_ms":0}`, "timeout_ms"},
		{"1pw_recover", `{}`, "takes no parameters"},
	} {
		t.Run(tc.operation+tc.params, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
			args := []string{"vaults", "items", "invoke", "user", "github", tc.operation}
			if tc.params != "" {
				args = append(args, "--params", tc.params)
			}
			_, _, err := executeVaultCommand(t, client, args...)
			require.ErrorContains(t, err, tc.err)
			assert.Zero(t, calls)
		})
	}

	posts := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, onePasswordCredentialFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user", "github", "1pw_fill", "--params", `{"browser_id":"b","page_url":"https://github.com/login"}`)
	require.ErrorContains(t, err, "not advertised")
	assert.Zero(t, posts)
}

func TestOnePasswordFillOutcomes(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		status int
		body   string
		err    string
		output string
	}{
		{200, `{"type":"1pw_fill","status":"fill_submitted"}`, "", "does not confirm"},
		{200, `{"type":"1pw_fill","status":"fill_failed","error_code":"fillFailed"}`, "fill fill_failed", "fillFailed"},
		{200, `{"type":"1pw_fill","status":"fill_unknown"}`, "fill fill_unknown", "do not retry in the same browser"},
		{200, `{"type":"fill","status":"completed","fields":[]}`, "invalid 1pw_fill result", ""},
		{403, `{"code":"destination_denied","message":"page is outside the approved login origin"}`, "destination_denied (HTTP 403)", ""},
		{409, `{"code":"conflict","message":"not ready"}`, "nothing was submitted", ""},
		{429, `{"code":"rate_limited","message":"slow down"}`, "may have been submitted", ""},
		{503, `{"code":"provider_unavailable","message":"unavailable"}`, "not available in this deployment", ""},
		{500, `{"code":"internal_error","message":"secret-echo"}`, "may have been submitted", ""},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					io.WriteString(w, onePasswordCredentialWithOperation("1pw_fill"))
					return
				}
				posts++
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"type":"1pw_fill","browser_id":"browser-1","page_url":"https://github.com/login"}`, string(body))
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			out, text, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user", "github", "1pw_fill", "--params", `{"browser_id":"browser-1","page_url":"https://github.com/login"}`)
			assert.Equal(t, 1, posts)
			if tc.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.err)
				assert.NotContains(t, err.Error(), "secret-echo")
			}
			assert.Contains(t, out+text, tc.output)
		})
	}
}

func TestCredentialHelpPresentsBothPaths(t *testing.T) {
	credentials, _, err := newVaultsCommand().Find([]string{"credentials", "create"})
	require.NoError(t, err)
	connect, _, err := newVaultsCommand().Find([]string{"credentials", "connect"})
	require.NoError(t, err)
	invoke, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	for _, long := range []string{newVaultsCommand().Long, credentials.Long} {
		assert.Contains(t, long, "Ask the user which one they want")
		assert.Contains(t, long, "Kernel-hosted collection")
		assert.Contains(t, long, "1Password brokered approval")
		assert.Contains(t, long, "Never ask the user to paste")
	}
	assert.Contains(t, connect.Long, "credential_account")
	assert.Contains(t, credentials.Example, `"provider":"1password"`)
	for _, operation := range []string{"1pw_request_access", "1pw_poll_access", "1pw_fill", "1pw_reconcile_access", "1pw_recover"} {
		assert.Contains(t, invoke.Long, operation)
	}
}

func TestOnePasswordFillLookupErrorKeepsStatus(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{404, `{"code":"not_found","message":"item not found"}`, "not_found (HTTP 404); 1pw_fill was not invoked"},
		{403, `{"code":"other","message":"secret-echo"}`, "(HTTP 403); 1pw_fill was not invoked"},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user", "github", "1pw_fill", "--params", `{"browser_id":"b","page_url":"https://github.com/login"}`)
			require.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "secret-echo")
			assert.Zero(t, posts)
		})
	}
}
