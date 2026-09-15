package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const credentialSpecFixture = `{"description":"Hacker News","fields":{"username":{"type":"text","sensitive":false},"password":{"type":"password"}}}`

const pendingCredentialFixture = `{"id":"item-1","key":"hacker-news","type":"credential","version":1,` +
	`"spec":{"description":"Hacker News","fields":{"username":{"type":"text","required":true,"sensitive":false},"password":{"type":"password","required":true,"sensitive":true}}},` +
	`"state":{"status":"pending_collection","fields":{"username":{"has_value":true,"value":"ada"},"password":{"has_value":false}}},` +
	`"action":{"name":"collect","url":"https://vault.kernel.sh/c/session-token","expires_at":"2026-09-14T00:30:00Z"},` +
	`"available_operations":[{"type":"collect","description":"Open the collection form."}],"available_expansions":[],` +
	`"created_at":"2026-09-14T00:00:00Z","updated_at":"2026-09-14T00:00:00Z"}`

const readyCredentialFixture = `{"id":"item-1","key":"hacker-news","type":"credential","version":2,` +
	`"spec":{"fields":{"username":{"type":"text","required":true,"sensitive":false},"password":{"type":"password","required":true,"sensitive":true}}},` +
	`"state":{"status":"ready","fields":{"username":{"has_value":true,"value":"ada"},"password":{"has_value":true}}},` +
	`"available_operations":[{"type":"collect","description":"Open the collection form."},{"type":"fill","description":"Fill login fields."}],"available_expansions":[],` +
	`"created_at":"2026-09-14T00:00:00Z","updated_at":"2026-09-14T00:00:00Z"}`

func writeVaultValuesFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "values.json")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func TestVaultCredentialCreateRequestMapping(t *testing.T) {
	var body []byte
	var method, path string
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, pendingCredentialFixture)
	})
	values := writeVaultValuesFile(t, `{"username":"ada"}`)
	out, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "logins", "hacker-news", "--spec", credentialSpecFixture, "--values-file", values, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, method)
	assert.Equal(t, "/vaults/logins/items/hacker-news", path)
	assert.JSONEq(t, `{"type":"credential","spec":{"description":"Hacker News","fields":{"username":{"type":"text","sensitive":false,"value":"ada"},"password":{"type":"password"}}}}`, string(body))
	assert.JSONEq(t, pendingCredentialFixture, out)
}

func TestVaultCredentialCreateValidation(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	values := writeVaultValuesFile(t, `{"username":"ada"}`)
	for name, args := range map[string][]string{
		"spec not an object":  {"--spec", `[]`},
		"spec scalar":         {"--spec", `"credential-sentinel"`},
		"unsupported key":     {"--spec", `{"fields":{"a":{"type":"text"}},"provider":"link"}`},
		"missing fields":      {"--spec", `{"description":"x"}`},
		"empty fields":        {"--spec", `{"fields":{}}`},
		"bad field name":      {"--spec", `{"fields":{"user-name":{"type":"text"}}}`},
		"bad field type":      {"--spec", `{"fields":{"username":{"type":"credential-sentinel"}}}`},
		"missing field type":  {"--spec", `{"fields":{"username":{}}}`},
		"inline value":        {"--spec", `{"fields":{"username":{"type":"text","value":"credential-sentinel"}}}`},
		"undeclared value":    {"--spec", credentialSpecFixture, "--values-file", writeVaultValuesFile(t, `{"nickname":"ada"}`)},
		"null value":          {"--spec", credentialSpecFixture, "--values-file", writeVaultValuesFile(t, `{"username":null}`)},
		"empty value":         {"--spec", credentialSpecFixture, "--values-file", writeVaultValuesFile(t, `{"username":""}`)},
		"values not object":   {"--spec", credentialSpecFixture, "--values-file", writeVaultValuesFile(t, `["ada"]`)},
		"missing values file": {"--spec", credentialSpecFixture, "--values-file", filepath.Join(t.TempDir(), "absent.json")},
		"empty values":        {"--spec", credentialSpecFixture, "--values-file", writeVaultValuesFile(t, `{}`)},
		"bad values name":     {"--spec", credentialSpecFixture, "--values-file", writeVaultValuesFile(t, `{"user-name":"ada"}`)},
	} {
		t.Run(name, func(t *testing.T) {
			out, human, err := executeVaultCommand(t, client, append([]string{"vaults", "credentials", "create", "logins", "hacker-news"}, args...)...)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "credential-sentinel")
			assert.Empty(t, out)
			assert.Empty(t, human)
		})
	}
	// --spec is required; values alone cannot declare a schema.
	_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "logins", "hacker-news", "--values-file", values)
	require.Error(t, err)
}

func TestVaultCredentialUpdateRequestMapping(t *testing.T) {
	var body []byte
	var method string
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, readyCredentialFixture)
	})
	values := writeVaultValuesFile(t, `{"password":"hunter2","username":null}`)
	out, _, err := executeVaultCommand(t, client, "vaults", "credentials", "update", "logins", "hacker-news",
		"--version", "1", "--expected-item-id", "item-1", "--description", "Hacker News", "--values-file", values, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, method)
	assert.JSONEq(t, `{"type":"credential","version":1,"expected_item_id":"item-1","spec":{"description":"Hacker News","fields":{"password":{"value":"hunter2"},"username":{"value":null}}}}`, string(body))
	assert.JSONEq(t, readyCredentialFixture, out)
	assert.NotContains(t, out, "hunter2")
}

