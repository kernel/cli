package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const checkoutParamsFixture = `{"checkout":{"browser_id":"browser-1","merchant_origin":"https://shop.example","environment":"production"}}`
const preparationCardFixture = `{"id":"card-1","key":"order-1","type":"card","spec":{"provider":"agentcard","wallet":"wallet-1","merchant":"Shop","amount":1234,"currency":"usd"},"state":{"provider":"agentcard","status":"preparing","status_reason":"Awaiting approval","preparation":{"id":"prep-1","status":"awaiting_approval","browser_id":"browser-1","merchant_origin":"https://shop.example","environment":"production","created_at":"2026-09-15T00:00:00Z","expires_at":"2026-09-15T00:00:30Z","approval_url":"https://approve.example/prepare","secret":"never-print"}},"action":{"name":"spend_approval","url":"https://approve.example/prepare"},"available_operations":[{"type":"prepare_checkout","description":"Prepare after approval"}],"available_expansions":[]}`

func TestVaultPrepareCheckout(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, file := range []bool{false, true} {
		t.Run(fmt.Sprint(file), func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					posts++
					assert.Equal(t, "/vaults/user-123/items/order-1/operations", r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, `{"type":"prepare_checkout","checkout":{"browser_id":"browser-1","merchant_origin":"https://shop.example","environment":"production"}}`, string(body))
				}
				fmt.Fprint(w, preparationCardFixture)
			})
			args := []string{"vaults", "items", "invoke", "user-123", "order-1", "prepare_checkout", "-o", "json"}
			if file {
				args = append(args, "--spec-file", credentialSpecFile(t, checkoutParamsFixture))
			} else {
				args = append(args, "--params", checkoutParamsFixture)
			}
			out, _, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 1, posts)
			assert.True(t, json.Valid([]byte(out)))
			for _, value := range []string{"prep-1", "browser-1", "https://shop.example", "production", "2026-09-15T00:00:30Z", "https://approve.example/prepare", "Awaiting approval"} {
				assert.Contains(t, out, value)
			}
			assert.NotContains(t, out, "never-print")
		})
	}
}

