package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const linkPayTokenFillParams = `{"browser_id":"browser-1","page_url":"https://shop.example/checkout"}`

func TestLinkCardFlowHelpIsShared(t *testing.T) {
	flow := strings.Join(strings.Fields(linkCardFlowHelp), " ")
	for _, path := range [][]string{nil, {"wallets", "create"}, {"cards", "create"}, {"cards", "update"}, {"items", "invoke"}} {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			cmd, _, err := newVaultsCommand().Find(path)
			require.NoError(t, err)
			assert.Contains(t, strings.Join(strings.Fields(cmd.Long), " "), flow)
		})
	}
}

func TestLinkCardFlowHelpOrder(t *testing.T) {
	flow := strings.Join(strings.Fields(linkCardFlowHelp), " ")
	last := -1
	for _, step := range []string{
		"1. reach the final checkout page",
		"2. create the card with cards create --provider link",
		"3. authorize with items invoke <vault> <key> authorize --browser-id <browser-session-id> --page-url",
		"4. share the approval url from the item's action",
		"5. fill as the item's fill description says",
		"6. submit the checkout yourself; fill never submits payment",
	} {
		index := strings.Index(flow, step)
		require.Greater(t, index, last, step)
		last = index
	}
	for _, want := range []string{
		"a Link Pay Token or a virtual card",
		"a Link Pay Token takes browser_id and page_url and omits fields",
		"page_not_found, ambiguous_page, browser_not_found, browser_unavailable, destination_denied, timeout, or browser_error",
		"make sure the checkout is open in exactly one tab at page_url, then authorize again",
		"authorize without --browser-id and --page-url to issue a virtual card",
		"cards update returns 409: delete the card and create a new one",
		"50000 minor-unit virtual card limit",
		"check the merchant's order confirmation or order history before paying again",
		"failed or unknown fills are safe to retry: check the page first, and never retry in an automatic loop",
	} {
		assert.Contains(t, flow, want)
	}
}

func TestVaultHelpAvoidsRetiredLinkWording(t *testing.T) {
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		text := strings.ToLower(strings.Join([]string{cmd.Short, cmd.Long, cmd.Example, cmd.Flags().FlagUsages()}, " "))
		for _, banned := range []string{"execution method", "execution mode", "credential type", "merchant_account", "authorize and restart_verification take no parameters"} {
			assert.NotContains(t, text, banned, cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(newVaultsCommand())
	assert.NotContains(t, strings.ToLower(linkCardFlowHelp), "never retry")

	update, _, err := newVaultsCommand().Find([]string{"cards", "update"})
	require.NoError(t, err)
	assert.Equal(t, "update a card spec before it is authorized at a checkout", update.Short)
	assert.Contains(t, strings.Join(strings.Fields(update.Long), " "), "a Link card authorized at a checkout cannot be updated: delete it and create a new one")
}

func TestVaultLinkAuthorizeSendsCheckout(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		name string
		args []string
		body string
	}{
		{"checkout", []string{"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout?step=pay#card"}, `{"type":"authorize","browser_id":"browser-1","page_url":"https://shop.example/checkout?step=pay#card"}`},
		{"without checkout", nil, `{"type":"authorize"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method == http.MethodPost {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, tc.body, string(body))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, requestedCardFixture)
			})
			args := append([]string{"vaults", "items", "invoke", "checkout", "order-1", "authorize", "-o", "json"}, tc.args...)
			_, _, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
		})
	}
}

func TestVaultLinkAuthorizeCheckoutValidation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid authorize reached the API") })
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"browser only":     {[]string{"authorize", "--browser-id", "browser-1"}, "must be passed together"},
		"page only":        {[]string{"authorize", "--page-url", "https://shop.example/checkout"}, "must be passed together"},
		"blank browser":    {[]string{"authorize", "--browser-id", " ", "--page-url", "https://shop.example/checkout"}, "--browser-id must be"},
		"http page":        {[]string{"authorize", "--browser-id", "browser-1", "--page-url", "http://shop.example/checkout"}, "--page-url must be"},
		"relative page":    {[]string{"authorize", "--browser-id", "browser-1", "--page-url", "/checkout"}, "--page-url must be"},
		"credentials page": {[]string{"authorize", "--browser-id", "browser-1", "--page-url", "https://user:pass@shop.example/"}, "--page-url must be"},
		"fill":             {[]string{"fill", "--params", linkPayTokenFillParams, "--browser-id", "browser-1", "--page-url", "https://shop.example/checkout"}, "only supported for authorize"},
		"params":           {[]string{"authorize", "--params", linkPayTokenFillParams}, "authorize takes --browser-id and --page-url"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "items", "invoke", "checkout", "order-1"}, tc.args...)...)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestVaultLinkAuthorizeErrorGuidance(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		status        int
		code, message string
		want          string
	}{
		{400, "page_not_found", "no open tab is at page_url; open the checkout page in the browser and authorize again", linkAuthorizeBrowserGuidance},
		{400, "ambiguous_page", "more than one tab is at page_url; close the extra tabs and authorize again", linkAuthorizeBrowserGuidance},
		{404, "browser_not_found", "browser not found", linkAuthorizeBrowserGuidance},
		{409, "browser_unavailable", "browser is unavailable for checkout inspection", linkAuthorizeBrowserGuidance},
		{403, "destination_denied", "checkout destination is not authorized", linkAuthorizeBrowserGuidance},
		{400, "timeout", "checkout inspection failed", linkAuthorizeBrowserGuidance},
		{500, "browser_error", "checkout inspection failed", linkAuthorizeBrowserGuidance},
		{400, "invalid_request", "amount exceeds the Link virtual card limit", "fix the reported input, then authorize again. if it reports that the amount exceeds the Link virtual card limit, delete this card and tell the user this checkout cannot be paid with a Link virtual card"},
		{429, "rate_limited", "too many requests", "wait, then authorize again"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, requestedCardFixture)
					return
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"code":%q,"message":%q}`, tc.code, tc.message)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize", "--browser-id", "browser-1", "--page-url", "https://shop.example/checkout")
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("%s: authorize failed (http %d): %s; %s", tc.code, tc.status, tc.message, tc.want))
		})
	}
}

