package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultInvokeUsesAdvertisedTypeAcrossItems(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	operation := `[{"type":"refresh","description":"Refresh this item explicitly."}]`
	for _, fixture := range []string{
		strings.ReplaceAll(requestedCardFixture, `[{"type":"authorize","description":"Use only after explicit user approval."}]`, operation),
		strings.ReplaceAll(connectedWalletFixture, `"available_operations":[]`, `"available_operations":`+operation),
		strings.ReplaceAll(strings.ReplaceAll(connectedWalletFixture, `"provider":"link"`, `"provider":"agentcard"`), `"available_operations":[]`, `"available_operations":`+operation),
	} {
		t.Run(fixture, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					assert.Equal(t, http.MethodGet, r.Method)
				} else {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/vaults/checkout/items/item-1/operations", r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, `{"type":"refresh"}`, string(body))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, fixture)
			})
			out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "item-1", "refresh", "-o", "json")
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
			assert.JSONEq(t, fixture, out)
			assert.Empty(t, human)
		})
	}
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
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "refresh")
	require.ErrorContains(t, err, `operation "refresh" is not advertised`)
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
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize")
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
		{"checkout", "order-1", "authorize", "extra"},
		{"checkout", "order-1", "authorize", "--spec", "{}"},
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
	assert.Contains(t, cmd.Long, "browser_id and page_url together")
	assert.Contains(t, cmd.Long, "available_operations")
}

func TestVaultInvokeOpensOnlyReturnedActionExplicitly(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(fmt.Sprint(open), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				body := requestedCardFixture
				if r.Method == http.MethodPost {
					body = strings.ReplaceAll(body, `"status":"requested"`, `"status":"pending_authorization"`)
					body = strings.ReplaceAll(body, `"available_operations":`, `"action":{"name":"spend_approval","url":"https://provider.example/approve"},"available_operations":`)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			})
			opened := ""
			c := VaultsCmd{vaults: &client.Vaults, openURL: func(url string) error { opened = url; return nil }}
			var err error
			out := captureStdout(t, func() {
				err = c.Invoke(context.Background(), "checkout", "order-1", "authorize", nil, "json", open)
			})
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
			assert.Contains(t, out, `"status": "pending_authorization"`)
			if open {
				assert.Equal(t, "https://provider.example/approve", opened)
			} else {
				assert.Empty(t, opened)
			}
		})
	}
}

func TestVaultGetOperationHintUsesExplicitProject(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "other-project")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "chosen-project", r.Header.Get("X-Kernel-Project"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, requestedCardFixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1", "--project", "chosen-project")
	require.NoError(t, err)
	assert.Contains(t, human, "Invoke: kernel vaults items invoke --project=chosen-project --params '<json>' -- checkout order-1 authorize")
	assert.NotContains(t, human, "other-project")
}

func TestVaultGetOperationHints(t *testing.T) {
	for _, project := range []string{"", "project-1", "team's $(touch /tmp/nope)"} {
		for _, wallet := range []bool{false, true} {
			t.Run(fmt.Sprint(project, wallet), func(t *testing.T) {
				t.Setenv("KERNEL_PROJECT", project)
				fixture := requestedCardFixture
				if wallet {
					fixture = strings.ReplaceAll(connectedWalletFixture, `"available_operations":[]`, `"available_operations":[{"type":"refresh","description":"Refresh this item explicitly."}]`)
				}
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, fixture)
				})
				_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "item-1")
				require.NoError(t, err)
				op := "authorize"
				if wallet {
					op = "refresh"
				}
				assert.Contains(t, human, "Available operation: "+op)
				assert.Contains(t, human, "Invoke: kernel vaults items invoke")
				assert.Contains(t, human, " -- checkout item-1 "+op)
				switch project {
				case "":
					assert.NotContains(t, human, "--project")
				case "project-1":
					assert.Contains(t, human, "--project=project-1")
				default:
					assert.Contains(t, human, `--project='team'\''s $(touch /tmp/nope)'`)
				}
				out, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "item-1", "-o", "json")
				require.NoError(t, err)
				assert.JSONEq(t, fixture, out)
				assert.Empty(t, human)
			})
		}
	}
}

func TestVaultLinkAuthorizeCheckoutBinding(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	const params = `{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout?step=2#payment"}`
	for name, tt := range map[string]struct {
		args []string
		body string
	}{
		"bound":   {[]string{"--params", params}, `{"type":"authorize","browser_id":"browser-session-id","page_url":"https://shop.example/checkout?step=2#payment"}`},
		"unbound": {nil, `{"type":"authorize"}`},
	} {
		t.Run(name, func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posts++
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, tt.body, string(body))
				}
				_, _ = io.WriteString(w, requestedCardFixture)
			})
			args := append([]string{"vaults", "items", "invoke", "checkout", "order-1", "authorize", "-o", "json"}, tt.args...)
			_, _, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 1, posts)
		})
	}
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid authorize parameters reached API") })
	for _, raw := range []string{
		`{}`,
		`{"browser_id":"browser-session-id"}`,
		`{"page_url":"https://shop.example/checkout"}`,
		`{"browser_id":" ","page_url":"https://shop.example/checkout"}`,
		`{"browser_id":"browser-session-id","page_url":"http://shop.example/checkout"}`,
		`{"browser_id":"browser-session-id","page_url":"https://user:secret@shop.example/checkout"}`,
		`{"browser_id":"browser-session-id","page_url":"/checkout"}`,
		`{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout","fields":[]}`,
		`{"type":"authorize","browser_id":"browser-session-id","page_url":"https://shop.example/checkout"}`,
	} {
		_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize", "--params", raw)
		require.Error(t, err, raw)
	}
	agentCard := strings.Replace(readyAgentCardFixture, `"available_operations":[]`, `"available_operations":[{"type":"authorize","description":"Approve."}]`, 1)
	client = vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "AgentCard authorize must not send a checkout binding")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, agentCard)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize", "--params", params)
	require.ErrorContains(t, err, "apply only to Link cards")
}

func TestVaultLinkAuthorizeErrors(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	const bound = `{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout"}`
	for _, tt := range []struct {
		status int
		code   string
		want   string
	}{
		{400, "ambiguous_page", "checkout inspection failed before approval started"},
		{400, "timeout", "checkout inspection failed before approval started"},
		{403, "destination_denied", "checkout inspection failed before approval started"},
		{404, "browser_not_found", "checkout inspection failed before approval started"},
		{409, "browser_unavailable", "checkout inspection failed before approval started"},
		{500, "browser_error", "checkout inspection failed before approval started"},
		{502, "timeout", "timeout: Request failed"},
		{409, "conflict", "repeat its browser_id and page_url or omit both"},
		{429, "rate_limited", "retry the same authorize"},
		{400, "invalid_request", "invalid_request: Request failed"},
	} {
		t.Run(fmt.Sprint(tt.status, tt.code), func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, requestedCardFixture)
					return
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"code":"`+tt.code+`","message":"Request failed"}`)
			})
			_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize", "--params", bound)
			require.Error(t, err)
			assert.Contains(t, util.CleanedUpSdkError{Err: err}.Error(), tt.want)
		})
	}
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, requestedCardFixture)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"timeout","message":"Request failed"}`)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "authorize")
	require.Error(t, err)
	assert.Equal(t, "timeout: Request failed", util.CleanedUpSdkError{Err: err}.Error(), "unbound authorize has no checkout inspection")
}