func TestVaultPrepareCheckoutProcessor(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	posts := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			posts++
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"prepare_checkout","checkout":{"browser_id":"browser-1","merchant_origin":"https://shop.example","environment":"shared","psp":"bambora"}}`, string(body))
		}
		fmt.Fprint(w, strings.Replace(strings.Replace(preparationCardFixture, `"environment":"production"`, `"environment":"shared"`, 1), `"merchant_origin":"https://shop.example"`, `"merchant_origin":"https://shop.example","psp":"bambora"`, 1))
	})
	params := strings.Replace(strings.Replace(checkoutParamsFixture, `"production"`, `"shared"`, 1), `"environment":`, `"psp":"bambora","environment":`, 1)
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user-123", "order-1", "prepare_checkout", "--params", params, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	assert.True(t, json.Valid([]byte(out)))
	assert.Contains(t, out, "bambora")
	assert.Contains(t, out, "shared")
	assert.NotContains(t, out, "never-print")

	_, text, err := executeVaultCommand(t, client, "vaults", "items", "get", "user-123", "order-1")
	require.NoError(t, err)
	assert.Contains(t, text, "Processor")
	assert.Contains(t, text, "bambora")
}

func TestVaultPrepareCheckoutInvalidParams(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, raw := range []string{
		`{}`, `null`, `{"checkout":null}`, `{"checkout":{}}`,
		`{"type":"prepare_checkout","checkout":{}}`, `{"checkout":{},"checkout":{}}`,
		strings.Replace(checkoutParamsFixture, `"browser-1"`, `null`, 1),
		strings.Replace(checkoutParamsFixture, `"browser-1"`, `""`, 1),
		strings.Replace(checkoutParamsFixture, `"production"`, `"invalid"`, 1),
		strings.Replace(checkoutParamsFixture, `"environment":`, `"psp":"stripe","environment":`, 1),
		strings.Replace(checkoutParamsFixture, `"environment":`, `"psp":null,"environment":`, 1),
		strings.Replace(checkoutParamsFixture, `"environment":`, `"psp":"square","psp":"braintree","environment":`, 1),
		strings.Replace(checkoutParamsFixture, `"environment":`, `"extra":"never-print","environment":`, 1),
		strings.Replace(checkoutParamsFixture, `"browser_id":`, `"browser_id":"duplicate","browser_id":`, 1),
		strings.Repeat("x", 128*1024+1),
	} {
		t.Run(fmt.Sprint(len(raw))+raw[:min(len(raw), 45)], func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user-123", "order-1", "prepare_checkout", "--params", raw)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "never-print")
			assert.Zero(t, calls)
		})
	}
	for _, origin := range []string{"http://shop.example", "https://user:pass@shop.example", "https://shop.example/checkout", "https://shop.example/", "https://shop.example?x=1", "https://shop.example#", "https://shop.example?", "https://*.example"} {
		_, err := parseVaultCheckoutParams(strings.Replace(checkoutParamsFixture, "https://shop.example", origin, 1))
		require.Error(t, err, origin)
	}
	_, err := parseVaultCheckoutParams(strings.Replace(checkoutParamsFixture, "https://shop.example", "http://localhost:3000", 1))
	require.NoError(t, err)
	for _, psp := range []string{"square", "braintree", "worldpay", "bambora", "mercado_pago", "adyen"} {
		params, err := parseVaultCheckoutParams(strings.Replace(checkoutParamsFixture, `"environment":`, `"psp":"`+psp+`","environment":`, 1))
		require.NoError(t, err, psp)
		assert.Equal(t, psp, string(params.Psp))
	}
	for _, environment := range []string{"production", "sandbox", "shared"} {
		params, err := parseVaultCheckoutParams(strings.Replace(checkoutParamsFixture, `"production"`, `"`+environment+`"`, 1))
		require.NoError(t, err, environment)
		assert.Equal(t, environment, string(params.Environment))
	}
	// psp is optional and must stay absent from the request body when omitted.
	params, err := parseVaultCheckoutParams(checkoutParamsFixture)
	require.NoError(t, err)
	assert.Empty(t, string(params.Psp))

	_, err = parseVaultOperationParams("prepare_checkout", "", false, false)
	require.Error(t, err)
}

func TestVaultPrepareCheckoutChecksAvailabilityAndNeverRetries(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, status := range []int{0, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" {
					fmt.Fprint(w, preparationCardFixture)
					return
				}
				posts++
				if status == 0 {
					conn, _, err := w.(http.Hijacker).Hijack()
					require.NoError(t, err)
					conn.Close()
					return
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"operation failed"}`)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user-123", "order-1", "prepare_checkout", "--params", checkoutParamsFixture)
			require.Error(t, err)
			assert.Equal(t, 1, posts)
		})
	}
	for _, fixture := range []string{
		strings.Replace(preparationCardFixture, `"type":"prepare_checkout"`, `"type":"other"`, 1),
		strings.Replace(preparationCardFixture, `"status":"preparing"`, `"status":"recovery_required"`, 1),
		strings.ReplaceAll(preparationCardFixture, `"agentcard"`, `"link"`),
	} {
		posts := 0
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				posts++
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, fixture)
		})
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user-123", "order-1", "prepare_checkout", "--params", checkoutParamsFixture)
		require.Error(t, err)
		assert.Zero(t, posts)
	}
}

func TestVaultPreparationOpenAndGuidance(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, preparationCardFixture)
	})
	opened := ""
	c := VaultsCmd{vaults: &client.Vaults, openURL: func(url string) error { opened = url; return nil }}
	params, err := parseVaultOperationParams("prepare_checkout", checkoutParamsFixture, true, true)
	require.NoError(t, err)
	captureStdout(t, func() {
		require.NoError(t, c.Invoke(context.Background(), "user-123", "order-1", "prepare_checkout", params, "json", true))
	})
	assert.Equal(t, "https://approve.example/prepare", opened)
	for _, status := range []string{"preparing", "ready_to_submit", "consumed"} {
		fixture := strings.Replace(preparationCardFixture, `"status":"preparing"`, `"status":"`+status+`"`, 1)
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, fixture)
		})
		_, text, err := executeVaultCommand(t, client, "vaults", "items", "get", "user-123", "order-1")
		require.NoError(t, err)
		assert.Contains(t, text, "Preparation ID")
		assert.Contains(t, text, "single-use")
		assert.NotContains(t, text, "never-print")
		if status == "ready_to_submit" {
			assert.Contains(t, text, "polling does not extend")
		}
	}
}
