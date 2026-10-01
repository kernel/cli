package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readyFillCredentialFixture = `{"id":"credential-1","type":"credential","spec":{"fields":[{"name":"expiration","type":"password"},{"name":"custom field","type":"text"},{"name":"otp","type":"totp"}]},"state":{"status":"ready"},"available_operations":[{"type":"fill","description":"Fill credential fields."}]}`

func TestVaultFillBothItemTypesAndInputs(t *testing.T) {
	for _, input := range []string{"params", "spec-file"} {
		for _, test := range []struct{ name, item, params, result string }{
			{"card", readyFillCardFixture, fillParamsFixture, completedFillFixture},
			{"credential", readyFillCredentialFixture, `{"browser_id":"browser-id","fields":[{"field":"expiration","selector":"#password"},{"field":"custom_field","selector":"#custom"},{"field":"otp","selector":"#code"}]}`, completedFillFixture},
			{"credential URL", readyFillCredentialFixture, `{"browser_id":"browser-id","page_url":"http://localhost/login","fields":[{"field":"expiration","selector":"#password"}]}`, `{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled"}]}`},
		} {
			t.Run(input+"/"+test.name, func(t *testing.T) {
				calls := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet {
						io.WriteString(w, test.item)
						return
					}
					require.Equal(t, http.MethodPost, r.Method)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, `{"type":"fill",`+test.params[1:], string(body))
					io.WriteString(w, test.result)
				})
				value := test.params
				if input == "spec-file" {
					value = credentialSpecFile(t, value)
				}
				out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "vault", "item", "fill", "--"+input, value, "-o", "json")
				require.NoError(t, err)
				assert.JSONEq(t, test.result, out)
				assert.Empty(t, human)
				assert.Equal(t, 2, calls)
			})
		}
	}
}

func TestVaultCredentialFillValidation(t *testing.T) {
	for _, input := range []string{"params", "spec-file"} {
		for _, params := range []string{
			`{"browser_id":"id","fields":[{"field":"unknown","selector":"#field"}]}`,
			`{"browser_id":"id","fields":[{"field":"expiration","selector":"#field","format":"MM/YY"}]}`,
			`{"browser_id":"id","fields":[{"field":"custom_field","selector":"#field","format":"MM/YYYY"}]}`,
			`{"browser_id":"id","fields":[{"field":"expiration","selector":"#field","value":"secret-sentinel"}]}`,
			`{"browser_id":"id","browser_id":"secret-sentinel","fields":[{"field":"expiration","selector":"#field"}]}`,
		} {
			t.Run(input+"/"+params, func(t *testing.T) {
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet {
						io.WriteString(w, readyFillCredentialFixture)
						return
					}
					require.Equal(t, http.MethodPost, r.Method)
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, `{"code":"invalid_request"}`)
				})
				value := params
				if input == "spec-file" {
					value = credentialSpecFile(t, params)
				}
				out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "vault", "item", "fill", "--"+input, value, "-o", "json")
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "secret-sentinel")
				assert.Empty(t, out+human)
			})
		}
	}
}

func TestVaultFillInputLimits(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "vault", "item", "fill", "--params", fillParamsFixture, "--spec-file", credentialSpecFile(t, fillParamsFixture))
	require.Error(t, err)
	_, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "vault", "item", "fill", "--params", strings.Repeat(" ", 128*1024+1))
	require.Error(t, err)
}

func TestCredentialFillCLIOutcomes(t *testing.T) {
	for _, result := range []string{completedFillFixture, failedFillFixture, unknownFillFixture} {
		t.Run(result, func(t *testing.T) {
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					io.WriteString(w, readyFillCredentialFixture)
					return
				}
				posts.Add(1)
				io.WriteString(w, result)
			}))
			defer server.Close()
			out, stderr, exit := runVaultFillCLI(t, server.URL, "fill", "--params", `{"browser_id":"id","fields":[{"field":"expiration","selector":"#password"},{"field":"custom_field","selector":"#custom"},{"field":"otp","selector":"#code"}]}`, "-o", "json")
			assert.True(t, json.Valid([]byte(out)))
			assert.JSONEq(t, result, out)
			assert.Empty(t, stderr)
			expectedExit := 1
			if result == completedFillFixture {
				expectedExit = 0
			}
			assert.Equal(t, expectedExit, exit)
			assert.EqualValues(t, 1, posts.Load())
		})
	}
}
