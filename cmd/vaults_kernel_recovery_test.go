package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKernelFillExpirationBindings(t *testing.T) {
	for _, fields := range []string{`[{"field":"exp_month","selector":"#month"},{"field":"exp_year","selector":"#year"}]`, `[{"field":"expiration","selector":"#expiry","format":"MM/YYYY"}]`} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, strings.Replace(kernelCardFixture, `"type":"authorize"`, `"type":"fill"`, 1))
				return
			}
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var request struct {
				Fields []json.RawMessage `json:"fields"`
			}
			require.NoError(t, json.Unmarshal(body, &request))
			actual, err := json.Marshal(request.Fields)
			require.NoError(t, err)
			assert.JSONEq(t, fields, string(actual))
			results := make([]map[string]any, 0, len(request.Fields))
			for i := range request.Fields {
				results = append(results, map[string]any{"index": i, "status": "filled"})
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"type": "fill", "status": "completed", "fields": results}))
		})
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", `{"browser_id":"browser-1","page_url":"https://shop.example/checkout","fields":`+fields+`}`, "-o", "json")
		require.NoError(t, err)
	}
}

func TestKernelCardholderOpenHelp(t *testing.T) {
	for _, command := range [][]string{{"wallets", "create"}, {"wallets", "get"}, {"items", "invoke"}} {
		cmd, _, err := newVaultsCommand().Find(command)
		require.NoError(t, err)
		assert.Contains(t, cmd.Flags().Lookup("open").Usage, "agents must share privately")
	}
}

func TestKernelOperationRecoveryRechecksAdvertisement(t *testing.T) {
	for _, operation := range []string{"authorize", "confirm_transaction", "restart_verification"} {
		t.Run(operation, func(t *testing.T) {
			fixture := strings.Replace(kernelCardFixture, `"type":"authorize"`, fmt.Sprintf(`"type":%q`, operation), 1)
			if operation == "restart_verification" {
				fixture = strings.Replace(fixture, `"status":"requested"`, `"status":"recovery_required"`, 1)
			}
			posts, gets := 0, 0
			var bodies []string
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					gets++
					if posts >= 2 {
						_, _ = io.WriteString(w, strings.Replace(fixture, fmt.Sprintf(`[{"type":%q,"description":"Authorize this purchase"}]`, operation), `[]`, 1))
					} else {
						_, _ = io.WriteString(w, fixture)
					}
					return
				}
				posts++
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				bodies = append(bodies, string(body))
				if posts == 1 {
					w.WriteHeader(500)
					_, _ = io.WriteString(w, `{"code":"provider_error","message":"reconcile before retrying https://vault.example/approval#secret HTTPS://vault.example/enroll?secret=x"}`)
				} else {
					_, _ = io.WriteString(w, kernelCardFixture)
				}
			})
			args := []string{"vaults", "items", "invoke", "checkout", "order-1", operation, "-o", "json"}
			if operation == "confirm_transaction" {
				args = append(args, "--params", confirmTransactionFixture)
			}
			_, _, err := executeVaultCommand(t, client, args...)
			require.Error(t, err)
			message := util.CleanedUpSdkError{Err: err}.Error()
			assert.Contains(t, message, "reconcile before retrying")
			assert.Contains(t, message, "invoke "+operation+" once more only if freshly advertised")
			assert.NotContains(t, message, "https://vault.example")
			assert.NotContains(t, message, "HTTPS://vault.example")
			var apiErr *kernel.Error
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, 500, apiErr.StatusCode)
			assert.NotContains(t, apiErr.RawJSON(), "secret")
			assert.Nil(t, apiErr.Request)
			assert.Nil(t, apiErr.Response)
			assert.Equal(t, 1, posts, "no automatic retry")
			_, _, err = executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			_, _, err = executeVaultCommand(t, client, args...)
			require.Error(t, err)
			assert.Equal(t, 2, posts)
			assert.Equal(t, 3, gets)
			assert.Equal(t, bodies[0], bodies[1])
		})
	}
}

