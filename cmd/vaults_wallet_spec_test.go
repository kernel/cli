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

func TestVaultSpecsPreserveOpaqueMetadata(t *testing.T) {
	metadata := `{"tokens":"loyalty-points","credentials":{"label":"member"},"client_secret":"field-description","entries":[{"access_token":"column-name"}]}`
	for _, provider := range []string{"link", "agentcard"} {
		for _, command := range []string{"wallets create", "cards create"} {
			t.Run(provider+"/"+command, func(t *testing.T) {
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					var body struct {
						Spec map[string]json.RawMessage `json:"spec"`
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					assert.JSONEq(t, metadata, string(body.Spec["metadata"]))
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, requestedCardFixture)
				})
				args := append([]string{"vaults"}, strings.Fields(command)...)
				args = append(args, "checkout", "item-1", "--provider", provider, "--spec", `{"metadata":`+metadata+`}`, "-o", "json")
				_, _, err := executeVaultInputCommand(t, client, "", args...)
				require.NoError(t, err)
			})
		}
	}
}

func TestVaultImportedAuthorizationPreservesRawFields(t *testing.T) {
	for _, fields := range []string{"", `,"optional":null,"options":[],"enabled":false,"sequence":9223372036854775807`} {
		t.Run(fields, func(t *testing.T) {
			secret := vaultTestSecret(t)
			authorization := `{"method":"oauth","client":{"type":"customer_managed","provider_config":{"name":"checkout-client"},"custom_option":null}` + fields + `}`
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Spec map[string]json.RawMessage `json:"spec"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.JSONEq(t, `{"tokens":"loyalty-points"}`, string(body.Spec["metadata"]))
				var auth map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body.Spec["authorization"], &auth))
				var tokens vaultLinkTokens
				require.NoError(t, json.Unmarshal(auth["tokens"], &tokens))
				assert.True(t, tokens.AccessToken == secret && tokens.RefreshToken == secret)
				delete(auth, "tokens")
				raw, err := json.Marshal(auth)
				require.NoError(t, err)
				assert.JSONEq(t, authorization, string(raw))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, connectedWalletFixture)
			})
			_, _, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"access_token":%q,"refresh_token":%q}`, secret, secret), "vaults", "wallets", "create", "checkout", "wallet-1", "--provider", "link", "--tokens-file", "-", "--spec", `{"authorization":`+authorization+`,"metadata":{"tokens":"loyalty-points"}}`, "-o", "json")
			require.NoError(t, err)
		})
	}
}
