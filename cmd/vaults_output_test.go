package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readyAgentCardFixture = `{
  "id":"card-id","key":"order-1","type":"card",
  "spec":{"provider":"agentcard","wallet":"wallet-1","amount":1234,"currency":"usd","merchant":"Example Shop","provider_secret":"SECRET_SPEC"},
  "state":{"provider":"agentcard","status":"ready","aliases":{"number":"9999999999999999","cvc":"999","exp_month":"01","exp_year":"2099","secret":"SECRET_ALIAS"},"card_number":"SECRET_CARD","secret_enc":"SECRET_CIPHERTEXT"},
  "available_operations":[],"available_expansions":[],"oauth_tokens":"SECRET_OAUTH"
}`

const readyLinkCardFixture = `{
  "id":"card-id","key":"order-1","type":"card",
  "spec":{"provider":"link","wallet":"wallet-1","payment_method_id":"pm-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example","provider_secret":"SECRET_SPEC"},
  "state":{"provider":"link","status":"ready","domains":["shop.example"],"masks":{"brand":"visa","last4":"1234"},"card_number":"SECRET_CARD","secret_enc":"SECRET_CIPHERTEXT"},
  "available_operations":[{"type":"fill","description":"Fill checkout fields."}],"available_expansions":[],"oauth_tokens":"SECRET_OAUTH"
}`

func TestVaultOutputAgentCardAliasesPresenceAndRedaction(t *testing.T) {
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(readyAgentCardFixture), &item))
	buf := capturePtermOutput(t)
	require.NoError(t, printVaultItem(&item, ""))
	human := buf.String()
	for _, value := range []string{"9999999999999999", "999", "01", "2099"} {
		assert.Contains(t, human, value)
	}
	for _, field := range []string{"number", "cvc", "exp_month", "exp_year"} {
		assert.Contains(t, human, "checkout alias: "+field)
	}
	assert.Contains(t, human, "ready does not mean paid")
	assert.Contains(t, human, "never retry automatically")
	assert.Contains(t, human, "explicit user confirmation")
	assert.NotContains(t, human, "SECRET")
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, "SECRET")
	var decoded struct {
		State vaultJSON `json:"state"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	assert.JSONEq(t, `{"number":"9999999999999999","cvc":"999","exp_month":"01","exp_year":"2099"}`, string(decoded.State["aliases"]))
	assert.NotContains(t, out, "authorization")
	assert.NotContains(t, out, "expires_at")

	withoutAliases := `{"id":"card-id","key":"order-1","type":"card","spec":{"provider":"agentcard"},"state":{"provider":"agentcard","status":"requested"},"available_operations":[]}`
	require.NoError(t, json.Unmarshal([]byte(withoutAliases), &item))
	buf.Reset()
	require.NoError(t, printVaultItem(&item, ""))
	assert.NotContains(t, buf.String(), "checkout alias")
	out = captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, "aliases")

	nullAliases := strings.Replace(withoutAliases, `"status":"requested"`, `"status":"requested","aliases":null`, 1)
	require.NoError(t, json.Unmarshal([]byte(nullAliases), &item))
	buf.Reset()
	require.NoError(t, printVaultItem(&item, ""))
	assert.NotContains(t, buf.String(), "checkout alias")
	out = captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.Contains(t, out, `"aliases": null`)
}

func TestVaultOutputLinkHasNoAliases(t *testing.T) {
	for _, fixture := range []string{requestedCardFixture, readyLinkCardFixture} {
		var item kernel.VaultItemUnion
		require.NoError(t, json.Unmarshal([]byte(fixture), &item))
		t.Run(item.State.Status, func(t *testing.T) {
			buf := capturePtermOutput(t)
			require.NoError(t, printVaultItem(&item, ""))
			assert.NotContains(t, buf.String(), "alias")
			assert.NotContains(t, buf.String(), "SECRET")
			operation := "authorize"
			if item.State.Status == "ready" {
				operation = "fill"
				assert.Contains(t, buf.String(), "permitted domains (provider-assigned)")
				assert.Contains(t, buf.String(), "shop.example")
			}
			assert.Contains(t, buf.String(), "available operation: "+operation)
			out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
			assert.NotContains(t, out, "aliases")
			assert.NotContains(t, out, "SECRET")
			var decoded struct {
				State vaultJSON `json:"state"`
			}
			require.NoError(t, json.Unmarshal([]byte(out), &decoded))
			assert.JSONEq(t, fmt.Sprintf("%q", item.State.Status), string(decoded.State["status"]))
			if item.State.Status == "ready" {
				assert.JSONEq(t, `["shop.example"]`, string(decoded.State["domains"]))
				assert.JSONEq(t, `{"brand":"visa","last4":"1234"}`, string(decoded.State["masks"]))
			}
		})
	}
}

func TestVaultOutputKernelCardMerchantCountry(t *testing.T) {
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"card-id","key":"order-1","type":"card",
		"spec":{"provider":"kernel","wallet":"wallet-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout","merchant_country":"US"},
		"state":{"provider":"kernel","status":"pending_authorization"},
		"available_operations":[],"available_expansions":[]
	}`), &item))
	buf := capturePtermOutput(t)
	require.NoError(t, printVaultItem(&item, ""))
	assert.Contains(t, buf.String(), "merchant country")
	assert.Contains(t, buf.String(), "US")
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	var decoded struct {
		Spec vaultJSON `json:"spec"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	assert.JSONEq(t, `"US"`, string(decoded.Spec["merchant_country"]))
}

func TestVaultOutputKernelPurchaseFunding(t *testing.T) {
	for _, tc := range []struct {
		state, want string
	}{
		{`"status":"ready"`, ""},
		{`"status":"ready","payment_method_id":"card","funding_status":"available"`, ""},
		{`"status":"ready","payment_method_id":"pm-2","funding_status":"unavailable"`, "funding_status unavailable"},
		{`"status":"consumed","payment_method_id":"pm-2","funding_status":"removed"`, "funding_status removed"},
	} {
		var item kernel.VaultItemUnion
		require.NoError(t, json.Unmarshal([]byte(`{
			"id":"card-id","key":"order-1","type":"card",
			"spec":{"provider":"kernel","wallet":"wallet-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example/checkout"},
			"state":{"provider":"kernel",`+tc.state+`},
			"available_operations":[],"available_expansions":[]
		}`), &item))
		buf := capturePtermOutput(t)
		require.NoError(t, printVaultItem(&item, ""))
		if tc.want == "" {
			assert.NotContains(t, buf.String(), "funding_status")
		} else {
			assert.Contains(t, buf.String(), tc.want)
			assert.Contains(t, buf.String(), "pm-2")
		}
		out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
		var decoded struct {
			State vaultJSON `json:"state"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &decoded))
		if strings.Contains(tc.state, "funding_status") {
			assert.NotEmpty(t, decoded.State["funding_status"])
			assert.NotEmpty(t, decoded.State["payment_method_id"])
		} else {
			assert.NotContains(t, decoded.State, "funding_status")
			assert.NotContains(t, buf.String(), "payment method id")
		}
	}
}