func TestKernelCreateRetryKeepsKeyAndSpec(t *testing.T) {
	for _, itemType := range []string{"wallet", "card"} {
		t.Run(itemType, func(t *testing.T) {
			var requests, paths []string
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPut, r.Method)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				requests = append(requests, string(body))
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if len(requests) == 1 {
					w.WriteHeader(500)
					_, _ = io.WriteString(w, `{"code":"provider_error","message":"provider unavailable"}`)
				} else if itemType == "wallet" {
					_, _ = io.WriteString(w, kernelWalletFixture)
				} else {
					_, _ = io.WriteString(w, kernelCardFixture)
				}
			})
			spec := `{}`
			if itemType == "card" {
				spec = `{"wallet":"cardholder","amount":1200,"currency":"USD","merchant_name":"Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US","merchant_category":"Books","merchant_category_code":"5942"}`
			}
			args := []string{"vaults", itemType + "s", "create", "checkout", "item-1", "--provider", "kernel", "--spec", spec, "-o", "json"}
			_, _, err := executeVaultCommand(t, client, args...)
			require.ErrorContains(t, err, "identical key and identical spec")
			assert.Len(t, requests, 1)
			_, _, err = executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, requests[0], requests[1])
			assert.Equal(t, paths[0], paths[1])
			if itemType == "card" {
				assert.Contains(t, requests[0], `"merchant_category_code":"5942"`)
			}
		})
	}
}

func TestKernelRateLimitAndAuthorizeConflictGuidance(t *testing.T) {
	for _, after := range []string{"30", "Wed, 14 Oct 2026 12:00:00 GMT"} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, kernelCardFixture)
				return
			}
			w.Header().Set("Retry-After", after)
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{"code":"rate_limited","message":"provider limit"}`)
		})
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize")
		require.ErrorContains(t, err, "Retry-After ("+after+")")
		assert.Contains(t, err.Error(), "only if freshly advertised")
	}
	var apiErr kernel.Error
	require.NoError(t, json.Unmarshal([]byte(`{"code":"conflict","message":"card authorization outcome is unknown"}`), &apiErr))
	apiErr.StatusCode = 409
	err := kernelVaultRequestError(&apiErr, "authorize")
	assert.Contains(t, err.Error(), "no authorize inputs to fix")
	assert.Contains(t, err.Error(), "card authorization outcome is unknown")
}

func TestKernelConfirmationMatchesApprovedPurchase(t *testing.T) {
	for _, raw := range []string{strings.Replace(confirmTransactionFixture, `"USD"`, `"EUR"`, 1), strings.Replace(confirmTransactionFixture, `"amount":100`, `"amount":1201`, 1)} {
		calls := 0
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, strings.Replace(kernelCardFixture, `"type":"authorize"`, `"type":"confirm_transaction"`, 1))
		})
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "confirm_transaction", "--params", raw)
		require.ErrorContains(t, err, "operation was not invoked")
		assert.Equal(t, 1, calls)
	}
}

func TestKernelFillFieldUnavailableDoesNotAskForCardValues(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, strings.Replace(kernelCardFixture, `"type":"authorize"`, `"type":"fill"`, 1))
			return
		}
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"code":"field_unavailable","message":"unsupported billing field"}`)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", fillParamsFixture)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enter billing details on the merchant page")
	assert.Contains(t, err.Error(), "no fields were written")
	assert.NotContains(t, err.Error(), "collect missing values")
}

func TestKernelRecoveryJSONAndStateGuidance(t *testing.T) {
	for _, status := range []string{"requested", "recovery_required", "expired", "declined", "degraded", "reconnect_required"} {
		fixture := strings.Replace(kernelCardFixture, `"status":"requested"`, fmt.Sprintf(`"status":%q`, status), 1)
		if status == "degraded" || status == "reconnect_required" {
			fixture = strings.Replace(kernelWalletFixture, `"status":"pending_authorization"`, fmt.Sprintf(`"status":%q`, status), 1)
		}
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, fixture)
		})
		_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1")
		require.NoError(t, err)
		switch status {
		case "degraded", "reconnect_required":
			assert.Contains(t, human, "delete the wallet only if the api permits it")
			assert.Contains(t, human, "reconnect_required: the wallet predates payment methods")
		case "requested":
			assert.Contains(t, human, "Visa may require")
			assert.NotContains(t, human, "merchant decline")
		case "recovery_required":
			assert.Contains(t, human, "support@kernel.sh")
			assert.Contains(t, human, "project, vault, item key")
			assert.Contains(t, human, "do not assume a new card is safe")
			out, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1", "-o", "json")
			require.NoError(t, err)
			assert.NotContains(t, out, `"authorize"`)
		default:
			assert.Contains(t, human, "merchant decline")
			assert.Contains(t, human, "if never submitted")
			assert.Contains(t, human, "PENDING means")
			assert.Contains(t, human, "ERROR means")
			assert.NotContains(t, human, "Visa may require")
		}
	}
}
