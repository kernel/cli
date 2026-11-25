package cmd

import (
	"context"
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

const linkBalanceSetupFixture = `{
  "id":"wallet-id","key":"link-wallet","type":"wallet",
  "spec":{"provider":"link"},
  "state":{"provider":"link","status":"connected"},
  "available_operations":[
    {"type":"link_balance_setup","description":"Set up or change Link balance"}
  ],
  "available_expansions":[{"type":"payment_methods","description":"List payment methods and balance"}]
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
		expected := `{"type":"add_payment_method"}`
		if posts == 2 {
			expected = `{"type":"add_payment_method","enrollment":"hosted"}`
		}
		assert.JSONEq(t, expected, string(body))
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

	_, text, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "cardholder", "add_payment_method", "--enrollment", "hosted")
	require.NoError(t, err)
	for _, want := range []string{"Started payment method enrollment", "card-2", "pending_enrollment", "card_enrollment url", "single-use", "expires after 30 minutes", "separate link"} {
		assert.Contains(t, text, want)
	}
	assert.NotContains(t, text, "secret-")
	assert.Equal(t, 2, posts)
}

func TestVaultAddPaymentMethodServerEnrollment(t *testing.T) {
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
		assert.JSONEq(t, `{"type":"add_payment_method","enrollment":"server"}`, string(body))
		_, _ = io.WriteString(w, `{
          "type":"add_payment_method",
          "payment_method":{
            "id":"card-3","provider":"kernel","type":"card","is_default":false,
            "status":"pending_enrollment","status_reason":"awaiting_issuer",
            "expires_at":"2026-09-01T12:05:00Z",
            "display":{},"capabilities":{},
            "action":{"name":"card_ingest","url":"https://cards.example.com/vault/card-ingest/card-3"}
          }
        }`)
	})
	out, text, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "cardholder", "add_payment_method", "--enrollment", "server", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	assert.Empty(t, text)
	assert.Contains(t, out, `"name": "card_ingest"`)
	assert.Contains(t, out, "https://cards.example.com/vault/card-ingest/card-3")

	_, text, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "cardholder", "add_payment_method", "--enrollment", "server")
	require.NoError(t, err)
	assert.Equal(t, 2, posts)
	for _, want := range []string{"card_ingest url", "issuer", "backend", "5 minutes", "never open", "separate url"} {
		assert.Contains(t, strings.ToLower(text), want)
	}
	assert.NotContains(t, strings.ToLower(text), "share the single-use card_enrollment")
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

func TestVaultLinkBalanceSetupOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	posts := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, linkBalanceSetupFixture)
			return
		}
		posts++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		if posts == 1 {
			assert.JSONEq(t, `{"type":"link_balance_setup","desired_balance_amount":10000,"desired_transaction_limit":5000}`, string(body))
		} else {
			assert.JSONEq(t, `{"type":"link_balance_setup"}`, string(body))
		}
		_, _ = io.WriteString(w, `{"type":"link_balance_setup","url":"https://link.example/balance/setup?code=setup-token","provider_secret":"secret-provider"}`)
	})

	out, text, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "link-wallet", "link_balance_setup", "--params", `{"desired_balance_amount":10000,"desired_transaction_limit":5000}`, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	assert.Empty(t, text)
	assert.Contains(t, out, `"type": "link_balance_setup"`)
	assert.Contains(t, out, `"url": "https://link.example/balance/setup?code=setup-token"`)
	assert.NotContains(t, out, "secret-provider")

	_, text, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "link-wallet", "link_balance_setup")
	require.NoError(t, err)
	assert.Equal(t, 2, posts)
	for _, want := range []string{"Created Link balance setup page", "does not expire", "only the Link user", "wallets payment-methods", "live available amount"} {
		assert.Contains(t, text, want)
	}

	opened := ""
	params, err := parseVaultOperationParams("link_balance_setup", "", false, true)
	require.NoError(t, err)
	err = (VaultsCmd{
		vaults:  &client.Vaults,
		openURL: func(address string) error { opened = address; return nil },
	}).Invoke(context.Background(), "checkout", "link-wallet", "link_balance_setup", params, "", true)
	require.NoError(t, err)
	assert.Equal(t, 3, posts)
	assert.Equal(t, "https://link.example/balance/setup?code=setup-token", opened)
}

func TestVaultLinkBalanceSetupValidation(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{`{"desired_balance_amount":99}`, "at least 100"},
		{`{"desired_balance_amount":150}`, "whole-dollar"},
		{`{"desired_transaction_limit":50100}`, "100 to 50000"},
		{`{"desired_transaction_limit":150}`, "whole-dollar"},
		{`{"other":true}`, "only supported"},
	} {
		_, err := parseVaultOperationParams("link_balance_setup", tc.raw, true, false)
		require.ErrorContains(t, err, tc.want)
	}
	params, err := parseVaultOperationParams("link_balance_setup", `{"desired_balance_amount":25000}`, true, false)
	require.NoError(t, err)
	require.NotNil(t, params.LinkBalanceSetup)
	assert.Equal(t, int64(25000), params.LinkBalanceSetup.OfLinkBalanceSetup.DesiredBalanceAmount.Value)
}

func TestVaultPaymentMethodOperationValidation(t *testing.T) {
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	for _, tc := range []struct {
		operation string
		params    string
		want      string
	}{
		{"add_payment_method", `{}`, "does not accept --params"},
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
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"vaults", "items", "invoke", "checkout", "cardholder", "add_payment_method", "--enrollment", "invalid"}, "--enrollment must be hosted or server"},
		{[]string{"vaults", "items", "invoke", "checkout", "cardholder", "remove_payment_method", "--enrollment", "server", "--params", `{"payment_method_id":"card-1"}`}, "only supported for add_payment_method"},
	} {
		_, _, err := executeVaultCommand(t, client, tc.args...)
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
      },{
        "id":"card-3","provider":"kernel","type":"card","is_default":false,
        "status":"active","status_reason":"","issuer_reference":"issuer-ref-3",
        "valid_until":"2026-09-02T12:00:00Z",
        "display":{"brand":"visa","last4":"5678"},"capabilities":{"single_use_card":{"eligible":true,"reasons":[]}}
      }]}}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "payment_methods", r.URL.Query().Get("expand"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder", "-o", "json")
	require.NoError(t, err)
	for _, want := range []string{`"status": "pending_enrollment"`, `"status_reason": "awaiting_cardholder"`, `"expires_at"`, `"name": "card_enrollment"`, "https://vault.example/enroll?state=opaque", `"issuer_reference": "issuer-ref-3"`, `"valid_until": "2026-09-02T12:00:00Z"`} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "secret-capture")

	_, text, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder")
	require.NoError(t, err)
	for _, want := range []string{"pending_enrollment", "awaiting_cardholder", "card_enrollment url", "issuer-ref-3", "valid until", "payment_method_id explicitly", "removal_pending", "card_ingest"} {
		assert.Contains(t, text, want)
	}
	assert.NotContains(t, text, "secret-capture")
}

func TestVaultPaymentMethodsExposeLiveBalance(t *testing.T) {
	fixture := strings.TrimSuffix(linkBalanceSetupFixture, "}") + `,
      "expanded":{"payment_methods":[{
        "id":"balance-1","provider":"link","type":"balance","is_default":true,
        "status":"active","display":{"label":"Link balance"},"capabilities":{},
        "balance":{"available":{"amount":12345,"currency":"usd"}},
        "provider_secret":"secret-provider"
      }]}}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "payment_methods", r.URL.Query().Get("expand"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})

	out, _, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "link-wallet", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"balance"`)
	assert.Contains(t, out, `"amount": 12345`)
	assert.Contains(t, out, `"currency": "usd"`)
	assert.NotContains(t, out, "secret-provider")

	_, text, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "link-wallet")
	require.NoError(t, err)
	for _, want := range []string{"available balance (minor units)", "balance currency", "12345", "usd"} {
		assert.Contains(t, text, want)
	}
}

func TestVaultPaymentMethodOperationHelp(t *testing.T) {
	invoke, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	flat := strings.Join(strings.Fields(invoke.Long), " ")
	for _, want := range []string{"add_payment_method is valid", "--enrollment hosted", "--enrollment server", "card_ingest", "5 minutes", "remove_payment_method requires", "payment_method_id", "separate pending enrollment", "without disconnecting the wallet", "removal_pending", "not proof that a payment did not occur", "vaults:write", "link_balance_setup is valid", "eligible_for_balance", "desired_balance_amount", "target balance", "desired_transaction_limit", "always shows that step", "$150", "50000 cents", "does not expire", "wallets payment-methods"} {
		assert.Contains(t, flat, want)
	}
	assert.Contains(t, invoke.Example, "add_payment_method --enrollment server")
	assert.Contains(t, invoke.Example, "remove_payment_method")
	assert.Contains(t, invoke.Example, "link_balance_setup")
	assert.Contains(t, invoke.Flags().Lookup("params").Usage, "remove_payment_method")
	assert.Contains(t, invoke.Flags().Lookup("params").Usage, "link_balance_setup")
	assert.Contains(t, invoke.Flags().Lookup("enrollment").Usage, "hosted")
	assert.Contains(t, invoke.Flags().Lookup("enrollment").Usage, "server")

	methods, _, err := newVaultsCommand().Find([]string{"wallets", "payment-methods"})
	require.NoError(t, err)
	assert.Contains(t, methods.Long, "lowercase ISO 4217")
}
