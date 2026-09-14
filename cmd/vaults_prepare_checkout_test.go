package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const prepareCheckoutOperation = `[{"type":"prepare_checkout","description":"Prepare this unused AgentCard card before the first Square Pay action."}]`

// A ready AgentCard card that advertises prepare_checkout.
var unusedAgentCardFixture = strings.ReplaceAll(strings.ReplaceAll(
	`{"id":"item-1","key":"order-1","type":"card","spec":{"provider":"agentcard","wallet":"wallet-1","merchant":"Example Shop","amount":2599,"currency":"usd"},"state":{"provider":"agentcard","status":"ready"},"available_operations":OPS,"available_expansions":[]}`,
	"OPS", prepareCheckoutOperation), "\n", "")

// The same card after preparation, awaiting cardholder device approval.
var preparingAgentCardFixture = `{"id":"item-1","key":"order-1","type":"card","spec":{"provider":"agentcard","wallet":"wallet-1","merchant":"Example Shop","amount":2599,"currency":"usd"},"state":{"provider":"agentcard","status":"preparing","preparation":{"id":"prep-1","status":"awaiting_approval","browser_id":"browser-session-id","merchant_origin":"https://shop.example.com","environment":"production","approval_url":"https://provider.example/approve","created_at":"2026-01-01T12:00:00Z","expires_at":"2026-01-01T12:00:30Z"}},"available_operations":[],"available_expansions":[]}`

func TestVaultPrepareCheckoutSendsCheckoutContext(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body := unusedAgentCardFixture
		if r.Method == http.MethodPost {
			assert.Equal(t, "/vaults/checkout/items/order-1/operations", r.URL.Path)
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"prepare_checkout","checkout":{"browser_id":"browser-session-id","merchant_origin":"https://shop.example.com","environment":"production"}}`, string(raw))
			body = preparingAgentCardFixture
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "prepare_checkout",
		"--params", `{"browser_id":"browser-session-id","merchant_origin":"https://shop.example.com/","environment":"production"}`, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.JSONEq(t, preparingAgentCardFixture, out)
}

func TestVaultPrepareCheckoutRendersPreparationAndDeadline(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := unusedAgentCardFixture
		if r.Method == http.MethodPost {
			body = preparingAgentCardFixture
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "prepare_checkout",
		"--params", `{"browser_id":"browser-session-id","merchant_origin":"https://shop.example.com","environment":"production"}`)
	require.NoError(t, err)
	assert.Contains(t, human, "prep-1")
	assert.Contains(t, human, "awaiting_approval")
	assert.Contains(t, human, "https://shop.example.com")
	assert.Contains(t, human, "Submit native Pay before")
	assert.Contains(t, human, "https://provider.example/approve")
	assert.Contains(t, human, "Keep the approval page open")
	assert.Contains(t, human, "display-only")
}

func TestVaultPrepareCheckoutOpensApprovalURL(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := unusedAgentCardFixture
		if r.Method == http.MethodPost {
			body = preparingAgentCardFixture
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	opened := ""
	handler := VaultsCmd{vaults: &client.Vaults, openURL: func(url string) error { opened = url; return nil }}
	params, err := parseVaultOperationParams("prepare_checkout", `{"browser_id":"browser-session-id","merchant_origin":"https://shop.example.com","environment":"production"}`, true, true)
	require.NoError(t, err)
	captureStdout(t, func() {
		err = handler.Invoke(t.Context(), "checkout", "order-1", "prepare_checkout", params, "json", true)
	})
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/approve", opened)
}

func TestVaultPrepareCheckoutRejectsInvalidParams(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	for name, params := range map[string]string{
		"missing":          "",
		"empty browser":    `{"browser_id":"  ","merchant_origin":"https://shop.example.com","environment":"production"}`,
		"bad environment":  `{"browser_id":"b","merchant_origin":"https://shop.example.com","environment":"staging"}`,
		"origin with path": `{"browser_id":"b","merchant_origin":"https://shop.example.com/checkout","environment":"production"}`,
		"origin query":     `{"browser_id":"b","merchant_origin":"https://shop.example.com?a=1","environment":"production"}`,
		"insecure origin":  `{"browser_id":"b","merchant_origin":"http://shop.example.com","environment":"production"}`,
		"credentials":      `{"browser_id":"b","merchant_origin":"https://user:pass@shop.example.com","environment":"production"}`,
		"unknown key":      `{"browser_id":"b","merchant_origin":"https://shop.example.com","environment":"production","tab_id":"1"}`,
		"explicit type":    `{"type":"prepare_checkout","browser_id":"b","merchant_origin":"https://shop.example.com","environment":"production"}`,
	} {
		t.Run(name, func(t *testing.T) {
			args := []string{"vaults", "items", "invoke", "checkout", "order-1", "prepare_checkout"}
			if params != "" {
				args = append(args, "--params", params)
			}
			_, _, err := executeVaultCommand(t, client, args...)
			require.Error(t, err)
		})
	}
}

func TestVaultMerchantOriginAcceptsLoopbackAndCanonicalizes(t *testing.T) {
	for input, want := range map[string]string{
		"https://shop.example.com":      "https://shop.example.com",
		"https://shop.example.com/":     "https://shop.example.com",
		"https://shop.example.com:8443": "https://shop.example.com:8443",
		"http://localhost:3000":         "http://localhost:3000",
		"http://127.0.0.1":              "http://127.0.0.1",
	} {
		got, err := vaultMerchantOrigin(input)
		require.NoError(t, err, input)
		assert.Equal(t, want, got)
	}
}

func TestVaultPrepareCheckoutFailureDoesNotSuggestRetry(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"code":"item_not_ready","message":"Item not ready"}`)
			return
		}
		_, _ = io.WriteString(w, unusedAgentCardFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "prepare_checkout",
		"--params", `{"browser_id":"browser-session-id","merchant_origin":"https://shop.example.com","environment":"production"}`)
	require.ErrorContains(t, err, "prepare_checkout failed (HTTP 409)")
	require.ErrorContains(t, err, "single-use preparation may still have been created")
	// One GET plus one POST: a single-use preparation is never retried.
	assert.Equal(t, 2, calls)
}

func TestVaultItemGuidanceForPreparedStatuses(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for status, want := range map[string]string{
		"ready_to_submit": "at most 30 seconds",
		"consumed":        "does not mean an order or charge succeeded",
		"stopped":         "cannot be reused",
		"outcome_unknown": "new requests are blocked",
	} {
		t.Run(status, func(t *testing.T) {
			body := strings.ReplaceAll(preparingAgentCardFixture, `"status":"preparing"`, `"status":"`+status+`"`)
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			})
			_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1")
			require.NoError(t, err)
			assert.Contains(t, human, want)
		})
	}
}
