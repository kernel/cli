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
		require.ErrorContains(t, err, "kernel wallets do not use provider configurations or tokens")
	}
}

func TestKernelSpecRejectsCardDetails(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("card details reached API") })
	for _, spec := range []string{
		`{"pan":"4111111111111111"}`,
		`{"metadata":{"card_number":"4111111111111111"}}`,
		`{"cards":[{"cvc":"123"}]}`,
		`{"expiry":"12/30"}`,
		`{"exp_month":"12","exp_year":"2030"}`,
		`{"expiration_month":"12","expiration_year":"2030"}`,
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
		assert.JSONEq(t, `{"type":"fill","browser_id":"browser-1","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#cardNumber"},{"field":"expiration","format":"MM/YY","selector":"#cardExpiry"},{"field":"cvc","selector":"#cardCvc"}]}`, string(body))
		_, _ = io.WriteString(w, `{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled"},{"index":1,"status":"filled"},{"index":2,"status":"filled"}]}`)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", `{"browser_id":"browser-1","page_url":"https://shop.example/checkout","fields":[{"field":"number","selector":"#cardNumber"},{"field":"expiration","format":"MM/YY","selector":"#cardExpiry"},{"field":"cvc","selector":"#cardCvc"}]}`, "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"completed"`)
	assert.Equal(t, 2, calls)
}

func TestKernelCardGuidance(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, kernelCardFixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1")
	require.NoError(t, err)
	for _, want := range []string{"report one observed merchant outcome only when confirm_transaction is advertised", "unknown means no reliable merchant outcome", "no stored billing fields", "billing_* returns field_unavailable", "enter billing details on the merchant page"} {
		assert.Contains(t, human, want)
	}
	assert.NotContains(t, human, "does not observe merchant charges")
	assert.NotContains(t, human, "never replace an item in recovery_required")
}

func TestKernelCardUpdateRejectedLocally(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("Kernel update reached API") })
	_, _, err := executeVaultCommand(t, client, "vaults", "cards", "update", "checkout", "order-1", "--provider", "kernel", "--spec", `{}`)
	require.ErrorContains(t, err, "kernel card updates are not supported")
}

const kernelMethodsWalletFixture = `{"id":"wallet-id","key":"cardholder","type":"wallet","spec":{"provider":"kernel"},"state":{"provider":"kernel","status":"connected"},"available_operations":[{"type":"add_payment_method","description":"Start hosted enrollment of a card in this wallet."},{"type":"remove_payment_method","description":"Remove one card from this wallet."}],"available_expansions":[{"type":"payment_methods","description":"List cards"}]}`
const kernelAddedMethodFixture = `{"type":"add_payment_method","payment_method":{"id":"pm-2","provider":"kernel","type":"card","is_default":false,"status":"pending_enrollment","display":{},"capabilities":{"single_use_card":{"eligible":false,"reasons":["enrollment_pending"]}},"action":{"name":"card_enrollment","url":"https://vault.example/card#token=pm-2.opaque","capture_token":"CAPTURE_SECRET"},"expires_at":"2026-10-28T12:30:00Z"}}`

