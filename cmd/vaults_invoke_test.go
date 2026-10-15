package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultInvokeUsesAdvertisedTypeAcrossItems(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	operation := `[{"type":"refresh","description":"Refresh this item explicitly."}]`
	for _, fixture := range []string{
		strings.ReplaceAll(requestedCardFixture, `[{"type":"authorize","description":"Use only after explicit user approval."}]`, operation),
		strings.ReplaceAll(connectedWalletFixture, `"available_operations":[]`, `"available_operations":`+operation),
		strings.ReplaceAll(strings.ReplaceAll(connectedWalletFixture, `"provider":"link"`, `"provider":"agentcard"`), `"available_operations":[]`, `"available_operations":`+operation),
	} {
		t.Run(fixture, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					assert.Equal(t, http.MethodGet, r.Method)
				} else {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/vaults/checkout/items/item-1/operations", r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, `{"type":"refresh"}`, string(body))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, fixture)
			})
			out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "item-1", "refresh", "-o", "json")
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
			assert.JSONEq(t, fixture, out)
			assert.Empty(t, human)
		})
	}
}

func TestVaultInvokeRejectsUnadvertisedOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, requestedCardFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "refresh")
	require.ErrorContains(t, err, `operation "refresh" is not advertised`)
	assert.Equal(t, 1, calls)
}

func TestVaultInvokeGetFailureDoesNotPostOrRetry(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, status := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"code":"item_unavailable","message":"Item unavailable"}`)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize")
			require.ErrorContains(t, err, "item_unavailable: Item unavailable")
			assert.Equal(t, 1, calls)
		})
	}
}

func TestVaultInvokeArgumentsAndHelp(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	for _, args := range [][]string{
		{"checkout", "order-1"},
		{"checkout", "order-1", ""},
		{"checkout", "order-1", "authorize", "extra"},
		{"checkout", "order-1", "authorize", "--spec", "{}"},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "items", "invoke"}, args...)...)
		require.Error(t, err)
	}
	cmd, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	assert.Nil(t, cmd.Flags().Lookup("spec"))
	assert.NotNil(t, cmd.Flags().Lookup("open"))
	assert.NotNil(t, cmd.Flags().Lookup("params"))
	assert.Contains(t, cmd.Long, "failed/unknown exit nonzero")
	assert.Contains(t, cmd.Long, `{"type":"authorize"}`)
	assert.Contains(t, cmd.Long, "available_operations")
}

func TestVaultInvokeOpensOnlyReturnedActionExplicitly(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(fmt.Sprint(open), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				body := requestedCardFixture
				if r.Method == http.MethodPost {
					body = strings.ReplaceAll(body, `"status":"requested"`, `"status":"pending_authorization"`)
					body = strings.ReplaceAll(body, `"available_operations":`, `"action":{"name":"spend_approval","url":"https://provider.example/approve"},"available_operations":`)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			})
			opened := ""
			c := VaultsCmd{vaults: &client.Vaults, openURL: func(url string) error { opened = url; return nil }}
			var err error
			out := captureStdout(t, func() {
				err = c.Invoke(context.Background(), "checkout", "order-1", "authorize", nil, "json", open)
			})
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
			assert.Contains(t, out, `"status": "pending_authorization"`)
			if open {
				assert.Equal(t, "https://provider.example/approve", opened)
			} else {
				assert.Empty(t, opened)
			}
		})
	}
}

func TestVaultGetOperationHintUsesExplicitProject(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "other-project")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "chosen-project", r.Header.Get("X-Kernel-Project"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, requestedCardFixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1", "--project", "chosen-project")
	require.NoError(t, err)
	assert.Contains(t, human, "invoke: kernel vaults items invoke --project=chosen-project --browser-id '<browser-session-id>' --page-url '<checkout-page-url>' -- checkout order-1 authorize")
	assert.NotContains(t, human, "other-project")
}

func TestVaultGetOperationHints(t *testing.T) {
	for _, project := range []string{"", "project-1", "team's $(touch /tmp/nope)"} {
		for _, wallet := range []bool{false, true} {
			t.Run(fmt.Sprint(project, wallet), func(t *testing.T) {
				t.Setenv("KERNEL_PROJECT", project)
				fixture := requestedCardFixture
				if wallet {
					fixture = strings.ReplaceAll(connectedWalletFixture, `"available_operations":[]`, `"available_operations":[{"type":"refresh","description":"Refresh this item explicitly."}]`)
				}
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, fixture)
				})
				_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "item-1")
				require.NoError(t, err)
				op := "authorize"
				if wallet {
					op = "refresh"
				}
				assert.Contains(t, human, "available operation: "+op)
				assert.Contains(t, human, "invoke: kernel vaults items invoke")
				assert.Contains(t, human, " -- checkout item-1 "+op)
				switch project {
				case "":
					assert.NotContains(t, human, "--project")
				case "project-1":
					assert.Contains(t, human, "--project=project-1")
				default:
					assert.Contains(t, human, `--project='team'\''s $(touch /tmp/nope)'`)
				}
				out, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "item-1", "-o", "json")
				require.NoError(t, err)
				assert.JSONEq(t, fixture, out)
				assert.Empty(t, human)
			})
		}
	}
}

