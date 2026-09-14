package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const credentialFixture = `{"id":"credential-1","key":"login","type":"credential","version":2,"spec":{"description":"Website login","fields":{"password":{"type":"password","required":true,"sensitive":true,"value":"never-print"}}},"state":{"status":"pending_collection","fields":{"password":{"has_value":false,"value":"never-print"}}},"action":{"name":"collect","url":"https://vault.kernel.sh/collect#token=item.random","expires_at":"2026-10-01T00:00:00Z"},"available_operations":[{"type":"collect","description":"Open the form"},{"type":"fill","description":"Fill the form"}],"available_expansions":[]}`

func credentialSpecFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.json")
	require.NoError(t, os.WriteFile(path, []byte(data), 0600))
	return path
}

func TestCredentialCreateAndUpdate(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, update := range []bool{false, true} {
		t.Run(fmt.Sprint(update), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "/vaults/user/items/login", r.URL.Path)
				var body map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.JSONEq(t, `"credential"`, string(body["type"]))
				if update {
					assert.Equal(t, "PATCH", r.Method)
					assert.JSONEq(t, `2`, string(body["version"]))
					assert.JSONEq(t, `{"fields":{"password":{"value":null}}}`, string(body["spec"]))
				} else {
					assert.Equal(t, "PUT", r.Method)
					assert.JSONEq(t, `{"fields":{"password":{"type":"password","required":true}}}`, string(body["spec"]))
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, credentialFixture)
			})
			args := []string{"vaults", "credentials", "create", "user", "login", "--spec-file", credentialSpecFile(t, `{"fields":{"password":{"type":"password","required":true}}}`), "-o", "json"}
			if update {
				args[2] = "update"
				args[6] = credentialSpecFile(t, `{"fields":{"password":{"value":null}}}`)
				args = append(args, "--version", "2")
			}
			out, _, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.NotContains(t, out, "never-print")
			assert.Contains(t, out, `"has_value": false`)
			assert.Contains(t, out, `"version": 2`)
			assert.Contains(t, out, "#token=item.random")
		})
	}
}

func TestCredentialWriteErrorsAreRedactedAndNotRetried(t *testing.T) {
	for _, status := range []int{400, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				io.WriteString(w, `{"message":"secret-echo"}`)
			})
			c := VaultsCmd{vaults: &client.Vaults}
			err := c.saveCredential(context.Background(), "user", "login", []byte(`{"fields":{"password":{"type":"password","value":"secret-echo"}}}`), false, 0, "json", false)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret-echo")
			assert.Equal(t, 1, calls)
		})
	}
}

func TestCredentialCollectAndFill(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, status := range []string{"collect", "completed", "failed", "unknown"} {
		t.Run(status, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					io.WriteString(w, credentialFixture)
					return
				}
				assert.Equal(t, "POST", r.Method)
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				if status == "collect" {
					assert.JSONEq(t, `{"type":"collect"}`, string(data))
					io.WriteString(w, credentialFixture)
					return
				}
				assert.JSONEq(t, `{"type":"fill","browser_id":"browser-1","fields":[{"field":"password","selector":"#password"}]}`, string(data))
				io.WriteString(w, fmt.Sprintf(`{"type":"fill","status":%q,"fields":[{"index":0,"status":"filled"}],"secret":"never-print"}`, status))
			})
			op := "fill"
			if status == "collect" {
				op = "collect"
			}
			args := []string{"vaults", "items", "invoke", "user", "login", op, "-o", "json"}
			if op == "fill" {
				args = append(args, "--spec-file", credentialSpecFile(t, `{"browser_id":"browser-1","fields":[{"field":"password","selector":"#password"}]}`))
			}
			out, _, err := executeVaultCommand(t, client, args...)
			if status == "failed" || status == "unknown" {
				require.ErrorContains(t, err, "do not automatically retry")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, 2, calls)
			assert.NotContains(t, out, "never-print")
		})
	}
}

func TestCredentialFillRejectsUnsafeOutcomes(t *testing.T) {
	for _, raw := range []string{
		`{"type":"fill","status":"secret-echo","fields":[]}`,
		`{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled","error_code":"secret-echo"}]}`,
		`{"type":"fill","status":"completed","fields":[]}`,
	} {
		var result kernel.VaultItemOperationResponseUnion
		require.NoError(t, json.Unmarshal([]byte(raw), &result))
		var err error
		out := captureStdout(t, func() { err = printVaultFill(&result, 1) })
		require.Error(t, err)
		assert.Empty(t, out)
		assert.NotContains(t, err.Error(), "secret-echo")
	}
}

func TestCredentialDiscoveryAndInvalidInput(t *testing.T) {
	for _, path := range []string{"credentials", "credentials create", "credentials update", "items", "items invoke"} {
		cmd, _, err := newVaultsCommand().Find(strings.Fields(path))
		require.NoError(t, err)
		assert.NotEmpty(t, cmd.Long)
	}
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid input reached API") })
	for _, body := range []string{"null", "[]", "{} {}", strings.Repeat("x", 128*1024+1)} {
		_, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "user", "login", "--spec-file", credentialSpecFile(t, body))
		require.Error(t, err)
	}
	cmd, _, err := newVaultsCommand().Find([]string{"credentials", "create"})
	require.NoError(t, err)
	cmd.Flags().Set("spec-file", "-")
	cmd.SetIn(strings.NewReader(`{"fields":{}}`))
	data, err := readVaultSpecFile(cmd)
	require.NoError(t, err)
	assert.JSONEq(t, `{"fields":{}}`, string(data))
}
