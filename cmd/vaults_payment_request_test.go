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

const linkPaymentRequestFixture = `{
  "id":"payment-request-id","key":"order-1","type":"payment_request",
  "spec":{"provider":"link","wallet":"wallet-1","payment_method_id":"pm-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","context":"Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose."},
  "state":{"provider":"link","status":"requested"},
  "available_operations":[{"type":"authorize","description":"Authorize this purchase"}],
  "available_expansions":[]
}`

const kernelPaymentRequestFixture = `{
  "id":"payment-request-id","key":"order-1","type":"payment_request",
  "spec":{"provider":"kernel","wallet":"cardholder","payment_method_id":"card-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US"},
  "state":{"provider":"kernel","status":"requested","funding_status":"available","payment_method_id":"card-1","masks":{"brand":"visa","last4":"1234","token_last4":"5678"}},
  "available_operations":[{"type":"authorize","description":"Authorize this purchase"}],
  "available_expansions":[]
}`

func TestVaultPaymentRequestCreateAndUpdate(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, operation := range []string{"create", "update"} {
		for _, provider := range []string{"link", "kernel"} {
			t.Run(operation+"/"+provider, func(t *testing.T) {
				fixture := linkPaymentRequestFixture
				spec := `{"wallet":"wallet-1","payment_method_id":"pm-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","context":"Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose. Purchase purpose.","metadata":{"reference":"order-1"}}`
				if provider == "kernel" {
					fixture = kernelPaymentRequestFixture
					spec = `{"wallet":"cardholder","payment_method_id":"card-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US"}`
				}
				calls := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					expectedMethod := http.MethodPut
					if operation == "update" {
						expectedMethod = http.MethodPatch
					}
					assert.Equal(t, expectedMethod, r.Method)
					assert.Equal(t, "/vaults/checkout/items/order-1", r.URL.Path)
					var body struct {
						Type string                     `json:"type"`
						Spec map[string]json.RawMessage `json:"spec"`
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					assert.Equal(t, "payment_request", body.Type)
					assert.JSONEq(t, fmt.Sprintf("%q", provider), string(body.Spec["provider"]))
					expectedMethodID := `"card-1"`
					if provider == "link" {
						expectedMethodID = `"pm-1"`
					}
					assert.JSONEq(t, expectedMethodID, string(body.Spec["payment_method_id"]))
					if provider == "link" {
						assert.JSONEq(t, `{"reference":"order-1"}`, string(body.Spec["metadata"]))
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, fixture)
				})
				out, _, err := executeVaultCommand(t, client, "vaults", "payment-requests", operation, "checkout", "order-1", "--provider", provider, "--spec", spec, "-o", "json")
				require.NoError(t, err)
				assert.Equal(t, 1, calls)
				assert.Contains(t, out, `"type": "payment_request"`)
				assert.Contains(t, out, `"payment_method_id"`)
			})
		}
	}
}

func TestVaultPaymentRequestRejectsAgentCard(t *testing.T) {
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	_, _, err := executeVaultCommand(t, client, "vaults", "payment-requests", "create", "checkout", "order-1", "--provider", "agentcard", "--spec", `{}`)
	require.ErrorContains(t, err, "support only link or kernel")
	assert.Zero(t, calls)
}

func TestVaultPaymentRequestOutputAndLinkAuthorize(t *testing.T) {
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, linkPaymentRequestFixture)
			return
		}
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"authorize","browser_id":"browser-1","page_url":"https://shop.example/checkout"}`, string(body))
		_, _ = io.WriteString(w, strings.Replace(linkPaymentRequestFixture, `"status":"requested"`, `"status":"pending_authorization"`, 1))
	})
	out, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1")
	require.NoError(t, err)
	for _, want := range []string{"payment_request", "payment method id", "pm-1", "wallet-1", "authorize"} {
		assert.Contains(t, out+human, want)
	}
	_, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize", "--browser-id", "browser-1", "--page-url", "https://shop.example/checkout", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestVaultKernelPaymentRequestFundingOutput(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, kernelPaymentRequestFixture)
	})
	out, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"funding_status": "available"`)
	assert.Contains(t, out, `"payment_method_id": "card-1"`)
	_, human, err = executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1")
	require.NoError(t, err)
	for _, want := range []string{"funding status", "available", "card-1", "network token last4", "5678", "funding_status is separate from purchase status", "removed means the method is gone"} {
		assert.Contains(t, human, want)
	}
}

func TestVaultPaymentRequestHelpAndFlags(t *testing.T) {
	for _, path := range [][]string{{"payment-requests", "create"}, {"payment-requests", "update"}} {
		cmd, _, err := newVaultsCommand().Find(path)
		require.NoError(t, err)
		assert.NotNil(t, cmd.Flags().Lookup("provider"))
		assert.NotNil(t, cmd.Flags().Lookup("spec"))
		for _, want := range []string{"payment_request", "LinkPaymentRequestSpec", "KernelPaymentRequestSpec", "payment_method_id", "payment-requests", "AgentCard reusable", "funding_status is separate from purchase status"} {
			assert.Contains(t, cmd.Long, want)
		}
		assert.Contains(t, cmd.Example, `"payment_method_id"`)
	}
}
