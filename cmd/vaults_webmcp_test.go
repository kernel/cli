package cmd

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readyWebMCPCredentialFixture = `{"id":"credential-1","type":"credential","spec":{"fields":[{"name":"email","type":"text"},{"name":"password","type":"password"}]},"state":{"status":"ready"},"available_operations":[{"type":"webmcp_invoke","description":"Invoke a WebMCP tool."}]}`

const webMCPParamsFixture = `{"browser_id":"browser-id","tool_ref":"tool-1","page_url":"https://accounts.example.com/signin","input":{"email":null,"password":null,"remember":1.50},"bindings":[{"field":"email","input_path":"/email"},{"field":"password","input_path":"/password"}],"timeout_sec":30}`

func TestVaultWebMCPInvoke(t *testing.T) {
	for _, test := range []struct {
		name, result string
		wantErr      bool
	}{
		{"completed", `{"type":"webmcp_invoke","status":"completed","invocation_id":"invoke-1","output":{"authenticated":true,"count":1.50}}`, false},
		{"awaiting submission", `{"type":"webmcp_invoke","status":"awaiting_submission","invocation_id":"invoke-1"}`, false},
		{"unknown", `{"type":"webmcp_invoke","status":"unknown","error_text":"lost"}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					io.WriteString(w, readyWebMCPCredentialFixture)
					return
				}
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"type":"webmcp_invoke",`+webMCPParamsFixture[1:], string(body))
				io.WriteString(w, test.result)
			})
			out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "vault", "item", "webmcp_invoke", "--params", webMCPParamsFixture, "-o", "json")
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.JSONEq(t, test.result, out)
			assert.Equal(t, 2, calls)
		})
	}
}

func TestVaultWebMCPInvokeValidation(t *testing.T) {
	for _, params := range []string{
		`{"tool_ref":"t","page_url":"https://a.example","input":{"p":null},"bindings":[{"field":"password","input_path":"/p"}]}`,
		`{"browser_id":"b","page_url":"https://a.example","input":{"p":null},"bindings":[{"field":"password","input_path":"/p"}]}`,
		`{"browser_id":"b","tool_ref":"t","page_url":"not a url","input":{"p":null},"bindings":[{"field":"password","input_path":"/p"}]}`,
		`{"browser_id":"b","tool_ref":"t","page_url":"https://a.example","input":[],"bindings":[{"field":"password","input_path":"/p"}]}`,
		`{"browser_id":"b","tool_ref":"t","page_url":"https://a.example","input":{"p":null},"bindings":[]}`,
		`{"browser_id":"b","tool_ref":"t","page_url":"https://a.example","input":{"p":null},"bindings":[{"field":"password","input_path":"p"}]}`,
		`{"browser_id":"b","tool_ref":"t","page_url":"https://a.example","input":{"p":null},"bindings":[{"field":"password","input_path":"/p","value":"secret"}]}`,
		`{"browser_id":"b","tool_ref":"t","page_url":"https://a.example","input":{"p":null},"bindings":[{"field":"password","input_path":"/p"}],"timeout_sec":121}`,
		`{"type":"webmcp_invoke","browser_id":"b","tool_ref":"t","page_url":"https://a.example","input":{"p":null},"bindings":[{"field":"password","input_path":"/p"}]}`,
	} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		})
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "vault", "item", "webmcp_invoke", "--params", params)
		assert.Error(t, err, params)
	}
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "vault", "item", "webmcp_invoke")
	assert.ErrorContains(t, err, "webmcp_invoke requires --params")
}
