package cmd

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultWritesAreNeverTransportReplayed(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	spec := `{"wallet":"cardholder","payment_method_id":"card-1","amount":1200,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout"}`

	t.Run("body", func(t *testing.T) {
		writes := 0
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_, _ = fmt.Fprint(w, kernelPaymentRequestFixture)
				return
			}
			writes++
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			_ = conn.Close()
		})
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "checkout")
		require.NoError(t, err) // Prime a reusable connection so net/http's replay path is exercised.
		_, _, err = executeVaultCommand(t, client, "vaults", "payment-requests", "update", "user", "checkout", "--provider", "kernel", "--spec", spec)
		require.Error(t, err)
		assert.Equal(t, 1, writes)
	})

	t.Run("bodyless", func(t *testing.T) {
		writes := 0
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_, _ = fmt.Fprint(w, kernelPaymentRequestFixture)
				return
			}
			writes++
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			_ = conn.Close()
		})
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "user", "checkout")
		require.NoError(t, err)
		_, _, err = executeVaultCommand(t, client, "vaults", "items", "delete", "user", "checkout", "--yes")
		require.Error(t, err)
		assert.Equal(t, 1, writes)
	})
}
