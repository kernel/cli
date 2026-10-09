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

const onePasswordCredentialFixture = `{"id":"credential-2","key":"github","type":"credential","version":1,"spec":{"provider":"1password","account":"onepassword","requests":{"version":2,"entries":[{"type":"login","parameters":{"website":"https://github.com"}}]}},"state":{"provider":"1password","status":"pending_authorization","access_request_id":"req-1","access_request":{"id":"req-1","state":"pending","identity":"never-print-identity","path":"never-print-path","has_autofill_token":false,"granted_count":0,"entries":[{"id":"entry-1","type":"login","parameters":{"website":"https://github.com"}}]}},"action":{"name":"1password_access_approval","url":"onepassword://grant-brokered-access?access_request_reference=ref-1","instructions":"Present this link to the account owner."},"available_operations":[{"type":"1pw_access_request_status","description":"Check the request."}],"available_expansions":[],"created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`

func onePasswordCredentialWithOperation(operation string) string {
	return strings.Replace(onePasswordCredentialFixture, `"1pw_access_request_status"`, `"`+operation+`"`, 1)
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
	requests := `"requests":{"version":2,"entries":[{"type":"login","parameters":{"website":"https://github.com"},"reason":"Work account"},{"type":"login","parameters":{"website":"https://github.com"},"reason":"Personal account"}]}`
	entry := `{"type":"login","parameters":{"website":"https://github.com"}}`
	for _, spec := range []string{
		`{"provider":"1password","account":"onepassword",` + requests + `}`,
		`{"provider":"1password","access_token":"token-secret","integration_key":"key-secret",` + requests + `}`,
	} {
		calls := 0
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"credential","spec":`+spec+`}`, string(body))
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, onePasswordCredentialFixture)
		})
		out, text, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "user", "github", "--spec-file", credentialSpecFile(t, spec), "-o", "json")
		require.NoError(t, err)
		assert.Equal(t, 1, calls)
		assert.NotContains(t, out+text, "-secret")
	}

	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	for _, tc := range []struct{ spec, err string }{
		{`{"provider":"1password",` + requests + `}`, "either account"},
		{`{"provider":"1password","account":"onepassword","access_token":"token-secret","integration_key":"key-secret",` + requests + `}`, "never both"},
		{`{"provider":"1password","access_token":"token-secret",` + requests + `}`, "both access_token and integration_key"},
		{`{"provider":"1password","account":"onepassword"}`, "1-5 login entries"},
		{`{"provider":"1password","access_token":"token-secret","integration_key":"key-secret","website":"https://github.com"}`, "1-5 login entries"},
		{`{"provider":"1password","account":"onepassword","requests":{"version":2,"entries":[` + strings.Repeat(entry+",", 5) + entry + `]}}`, "1-5 login entries"},
		{`{"provider":"lastpass","fields":[{"name":"password","type":"password"}]}`, "kernel, 1password, or managed_auth"},
	} {
		_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "user", "github", "--spec-file", credentialSpecFile(t, tc.spec))
		require.ErrorContains(t, err, tc.err, tc.spec)
		assert.NotContains(t, err.Error(), "-secret")
	}
	assert.Zero(t, calls)
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
	for _, want := range []string{`"account": "onepassword"`, `"id": "entry-1"`, `"website": "https://github.com"`, `"access_request_id": "req-1"`, `"state": "pending"`, "onepassword://grant-brokered-access?access_request_reference=ref-1", `"instructions": "Present this link to the account owner."`} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "never-print")

	out, text, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "github")
	require.NoError(t, err)
	output := out + text
	assert.Contains(t, output, "onepassword://grant-brokered-access?access_request_reference=ref-1")
	assert.Contains(t, output, "Present this link to the account owner.")
	assert.Contains(t, output, "onepassword")
	assert.Contains(t, output, "entry-1 https://github.com")
	assert.NotContains(t, output, "stored token")
	assert.Contains(t, output, "Invoke: kernel vaults items invoke --params '<json>' -- user github 1pw_access_request_status")
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
		{"1pw_create_access_request", `{"goal":"Manage billing","reason":"Sign in","keywords":["personal"]}`, `{"type":"1pw_create_access_request","goal":"Manage billing","reason":"Sign in","keywords":["personal"]}`, onePasswordCredentialFixture},
		{"1pw_create_access_request", "", `{"type":"1pw_create_access_request"}`, onePasswordCredentialFixture},
		{"1pw_access_request_status", `{"timeout_seconds":60}`, `{"type":"1pw_access_request_status","timeout_seconds":60}`, onePasswordCredentialFixture},
		{"1pw_access_request_status", "", `{"type":"1pw_access_request_status"}`, onePasswordCredentialFixture},
		{"1pw_recover", "", `{"type":"1pw_recover"}`, onePasswordAccountFixture},
	} {
		t.Run(tc.operation+tc.params, func(t *testing.T) {
			item := strings.Replace(tc.item, `"available_operations":[]`, `"available_operations":[{"type":"1pw_access_request_status","description":"x"}]`, 1)
			item = strings.Replace(item, `"1pw_access_request_status"`, `"`+tc.operation+`"`, 1)
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
		{"1pw_create_access_request", `{"browser_id":"b"}`, "only supported"},
		{"1pw_create_access_request", `{"password":"x"}`, "only supported"},
		{"1pw_create_access_request", `{"type":"1pw_create_access_request"}`, "must not contain type"},
		{"1pw_access_request_status", `{"browser_id":"b"}`, "only supported"},
		{"1pw_access_request_status", `{"timeout_seconds":121}`, "timeout_seconds"},
		{"1pw_fill", "", "requires --params"},
		{"1pw_fill", `{"browser_id":"","page_url":"https://github.com/login"}`, "browser_id"},
		{"1pw_reconcile_access", `{"acknowledge_unconfirmed":true}`, "unsupported 1Password operation"},
		{"1pw_fill", `{"browser_id":"b"}`, "page_url"},
		{"1pw_fill", `{"browser_id":"b","page_url":"https://github.com/login","timeout_ms":0}`, "timeout_ms"},
		{"1pw_fill", `{"browser_id":"b","page_url":"https://github.com/login","entry_id":""}`, "entry_id"},
		{"1pw_update_access_token", `{"access_token":"token-secret"}`, "only through --spec-file"},
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
		for _, want := range []string{
			"items list <vault> -o json",
			"ask the user where this login lives",
			"do not\n   choose for them",
			"Kernel-hosted collection",
			"1Password brokered approval",
			"non-shared 1Password vault",
			"State this requirement when asking",
			"as plain text rather than code so they stay clickable",
			"passkeys",
			"offer Kernel-hosted collection",
			"Never ask the user to paste",
			"not proof of sign-in",
		} {
			assert.Contains(t, long, want)
		}
	}
	for _, want := range []string{
		"Ask whose 1Password account holds the login and confirm it is in a non-shared vault",
		"1-5 login entries",
		"Invoke 1pw_create_access_request once",
		"pass entry_id",
		"not that sign-in succeeded",
		"do not\ndelete and recreate the item to reset it",
		"After a confirmed failed status, ask the\nuser",
		"credentials connect again with the same key",
	} {
		assert.Contains(t, credentials.Long, want)
		assert.Contains(t, connect.Long, want)
	}
	assert.Contains(t, credentials.Long, "Stored-token 1Password credentials (developer integrations only")
	assert.Contains(t, credentials.Long, "Never ask an end user for them")
	assert.NotContains(t, connect.Long, "Stored-token")
	assert.Contains(t, connect.Long, "credential_account")
	assert.Contains(t, credentials.Example, `"provider":"1password","account":"onepassword"`)
	for _, operation := range []string{"1pw_create_access_request", "1pw_access_request_status", "1pw_fill", "1pw_update_access_token", "1pw_recover"} {
		assert.Contains(t, invoke.Long, operation)
	}
	assert.Contains(t, invoke.Long, "--spec-file, never --params")
	assert.Contains(t, invoke.Example, `"entry_id":"<entry-id>"`)
	for _, help := range []string{invoke.Long, invoke.Example, credentials.Long, credentials.Example, connect.Long} {
		for _, removed := range []string{"1pw_request_access", "1pw_poll_access", "reconcile", "account_id", "Family", "customer_managed", "access_token_expires_at", "RFC 3339"} {
			assert.NotContains(t, help, removed)
		}
	}
	for _, help := range []string{invoke.Long, connect.Long} {
		assert.NotContains(t, help, "integration key")
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

func TestOnePasswordFillEntryAndTokenUpdateRequests(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		operation, params, body, response string
	}{
		{"1pw_fill", `{"browser_id":"browser-1","page_url":"https://github.com/login","entry_id":"entry-1"}`, `{"type":"1pw_fill","browser_id":"browser-1","page_url":"https://github.com/login","entry_id":"entry-1"}`, `{"type":"1pw_fill","status":"fill_submitted"}`},
		{"1pw_update_access_token", `{"access_token":"token-secret"}`, `{"type":"1pw_update_access_token","access_token":"token-secret"}`, onePasswordCredentialWithOperation("1pw_update_access_token")},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					io.WriteString(w, onePasswordCredentialWithOperation(tc.operation))
					return
				}
				posts++
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, tc.body, string(body))
				io.WriteString(w, tc.response)
			})
			out, text, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user", "github", tc.operation, "--spec-file", credentialSpecFile(t, tc.params))
			require.NoError(t, err)
			assert.Equal(t, 1, posts)
			assert.NotContains(t, out+text, "token-secret")
		})
	}

	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	for _, params := range []string{`{"access_token":""}`, `{"access_token":"token-secret","access_token_expires_at":"2026-10-01T00:00:00Z"}`, `{"access_token":"token-secret","integration_key":"key-secret"}`} {
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user", "github", "1pw_update_access_token", "--spec-file", credentialSpecFile(t, params))
		require.Error(t, err, params)
		assert.NotContains(t, err.Error(), "-secret")
	}
	assert.Zero(t, calls)
}

func TestOnePasswordOperationErrorGuidance(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{400, `{"code":"invalid_request","message":"secret-echo"}`, "rejected (HTTP 400)"},
		{409, `{"code":"conflict","message":"operation is not currently available"}`, "do not retry, delete, or recreate the item to reset it"},
		{503, `{"code":"provider_unavailable","message":"secret-echo"}`, "offer Kernel-hosted collection"},
		{500, `{"code":"internal_error","message":"secret-echo"}`, "outcome is unknown"},
		{502, `{"code":"bad_gateway","message":"secret-echo"}`, "do not retry, delete, or recreate the item"},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					io.WriteString(w, onePasswordCredentialWithOperation("1pw_create_access_request"))
					return
				}
				posts++
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user", "github", "1pw_create_access_request", "--params", `{}`)
			require.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "secret-echo")
			assert.NotContains(t, err.Error(), "before retrying")
			assert.Equal(t, 1, posts)
		})
	}

	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, onePasswordCredentialFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user", "github", "1pw_create_access_request", "--params", `{}`)
	require.ErrorContains(t, err, "check that the linked credential_account is connected")
	assert.Contains(t, err.Error(), "do not delete or recreate the item")
}

func TestCredentialConnectOnePasswordUnavailable(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"code":"provider_unavailable","message":"1Password OAuth is not configured"}`)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "connect", "user", "onepassword", "--provider", "1password")
	require.ErrorContains(t, err, "1Password account linking unavailable (HTTP 500)")
	assert.Contains(t, err.Error(), "offer Kernel-hosted collection")
}

