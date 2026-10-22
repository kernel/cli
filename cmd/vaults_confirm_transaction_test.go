package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const confirmTransactionFixture = `{"status":"APPROVED","transaction_type":"PURCHASE","amount":100,"currency":"USD","occurred_at":"2026-09-01T12:00:00Z"}`

func TestVaultConfirmTransactionBody(t *testing.T) {
	for _, input := range []string{"params", "file", "stdin"} {
		t.Run(input, func(t *testing.T) {
			posts := 0
			fixture := strings.Replace(kernelCardFixture, `"type":"authorize"`, `"type":"confirm_transaction"`, 1)
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posts++
					assert.Equal(t, "/vaults/checkout/items/order-1/operations", r.URL.Path)
					assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, `{"type":"confirm_transaction","status":"APPROVED","transaction_type":"PURCHASE","amount":100,"currency":"USD","occurred_at":"2026-09-01T12:00:00Z"}`, string(body))
					fixture = strings.Replace(fixture, `[{"type":"confirm_transaction","description":"Authorize this purchase"}]`, `[]`, 1)
				} else {
					assert.Equal(t, http.MethodGet, r.Method)
				}
				_, _ = io.WriteString(w, fixture)
			})
			args := []string{"vaults", "items", "invoke", "checkout", "order-1", "confirm_transaction", "-o", "json"}
			stdin := ""
			switch input {
			case "params":
				args = append(args, "--params", confirmTransactionFixture)
			case "file":
				path := filepath.Join(t.TempDir(), "confirmation.json")
				require.NoError(t, os.WriteFile(path, []byte(confirmTransactionFixture), 0600))
				args = append(args, "--spec-file", path)
			case "stdin":
				args = append(args, "--spec-file", "-")
				stdin = confirmTransactionFixture
			}
			out, human, err := executeVaultInputCommand(t, client, stdin, args...)
			require.NoError(t, err)
			assert.Contains(t, out, `"available_operations": []`)
			assert.Empty(t, human)
			_, _, err = executeVaultInputCommand(t, client, stdin, args...)
			require.ErrorContains(t, err, "not advertised")
			assert.Equal(t, 1, posts)
		})
	}
}

func TestVaultConfirmTransactionInvalidParamsDoNotRequest(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid confirmation reached API") })
	invalid := []string{"{}", "null", "[]", "{} {}", "{", strings.TrimSuffix(confirmTransactionFixture, "}") + `,"extra":true}`, strings.TrimSuffix(confirmTransactionFixture, "}") + `,"type":"confirm_transaction"}`, strings.TrimSuffix(confirmTransactionFixture, "}") + `,"amount":100}`}
	for _, field := range []string{"status", "transaction_type", "amount", "currency", "occurred_at"} {
		var object map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(confirmTransactionFixture), &object))
		delete(object, field)
		raw, err := json.Marshal(object)
		require.NoError(t, err)
		invalid = append(invalid, string(raw))
		object[field] = json.RawMessage(`null`)
		raw, err = json.Marshal(object)
		require.NoError(t, err)
		invalid = append(invalid, string(raw))
	}
	for field, values := range map[string][]string{
		"status":           {`"approved"`, `"UNKNOWN"`, `42`},
		"transaction_type": {`"purchase"`, `"SALE"`, `false`},
		"amount":           {`-1`, `1000001`, `100.5`, `100.0`, `"100"`, `9223372036854775808`},
		"currency":         {`"usd"`, `"US"`, `"USDD"`, `"U1D"`, `123`},
		"occurred_at":      {`"2026-09-01"`, `"2026-09-01T12:00:00"`, `"not-a-date"`, `42`},
	} {
		for _, value := range values {
			var object map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(confirmTransactionFixture), &object))
			object[field] = json.RawMessage(value)
			raw, err := json.Marshal(object)
			require.NoError(t, err)
			invalid = append(invalid, string(raw))
		}
	}
	for i, raw := range invalid {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			for _, input := range []string{"--params", "--spec-file"} {
				value, stdin := raw, ""
				if input == "--spec-file" {
					value, stdin = "-", raw
				}
				_, _, err := executeVaultInputCommand(t, client, stdin, "vaults", "items", "invoke", "checkout", "order-1", "confirm_transaction", input, value)
				require.Error(t, err)
			}
		})
	}
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "confirm_transaction")
	require.ErrorContains(t, err, "requires --params or --spec-file")
}

func TestVaultConfirmTransactionValidValues(t *testing.T) {
	for _, status := range []string{"APPROVED", "DECLINED", "PENDING", "ERROR", "CANCELLED"} {
		for _, transactionType := range []string{"PURCHASE", "AUTHORIZATION", "CAPTURE", "REFUND", "REVERSAL", "VERIFICATION", "CHARGEBACK", "FRAUD"} {
			for _, amount := range []int{0, 1000000} {
				raw := fmt.Sprintf(`{"status":%q,"transaction_type":%q,"amount":%d,"currency":"USD","occurred_at":"2026-09-01T12:00:00.123+01:00"}`, status, transactionType, amount)
				params, err := parseVaultOperationParams("confirm_transaction", raw, true, false)
				require.NoError(t, err)
				assert.Equal(t, int64(amount), params.ConfirmTransaction.Amount)
			}
		}
	}
}

func TestVaultConfirmTransactionDoesNotRetry(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posts++
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"message":"outcome unknown"}`)
					return
				}
				_, _ = io.WriteString(w, strings.Replace(kernelCardFixture, `"type":"authorize"`, `"type":"confirm_transaction"`, 1))
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "confirm_transaction", "--params", confirmTransactionFixture)
			require.Error(t, err)
			assert.Equal(t, 1, posts)
		})
	}
}
