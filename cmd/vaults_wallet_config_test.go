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

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultWalletConfigSelection(t *testing.T) {
	for _, provider := range []string{"link", "agentcard"} {
		for _, selection := range []string{"id", "name", "spec"} {
			for _, output := range []string{"", "json"} {
				t.Run(provider+"/"+selection+"/"+output, func(t *testing.T) {
					access, refresh := vaultTestSecret(t), vaultTestSecret(t)
					tokens := fmt.Sprintf(`{"access_token":%q,"refresh_token":%q}`, access, refresh)
					ref := `{"id":"config-1"}`
					if selection == "name" {
						ref = `{"name":"checkout-client"}`
					}
					spec := `{"provider":"agentcard","provider_config":` + ref + `,"user_id":"usr_enrolled"}`
					if provider == "link" {
						spec = `{"provider":"link","authorization":{"method":"oauth","client":{"type":"customer_managed","provider_config":` + ref + `}}}`
					}
					response := `{"id":"wallet-id","key":"wallet-1","type":"wallet","spec":` + strings.ReplaceAll(spec, ref, `{"id":"config-1"}`) + `,"state":{"provider":"` + provider + `","status":"connected"},"available_operations":[],"available_expansions":[]}`
					// Exercise output filtering even if an API accidentally echoes write-only fields.
					response = strings.TrimSuffix(response, "}") + fmt.Sprintf(`,"tokens":{"access_token":%q,"refresh_token":%q}}`, access, refresh)
					client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, http.MethodPut, r.Method)
						assert.Equal(t, "/vaults/checkout/items/wallet-1", r.URL.Path)
						var body struct {
							Type string                     `json:"type"`
							Spec map[string]json.RawMessage `json:"spec"`
						}
						require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
						assert.Equal(t, "wallet", body.Type)
						if provider == "link" {
							var auth struct {
								Method string            `json:"method"`
								Client json.RawMessage   `json:"client"`
								Tokens map[string]string `json:"tokens"`
							}
							require.NoError(t, json.Unmarshal(body.Spec["authorization"], &auth))
							assert.Equal(t, "oauth", auth.Method)
							assert.JSONEq(t, `{"type":"customer_managed","provider_config":`+ref+`}`, string(auth.Client))
							assert.True(t, auth.Tokens["access_token"] == access)
							assert.True(t, auth.Tokens["refresh_token"] == refresh)
							assert.Equal(t, 2, len(auth.Tokens))
						} else {
							assert.JSONEq(t, ref, string(body.Spec["provider_config"]))
							assert.JSONEq(t, `"usr_enrolled"`, string(body.Spec["user_id"]))
							assert.NotContains(t, body.Spec, "authorization")
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, response)
					})
					args := []string{"vaults", "wallets", "create", "checkout", "wallet-1", "--provider", provider}
					inputSpec := spec
					if selection != "spec" {
						value := "config-1"
						if selection == "name" {
							value = "checkout-client"
						}
						args = append(args, "--provider-config-"+selection, value)
						inputSpec = "{}"
						if provider == "agentcard" {
							inputSpec = `{"user_id":"usr_enrolled"}`
						}
					}
					args = append(args, "--spec", inputSpec)
					if provider == "link" {
						path := "-"
						if selection == "name" {
							path = filepath.Join(t.TempDir(), "grant.json")
							require.NoError(t, os.WriteFile(path, []byte(tokens), 0600))
						}
						args = append(args, "--tokens-file", path)
					}
					if output != "" {
						args = append(args, "-o", output)
					}
					out, human, err := executeVaultInputCommand(t, client, tokens, args...)
					require.NoError(t, err)
					assert.False(t, strings.Contains(out+human, access) || strings.Contains(out+human, refresh))
					assert.Contains(t, out+human, "config-1")
					if output == "json" {
						assert.Contains(t, out, `"provider_config"`)
						assert.Empty(t, human)
					} else {
						assert.Contains(t, human, "Provider config ID (immutable)")
					}
				})
			}
		}
	}
}