func TestVaultItemsListShowsCredentialSite(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	kernelCredential := `{"id":"credential-1","key":"hn","type":"credential","version":1,"spec":{"provider":"kernel","description":"Hacker News","fields":[{"name":"username","type":"text","required":true,"sensitive":false}]},"state":{"provider":"kernel","status":"ready","fields":{"username":{"has_value":true}}},"available_operations":[],"available_expansions":[],"created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`
	storedToken := strings.Replace(onePasswordCredentialFixture, `"account":"onepassword",`, `"access_token_expires_at":"2026-10-01T00:00:00Z",`, 1)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "["+kernelCredential+","+onePasswordCredentialFixture+","+onePasswordAccountFixture+"]")
	})
	out, text, err := executeVaultCommand(t, client, "vaults", "items", "list", "user")
	require.NoError(t, err)
	output := out + text
	assert.Contains(t, output, "Site")
	assert.Contains(t, output, "Hacker News")
	assert.Contains(t, output, "https://github.com")

	client = vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, storedToken)
	})
	out, text, err = executeVaultCommand(t, client, "vaults", "items", "get", "user", "github")
	require.NoError(t, err)
	assert.Contains(t, out+text, "stored token (developer-supplied)")
	assert.NotContains(t, out+text, "2026-10-01T00:00:00Z")
	assert.NotContains(t, out+text, "expires")

	out, text, err = executeVaultCommand(t, client, "vaults", "items", "get", "user", "github", "-o", "json")
	require.NoError(t, err)
	assert.NotContains(t, out+text, "access_token_expires_at")
}
