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

const shopPayWalletFixture = `{"id":"wallet-id","key":"shopper","type":"wallet","spec":{"provider":"shop_pay","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}},"state":{"provider":"shop_pay","status":"pending_authorization"},"action":{"name":"shop_pay_oauth","url":"https://shop.example/connect"},"available_operations":[],"available_expansions":[{"type":"payment_methods","description":"List saved Shop Pay cards."}]}`

const shopPayCardFixture = `{"id":"card-id","key":"order-1","type":"card","spec":{"provider":"shop_pay","wallet":"shopper","amount":1234,"currency":"USD","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US","context":"Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose.","totals":[{"type":"total","amount":1234}]},"state":{"provider":"shop_pay","status":"requested"},"available_operations":[{"type":"authorize","description":"Authorize this Shop Pay purchase."}],"available_expansions":[]}`

func TestVaultShopPayWalletCreate(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/vaults/checkout/items/shopper", r.URL.Path)
		var body struct {
			Type string          `json:"type"`
			Spec json.RawMessage `json:"spec"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "wallet", body.Type)
		assert.JSONEq(t, `{"provider":"shop_pay","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`, string(body.Spec))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, shopPayWalletFixture)
	})

	out, _, err := executeVaultCommand(t, client, "vaults", "wallets", "create", "checkout", "shopper",
		"--provider", "shop_pay", "--spec", `{"authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Contains(t, out, `"provider": "shop_pay"`)
	assert.Contains(t, out, `"name": "shop_pay_oauth"`)
}

func TestVaultShopPayWalletValidation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("request should not be sent") })
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing authorization", []string{"--spec", `{}`}, "spec.authorization is required"},
		{"wrong client", []string{"--spec", `{"authorization":{"method":"oauth","client":{"type":"customer_managed"}}}`}, "client.type kernel_managed"},
		{"provider config", []string{"--spec", `{"authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`, "--provider-config-id", "config-1"}, "do not accept provider configurations or tokens"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"vaults", "wallets", "create", "checkout", "shopper", "--provider", "shop_pay"}
			args = append(args, tt.args...)
			_, _, err := executeVaultCommand(t, client, args...)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestVaultShopPayCardCreateAndUpdate(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	spec := `{"wallet":"shopper","amount":1234,"currency":"USD","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US","context":"Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose.","line_items":[{"name":"Item","quantity":1,"unit_amount":1234}],"totals":[{"type":"total","amount":1234}]}`
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if operation == "create" {
					assert.Equal(t, http.MethodPut, r.Method)
				} else {
					assert.Equal(t, http.MethodPatch, r.Method)
				}
				var body struct {
					Type string                     `json:"type"`
					Spec map[string]json.RawMessage `json:"spec"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "card", body.Type)
				assert.JSONEq(t, `"shop_pay"`, string(body.Spec["provider"]))
				assert.JSONEq(t, `[{"type":"total","amount":1234}]`, string(body.Spec["totals"]))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, shopPayCardFixture)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "cards", operation, "checkout", "order-1", "--provider", "shop_pay", "--spec", spec, "-o", "json")
			require.NoError(t, err)
		})
	}
}

func TestVaultPaymentRequestRejectsShopPay(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(http.ResponseWriter, *http.Request) { t.Fatal("request should not be sent") })
	_, _, err := executeVaultCommand(t, client, "vaults", "payment-requests", "create", "checkout", "order-1", "--provider", "shop_pay", "--spec", `{}`)
	require.ErrorContains(t, err, "support only link or kernel")
}

func TestVaultShopPayAuthorizeAcceptsCheckoutContext(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			assert.Equal(t, http.MethodGet, r.Method)
			_, _ = io.WriteString(w, shopPayCardFixture)
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"authorize","browser_id":"browser-1","page_url":"https://shop.example/checkout"}`, string(body))
		_, _ = io.WriteString(w, strings.Replace(shopPayCardFixture, `"status":"requested"`, `"status":"pending_authorization"`, 1))
	})

	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize",
		"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}