func TestVaultWalletConfigInvalidInput(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid config selection reached API") })
	for _, tc := range []struct {
		provider, spec string
		flags          []string
	}{
		{"agentcard", "{}", []string{"--provider-config-id", "config", "--provider-config-name", "name"}},
		{"agentcard", "{}", []string{"--provider-config-id="}},
		{"agentcard", "{}", []string{"--provider-config-name", "bad/name"}},
		{"agentcard", "{}", []string{"--tokens-file", "-"}},
		{"agentcard", `{"provider_config":{"id":"config"}}`, []string{"--provider-config-id", "config"}},
		{"agentcard", `{"provider_config":{"id":"config","name":"name"}}`, nil},
		{"agentcard", `{"provider_config":null}`, nil},
		{"agentcard", `{"provider_config":{"unknown":"config"}}`, nil},
		{"link", "{}", []string{"--tokens-file", "-"}},
		{"link", "{}", []string{"--provider-config-name", "config"}},
		{"link", linkWalletSpecFixture, []string{"--provider-config-id", "config", "--tokens-file", "-"}},
		{"link", `{"authorization":{"method":"oauth","client":{"type":"customer_managed"}}}`, []string{"--tokens-file", "-"}},
		{"link", `{"authorization":{"method":"other","client":{"type":"customer_managed","provider_config":{"id":"config"}}}}`, []string{"--tokens-file", "-"}},
	} {
		args := append([]string{"vaults", "wallets", "create", "checkout", "wallet-1", "--provider", tc.provider, "--spec", tc.spec}, tc.flags...)
		_, _, err := executeVaultInputCommand(t, client, "{}", args...)
		require.Error(t, err)
	}
	secret := vaultTestSecret(t)
	for _, raw := range []string{secret, `null`, `[]`, `{}`, `{"access_token":""}`, fmt.Sprintf(`{"client_id":"client-1","client_secret":%q}`, secret)} {
		out, human, err := executeVaultInputCommand(t, client, raw, "vaults", "wallets", "create", "checkout", "wallet-1", "--provider", "link", "--spec", "{}", "--provider-config-id", "config-1", "--tokens-file", "-")
		require.Error(t, err)
		assert.False(t, strings.Contains(out+human+err.Error(), secret))
	}
	for _, path := range []string{"wallets create", "cards create", "cards update"} {
		for _, raw := range []string{
			fmt.Sprintf(`{"authorization":{"tokens":{"access_token":%q}}}`, secret),
			fmt.Sprintf(`{"credentials":{"client_secret":%q}}`, secret),
			fmt.Sprintf(`{"authorization":{"client":{"client_secret":%q}}}`, secret),
			fmt.Sprintf(`{"provider_config":{"client_secret":%q}}`, secret),
		} {
			args := append([]string{"vaults"}, strings.Fields(path)...)
			args = append(args, "checkout", "item-1", "--provider", "link", "--spec", raw)
			out, human, err := executeVaultInputCommand(t, client, "", args...)
			require.Error(t, err)
			assert.False(t, strings.Contains(out+human+err.Error(), secret))
		}
	}
}

