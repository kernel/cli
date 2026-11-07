package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const vgsWalletFixture = `{"id":"wallet-vgs-id","key":"vgs-wallet","type":"wallet","spec":{"provider":"vgs","provider_config":{"id":"config-vgs"}},"state":{"provider":"vgs","status":"connected"},"available_operations":[],"available_expansions":[]}`

const vgsRequestedCardFixture = `{"id":"card-vgs-id","key":"vgs-order","type":"card","spec":{"provider":"vgs","wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout"},"state":{"provider":"vgs","status":"requested"},"available_operations":[{"type":"authorize","description":"Reveal the VGS aliases through the outbound proxy."}],"available_expansions":[]}`

const vgsReadyCardFixture = `{"id":"card-vgs-id","key":"vgs-order","type":"card","spec":{"provider":"vgs","wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout"},"state":{"provider":"vgs","status":"ready","masks":{"last4":"4242","brand":"visa"}},"expires_at":"2026-09-01T00:05:00Z","permitted_domains":["https://www.example.com"],"available_operations":[{"type":"fill","description":"Fill this VGS card before it expires."}],"available_expansions":[]}`

func TestVaultVgsWalletConfigSelection(t *testing.T) {
	for _, selection := range []string{"id", "name", "spec"} {
		t.Run(selection, func(t *testing.T) {
			ref := `{"id":"config-vgs"}`
			if selection == "name" {
				ref = `{"name":"customer-vault"}`
			}
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/vaults/checkout/items/vgs-wallet", r.URL.Path)
				var body struct {
					Type string `json:"type"`
					Spec struct {
						Provider       string          `json:"provider"`
						ProviderConfig json.RawMessage `json:"provider_config"`
					} `json:"spec"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "wallet", body.Type)
				assert.Equal(t, "vgs", body.Spec.Provider)
				assert.JSONEq(t, ref, string(body.Spec.ProviderConfig))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, vgsWalletFixture)
			})

			args := []string{"vaults", "wallets", "create", "checkout", "vgs-wallet", "--provider", "vgs"}
			if selection == "spec" {
				args = append(args, "--spec", `{"provider_config":`+ref+`}`)
			} else {
				value := "config-vgs"
				if selection == "name" {
					value = "customer-vault"
				}
				args = append(args, "--spec", "{}", "--provider-config-"+selection, value)
			}
			args = append(args, "-o", "json")
			out, human, err := executeVaultInputCommand(t, client, "", args...)
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.Empty(t, human)
			assert.Contains(t, out, `"provider": "vgs"`)
			assert.Contains(t, out, `"provider_config"`)
		})
	}
}

func TestVaultVgsWalletRequiresConfigAndRejectsTokens(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid VGS wallet reached API") })
	for _, args := range [][]string{
		{"vaults", "wallets", "create", "checkout", "vgs-wallet", "--provider", "vgs", "--spec", "{}"},
		{"vaults", "wallets", "create", "checkout", "vgs-wallet", "--provider", "vgs", "--spec", `{"provider_config":{"id":"config-vgs"}}`, "--tokens-file", "-"},
		{"vaults", "wallets", "create", "checkout", "vgs-wallet", "--provider", "vgs", "--spec", `{"provider_config":{"id":"config-vgs"}}`, "--provider-config-name", "customer-vault"},
	} {
		_, _, err := executeVaultInputCommand(t, client, `{}`, args...)
		require.Error(t, err)
	}
}

func TestVaultVgsCardCreateUsesProtectedAliases(t *testing.T) {
	panAlias, cvcAlias := vaultTestSecret(t), vaultTestSecret(t)
	input := fmt.Sprintf(`{"wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","card":{"pan_alias":%q,"cvc_alias":%q,"exp_month":11,"exp_year":2030,"last4":"4242"}}`, panAlias, cvcAlias)
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/vaults/checkout/items/vgs-order", r.URL.Path)
		var body struct {
			Type string `json:"type"`
			Spec struct {
				Provider string `json:"provider"`
				Card     struct {
					PanAlias string `json:"pan_alias"`
					CvcAlias string `json:"cvc_alias"`
					ExpMonth int64  `json:"exp_month"`
					ExpYear  int64  `json:"exp_year"`
					Last4    string `json:"last4"`
				} `json:"card"`
			} `json:"spec"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "card", body.Type)
		assert.Equal(t, "vgs", body.Spec.Provider)
		assert.Equal(t, panAlias, body.Spec.Card.PanAlias)
		assert.Equal(t, cvcAlias, body.Spec.Card.CvcAlias)
		assert.Equal(t, int64(11), body.Spec.Card.ExpMonth)
		assert.Equal(t, int64(2030), body.Spec.Card.ExpYear)
		assert.Equal(t, "4242", body.Spec.Card.Last4)
		w.Header().Set("Content-Type", "application/json")
		// Deliberately echo write-only aliases to verify display-safe filtering.
		response := strings.Replace(vgsRequestedCardFixture, `"merchant_url":"https://www.example.com/checkout"`, fmt.Sprintf(`"merchant_url":"https://www.example.com/checkout","card":{"pan_alias":%q,"cvc_alias":%q,"exp_month":11,"exp_year":2030,"last4":"4242"}`, panAlias, cvcAlias), 1)
		_, _ = io.WriteString(w, response)
	})

	out, human, err := executeVaultInputCommand(t, client, input, "vaults", "cards", "create", "checkout", "vgs-order", "--provider", "vgs", "--spec-file", "-", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Empty(t, human)
	assert.Contains(t, out, `"provider": "vgs"`)
	assert.NotContains(t, out, "pan_alias")
	assert.NotContains(t, out, "cvc_alias")
	assert.NotContains(t, out, panAlias)
	assert.NotContains(t, out, cvcAlias)
}

// A card the merchant holds on file sends card_mode and the CVC alias alone.
// The CLI forwards the spec as written, so it must not add the number fields
// the API rejects on a cvc_only card, even as empty values.
func TestVaultVgsCVCOnlyCardSendsOnlyTheCVCAlias(t *testing.T) {
	cvcAlias := vaultTestSecret(t)
	input := fmt.Sprintf(`{"wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","card_mode":"cvc_only","card":{"cvc_alias":%q}}`, cvcAlias)
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Spec struct {
				CardMode string                     `json:"card_mode"`
				Card     map[string]json.RawMessage `json:"card"`
			} `json:"spec"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "cvc_only", body.Spec.CardMode)
		assert.Equal(t, map[string]json.RawMessage{"cvc_alias": json.RawMessage(fmt.Sprintf("%q", cvcAlias))}, body.Spec.Card)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Replace(vgsRequestedCardFixture, `"merchant_url":"https://www.example.com/checkout"`, `"merchant_url":"https://www.example.com/checkout","card_mode":"cvc_only"`, 1))
	})

	out, _, err := executeVaultInputCommand(t, client, input, "vaults", "cards", "create", "checkout", "vgs-order", "--provider", "vgs", "--spec-file", "-", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Contains(t, out, `"card_mode": "cvc_only"`)
	assert.NotContains(t, out, cvcAlias)
}

func TestVaultVgsCardRejectsInlineAliasesAndUpdates(t *testing.T) {
	input := `{"wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","card":{"pan_alias":"tok_pan","cvc_alias":"tok_cvc","exp_month":11,"exp_year":2030,"last4":"4242"}}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unsafe VGS card request reached API") })

	_, _, err := executeVaultInputCommand(t, client, "", "vaults", "cards", "create", "checkout", "vgs-order", "--provider", "vgs", "--spec", input)
	require.ErrorContains(t, err, "--spec-file")

	_, _, err = executeVaultInputCommand(t, client, "", "vaults", "cards", "update", "checkout", "vgs-order", "--provider", "vgs", "--spec", `{"wallet":"vgs-wallet"}`)
	require.ErrorContains(t, err, "VGS card updates are not supported")
}

func TestVaultVgsCardErrorDoesNotExposeAliases(t *testing.T) {
	secret := vaultTestSecret(t)
	input := fmt.Sprintf(`{"wallet":"vgs-wallet","amount":4999,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://www.example.com/checkout","card":{"pan_alias":%q,"cvc_alias":%q,"exp_month":11,"exp_year":2030,"last4":"4242"}}`, secret, secret)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"code":"invalid_alias","message":%q}`, secret)
	})
	out, human, err := executeVaultInputCommand(t, client, input, "vaults", "cards", "create", "checkout", "vgs-order", "--provider", "vgs", "--spec-file", "-")
	require.Error(t, err)
	assert.NotContains(t, out+human+util.CleanedUpSdkError{Err: err}.Error(), secret)
}

func TestVaultVgsDeclineGuidance(t *testing.T) {
	for reason, want := range map[string]string{
		"invalid_card":                  "aliases that do not match the card",
		"alias_not_found":               "collect the CVC again",
		"provider_credentials_rejected": "rotate its client secret",
	} {
		t.Run(reason, func(t *testing.T) {
			body := strings.Replace(vgsRequestedCardFixture, `"status":"requested"`, `"status":"declined","status_reason":"`+reason+`"`, 1)
			body = strings.Replace(body, `"available_operations":[{"type":"authorize","description":"Reveal the VGS aliases through the outbound proxy."}]`, `"available_operations":[]`, 1)
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			})
			_, human, err := executeVaultInputCommand(t, client, "", "vaults", "items", "get", "checkout", "vgs-order")
			require.NoError(t, err)
			assert.Contains(t, human, want)
			assert.Contains(t, human, "status reason")
			assert.Contains(t, human, reason)
		})
	}
}

func TestVaultVgsAuthorizeAndReadyGuidance(t *testing.T) {
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			assert.Equal(t, http.MethodGet, r.Method)
			_, _ = io.WriteString(w, vgsRequestedCardFixture)
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/vaults/checkout/items/vgs-order/operations", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"authorize"}`, string(body))
		_, _ = io.WriteString(w, vgsReadyCardFixture)
	})

	out, human, err := executeVaultInputCommand(t, client, "", "vaults", "items", "invoke", "checkout", "vgs-order", "authorize")
	require.NoError(t, err)
	assert.Empty(t, out)
	assert.Equal(t, 2, calls)
	assert.Contains(t, human, "VGS proxy replaces them with card details")
	assert.Contains(t, human, "five minutes after authorize")
	assert.Contains(t, human, "merchant url")
	assert.Contains(t, human, "https://www.example.com/checkout")
	assert.Contains(t, human, "card last4")
	assert.Contains(t, human, "4242")
}