func TestVaultOutputAgentCardAuthorizationIsNotPaymentSuccess(t *testing.T) {
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"card-id","key":"order-1","type":"card",
		"spec":{"provider":"agentcard","wallet":"wallet-1","amount":1234,"currency":"usd","merchant":"Example Shop"},
		"state":{"provider":"agentcard","status":"ready","authorization":{"id":"cauth_test","status":"declined","psp":"stripe","merchant":"Example Shop","amount_cents":1234,"currency":"usd","reason":"expired","charged_kind":"none","replay_delivered":false,"raw_response":"SECRET_RESPONSE"}},
		"available_operations":[],"available_expansions":[]
	}`), &item))
	buf := capturePtermOutput(t)
	require.NoError(t, printVaultItem(&item, ""))
	for _, text := range []string{"authorization status", "declined", "expired", "charged kind", "none", "processor response delivered", "false"} {
		assert.Contains(t, buf.String(), text)
	}
	assert.NotContains(t, buf.String(), "SECRET_RESPONSE")
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, `"test"`)
	assert.NotContains(t, out, "SECRET_RESPONSE")
}

func TestVaultOutputOmitsCardTestMode(t *testing.T) {
	for _, provider := range []string{"link", "agentcard"} {
		t.Run(provider, func(t *testing.T) {
			body := strings.ReplaceAll(requestedCardFixture, `"provider":"link"`, `"provider":"`+provider+`"`)
			body = strings.ReplaceAll(body, `"amount":1234`, `"amount":1234,"test":true`)
			var item kernel.VaultItemUnion
			require.NoError(t, json.Unmarshal([]byte(body), &item))
			buf := capturePtermOutput(t)
			require.NoError(t, printVaultItem(&item, ""))
			assert.NotContains(t, buf.String(), "Test")
			assert.NotContains(t, buf.String(), "Mode")
			out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
			assert.NotContains(t, out, `"test"`)
		})
	}
}

func TestVaultOutputPaymentMethodsAdvisoryUnknownVsFalse(t *testing.T) {
	body := strings.TrimSuffix(connectedWalletFixture, "}") + `,"expanded":{"payment_methods":[
		{"id":"pm-unknown","provider":"link","type":"card","is_default":true,"display":{"label":"Personal","brand":"visa","last4":"1234"},"capabilities":{},"provider_secret":"SECRET_METHOD"},
		{"id":"pm-ineligible","provider":"link","type":"card","is_default":false,"display":{},"capabilities":{"single_use_card":{"eligible":false,"reasons":["not_supported"]}}}
	]}}`
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(body), &item))
	buf := capturePtermOutput(t)
	require.NoError(t, printVaultItem(&item, ""))
	for _, text := range []string{"payment method id", "pm-unknown", "unknown", "pm-ineligible", "false", "not_supported", "payment_method_id", "--spec json", "advisory"} {
		assert.Contains(t, buf.String(), text)
	}
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, "SECRET_METHOD")
	assert.Contains(t, out, `"capabilities": {}`)
}

func TestVaultOutputLinkRecurringTerms(t *testing.T) {
	body := strings.Replace(readyLinkCardFixture, `"merchant_url":"https://shop.example"`, `"merchant_url":"https://shop.example","recurring":{"interval":"week","interval_count":2,"provider_secret":"SECRET_RECURRING"}`, 1)
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(body), &item))
	buf := capturePtermOutput(t)
	require.NoError(t, printVaultItem(&item, ""))
	for _, text := range []string{"recurring interval", "week", "recurring interval count", "2", "merchant-scheduled charges", "do not create a new card each period", "does not cancel the merchant subscription"} {
		assert.Contains(t, buf.String(), text)
	}
	assert.NotContains(t, buf.String(), "SECRET_RECURRING")
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.Contains(t, out, `"recurring"`)
	assert.Contains(t, out, `"interval": "week"`)
	assert.Contains(t, out, `"interval_count": 2`)
	assert.NotContains(t, out, "SECRET_RECURRING")
}

func TestVaultOutputLinkLiveExpansions(t *testing.T) {
	body := strings.TrimSuffix(connectedWalletFixture, "}") + `,"expanded":{
		"link_user_info":{"id":"usr_1","email":"jane@example.com","name":"Jane Doe","first_name":"Jane","last_name":"Doe","phone":null,"address":{"line1":"510 Townsend St","line2":null,"city":"San Francisco","state":"CA","postal_code":"94103","country":"US","provider_secret":"SECRET_ADDRESS"},"eligible_for_balance":true,"default_spend_request_currency":"usd","agent_wallet_spend_limits":{"per_transaction":{"limit":50000},"daily":{"limit":500000,"used":120000,"remaining":380000},"thirty_day":{"limit":null,"used":600000,"remaining":null}},"agent_wallet_step_up":{"status":"identity_verification","action_url":"https://link.example/verify?code=SECRET_STEPUP"},"provider_secret":"SECRET_USER"},
		"link_shipping_addresses":[{"id":"sa_1","is_default":true,"nickname":null,"address":{"name":"Jane Doe","line_1":"510 Townsend St","line_2":null,"locality":"San Francisco","dependent_locality":null,"administrative_area":"CA","postal_code":"94103","sorting_code":null,"country_code":"US","provider_secret":"SECRET_SHIPPING"},"provider_secret":"SECRET_ENTRY"}]
	}}`
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(body), &item))
	buf := capturePtermOutput(t)
	require.NoError(t, printVaultItem(&item, ""))
	for _, text := range []string{"Link user property", "Jane Doe", "jane@example.com", "eligible for balance", "per-transaction limit", "50000", "30-day limit", "unlimited", "shipping address id", "sa_1", "510 Townsend St", "live provider data"} {
		assert.Contains(t, buf.String(), text)
	}
	assert.NotContains(t, buf.String(), "SECRET")
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	for _, text := range []string{`"link_user_info"`, `"link_shipping_addresses"`, `"phone": null`, `"nickname": null`, `"limit": null`} {
		assert.Contains(t, out, text)
	}
	assert.NotContains(t, out, "SECRET")
}

func TestVaultActionOutputNoInventedURLs(t *testing.T) {
	for _, action := range []struct{ name, url string }{
		{"link_oauth", "https://provider.example/auth?state=state&code_challenge=challenge"},
		{"spend_approval", "https://provider.example/approve"},
		{"card_enrollment", "https://provider.example/enroll"},
		{"collect", ""}, {"mfa", ""}, {"push_approval", ""}, {"embedded_ceremony", ""},
	} {
		t.Run(action.name, func(t *testing.T) {
			var item kernel.VaultItemUnion
			body := strings.TrimSuffix(connectedWalletFixture, "}") + fmt.Sprintf(`,"action":{"name":%q`, action.name)
			if action.url != "" {
				body += fmt.Sprintf(`,"url":%q`, action.url)
			}
			body += "}}"
			require.NoError(t, json.Unmarshal([]byte(body), &item))
			buf := capturePtermOutput(t)
			require.NoError(t, printVaultItem(&item, ""))
			assert.Contains(t, buf.String(), action.name)
			if action.url != "" {
				assert.Contains(t, buf.String(), action.url)
			} else {
				assert.NotContains(t, buf.String(), "action url")
			}
		})
	}
}

func TestVaultLongURLsPrintedOutsideTable(t *testing.T) {
	actionURL := "https://provider.example/auth?state=" + strings.Repeat("a", 512) + "&code_challenge=complete-challenge"
	approvalURL := "https://provider.example/approve?request=" + strings.Repeat("b", 512)
	for _, tt := range []struct {
		name, body, label, url string
	}{
		{"action", strings.TrimSuffix(connectedWalletFixture, "}") + fmt.Sprintf(`,"action":{"name":"link_oauth","url":%q}}`, actionURL), "action url", actionURL},
		{"approval", fmt.Sprintf(`{"key":"order-1","type":"card","spec":{"provider":"agentcard"},"state":{"provider":"agentcard","status":"ready","authorization":{"status":"pending","approval_url":%q}}}`, approvalURL), "approval url", approvalURL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var item kernel.VaultItemUnion
			require.NoError(t, json.Unmarshal([]byte(tt.body), &item))
			buf := capturePtermOutput(t)
			require.NoError(t, printVaultItem(&item, ""))
			assert.Contains(t, buf.String(), tt.label+":\n"+tt.url+"\n")
			assert.Equal(t, 1, strings.Count(buf.String(), tt.url))
			out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
			var decoded kernel.VaultItemUnion
			require.NoError(t, json.Unmarshal([]byte(out), &decoded))
			if tt.name == "action" {
				assert.Equal(t, tt.url, decoded.Action.URL)
			} else {
				assert.Equal(t, tt.url, decoded.State.Authorization.ApprovalURL)
			}
		})
	}
}

func TestVaultURLsWithSecretsAreWithheld(t *testing.T) {
	for _, address := range []string{
		"https://user:SECRET@provider.example/", "https://provider.example/?code=SECRET",
		"https://provider.example/#access_token=SECRET", "javascript:SECRET",
	} {
		var item kernel.VaultItemUnion
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(connectedWalletFixture, "}")+fmt.Sprintf(`,"action":{"name":"link_oauth","url":%q}}`, address)), &item))
		buf := capturePtermOutput(t)
		require.NoError(t, printVaultItem(&item, ""))
		assert.NotContains(t, buf.String(), "SECRET")
		out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
		assert.NotContains(t, out, "SECRET")
	}
}

func TestVaultGetCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cancel()
		<-r.Context().Done()
	})
	c := VaultsCmd{vaults: &client.Vaults}
	err := c.GetItem(ctx, "checkout", "order-1", 60, nil, "", "json", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), calls.Load())
}
