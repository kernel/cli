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

func TestVaultKernelSingleUseCardFromSpecFile(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	const number = "4242424242424242"
	const cvc = "123"
	const spec = `{"wallet":"wallet-1","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","expires_at":"2030-11-01T12:00:00Z","single_use_card":{"number":"` + number + `","cvc":"` + cvc + `","exp_month":"11","exp_year":"2030","name":"Jane Doe","billing_address":{"line1":"1 Main St","line2":"Unit 2","city":"Springfield","state":"IL","postal_code":"62701","country":"US"},"reference":"vc_example"}}`
	response := `{"id":"card-1","key":"single-use-order","type":"card","spec":{"provider":"kernel","wallet":"wallet-1","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","expires_at":"2030-11-01T12:00:00Z","single_use_card":{"number":"` + number + `","cvc":"` + cvc + `","exp_month":"11","exp_year":"2030","name":"Jane Doe","billing_address":{"line1":"1 Main St"},"reference":"vc_example"}},"state":{"provider":"kernel","status":"ready","source":"single_use_card","domains":["example.com"],"masks":{"brand":"visa","last4":"4242"}},"available_operations":[{"type":"fill","description":"Fill checkout fields"}],"available_expansions":[],"expires_at":"2030-11-01T12:00:00Z"}`

	for _, output := range []string{"", "json"} {
		t.Run(output, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/vaults/checkout/items/single-use-order", r.URL.Path)
				var body struct {
					Type string                     `json:"type"`
					Spec map[string]json.RawMessage `json:"spec"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "card", body.Type)
				assert.JSONEq(t, `"kernel"`, string(body.Spec["provider"]))
				assert.JSONEq(t, `"2030-11-01T12:00:00Z"`, string(body.Spec["expires_at"]))
				assert.JSONEq(t, `{"number":"`+number+`","cvc":"`+cvc+`","exp_month":"11","exp_year":"2030","name":"Jane Doe","billing_address":{"line1":"1 Main St","line2":"Unit 2","city":"Springfield","state":"IL","postal_code":"62701","country":"US"},"reference":"vc_example"}`, string(body.Spec["single_use_card"]))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			})
			args := []string{"vaults", "cards", "create", "checkout", "single-use-order", "--provider", "kernel", "--spec-file", "-"}
			if output != "" {
				args = append(args, "-o", output)
			}
			out, human, err := executeVaultInputCommand(t, client, spec, args...)
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.NotContains(t, out+human, number)
			assert.NotContains(t, out+human, cvc)
			assert.NotContains(t, out+human, "1 Main St")
			assert.Contains(t, out+human, "vc_example")
			assert.Contains(t, out+human, "single_use_card")
			if output == "json" {
				assert.Contains(t, out, `"source": "single_use_card"`)
				assert.JSONEq(t, `{"reference":"vc_example"}`, extractVaultJSONField(t, out, "spec", "single_use_card"))
			} else {
				assert.Contains(t, human, "funding source")
				assert.Contains(t, human, "supports only fill")
			}
		})
	}
}

func extractVaultJSONField(t *testing.T, raw string, path ...string) string {
	t.Helper()
	var value map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &value))
	for i, field := range path {
		next := value[field]
		if i == len(path)-1 {
			return string(next)
		}
		require.NoError(t, json.Unmarshal(next, &value))
	}
	return ""
}

func TestVaultKernelSingleUseCardErrorDoesNotLeakInput(t *testing.T) {
	secret := vaultTestSecret(t)
	spec := fmt.Sprintf(`{"wallet":"wallet-1","amount":100,"currency":"usd","merchant_name":"Shop","merchant_url":"https://example.com","single_use_card":{"number":%q,"cvc":"123","exp_month":"11","exp_year":"2030"}}`, secret)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"code":"invalid_request","message":%q}`, secret)
	})
	out, human, err := executeVaultInputCommand(t, client, spec,
		"vaults", "cards", "create", "checkout", "single-use-order", "--provider", "kernel", "--spec-file", "-")
	require.Error(t, err)
	assert.False(t, strings.Contains(out+human+err.Error(), secret))
	assert.Contains(t, err.Error(), "vault request rejected")
}

func TestVaultKernelSingleUseCardRequiresProtectedSpecFile(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unsafe card input reached API") })
	secret := "4242424242424242"
	inline := fmt.Sprintf(`{"wallet":"wallet-1","amount":100,"currency":"usd","merchant_name":"Shop","merchant_url":"https://example.com","single_use_card":{"number":%q,"cvc":"123","exp_month":"11","exp_year":"2030"}}`, secret)
	out, human, err := executeVaultInputCommand(t, client, "", "vaults", "cards", "create", "checkout", "single-use-order", "--provider", "kernel", "--spec", inline)
	require.ErrorContains(t, err, "--spec-file single_use_card")
	assert.False(t, strings.Contains(out+human+err.Error(), secret))

	misplaced := fmt.Sprintf(`{"wallet":"wallet-1","amount":100,"currency":"usd","merchant_name":"Shop","merchant_url":"https://example.com","number":%q,"single_use_card":{"number":%q,"cvc":"123","exp_month":"11","exp_year":"2030"}}`, secret, secret)
	out, human, err = executeVaultInputCommand(t, client, misplaced, "vaults", "cards", "create", "checkout", "single-use-order", "--provider", "kernel", "--spec-file", "-")
	require.ErrorContains(t, err, "--spec-file single_use_card")
	assert.False(t, strings.Contains(out+human+err.Error(), secret))
}