func TestVaultImportedWalletErrorSafety(t *testing.T) {
	for _, status := range []int{400, 403, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			secret := vaultTestSecret(t)
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = fmt.Fprintf(w, `{"message":%q}`, secret)
			})
			out, human, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"access_token":%q,"refresh_token":%q}`, secret, secret), "vaults", "wallets", "create", "checkout", "wallet-1", "--provider", "link", "--spec", "{}", "--provider-config-id", "config-1", "--tokens-file", "-")
			require.Error(t, err)
			assert.Equal(t, 1, calls)
			assert.False(t, strings.Contains(out+human+util.CleanedUpSdkError{Err: err}.Error(), secret))
		})
	}
}

func TestVaultRecoveryRequired(t *testing.T) {
	for _, provider := range []string{"link", "agentcard"} {
		body := strings.ReplaceAll(requestedCardFixture, `"status":"requested"`, `"status":"recovery_required"`)
		body = strings.ReplaceAll(body, `"provider":"link"`, `"provider":"`+provider+`"`)
		for _, operation := range []string{"get", "invoke"} {
			t.Run(provider+"/"+operation, func(t *testing.T) {
				calls := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, http.MethodGet, r.Method, "must not dispatch even if a stale operation is advertised")
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, body)
				})
				args := []string{"vaults", "items", operation, "checkout", "order-1"}
				if operation == "invoke" {
					args = append(args, "authorize")
				} else {
					args = append(args, "--wait", "60")
				}
				_, human, err := executeVaultInputCommand(t, client, "", args...)
				assert.Equal(t, 1, calls)
				if operation == "invoke" {
					require.ErrorContains(t, err, "recovery_required")
				} else {
					require.NoError(t, err)
					assert.Contains(t, human, "not declined or expired")
					assert.NotContains(t, human, "Invoke:")
					assert.NotContains(t, human, "Available operation:")
				}
			})
		}
	}
}

func TestVaultRecoveryDoesNotOpenStaleAction(t *testing.T) {
	body := strings.ReplaceAll(requestedCardFixture, `"status":"requested"`, `"status":"recovery_required"`)
	body = strings.TrimSuffix(body, "}") + `,"action":{"name":"spend_approval","url":"https://example.test/approve"}}`
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(body), &item))
	c := VaultsCmd{openURL: func(string) error { t.Error("recovery must not open an action"); return nil }}
	buf := capturePtermOutput(t)
	require.NoError(t, c.showItem(&item, "", true))
	assert.NotContains(t, buf.String(), "https://example.test/approve")
	assert.NotContains(t, buf.String(), "Required action")
	assert.NotContains(t, buf.String(), "spend_approval")
}

func TestVaultImportedWalletDegradedOutput(t *testing.T) {
	secret := vaultTestSecret(t)
	body := fmt.Sprintf(`{"id":"wallet-id","key":"wallet-1","type":"wallet","spec":{"provider":"link","authorization":{"method":"oauth","client":{"type":"customer_managed","provider_config":{"id":"config-1","client_secret":%q}},"tokens":{"access_token":%q,"refresh_token":%q}}},"state":{"provider":"link","status":"degraded"},"available_operations":[],"available_expansions":[]}`, secret, secret, secret)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	for _, output := range []string{"", "json"} {
		args := []string{"vaults", "items", "get", "checkout", "wallet-1"}
		if output != "" {
			args = append(args, "-o", output)
		}
		out, human, err := executeVaultInputCommand(t, client, "", args...)
		require.NoError(t, err)
		assert.False(t, strings.Contains(out+human, secret))
		assert.Contains(t, out+human, "config-1")
		if output == "" {
			assert.Contains(t, human, "no in-place reauthorization")
			assert.Contains(t, human, "new work only")
			assert.Contains(t, human, "Existing cards stay bound")
		}
	}
}

func TestVaultRecoveryEventProjection(t *testing.T) {
	secret := vaultTestSecret(t)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id":"event-1","name":"recovery_required","created_at":"2026-09-01T00:00:00Z","data":{"reason":"outcome_unknown","operation":"card_update","evidence_enc":%q}}]`, secret)
	})
	for _, output := range []string{"", "json"} {
		args := []string{"vaults", "items", "events", "checkout", "order-1"}
		if output != "" {
			args = append(args, "-o", output)
		}
		out, human, err := executeVaultInputCommand(t, client, "", args...)
		require.NoError(t, err)
		require.False(t, strings.Contains(out+human, secret))
		assert.Contains(t, out+human, "card_update")
		assert.Contains(t, out+human, "outcome_unknown")
	}
}

func TestVaultPendingUpdatePreservesOmissionsAndEmptyLists(t *testing.T) {
	for _, fields := range []string{"", `,"line_items":[],"totals":[]`} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPatch, r.Method)
			body, _ := io.ReadAll(r.Body)
			assert.JSONEq(t, `{"spec":{"provider":"link","wallet":"wallet-1","amount":2000`+fields+`}}`, string(body))
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, strings.ReplaceAll(requestedCardFixture, "requested", "recovery_required"))
		})
		out, _, err := executeVaultInputCommand(t, client, "", "vaults", "cards", "update", "checkout", "order-1", "--provider", "link", "--spec", `{"wallet":"wallet-1","amount":2000`+fields+`}`, "-o", "json")
		require.NoError(t, err)
		assert.Contains(t, out, "recovery_required")
	}
}
