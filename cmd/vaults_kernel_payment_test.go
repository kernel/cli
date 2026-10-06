package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const kernelWalletFixture = `{"id":"wallet-id","key":"cardholder","type":"wallet","spec":{"provider":"kernel"},"state":{"provider":"kernel","status":"pending_authorization"},"action":{"name":"card_enrollment","url":"https://vault.example/enroll?state=opaque"},"available_operations":[],"available_expansions":[{"type":"payment_methods","description":"Check token eligibility"}],"pan":"4111111111111111"}`
const kernelCardFixture = `{"id":"card-id","key":"order-1","type":"card","spec":{"provider":"kernel","wallet":"cardholder","amount":1200,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US"},"state":{"provider":"kernel","status":"requested","masks":{"brand":"visa","last4":"1234","token_last4":"5678","pan":"4111111111111111"}},"available_operations":[{"type":"authorize","description":"Authorize this purchase"}],"available_expansions":[]}`

func TestKernelWalletCreateAndGet(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vaults/checkout/items/cardholder", r.URL.Path)
		if r.Method == http.MethodPut {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"wallet","spec":{"provider":"kernel"}}`, string(body))
		} else {
			assert.Equal(t, http.MethodGet, r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, kernelWalletFixture)
	})
	for _, args := range [][]string{
		{"wallets", "create", "checkout", "cardholder", "--provider", "kernel", "--spec", "{}"},
		{"wallets", "get", "checkout", "cardholder", "--wait", "0"},
	} {
		out, human, err := executeVaultCommand(t, client, append([]string{"vaults"}, args...)...)
		require.NoError(t, err)
		assert.Contains(t, human, "https://vault.example/enroll?state=opaque")
		assert.Contains(t, human, "card_enrollment")
		assert.NotContains(t, out+human, "4111111111111111")
		jsonArgs := append(append([]string{"vaults"}, args...), "-o", "json")
		out, _, err = executeVaultCommand(t, client, jsonArgs...)
		require.NoError(t, err)
		assert.Contains(t, out, `"card_enrollment"`)
		assert.NotContains(t, out, "4111111111111111")
	}
}

func TestKernelWalletRejectsExternalCredentials(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid wallet reached API") })
	for _, flag := range [][]string{{"--tokens-file", "-"}, {"--provider-config-id", "config-1"}, {"--provider-config-name", "config-1"}} {
		args := append([]string{"vaults", "wallets", "create", "checkout", "cardholder", "--provider", "kernel", "--spec", "{}"}, flag...)
		_, _, err := executeVaultCommand(t, client, args...)
		require.ErrorContains(t, err, "Kernel wallets do not use provider configurations or tokens")
	}
}

func TestKernelSpecRejectsCardDetails(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("card details reached API") })
	for _, spec := range []string{
		`{"pan":"4111111111111111"}`,
		`{"metadata":{"card_number":"4111111111111111"}}`,
		`{"cards":[{"cvc":"123"}]}`,
	} {
		for _, path := range [][]string{{"wallets", "create"}, {"cards", "create"}} {
			args := append([]string{"vaults"}, path...)
			args = append(args, "checkout", "item-1", "--provider", "kernel", "--spec", spec)
			_, _, err := executeVaultCommand(t, client, args...)
			require.ErrorContains(t, err, "card details")
			assert.NotContains(t, err.Error(), "4111111111111111")
		}
	}
}

func TestKernelPaymentMethodsEligibility(t *testing.T) {
	wallet := strings.TrimSuffix(kernelWalletFixture, "}") + `,"expanded":{"payment_methods":[{"id":"card-1","provider":"kernel","type":"card","display":{"brand":"visa","last4":"1234","pan":"4111111111111111"},"capabilities":{"single_use_card":{"eligible":false,"reasons":["network_token_pending"]}}}]}}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "payment_methods", r.URL.Query().Get("expand"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, wallet)
	})
	out, human, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder")
	require.NoError(t, err)
	assert.Contains(t, human, "network_token_pending")
	assert.Contains(t, human, "false")
	assert.NotContains(t, human+out, "4111111111111111")
	out, _, err = executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"network_token_pending"`)
	assert.NotContains(t, out, "4111111111111111")
}

func TestKernelCardCreateAuthorizeAndApproval(t *testing.T) {
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		response := kernelCardFixture
		switch calls {
		case 1:
			assert.Equal(t, "/vaults/checkout/items/order-1", r.URL.Path)
			assert.Equal(t, http.MethodPut, r.Method)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"card","spec":{"provider":"kernel","wallet":"cardholder","amount":1200,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US"}}`, string(body))
		case 2:
			assert.Equal(t, "/vaults/checkout/items/order-1", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
		case 3:
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/vaults/checkout/items/order-1/operations", r.URL.Path)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"authorize"}`, string(body))
			response = strings.TrimSuffix(kernelCardFixture, "}") + `,"action":{"name":"spend_approval","url":"https://vault.example/approve?state=opaque"}}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	})
	spec := `{"wallet":"cardholder","amount":1200,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US"}`
	out, human, err := executeVaultCommand(t, client, "vaults", "cards", "create", "checkout", "order-1", "--provider", "kernel", "--spec", spec, "-o", "json")
	require.NoError(t, err)
	var created map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &created))
	assert.Equal(t, "US", created["spec"].(map[string]any)["merchant_country"])
	assert.Contains(t, out, `"token_last4": "5678"`)
	assert.NotContains(t, out+human, "4111111111111111")
	assert.Equal(t, 1, calls, "creation must not authorize")
	out, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"spend_approval"`)
	assert.Contains(t, out, "https://vault.example/approve?state=opaque")
	assert.NotContains(t, out, "4111111111111111")
	assert.Equal(t, 3, calls)
}

func TestKernelReadyCardUsesAdvertisedFill(t *testing.T) {
	ready := strings.Replace(kernelCardFixture, `"status":"requested"`, `"status":"ready"`, 1)
	ready = strings.Replace(ready, `"type":"authorize","description":"Authorize this purchase"`, `"type":"fill","description":"Fill checkout fields"`, 1)
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, ready)
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"fill","browser_id":"browser-1","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#card-number"}]}`, string(body))
		_, _ = io.WriteString(w, `{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled"}]}`)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", `{"browser_id":"browser-1","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#card-number"}]}`, "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"completed"`)
	assert.Equal(t, 2, calls)
}

func TestKernelCardUpdateRejectedLocally(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("Kernel update reached API") })
	_, _, err := executeVaultCommand(t, client, "vaults", "cards", "update", "checkout", "order-1", "--provider", "kernel", "--spec", `{}`)
	require.ErrorContains(t, err, "Kernel card updates are not supported")
}
