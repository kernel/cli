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

func TestVaultRawSpecForwarding(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, path := range []string{"wallets create", "cards create"} {
		for _, provider := range []string{"link", "agentcard"} {
			for _, raw := range []string{
				`{}`,
				`{"custom_option":false,"amount":0,"currency":"USD","metadata":null}`,
				`{"expires_at":9223372036854775807,"line_items":[{"name":"Item","quantity":2,"unit_amount":100,"totals":[{"type":"tax","display_text":"Tax","amount":10}]}],"totals":[],"metadata":{"reference":"order-1"}}`,
			} {
				t.Run(path+"/"+provider+"/"+raw, func(t *testing.T) {
					var expected map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(raw), &expected))
					expected["provider"], _ = json.Marshal(provider)
					calls := 0
					client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
						calls++
						var body struct {
							Type string                     `json:"type"`
							Spec map[string]json.RawMessage `json:"spec"`
						}
						require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
						assert.Equal(t, expected, body.Spec, "preserve exact numbers, false, zero, null, nested fields, and omissions")
						assert.Equal(t, "/vaults/checkout/items/item-1", r.URL.Path)
						assert.Equal(t, http.MethodPut, r.Method)
						assert.Equal(t, strings.TrimSuffix(strings.Fields(path)[0], "s"), body.Type)
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, requestedCardFixture)
					})
					args := append([]string{"vaults"}, strings.Fields(path)...)
					args = append(args, "checkout", "item-1", "--provider", provider, "--spec", raw, "-o", "json")
					_, _, err := executeVaultCommand(t, client, args...)
					require.NoError(t, err)
					assert.Equal(t, 1, calls)
				})
			}
		}
	}
}

func TestVaultPaymentTokenRequestKeepsMerchantBindingServerOwned(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		var body struct {
			Type string                     `json:"type"`
			Spec map[string]json.RawMessage `json:"spec"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "payment_token", body.Type)
		assert.JSONEq(t, `"link"`, string(body.Spec["provider"]))
		assert.NotContains(t, body.Spec, "merchant_account_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, paymentTokenFixture)
	})
	spec := `{"wallet":"wallet-1","browser_id":"browser-1","page_url":"https://shop.example/checkout","payment_method_id":"pm-1","amount":1234,"currency":"usd","context":"Final checkout purchase context."}`
	out, _, err := executeVaultCommand(t, client, "vaults", "payment-tokens", "create", "checkout", "order-token", "--spec", spec, "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"type": "payment_token"`)
	assert.Contains(t, out, `"browser_id": "browser-1"`)
}

func TestVaultPaymentTokenFallbackOnlyForUnsupportedCheckout(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   string
	}{
		{400, "lpt_not_supported", "create a card instead"},
		{409, "browser_unavailable", "correct the browser or page and retry"},
		{409, "conflict", "vault conflict"},
		{500, "provider_error", "outcome may be unresolved"},
	} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, `{"code":"`+tc.code+`","message":"detail"}`)
		})
		spec := `{"wallet":"wallet-1","browser_id":"browser-1","page_url":"https://shop.example/checkout","payment_method_id":"pm-1","amount":1234,"currency":"usd","context":"Final checkout purchase context."}`
		_, _, err := executeVaultCommand(t, client, "vaults", "payment-tokens", "create", "checkout", "order-token", "--spec", spec)
		require.ErrorContains(t, err, tc.want)
		if tc.code != "lpt_not_supported" {
			assert.NotContains(t, err.Error(), "create a card")
		}
		if tc.code == "conflict" {
			assert.NotContains(t, err.Error(), "retry")
		}
	}
}

func TestVaultRawSpecValidationIsLeftToAPI(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Spec map[string]json.RawMessage `json:"spec"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotContains(t, body.Spec, "wallet")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"invalid_request","message":"wallet is required"}`)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "cards", "create", "checkout", "order-1", "--provider", "link", "--spec", "{}")
	require.ErrorContains(t, err, "invalid_request: wallet is required")
}

func TestVaultPaymentTokenHelp(t *testing.T) {
	cmd, _, err := newVaultsCommand().Find([]string{"payment-tokens", "create"})
	require.NoError(t, err)
	for _, text := range []string{"browser_id", "page_url", "lpt_not_supported", "merchant_account_id", "immutable", "does not submit"} {
		assert.Contains(t, cmd.Long, text)
	}
	assert.Nil(t, cmd.Flags().Lookup("provider"))
	assert.NotNil(t, cmd.Flags().Lookup("spec"))
}

func TestVaultSpecHelpAndFlags(t *testing.T) {
	for _, path := range []string{"wallets create", "cards create"} {
		t.Run(path, func(t *testing.T) {
			cmd, _, err := newVaultsCommand().Find(strings.Fields(path))
			require.NoError(t, err)
			require.NotNil(t, cmd.Flags().Lookup("provider"))
			require.NotNil(t, cmd.Flags().Lookup("spec"))
			for _, removed := range []string{"wallet", "amount", "currency", "merchant", "merchant-url", "payment-method-id", "context", "test", "live", "card-id", "user-id"} {
				assert.Nil(t, cmd.Flags().Lookup(removed), removed)
			}
			assert.Contains(t, cmd.Long, "Keep these types in sync with https://api.onkernel.com/spec.yaml")
			assert.Contains(t, cmd.Long, "TypeScript notation")
			assert.Contains(t, cmd.Long, "provider: \"link\"")
			assert.Contains(t, cmd.Long, "provider: \"agentcard\"")
			assert.Contains(t, cmd.Example, "--spec '")
			assert.NotContains(t, cmd.Long, "test: boolean")
			assert.NotContains(t, cmd.Long, "sandbox/live")
			if strings.HasPrefix(path, "cards") {
				for _, field := range []string{"merchant_name:", "merchant:", "line_items?:", "metadata?:", "expires_at?:", "type LinkLineItem", "type LinkTotal"} {
					assert.Contains(t, cmd.Long, field)
				}
			} else {
				assert.Contains(t, cmd.Long, "authorization:")
				assert.Contains(t, cmd.Long, "user_id?:")
			}
		})
	}
}
