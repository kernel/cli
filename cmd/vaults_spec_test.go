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
	for _, path := range []string{"wallets create", "cards create", "cards update"} {
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
						if path == "cards update" {
							assert.Equal(t, http.MethodPatch, r.Method)
							assert.Empty(t, body.Type)
						} else {
							assert.Equal(t, http.MethodPut, r.Method)
							assert.Equal(t, strings.TrimSuffix(strings.Fields(path)[0], "s"), body.Type)
						}
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

func TestVaultSpecHelpAndFlags(t *testing.T) {
	for _, path := range []string{"wallets create", "cards create", "cards update"} {
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
