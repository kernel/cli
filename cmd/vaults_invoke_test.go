package cmd

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fillCardFixture = `{"id":"item-1","key":"order-1","type":"card","spec":{"provider":"link","wallet":"wallet-1","payment_method_id":"pm-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example","context":"Purchase description"},"state":{"provider":"link","status":"ready"},"available_operations":[{"type":"fill","description":"Fill this approved credential without submitting payment."}],"available_expansions":[]}`

func TestVaultInvokeRejectsUnsupportedOrUnadvertisedOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, requestedCardFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize", "--spec", `{}`)
	require.ErrorContains(t, err, "operation must be fill")
	_, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--spec", `{"browser_id":"browser","page_url":"https://shop.example"}`)
	require.ErrorContains(t, err, `operation "fill" is not advertised`)
}

func TestVaultInvokeArgumentsAndHelp(t *testing.T) {
	client := vaultTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid input reached API") })
	for _, args := range [][]string{
		{"checkout", "order-1"},
		{"checkout", "order-1", "fill"},
		{"checkout", "order-1", "fill", "--spec", `{"type":"fill"}`},
		{"checkout", "order-1", "fill", "--spec", `{"link_pay_token":"secret"}`},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "items", "invoke"}, args...)...)
		require.Error(t, err)
	}
	cmd, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	assert.NotNil(t, cmd.Flags().Lookup("spec"))
	assert.Nil(t, cmd.Flags().Lookup("open"))
	assert.Contains(t, cmd.Long, "never clicks Pay")
	assert.Contains(t, cmd.Long, "available_operations")
}

func TestVaultGetFillHintUsesExplicitProject(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "other-project")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "chosen-project", r.Header.Get("X-Kernel-Project"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fillCardFixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1", "--project", "chosen-project")
	require.NoError(t, err)
	assert.Contains(t, human, "Available operation: fill")
	assert.Contains(t, human, "Invoke: kernel vaults items invoke --project=chosen-project -- checkout order-1 fill")
	assert.NotContains(t, human, "other-project")
}
