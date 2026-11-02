package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const kernelWalletPaymentOperationsFixture = `{
  "id":"wallet-id","key":"cardholder","type":"wallet",
  "spec":{"provider":"kernel"},
  "state":{"provider":"kernel","status":"connected"},
  "available_operations":[
    {"type":"add_payment_method","description":"Enroll another card"},
    {"type":"remove_payment_method","description":"Remove one enrolled card"}
  ],
  "available_expansions":[{"type":"payment_methods","description":"List cards"}]
}`

func TestVaultAddPaymentMethodOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	posts := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, kernelWalletPaymentOperationsFixture)
			return
		}
		posts++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"add_payment_method"}`, string(body))
		_, _ = io.WriteString(w, `{
          "type":"add_payment_method",
          "payment_method":{
            "id":"card-2","provider":"kernel","type":"card","is_default":false,
            "status":"pending_enrollment","status_reason":"awaiting_cardholder",
            "expires_at":"2026-09-01T12:30:00Z",
            "display":{},"capabilities":{},
            "action":{"name":"card_enrollment","url":"https://vault.example/enroll?state=opaque","capture_token":"secret-capture"},
            "provider_secret":"secret-provider"
          }
        }`)
	})
	out, text, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "cardholder", "add_payment_method", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	for _, want := range []string{`"type": "add_payment_method"`, `"status": "pending_enrollment"`, `"card_enrollment"`, "https://vault.example/enroll?state=opaque"} {
		assert.Contains(t, out, want)
	}
	assert.Empty(t, text)
	assert.NotContains(t, out, "secret-capture")
	assert.NotContains(t, out, "secret-provider")

	_, text, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "cardholder", "add_payment_method")
	require.NoError(t, err)
	for _, want := range []string{"Started payment method enrollment", "card-2", "pending_enrollment", "card_enrollment url", "single-use", "expires after 30 minutes", "separate link"} {
		assert.Contains(t, text, want)
	}
	assert.NotContains(t, text, "secret-")
	assert.Equal(t, 2, posts)
}

func TestVaultRemovePaymentMethodOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	posts := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, kernelWalletPaymentOperationsFixture)
			return
		}
		posts++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"remove_payment_method","payment_method_id":"card-2"}`, string(body))
		_, _ = io.WriteString(w, `{"type":"remove_payment_method","status":"removed","payment_method_id":"card-2","provider_secret":"secret-provider"}`)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "cardholder", "remove_payment_method", "--params", `{"payment_method_id":"card-2"}`, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	assert.Contains(t, out, `"status": "removed"`)
	assert.Contains(t, out, `"payment_method_id": "card-2"`)
	assert.NotContains(t, out, "secret-provider")
}

func TestVaultPaymentMethodOperationValidation(t *testing.T) {
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	for _, tc := range []struct {
		operation string
		params    string
		want      string
	}{
		{"add_payment_method", `{}`, "takes no parameters"},
		{"remove_payment_method", "", "requires --params"},
		{"remove_payment_method", `{}`, "payment_method_id"},
		{"remove_payment_method", `{"payment_method_id":""}`, "payment_method_id"},
		{"remove_payment_method", `{"payment_method_id":"card-1","other":true}`, "only supported"},
	} {
		args := []string{"vaults", "items", "invoke", "checkout", "cardholder", tc.operation}
		if tc.params != "" {
			args = append(args, "--params", tc.params)
		}
		_, _, err := executeVaultCommand(t, client, args...)
		require.ErrorContains(t, err, tc.want)
	}
	assert.Zero(t, calls)
}

func TestVaultPaymentMethodsExposeLifecycleWithoutCaptureToken(t *testing.T) {
	fixture := strings.TrimSuffix(kernelWalletPaymentOperationsFixture, "}") + `,
      "expanded":{"payment_methods":[{
        "id":"card-2","provider":"kernel","type":"card","is_default":false,
        "status":"pending_enrollment","status_reason":"awaiting_cardholder",
        "expires_at":"2026-09-01T12:30:00Z",
        "display":{"brand":"visa","last4":"1234"},"capabilities":{},
        "action":{"name":"card_enrollment","url":"https://vault.example/enroll?state=opaque","capture_token":"secret-capture"}
      }]}}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "payment_methods", r.URL.Query().Get("expand"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder", "-o", "json")
	require.NoError(t, err)
	for _, want := range []string{`"status": "pending_enrollment"`, `"status_reason": "awaiting_cardholder"`, `"expires_at"`, `"name": "card_enrollment"`, "https://vault.example/enroll?state=opaque"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "secret-capture")

	_, text, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder")
	require.NoError(t, err)
	for _, want := range []string{"pending_enrollment", "awaiting_cardholder", "card_enrollment url", "payment_method_id explicitly", "removal_pending"} {
		assert.Contains(t, text, want)
	}
	assert.NotContains(t, text, "secret-capture")
}

func TestVaultPaymentMethodOperationHelp(t *testing.T) {
	invoke, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	flat := strings.Join(strings.Fields(invoke.Long), " ")
	for _, want := range []string{"add_payment_method takes no parameters", "remove_payment_method requires", "payment_method_id", "expires after 30 minutes", "separate pending enrollment", "without disconnecting the wallet", "removal_pending", "not proof that a payment did not occur", "vaults:write"} {
		assert.Contains(t, flat, want)
	}
	assert.Contains(t, invoke.Example, "add_payment_method")
	assert.Contains(t, invoke.Example, "remove_payment_method")
	assert.Contains(t, invoke.Flags().Lookup("params").Usage, "remove_payment_method")
}