func TestKernelAddPaymentMethod(t *testing.T) {
	for _, args := range [][]string{
		{"items", "invoke", "checkout", "cardholder", "add_payment_method"},
		{"wallets", "add-payment-method", "checkout", "cardholder"},
	} {
		calls := 0
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			response := kernelMethodsWalletFixture
			if r.Method == http.MethodPost {
				assert.Equal(t, "/vaults/checkout/items/cardholder/operations", r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"type":"add_payment_method"}`, string(body))
				response = kernelAddedMethodFixture
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, response)
		})
		out, human, err := executeVaultCommand(t, client, append([]string{"vaults"}, args...)...)
		require.NoError(t, err)
		assert.Equal(t, 2, calls)
		for _, text := range []string{"pm-2", "pending_enrollment", "https://vault.example/card#token=pm-2.opaque", "never open it for them", "30 minutes"} {
			assert.Contains(t, human, text)
		}
		assert.NotContains(t, out+human, "CAPTURE_SECRET")
		out, _, err = executeVaultCommand(t, client, append(append([]string{"vaults"}, args...), "-o", "json")...)
		require.NoError(t, err)
		assert.Contains(t, out, `"status": "pending_enrollment"`)
		assert.Contains(t, out, `"expires_at": "2026-10-28T12:30:00Z"`)
		assert.Contains(t, out, `"url": "https://vault.example/card#token=pm-2.opaque"`)
		assert.NotContains(t, out, "capture_token")
		assert.NotContains(t, out, "CAPTURE_SECRET")
	}
}

func TestKernelRemovePaymentMethod(t *testing.T) {
	for _, args := range [][]string{
		{"items", "invoke", "checkout", "cardholder", "remove_payment_method", "--params", `{"payment_method_id":"pm-2"}`},
		{"wallets", "remove-payment-method", "checkout", "cardholder", "--payment-method-id", "pm-2", "--yes"},
	} {
		posts := 0
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			response := kernelMethodsWalletFixture
			if r.Method == http.MethodPost {
				posts++
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"type":"remove_payment_method","payment_method_id":"pm-2"}`, string(body))
				response = `{"type":"remove_payment_method","payment_method_id":"pm-2","status":"removed"}`
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, response)
		})
		_, human, err := executeVaultCommand(t, client, append([]string{"vaults"}, args...)...)
		require.NoError(t, err)
		assert.Equal(t, 1, posts)
		assert.Contains(t, human, "removed payment method pm-2")
		assert.Contains(t, human, "not proof that a payment did not occur")
		out, _, err := executeVaultCommand(t, client, append(append([]string{"vaults"}, args...), "-o", "json")...)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"remove_payment_method","payment_method_id":"pm-2","status":"removed"}`, out)
	}
}

func TestKernelPaymentMethodOperationsRequireAdvertisement(t *testing.T) {
	for _, operation := range []string{"add_payment_method", "remove_payment_method"} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, strings.Replace(kernelWalletFixture, `"pending_authorization"`, `"connected"`, 1))
		})
		args := []string{"vaults", "items", "invoke", "checkout", "cardholder", operation}
		if operation == "remove_payment_method" {
			args = append(args, "--params", `{"payment_method_id":"card"}`)
		}
		_, _, err := executeVaultCommand(t, client, args...)
		require.ErrorContains(t, err, "is not advertised")
		assert.Contains(t, err.Error(), "vaults:write")
		assert.Contains(t, err.Error(), "payment methods are enabled")
	}
}

func TestKernelPaymentMethodOperationErrors(t *testing.T) {
	for _, tc := range []struct {
		operation, code, want string
		status                int
	}{
		{"remove_payment_method", "purchase_unresolved", "reconcile the unresolved purchase", 409},
		{"remove_payment_method", "provider_revocation_pending", "stays removal_pending", 409},
		{"remove_payment_method", "provider_rate_limited", "Retry-After (30)", 429},
		{"remove_payment_method", "insufficient_scope", "vaults:write in addition to vaults:use", 403},
		{"add_payment_method", "insufficient_scope", "vaults:write in addition to vaults:use", 403},
		{"add_payment_method", "conflict", "at most 10 payment methods and 3 open enrollment links", 409},
	} {
		t.Run(tc.operation+" "+tc.code, func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, kernelMethodsWalletFixture)
					return
				}
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"code":"`+tc.code+`","message":"refused"}`)
			})
			args := []string{"vaults", "items", "invoke", "checkout", "cardholder", tc.operation}
			if tc.operation == "remove_payment_method" {
				args = append(args, "--params", `{"payment_method_id":"pm-2"}`)
			}
			_, _, err := executeVaultCommand(t, client, args...)
			require.ErrorContains(t, err, tc.want)
			assert.Contains(t, err.Error(), "refused")
		})
	}
}

