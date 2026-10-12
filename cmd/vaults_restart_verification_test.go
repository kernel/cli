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

func TestVaultRestartVerificationRecovery(t *testing.T) {
	for _, advertised := range []bool{false, true} {
		for _, open := range []bool{false, true} {
			t.Run(fmt.Sprintf("advertised=%t/open=%t", advertised, open), func(t *testing.T) {
				fixture := strings.Replace(kernelCardFixture, `"status":"requested"`, `"status":"recovery_required"`, 1)
				if advertised {
					fixture = strings.Replace(fixture, `"type":"authorize"`, `"type":"restart_verification"`, 1)
				} else {
					fixture = strings.Replace(fixture, `[{"type":"authorize","description":"Authorize this purchase"}]`, `[]`, 1)
				}
				posts := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodPost {
						posts++
						assert.Equal(t, "/vaults/checkout/items/order-1/operations", r.URL.Path)
						body, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						assert.JSONEq(t, `{"type":"restart_verification"}`, string(body))
						updated := strings.Replace(kernelCardFixture, `"status":"requested"`, `"status":"pending_authorization"`, 1)
						_, _ = io.WriteString(w, strings.TrimSuffix(updated, "}")+`,"action":{"name":"spend_approval","url":"https://vault.example/approve?state=fresh"}}`)
						return
					}
					assert.Equal(t, http.MethodGet, r.Method)
					_, _ = io.WriteString(w, fixture)
				})
				opened := ""
				c := VaultsCmd{vaults: &client.Vaults, openURL: func(url string) error { opened = url; return nil }}
				invoke, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
				require.NoError(t, err)
				if open {
					require.NoError(t, invoke.ParseFlags([]string{"--open"}))
				}
				params, err := parseVaultOperationParams("restart_verification", "", false, invoke.Flags().Changed("open"))
				require.NoError(t, err)
				out := captureStdout(t, func() {
					err = c.Invoke(context.Background(), "checkout", "order-1", "restart_verification", params, "json", open)
				})
				if !advertised {
					require.ErrorContains(t, err, "recovery_required")
					assert.Contains(t, err.Error(), "do not retry, delete, or replace")
					assert.Zero(t, posts)
					assert.Empty(t, opened)
					assert.Empty(t, out)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, 1, posts)
				assert.Contains(t, out, `"spend_approval"`)
				if open {
					assert.Equal(t, "https://vault.example/approve?state=fresh", opened)
				} else {
					assert.Empty(t, opened)
				}
			})
		}
	}
}

func TestVaultRestartVerificationRejectsParams(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("parameterized restart reached API") })
	for _, input := range []string{"--params", "--spec-file"} {
		_, _, err := executeVaultInputCommand(t, client, "{}", "vaults", "items", "invoke", "checkout", "order-1", "restart_verification", input, "{}", "--open")
		require.Error(t, err)
	}
}

func TestVaultRecoveryDisplaysAdvertisedRestart(t *testing.T) {
	fixture := strings.Replace(kernelCardFixture, `"status":"requested"`, `"status":"recovery_required"`, 1)
	fixture = strings.Replace(fixture, `"type":"authorize"`, `"type":"restart_verification"`, 1)
	fixture = strings.TrimSuffix(fixture, "}") + `,"action":{"name":"spend_approval","url":"https://vault.example/stale"}}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1")
	require.NoError(t, err)
	assert.Contains(t, human, "available operation: restart_verification")
	assert.Contains(t, human, "invoke: kernel vaults items invoke -- checkout order-1 restart_verification")
	assert.Contains(t, human, "invoke it once")
	assert.NotContains(t, human, "https://vault.example/stale")
}
