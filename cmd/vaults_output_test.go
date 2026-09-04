package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readyCardFixture = `{
  "id":"card-id","key":"order-1","type":"card",
  "spec":{"provider":"link","wallet":"wallet-1","payment_method_id":"pm-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example","test":true,"provider_secret":"SECRET_SPEC"},
  "state":{"provider":"link","status":"ready","domains":["shop.example"],"aliases":{"number":"9999999999999999","cvc":"999","exp_month":"01","exp_year":"2099","secret":"SECRET_ALIAS"},"card_number":"SECRET_CARD","secret_enc":"SECRET_CIPHERTEXT"},
  "available_operations":[],"available_expansions":[],"oauth_tokens":"SECRET_OAUTH"
}`

func TestVaultOutputAliasesPresenceAndRedaction(t *testing.T) {
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(readyCardFixture), &item))
	buf := capturePtermOutput(t)
	require.NoError(t, printVaultItem(&item, ""))
	human := buf.String()
	assert.Contains(t, human, "9999999999999999")
	assert.Contains(t, human, "Checkout alias: cvc")
	assert.Contains(t, human, "Permitted domains (provider-assigned)")
	assert.Contains(t, human, "shop.example")
	assert.Contains(t, human, "ready does not mean paid")
	assert.Contains(t, human, "Do not retry")
	assert.NotContains(t, human, "SECRET")
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, "SECRET")
	assert.Contains(t, out, "9999999999999999")
	assert.NotContains(t, out, "authorization")
	assert.NotContains(t, out, "expires_at")

	require.NoError(t, json.Unmarshal([]byte(requestedCardFixture), &item))
	buf.Reset()
	require.NoError(t, printVaultItem(&item, ""))
	assert.NotContains(t, buf.String(), "Checkout alias")
	assert.Contains(t, buf.String(), "Available operation: authorize")
	out = captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, "aliases")

	nullAliases := strings.Replace(requestedCardFixture, `"status":"requested"`, `"status":"requested","aliases":null`, 1)
	require.NoError(t, json.Unmarshal([]byte(nullAliases), &item))
	buf.Reset()
	require.NoError(t, printVaultItem(&item, ""))
	assert.NotContains(t, buf.String(), "Checkout alias")
	out = captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.Contains(t, out, `"aliases": null`)
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
	for _, text := range []string{"Deployment-controlled", "Authorization status", "declined", "expired", "Charged kind", "none", "Processor response delivered", "false"} {
		assert.Contains(t, buf.String(), text)
	}
	assert.NotContains(t, buf.String(), "SECRET_RESPONSE")
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, `"test"`)
	assert.NotContains(t, out, "SECRET_RESPONSE")
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
	for _, text := range []string{"Payment method ID", "pm-unknown", "unknown", "pm-ineligible", "false", "not_supported", "--payment-method-id", "advisory"} {
		assert.Contains(t, buf.String(), text)
	}
	out := captureStdout(t, func() { require.NoError(t, printVaultItem(&item, "json")) })
	assert.NotContains(t, out, "SECRET_METHOD")
	assert.Contains(t, out, `"capabilities": {}`)
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
				assert.NotContains(t, buf.String(), "Action URL")
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

func TestVaultCancellationDoesNotLeakTransportDetails(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("SECRET_TRANSPORT")} {
		err := vaultRequestError(cause)
		require.Error(t, err)
		assert.NotContains(t, util.CleanedUpSdkError{Err: err}.Error(), "SECRET")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cancel()
		<-r.Context().Done()
	})
	c := VaultsCmd{vaults: &client.Vaults}
	err := c.GetItem(ctx, "checkout", "order-1", 60, nil, "json", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interrupted")
	assert.Equal(t, int32(1), calls.Load())
}