func TestKernelPaymentMethodStatuses(t *testing.T) {
	wallet := strings.TrimSuffix(kernelMethodsWalletFixture, "}") + `,"expanded":{"payment_methods":[
		{"id":"card","provider":"kernel","type":"card","is_default":true,"status":"active","display":{"brand":"visa","last4":"1234"},"capabilities":{"single_use_card":{"eligible":true,"reasons":[]}}},
		{"id":"pm-2","provider":"kernel","type":"card","is_default":false,"status":"pending_enrollment","display":{},"capabilities":{"single_use_card":{"eligible":false,"reasons":["enrollment_pending"]}},"action":{"name":"card_enrollment","url":"https://vault.example/card#token=pm-2.opaque","capture_token":"CAPTURE_SECRET"},"expires_at":"2026-10-28T12:30:00Z"},
		{"id":"pm-3","provider":"kernel","type":"card","is_default":false,"status":"recovery_required","status_reason":"provider outcome unknown; remove this payment method","display":{},"capabilities":{"single_use_card":{"eligible":false,"reasons":[]}}}
	]}}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "payment_methods", r.URL.Query().Get("expand"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, wallet)
	})
	out, human, err := executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder")
	require.NoError(t, err)
	for _, text := range []string{"status", "active", "pending_enrollment", "recovery_required", "pm-3 status reason: provider outcome unknown", "card enrollment url for pm-2", "https://vault.example/card#token=pm-2.opaque", "payment_request items require payment_method_id", "--params '<json>'"} {
		assert.Contains(t, human, text)
	}
	assert.NotContains(t, out+human, "CAPTURE_SECRET")
	out, _, err = executeVaultCommand(t, client, "vaults", "wallets", "payment-methods", "checkout", "cardholder", "-o", "json")
	require.NoError(t, err)
	for _, text := range []string{`"status": "recovery_required"`, `"status_reason": "provider outcome unknown; remove this payment method"`, `"expires_at": "2026-10-28T12:30:00Z"`, `"name": "card_enrollment"`} {
		assert.Contains(t, out, text)
	}
	assert.NotContains(t, out, "CAPTURE_SECRET")
}

func TestKernelPaymentRequestCreate(t *testing.T) {
	spec := `{"wallet":"cardholder","payment_method_id":"pm-2","amount":1200,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US"}`
	fixture := strings.Replace(strings.Replace(kernelCardFixture, `"type":"card"`, `"type":"payment_request"`, 1), `"status":"requested"`, `"status":"requested","payment_method_id":"pm-2","funding_status":"removed"`, 1)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"payment_request","spec":{"provider":"kernel",`+spec[1:]+`}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})
	args := []string{"vaults", "cards", "create", "checkout", "order-1", "--type", "payment_request", "--provider", "kernel", "--spec", spec}
	_, human, err := executeVaultCommand(t, client, args...)
	require.NoError(t, err)
	for _, text := range []string{"payment_request", "payment method id", "pm-2", "funding status", "funding_status removed", "Example Shop"} {
		assert.Contains(t, human, text)
	}
	assert.NotContains(t, human, "a connected kernel wallet holds")
	out, _, err := executeVaultCommand(t, client, append(args, "-o", "json")...)
	require.NoError(t, err)
	assert.Contains(t, out, `"funding_status": "removed"`)
	assert.Contains(t, out, `"payment_method_id": "pm-2"`)
}

func TestKernelPaymentRequestFlagValidation(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid request reached API") })
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"cards", "create", "checkout", "order-1", "--type", "purchase", "--provider", "link", "--spec", "{}"}, "--type must be card or payment_request"},
		{[]string{"cards", "create", "checkout", "order-1", "--type", "payment_request", "--provider", "agentcard", "--spec", "{}"}, "AgentCard reusable cards remain card items"},
		{[]string{"cards", "update", "checkout", "order-1", "--type", "payment_request", "--provider", "kernel", "--spec", "{}"}, "kernel card updates are not supported"},
		{[]string{"items", "invoke", "checkout", "cardholder", "remove_payment_method"}, "requires --params or --spec-file with payment_method_id"},
		{[]string{"items", "invoke", "checkout", "cardholder", "remove_payment_method", "--params", `{"payment_method_id":""}`}, "payment_method_id must be a non-empty id"},
		{[]string{"items", "invoke", "checkout", "cardholder", "remove_payment_method", "--params", `{"payment_method_id":"pm-2","force":true}`}, "only supported, non-duplicate properties"},
		{[]string{"items", "invoke", "checkout", "cardholder", "add_payment_method", "--params", `{}`}, "add_payment_method take no parameters"},
		{[]string{"wallets", "remove-payment-method", "checkout", "cardholder", "--payment-method-id", " ", "--yes"}, "payment_method_id must be a non-empty id"},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults"}, tc.args...)...)
		require.ErrorContains(t, err, tc.want)
	}
}