func TestVaultInvokeConfirmTransactionSendsObservedOutcome(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	fixture := strings.ReplaceAll(requestedCardFixture, `[{"type":"authorize","description":"Use only after explicit user approval."}]`, `[{"type":"confirm_transaction","description":"Report the observed merchant outcome."}]`)
	for _, occurredAt := range []string{
		"2026-10-10T12:00:00Z",
		"2026-10-10T12:00:00.123Z",
		"2026-10-10T12:00:00.123456789Z",
		"2026-10-10T12:00:00.123+05:30",
	} {
		t.Run(occurredAt, func(t *testing.T) {
			raw := fmt.Sprintf(`{"status":"APPROVED","transaction_type":"PURCHASE","amount":1234,"currency":"USD","occurred_at":%q}`, occurredAt)
			params, err := parseVaultOperationParams("confirm_transaction", raw, true, false)
			require.NoError(t, err)
			assert.Equal(t, occurredAt, params.ConfirmTransaction.OccurredAt)

			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 2 {
					assert.Equal(t, http.MethodPost, r.Method)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, fmt.Sprintf(`{"type":"confirm_transaction","status":"APPROVED","transaction_type":"PURCHASE","amount":1234,"currency":"USD","occurred_at":%q}`, occurredAt), string(body))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, fixture)
			})
			_, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "confirm_transaction",
				"--params", raw, "-o", "json")
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
		})
	}
}

func TestParseVaultConfirmTransactionParamsValidates(t *testing.T) {
	_, err := parseVaultOperationParams("confirm_transaction", "", false, false)
	require.ErrorContains(t, err, "confirm_transaction requires --params")
	for raw, message := range map[string]string{
		`{"status":"OK","transaction_type":"PURCHASE","amount":1,"currency":"USD","occurred_at":"2026-10-10T12:00:00Z"}`:                 "status must be",
		`{"status":"APPROVED","transaction_type":"SALE","amount":1,"currency":"USD","occurred_at":"2026-10-10T12:00:00Z"}`:               "transaction_type must be",
		`{"status":"APPROVED","transaction_type":"PURCHASE","amount":1.5,"currency":"USD","occurred_at":"2026-10-10T12:00:00Z"}`:         "amount must be",
		`{"status":"APPROVED","transaction_type":"PURCHASE","amount":1,"currency":"USD","occurred_at":"yesterday"}`:                      "occurred_at must be",
		`{"status":"APPROVED","transaction_type":"PURCHASE","amount":1,"currency":"USD","occurred_at":"2026-10-10T12:00:00Z","extra":1}`: "supported, non-duplicate",
	} {
		_, err := parseVaultOperationParams("confirm_transaction", raw, true, false)
		require.ErrorContains(t, err, message, raw)
	}
}