func TestVaultLinkPayTokenFillOmitsFields(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		status string
		human  string
		fails  bool
	}{
		{"completed", "the checkout received the payment credential; fill never submits payment. submit the checkout yourself.", false},
		{"failed", vaultFillUncertain, true},
		{"unknown", vaultFillUncertain, true},
	} {
		t.Run(tc.status, func(t *testing.T) {
			result := fmt.Sprintf(`{"type":"fill","status":%q,"fields":[]}`, tc.status)
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, readyFillCardFixture)
					return
				}
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"type":"fill","browser_id":"browser-1","page_url":"https://shop.example/checkout"}`, string(body))
				_, _ = io.WriteString(w, result)
			})
			out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", linkPayTokenFillParams, "-o", "json")
			assert.JSONEq(t, result, out)
			_, human, humanErr := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", linkPayTokenFillParams)
			assert.Contains(t, human, "fill: "+tc.status)
			assert.Contains(t, human, tc.human)
			assert.NotContains(t, human, "field index")
			if tc.fails {
				require.Error(t, err)
				require.Error(t, humanErr)
			} else {
				require.NoError(t, err)
				require.NoError(t, humanErr)
			}
		})
	}
}

func TestVaultFillWithoutFieldsRequiresFieldsForCredentials(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, readyFillCredentialFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user-vault", "login", "fill", "--params", `{"browser_id":"browser-1"}`)
	require.ErrorContains(t, err, "fill for credential items requires 1-32 fields")
	assert.Equal(t, 1, calls)

	_, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", `{"browser_id":"browser-1","page_url":"https://shop.example/checkout","fields":[]}`)
	require.ErrorContains(t, err, "omit fields when the item's fill description says to")
	assert.Equal(t, 1, calls)
}

func TestVaultLinkCardUpdate(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, status := range []int{200, 409} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPatch, r.Method)
				assert.Equal(t, "/vaults/checkout/items/order-1", r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), `"type":"card"`)
				assert.Contains(t, string(body), `"provider":"link"`)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == 200 {
					_, _ = io.WriteString(w, requestedCardFixture)
				} else {
					_, _ = io.WriteString(w, `{"code":"conflict","message":"this card was authorized at a checkout and cannot be changed; delete this card and create a new one with the updated details"}`)
				}
			})
			_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "cards", "update", "checkout", "order-1"}, linkCardArgs()...)...)
			if status == 200 {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "conflict: this card was authorized at a checkout and cannot be changed; delete this card and create a new one")
		})
	}
}

func TestVaultLinkWalletDescription(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	description := "Use this wallet to pay at checkout."
	fixture := strings.Replace(connectedWalletFixture, `"type":"wallet",`, `"type":"wallet","description":"`+description+`",`, 1)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "wallets", "get", "checkout", "wallet-1", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, fixture, out)
	_, human, err := executeVaultCommand(t, client, "vaults", "wallets", "get", "checkout", "wallet-1")
	require.NoError(t, err)
	assert.Contains(t, human, description)
}
