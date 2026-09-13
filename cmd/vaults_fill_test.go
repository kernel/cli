package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fillCardFixture = `{"id":"item-1","key":"order-1","type":"card","spec":{"provider":"link","wallet":"wallet-1","payment_method_id":"pm-1","amount":1234,"currency":"usd","merchant_name":"Example Shop"},"state":{"provider":"link","status":"ready"},"available_operations":[{"type":"fill","description":"Fill this card into a page open in a browser with this vault attached."}],"available_expansions":[]}`

const fillCompletedFixture = `{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled"},{"index":1,"status":"filled"}]}`

const fillFailedFixture = `{"type":"fill","status":"failed","fields":[{"index":0,"status":"filled"},{"index":1,"status":"failed","error_code":"ambiguous_selector"},{"index":2,"status":"not_attempted"}]}`

// vaultFillServer answers the advertisement GET with a fillable card and records
// the body of the operations POST.
func vaultFillServer(t *testing.T, result string, body *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, fillCardFixture)
			return
		}
		assert.Equal(t, "/vaults/checkout/items/order-1/operations", r.URL.Path)
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		*body = string(raw)
		_, _ = io.WriteString(w, result)
	}
}

func TestVaultFillSendsBindingsInOrder(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	body := ""
	client := vaultTestClient(t, vaultFillServer(t, fillCompletedFixture, &body))
	out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill",
		"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout?step=pay",
		"--field", "number=input[name=cardnumber]", "--field", "expiration:MM/YY=#expiry",
		"--timeout-ms", "5000", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"fill","browser_id":"browser-1","page_url":"https://shop.example/checkout?step=pay","timeout_ms":5000,"fields":[{"field":"number","selector":"input[name=cardnumber]"},{"field":"expiration","format":"MM/YY","selector":"#expiry"}]}`, body)
	assert.JSONEq(t, fillCompletedFixture, out)
	assert.Empty(t, human)
}

func TestVaultFillOmitsUnsetTimeout(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	body := ""
	client := vaultTestClient(t, vaultFillServer(t, fillCompletedFixture, &body))
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill",
		"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout", "--field", "cvc=#cvc", "-o", "json")
	require.NoError(t, err)
	assert.NotContains(t, body, "timeout_ms")
}

func TestVaultFillPrintsPerFieldOutcomesAndGuidance(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		result   string
		contains []string
	}{
		{fillCompletedFixture, []string{"Fill completed", "filled"}},
		{fillFailedFixture, []string{"ambiguous_selector", "not_attempted", "were not rolled back", "Do not automatically retry"}},
	} {
		t.Run(tc.result, func(t *testing.T) {
			body := ""
			client := vaultTestClient(t, vaultFillServer(t, tc.result, &body))
			_, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill",
				"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout",
				"--field", "number=#card-number", "--field", "cvc=#cvc", "--field", "billing_postal_code=#zip")
			require.NoError(t, err)
			// Request bindings label each result row; the API never returns values.
			assert.Contains(t, human, "#card-number")
			assert.NotContains(t, human, "browser-1")
			for _, want := range tc.contains {
				assert.Contains(t, human, want)
			}
		})
	}
}

func TestVaultFillRejectsInvalidInputBeforeCallingAPI(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	valid := []string{"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout", "--field", "number=#card-number"}
	for _, tc := range []struct {
		args    []string
		message string
	}{
		{[]string{"--page-url", "https://shop.example/checkout", "--field", "number=#n"}, "--browser-id"},
		{[]string{"--browser-id", "browser-1", "--field", "number=#n"}, "--page-url"},
		{[]string{"--browser-id", "browser-1", "--page-url", "http://shop.example/checkout", "--field", "number=#n"}, "exact current HTTPS page URL"},
		{[]string{"--browser-id", "browser-1", "--page-url", "https://user:pass@shop.example/checkout", "--field", "number=#n"}, "exact current HTTPS page URL"},
		{[]string{"--browser-id", "browser-1", "--page-url", "https://shop.example/*", "--field", "number=#n"}, "exact current HTTPS page URL"},
		{[]string{"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout"}, "at least one --field"},
		{append(valid, "--field", "cardnumber=#n"), "is not a card field"},
		{append(valid, "--field", "expiration=#expiry"), "expiration requires a format"},
		{append(valid, "--field", "expiration:MM=#expiry"), "expiration format must be one of"},
		{append(valid, "--field", "cvc:MM/YY=#cvc"), "only expiration takes a format"},
		{append(valid, "--field", "#stray-selector"), "<field>=<css-selector>"},
		{append(valid, "--field", "cvc="), "<field>=<css-selector>"},
		{append(valid, "--field", "cvc=#card-number"), "distinct selector"},
		{append(valid, "--timeout-ms", "60000"), "--timeout-ms must be between"},
		{append(valid, "--timeout-ms", "0"), "--timeout-ms must be between"},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "items", "invoke", "checkout", "order-1", "fill"}, tc.args...)...)
		require.ErrorContains(t, err, tc.message)
	}
}

func TestVaultFillRejectsTooManyBindings(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	args := []string{"vaults", "items", "invoke", "checkout", "order-1", "fill", "--browser-id", "browser-1", "--page-url", "https://shop.example/checkout"}
	for i := 0; i <= vaultFillMaxFields; i++ {
		args = append(args, "--field", "cvc=#field-"+strings.Repeat("x", i+1))
	}
	_, _, err := executeVaultCommand(t, client, args...)
	require.ErrorContains(t, err, "at most 32")
}

func TestVaultFillFlagsRejectedForOtherOperations(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	for _, args := range [][]string{
		{"--browser-id", "browser-1"},
		{"--page-url", "https://shop.example/checkout"},
		{"--field", "number=#card-number"},
		{"--timeout-ms", "5000"},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "items", "invoke", "checkout", "order-1", "authorize"}, args...)...)
		require.ErrorContains(t, err, "applies only to the fill operation")
	}
}

func TestVaultFillRequiresAdvertisedOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, requestedCardFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill",
		"--browser-id", "browser-1", "--page-url", "https://shop.example/checkout", "--field", "number=#card-number")
	require.ErrorContains(t, err, `operation "fill" is not advertised`)
	assert.Equal(t, 1, calls)
}

func TestVaultFillHintIncludesRequiredFlags(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fillCardFixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1")
	require.NoError(t, err)
	assert.Contains(t, human, "Invoke: kernel vaults items invoke -- checkout order-1 fill --browser-id <browser-session-id> --page-url <https://exact-page-url> --field number='<css-selector>'")
}

func TestVaultInvokeHelpDocumentsFill(t *testing.T) {
	cmd, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	for _, name := range vaultFillFlags {
		assert.NotNil(t, cmd.Flags().Lookup(name), name)
	}
	assert.Contains(t, cmd.Long, "expiration:MM/YY")
	assert.Contains(t, cmd.Long, "billing_postal_code")
	assert.Contains(t, cmd.Long, "never automatically retry")
	assert.Contains(t, cmd.Example, "--browser-id")
}