func TestVaultCredentialUpdateDescriptionOnlyAndValidation(t *testing.T) {
	var body []byte
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, readyCredentialFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "update", "logins", "hacker-news", "--version", "2", "--description", "", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"credential","version":2,"spec":{"description":""}}`, string(body))

	strict := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	for name, args := range map[string][]string{
		"no changes":   {"--version", "2"},
		"zero version": {"--version", "0", "--description", "x"},
		"bad version":  {"--version", "-1", "--description", "x"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := executeVaultCommand(t, strict, append([]string{"vaults", "credentials", "update", "logins", "hacker-news"}, args...)...)
			require.Error(t, err)
		})
	}
	_, _, err = executeVaultCommand(t, strict, "vaults", "credentials", "update", "logins", "hacker-news", "--description", "x")
	require.ErrorContains(t, err, "version")
}

func TestVaultCredentialValuesFromStdin(t *testing.T) {
	var body []byte
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, readyCredentialFixture)
	})
	stdin := strings.NewReader(`{"password":"hunter2"}`)
	_, _, err := executeVaultCommandWithStdin(t, client, stdin, "vaults", "credentials", "update", "logins", "hacker-news", "--version", "1", "--values-file", "-", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"credential","version":1,"spec":{"fields":{"password":{"value":"hunter2"}}}}`, string(body))
}

func TestVaultCollectOperation(t *testing.T) {
	calls := 0
	var body []byte
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/vaults/logins/items/hacker-news/operations", r.URL.Path)
			var err error
			body, err = io.ReadAll(r.Body)
			require.NoError(t, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, pendingCredentialFixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "logins", "hacker-news", "collect", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.JSONEq(t, `{"type":"collect"}`, string(body))
	assert.JSONEq(t, pendingCredentialFixture, out)

	// collect takes no parameters.
	_, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "logins", "hacker-news", "collect", "--params", `{}`)
	require.ErrorContains(t, err, "--params")
}

func TestVaultCredentialItemHumanOutput(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, pendingCredentialFixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "logins", "hacker-news")
	require.NoError(t, err)
	assert.Contains(t, human, "credential")
	assert.Contains(t, human, "Version")
	assert.Contains(t, human, "Hacker News")
	assert.Contains(t, human, "pending_collection")
	// The declared schema and per-field presence are both shown.
	assert.Contains(t, human, "username")
	assert.Contains(t, human, "ada")
	assert.Contains(t, human, "password")
	assert.Contains(t, human, "(withheld)")
	assert.Contains(t, human, "https://vault.kernel.sh/c/session-token")
	assert.Contains(t, human, "Collection action")
}

func TestVaultCredentialFillBindings(t *testing.T) {
	calls := 0
	var body []byte
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, readyCredentialFixture)
			return
		}
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		_, _ = io.WriteString(w, `{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled"},{"index":1,"status":"filled"}]}`)
	})
	// Credential items may omit page_url to require exactly one open page.
	params := `{"browser_id":"browser-session-id","fields":[{"field":"username","selector":"#login"},{"field":"password","selector":"#password"}]}`
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "logins", "hacker-news", "fill", "--params", params, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	var request map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &request))
	_, hasPageURL := request["page_url"]
	assert.False(t, hasPageURL)
	assert.JSONEq(t, `[{"field":"username","selector":"#login"},{"field":"password","selector":"#password"}]`, string(request["fields"]))
	assert.Contains(t, out, "completed")
}

func TestVaultFillItemTypeValidation(t *testing.T) {
	for name, tc := range map[string]struct{ fixture, params, message string }{
		"card without page_url": {
			readyFillCardFixture,
			`{"browser_id":"b","fields":[{"field":"number","selector":"#n"}]}`,
			"page_url",
		},
		"credential field not declared": {
			readyCredentialFixture,
			`{"browser_id":"b","fields":[{"field":"nickname","selector":"#n"}]}`,
			"not declared",
		},
		"credential expiration format": {
			readyCredentialFixture,
			`{"browser_id":"b","fields":[{"field":"expiration","format":"MM/YY","selector":"#e"}]}`,
			"expiration",
		},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.fixture)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "logins", "hacker-news", "fill", "--params", tc.params, "-o", "json")
			require.ErrorContains(t, err, tc.message)
			assert.Equal(t, 1, calls, "no operation may be posted after a rejected binding")
		})
	}
}

func TestVaultCredentialAPIErrorsAreSpecificAndSafe(t *testing.T) {
	for status, message := range map[int]string{400: "declared", 409: "items get", 500: "inspect existing state"} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":{"message":"credential-sentinel"}}`)
			})
			values := writeVaultValuesFile(t, `{"username":"credential-sentinel"}`)
			_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "logins", "hacker-news", "--spec", credentialSpecFixture, "--values-file", values)
			require.ErrorContains(t, err, message)
			assert.NotContains(t, err.Error(), "credential-sentinel")

			_, _, err = executeVaultCommand(t, client, "vaults", "credentials", "update", "logins", "hacker-news", "--version", "1", "--values-file", values)
			require.ErrorContains(t, err, message)
			assert.NotContains(t, err.Error(), "credential-sentinel")
		})
	}
}
