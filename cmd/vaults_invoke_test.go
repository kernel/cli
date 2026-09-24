package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultInvokeCollect(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			assert.Equal(t, http.MethodGet, r.Method)
		} else {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/vaults/checkout/items/login/operations", r.URL.Path)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"collect"}`, string(body))
		}
		_, _ = io.WriteString(w, credentialFixture)
	})
	out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "login", "collect", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Contains(t, out, `"type": "credential"`)
	assert.Empty(t, human)
}

func TestVaultInvokeRejectsUnadvertisedOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, requestedCardFixture)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize")
	require.ErrorContains(t, err, `operation "authorize" is not advertised`)
	assert.Equal(t, 1, calls)
}

func TestVaultInvokeGetFailureDoesNotPostOrRetry(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, status := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"code":"item_unavailable","message":"Item unavailable"}`)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "login", "collect")
			require.ErrorContains(t, err, "item_unavailable: Item unavailable")
			assert.Equal(t, 1, calls)
		})
	}
}

func TestVaultInvokeArgumentsAndHelp(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	for _, args := range [][]string{
		{"checkout", "order-1"},
		{"checkout", "order-1", ""},
		{"checkout", "order-1", "fill", "extra"},
		{"checkout", "order-1", "collect", "--params", "{}"},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "items", "invoke"}, args...)...)
		require.Error(t, err)
	}
	cmd, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	assert.Nil(t, cmd.Flags().Lookup("spec"))
	assert.NotNil(t, cmd.Flags().Lookup("open"))
	assert.NotNil(t, cmd.Flags().Lookup("params"))
	assert.Contains(t, cmd.Long, "failed/unknown exit nonzero")
	assert.Contains(t, cmd.Long, "there is no separate authorize operation")
	assert.Contains(t, cmd.Long, "Link cards require")
}

func TestVaultInvokeOpensOnlyReturnedActionExplicitly(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(fmt.Sprint(open), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, credentialFixture)
			})
			opened := ""
			c := VaultsCmd{vaults: &client.Vaults, openURL: func(url string) error { opened = url; return nil }}
			var err error
			captureStdout(t, func() {
				err = c.Invoke(context.Background(), "checkout", "login", "collect", nil, "json", open)
			})
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
			if open {
				assert.Equal(t, "https://vault.kernel.sh/collect#token=item.random", opened)
			} else {
				assert.Empty(t, opened)
			}
		})
	}
}

func TestVaultGetOperationHints(t *testing.T) {
	for _, project := range []string{"", "project-1", "team's $(touch /tmp/nope)"} {
		t.Run(project, func(t *testing.T) {
			t.Setenv("KERNEL_PROJECT", project)
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, credentialFixture)
			})
			_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "login")
			require.NoError(t, err)
			assert.Contains(t, human, "Available operation: collect")
			assert.Contains(t, human, "Invoke: kernel vaults items invoke")
			assert.Contains(t, human, " -- checkout login collect")
			switch project {
			case "":
				assert.NotContains(t, human, "--project")
			case "project-1":
				assert.Contains(t, human, "--project=project-1")
			default:
				assert.Contains(t, human, `--project='team'\''s $(touch /tmp/nope)'`)
			}
			out, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "login", "-o", "json")
			require.NoError(t, err)
			assert.Contains(t, out, `"type": "credential"`)
			assert.Empty(t, human)
		})
	}
}
