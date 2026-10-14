package cmd

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultInvokeAuthorizeSendsCheckoutContext(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for name, tc := range map[string]struct{ params, body string }{
		"checkout": {`{"browser_id":"browser-1","page_url":"https://shop.example/checkout"}`, `{"type":"authorize","browser_id":"browser-1","page_url":"https://shop.example/checkout"}`},
		"none":     {"", `{"type":"authorize"}`},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 2 {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, tc.body, string(body))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, requestedCardFixture)
			})
			args := []string{"vaults", "items", "invoke", "checkout", "order-1", "authorize", "-o", "json"}
			if tc.params != "" {
				args = append(args, "--params", tc.params)
			}
			_, _, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
		})
	}
}

func TestParseVaultAuthorizeParamsValidates(t *testing.T) {
	for raw, want := range map[string]string{
		`{"browser_id":"browser-1"}`:                          "together",
		`{"page_url":"https://shop.example/checkout"}`:        "together",
		`{"browser_id":"","page_url":"https://shop.example"}`: "browser_id",
		`{"browser_id":"b","page_url":"checkout"}`:            "page_url",
		`{"browser_id":"b","page_url":"https://x","x":1}`:     "supported",
	} {
		_, err := parseVaultOperationParams("authorize", raw, true, false)
		require.Error(t, err, raw)
		assert.Contains(t, err.Error(), want, raw)
	}
}

func TestParseVaultFillParamsFieldsOptional(t *testing.T) {
	params, err := parseVaultOperationParams("fill", `{"browser_id":"browser-1","page_url":"https://shop.example/checkout"}`, true, false)
	require.NoError(t, err)
	assert.Empty(t, params.Fill.Fields)
	_, err = parseVaultOperationParams("fill", `{"browser_id":"browser-1","fields":null}`, true, false)
	require.Error(t, err)
}
