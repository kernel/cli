package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const linkWalletSpecFixture = `{"authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`

const vaultFixture = `{"id":"vault-1","name":"checkout","created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`
const requestedCardFixture = `{"id":"item-1","key":"order-1","type":"card","spec":{"provider":"link","wallet":"wallet-1","payment_method_id":"pm-1","amount":1234,"currency":"usd","merchant_name":"Example Shop","merchant_url":"https://shop.example","context":"Purchase description","test":true},"state":{"provider":"link","status":"requested"},"available_operations":[{"type":"authorize","description":"Use only after explicit user approval."}],"available_expansions":[]}`
const connectedWalletFixture = `{"id":"wallet-id","key":"wallet-1","type":"wallet","spec":{"provider":"link","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}},"state":{"provider":"link","status":"connected"},"available_operations":[],"available_expansions":[{"type":"payment_methods","description":"Select a payment method explicitly."}]}`

func vaultTestClient(t *testing.T, handler http.HandlerFunc) kernel.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"))
}

func executeVaultCommand(t *testing.T, client kernel.Client, args ...string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "kernel", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("project", "", "Project")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		scoped := client
		scoped.Vaults = kernel.NewVaultService(append(client.Options, option.WithProject(resolveProjectSelection(project)))...)
		cmd.SetContext(context.WithValue(cmd.Context(), util.KernelClientKey, scoped))
		return nil
	}
	root.AddCommand(newVaultsCommand())
	root.SetArgs(args)
	buf := capturePtermOutput(t)
	var err error
	stdout := captureStdout(t, func() { err = root.Execute() })
	return stdout, buf.String(), err
}

func TestVaultCommandConstruction(t *testing.T) {
	for _, path := range []string{"create", "list", "get", "delete", "items list", "items get", "items delete", "items events", "wallets create", "wallets payment-methods", "cards create", "cards update", "cards authorize"} {
		t.Run(path, func(t *testing.T) {
			cmd, remaining, err := newVaultsCommand().Find(strings.Fields(path))
			require.NoError(t, err)
			require.Empty(t, remaining)
			assert.NotNil(t, cmd.RunE)
			assert.NotNil(t, cmd.PreRunE)
			assert.NotNil(t, cmd.Args)
			if cmd.Name() == "delete" {
				assert.NotNil(t, cmd.Flags().Lookup("yes"))
			} else {
				require.NotNil(t, cmd.Flags().Lookup("output"))
				assert.Contains(t, cmd.Flags().Lookup("output").Usage, "display-safe")
			}
		})
	}
	cmd, _, err := rootCmd.Find([]string{"vaults", "cards", "authorize"})
	require.NoError(t, err)
	assert.False(t, isAuthExempt(cmd))
	for _, unsupported := range []string{"rename", "update", "items put", "items action", "wallets callback", "cards pay"} {
		cmd, remaining, _ := newVaultsCommand().Find(strings.Fields(unsupported))
		assert.True(t, len(remaining) > 0 || cmd.RunE == nil, unsupported)
	}
}

func TestVaultRequiredAndInvalidFlags(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	tests := []struct {
		args string
		want string
	}{
		{"vaults create", "required flag"},
		{"vaults create --name=", "--name"},
		{"vaults create --name=bad/name", "--name"},
		{"vaults create --name=..", "--name"},
		{"vaults get", "accepts 1 arg"},
		{"vaults get bad%2Fname", "vault ID or name"},
		{"vaults items get checkout bad/key", "item key"},
		{"vaults list --limit 0", "--limit"},
		{"vaults list --limit 101", "--limit"},
		{"vaults list --offset -1", "--offset"},
		{"vaults list -o yaml", "output"},
		{"vaults items get checkout wallet-1 --wait -1", "--wait"},
		{"vaults items get checkout wallet-1 --wait 61", "--wait"},
		{"vaults items events checkout wallet-1 --wait 61", "--wait"},
		{"vaults items get checkout wallet-1 --expand secret", "--expand"},
		{"vaults wallets create checkout wallet-1", "required flag"},
		{"vaults wallets create checkout wallet-1 --provider unknown --spec {}", "--provider"},
		{"vaults wallets create checkout wallet-1 --provider link", "required flag"},
		{"vaults cards create checkout order-1 --provider link", "required flag"},
		{"vaults cards update checkout order-1 --spec {}", "required flag"},
		{"vaults wallets create checkout wallet-1 --provider link --user-id usr_123", "--user-id"},
		{"vaults wallets create checkout wallet-1 --provider agentcard --user-id wrong", "--user-id"},
		{"vaults cards create checkout order-1", "required flag"},
		{"vaults delete checkout", "--yes"},
		{"vaults items delete checkout order-1", "--yes"},
		{"vaults cards create checkout order-1 --domain shop.example", "unknown flag"},
		{"vaults create --name checkout --project-id project-test", "unknown flag"},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			_, _, err := executeVaultCommand(t, client, strings.Fields(tt.args)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestVaultCommandsWithoutProject(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	tests := []struct {
		args     []string
		response string
		calls    int
	}{
		{[]string{"create", "--name", "checkout"}, vaultFixture, 1},
		{[]string{"list"}, "[" + vaultFixture + "]", 1},
		{[]string{"get", "checkout"}, vaultFixture, 1},
		{[]string{"delete", "checkout", "--yes"}, "", 1},
		{[]string{"items", "list", "checkout"}, "[" + requestedCardFixture + "]", 1},
		{[]string{"items", "get", "checkout", "order-1"}, requestedCardFixture, 1},
		{[]string{"items", "delete", "checkout", "order-1", "--yes"}, "", 1},
		{[]string{"items", "events", "checkout", "order-1"}, "[]", 1},
		{[]string{"wallets", "create", "checkout", "wallet-1", "--provider", "link", "--spec", linkWalletSpecFixture}, connectedWalletFixture, 1},
		{[]string{"wallets", "payment-methods", "checkout", "wallet-1"}, connectedWalletFixture, 1},
		{append([]string{"cards", "create", "checkout", "order-1"}, linkCardArgs()...), requestedCardFixture, 1},
		{append([]string{"cards", "update", "checkout", "order-1"}, linkCardArgs()...), requestedCardFixture, 1},
		{[]string{"cards", "authorize", "checkout", "order-1"}, requestedCardFixture, 2},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args[:min(2, len(tt.args))], " "), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Empty(t, r.Header.Get("X-Kernel-Project"))
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Has-More", "false")
				w.Header().Set("X-Next-Offset", "0")
				_, _ = io.WriteString(w, tt.response)
			})
			_, _, err := executeVaultCommand(t, client, append([]string{"vaults"}, tt.args...)...)
			require.NoError(t, err)
			assert.Equal(t, tt.calls, calls)
		})
	}
}

func linkCardArgs() []string {
	return []string{"--provider", "link", "--spec", fmt.Sprintf(`{"wallet":"wallet-1","amount":1234,"currency":"USD","merchant_name":"Example Shop","payment_method_id":"pm-1","merchant_url":"https://shop.example","context":%q,"test":true}`, strings.Repeat("Purchase purpose. ", 7))}
}

func TestVaultSpecValidation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid JSON reached API") })
	for _, path := range []string{"wallets create", "cards create", "cards update"} {
		for _, spec := range []string{"", "null", "[]", "42", `"text"`, "{", "{} {}", `{"provider":"agentcard"}`, `{"provider":null}`, `{"provider":1}`} {
			t.Run(path+"/"+spec, func(t *testing.T) {
				args := append([]string{"vaults"}, strings.Fields(path)...)
				args = append(args, "checkout", "item-1", "--provider", "link", "--spec", spec)
				_, _, err := executeVaultCommand(t, client, args...)
				require.Error(t, err)
			})
		}
	}
}

func TestVaultCreateScopeAndImmutableName(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "env-project")
	project := "env-project"
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/vaults", r.URL.Path)
		assert.Equal(t, project, r.Header.Get("X-Kernel-Project"))
		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"name":"checkout"}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, vaultFixture)
	})
	out, human, err := executeVaultCommand(t, client, "vaults", "create", "--name", "checkout", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, vaultFixture, out)
	assert.Empty(t, human)
	project = "flag-project"
	_, human, err = executeVaultCommand(t, client, "--project", project, "vaults", "create", "--name", "checkout")
	require.NoError(t, err)
	assert.Contains(t, human, "Name (immutable)")
	assert.Contains(t, human, "checkout")
}

func TestVaultListPaginationAndEmptyJSON(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	body, hasMore, next := "["+vaultFixture+"]", "true", "21"
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vaults", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("limit"))
		assert.Equal(t, "20", r.URL.Query().Get("offset"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Has-More", hasMore)
		w.Header().Set("X-Next-Offset", next)
		_, _ = io.WriteString(w, body)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "list", "--limit", "1", "--offset", "20", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"vaults":[`+vaultFixture+`],"next_offset":21}`, out)
	_, human, err := executeVaultCommand(t, client, "vaults", "list", "--limit", "1", "--offset", "20")
	require.NoError(t, err)
	assert.Contains(t, human, `kernel --project "project-test" vaults list --limit 1 --offset 21`)
	t.Setenv("KERNEL_PROJECT", "")
	_, human, err = executeVaultCommand(t, client, "vaults", "list", "--limit", "1", "--offset", "20")
	require.NoError(t, err)
	assert.Contains(t, human, "kernel vaults list --limit 1 --offset 21")
	assert.NotContains(t, human, "--project")
	body, hasMore, next = "[]", "false", "0"
	out, _, err = executeVaultCommand(t, client, "vaults", "list", "--limit", "1", "--offset", "20", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"vaults":[]}`, out)
}

func TestVaultWalletRequestMapping(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	for _, tt := range []struct {
		provider, userID, spec string
	}{
		{"link", "", `{"provider":"link","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`},
		{"agentcard", "", `{"provider":"agentcard"}`},
		{"agentcard", "usr_enrolled", `{"provider":"agentcard","user_id":"usr_enrolled"}`},
	} {
		t.Run(tt.provider+tt.userID, func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/vaults/checkout/items/wallet-1", r.URL.Path)
				body, _ := io.ReadAll(r.Body)
				assert.JSONEq(t, `{"type":"wallet","spec":`+tt.spec+`}`, string(body))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, connectedWalletFixture)
			})
			args := []string{"vaults", "wallets", "create", "checkout", "wallet-1", "--provider", tt.provider, "--spec", tt.spec, "-o", "json"}
			out, human, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.JSONEq(t, connectedWalletFixture, out)
			assert.Empty(t, human)
		})
	}
}

func TestVaultCardRequestMapping(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	for _, operation := range []string{"create", "update"} {
		for _, provider := range []string{"link", "agentcard"} {
			t.Run(operation+provider, func(t *testing.T) {
				calls := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					expectedMethod := http.MethodPut
					if operation == "update" {
						expectedMethod = http.MethodPatch
					}
					assert.Equal(t, expectedMethod, r.Method)
					assert.Equal(t, "/vaults/checkout/items/order-1", r.URL.Path)
					var body map[string]json.RawMessage
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					if operation == "create" {
						assert.JSONEq(t, `"card"`, string(body["type"]))
						assert.Len(t, body, 2)
					} else {
						assert.Len(t, body, 1)
					}
					if provider == "link" {
						assert.JSONEq(t, fmt.Sprintf(`{"provider":"link","wallet":"wallet-1","amount":1234,"currency":"USD","merchant_name":"Example Shop","merchant_url":"https://shop.example","payment_method_id":"pm-1","context":%q,"test":true}`, strings.Repeat("Purchase purpose. ", 7)), string(body["spec"]))
					} else {
						assert.JSONEq(t, `{"provider":"agentcard","wallet":"wallet-1","amount":1234,"currency":"usd","merchant":"Example Shop","card_id":"vc_chosen"}`, string(body["spec"]))
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, requestedCardFixture)
				})
				flags := linkCardArgs()
				if provider == "agentcard" {
					flags = []string{"--provider", provider, "--spec", `{"wallet":"wallet-1","amount":1234,"currency":"usd","merchant":"Example Shop","card_id":"vc_chosen"}`}
				}
				args := append([]string{"vaults", "cards", operation, "checkout", "order-1", "-o", "json"}, flags...)
				out, human, err := executeVaultCommand(t, client, args...)
				require.NoError(t, err)
				assert.JSONEq(t, requestedCardFixture, out)
				assert.Empty(t, human)
				assert.Equal(t, 1, calls, "card writes must not authorize implicitly")
			})
		}
	}
}

func TestVaultAuthorizeRequiresAdvertisedRequestedLinkCard(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	for _, state := range []string{"requested", "pending_authorization", "ready", "consumed", "expired", "declined"} {
		for _, advertised := range []bool{false, true} {
			t.Run(fmt.Sprint(state, advertised), func(t *testing.T) {
				getCalls, postCalls := 0, 0
				body := strings.ReplaceAll(requestedCardFixture, `"status":"requested"`, `"status":"`+state+`"`)
				if !advertised {
					body = strings.ReplaceAll(body, `[{"type":"authorize","description":"Use only after explicit user approval."}]`, `[]`)
				}
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet {
						getCalls++
						_, _ = io.WriteString(w, body)
						return
					}
					postCalls++
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/vaults/checkout/items/order-1/operations", r.URL.Path)
					payload, _ := io.ReadAll(r.Body)
					assert.JSONEq(t, `{"type":"authorize"}`, string(payload))
					_, _ = io.WriteString(w, body)
				})
				_, _, err := executeVaultCommand(t, client, "vaults", "cards", "authorize", "checkout", "order-1", "-o", "json")
				assert.Equal(t, 1, getCalls)
				if state == "requested" && advertised {
					require.NoError(t, err)
					assert.Equal(t, 1, postCalls)
				} else {
					require.Error(t, err)
					assert.Zero(t, postCalls)
				}
			})
		}
	}
}

func TestVaultNoSDKRetriesAndAPIErrorMessages(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	for _, status := range []int{409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, requestedCardFixture)
					return
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"message":"Authorization service unavailable","code":"authorization_failed"}`)
			})
			out, _, err := executeVaultCommand(t, client, "vaults", "cards", "authorize", "checkout", "order-1", "-o", "json")
			require.Error(t, err)
			assert.Equal(t, 2, calls)
			assert.Empty(t, out)
			var apiErr *kernel.Error
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, status, apiErr.StatusCode)
			assert.Equal(t, "authorization_failed: Authorization service unavailable", util.CleanedUpSdkError{Err: err}.Error())
		})
	}
}

func TestVaultInvalidProjectErrors(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	commands := [][]string{
		{"list"}, {"get", "checkout"}, {"create", "--name", "checkout"},
		{"items", "list", "checkout"}, {"items", "get", "checkout", "order-1"},
		{"items", "events", "checkout", "order-1"},
		{"wallets", "create", "checkout", "wallet-1", "--provider", "link", "--spec", linkWalletSpecFixture},
		{"wallets", "payment-methods", "checkout", "wallet-1"},
		append([]string{"cards", "create", "checkout", "order-1"}, linkCardArgs()...),
		append([]string{"cards", "update", "checkout", "order-1"}, linkCardArgs()...),
		{"cards", "authorize", "checkout", "order-1"},
	}
	for _, project := range []string{"doesntexist", "abcdefghijklmnopqrstuvwx"} {
		for _, args := range commands {
			t.Run(project+"/"+strings.Join(args[:min(2, len(args))], " "), func(t *testing.T) {
				calls := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, project, r.Header.Get("X-Kernel-Project"))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"code":"project_not_found","message":"Project not found or inactive"}`)
				})
				out, human, err := executeVaultCommand(t, client, append([]string{"--project", project, "vaults"}, args...)...)
				require.Error(t, err)
				assert.Equal(t, "project_not_found: Project not found or inactive", util.CleanedUpSdkError{Err: err}.Error())
				assert.Equal(t, 1, calls)
				assert.Empty(t, out)
				assert.NotContains(t, human, "Deleted")
			})
		}
	}
}

func TestVaultPlainTextAPIError(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Credential is scoped to a different project", http.StatusForbidden)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "list", "--project", "other-project", "-o", "json")
	require.Error(t, err)
	assert.Empty(t, out)
	assert.Contains(t, util.CleanedUpSdkError{Err: err}.Error(), "Credential is scoped to a different project")
	assert.NotContains(t, err.Error(), "withheld")
}

func TestVaultGetWaitExpansionAndEvents(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/vaults/checkout/items/wallet-1":
			assert.Equal(t, "60", r.URL.Query().Get("wait"))
			assert.Equal(t, "payment_methods", r.URL.Query().Get("expand"))
			_, _ = io.WriteString(w, connectedWalletFixture)
		case "/vaults/checkout/items/order-1/events":
			assert.Equal(t, "60", r.URL.Query().Get("wait"))
			assert.Equal(t, "event-before", r.URL.Query().Get("after"))
			_, _ = io.WriteString(w, `[{"id":"event-next","name":"payment_failed","browser_id":"browser-1","created_at":"2026-09-01T00:00:00Z","data":{"reason":"declined","outcome_reason":"provider_error","provider_http_status":402,"provider_decline_code":"insufficient_funds","actual_currency":"usd","provider_response":{"card_number":"SECRET"}}}]`)
		default:
			t.Errorf("unexpected request: %s", r.URL)
		}
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "wallet-1", "--wait", "60", "--expand", "payment_methods", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, connectedWalletFixture, out)
	args := []string{"vaults", "items", "events", "checkout", "order-1", "--wait", "60", "--after", "event-before"}
	out, _, err = executeVaultCommand(t, client, append(args, "-o", "json")...)
	require.NoError(t, err)
	assert.NotContains(t, out, "SECRET")
	assert.Contains(t, out, "declined")
	assert.Contains(t, out, `"provider_http_status": 402`)
	assert.Contains(t, out, `"outcome_reason": "provider_error"`)
	assert.Contains(t, out, `"actual_currency": "usd"`)
	_, human, err := executeVaultCommand(t, client, args...)
	require.NoError(t, err)
	assert.Contains(t, human, "--after event-next")
	assert.Contains(t, human, "payment_failed")
	assert.Contains(t, human, "insufficient_funds")
	assert.NotContains(t, human, "SECRET")
}

func TestVaultDeleteResponses(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	for _, path := range []string{"vaults delete checkout --yes", "vaults items delete checkout order-1 --yes"} {
		for _, response := range []struct {
			status int
			code   string
		}{
			{204, ""}, {404, "not_found"}, {404, "project_not_found"}, {403, "forbidden"}, {409, "conflict"}, {500, "internal_error"},
		} {
			t.Run(fmt.Sprintf("%s/%d/%s", path, response.status, response.code), func(t *testing.T) {
				calls := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, http.MethodDelete, r.Method)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(response.status)
					if response.code != "" {
						_, _ = fmt.Fprintf(w, `{"code":%q,"message":"API error details"}`, response.code)
					}
				})
				out, human, err := executeVaultCommand(t, client, strings.Fields(path)...)
				assert.Equal(t, 1, calls)
				assert.Empty(t, out)
				if response.status == http.StatusNoContent || response.status == http.StatusNotFound {
					require.NoError(t, err)
					assert.Contains(t, human, "Deleted or not found: vault")
					assert.Contains(t, human, "checkout")
				} else {
					require.Error(t, err)
					assert.Equal(t, response.code+": API error details", util.CleanedUpSdkError{Err: err}.Error())
					assert.Empty(t, human)
				}
			})
		}
	}
}

func TestVaultEmptyItemLists(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "project-test")
	for _, path := range []string{"vaults items list checkout", "vaults items events checkout order-1"} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[]`)
		})
		out, _, err := executeVaultCommand(t, client, append(strings.Fields(path), "-o", "json")...)
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, out)
	}
}

func TestVaultOpenActionOnlyWhenRequested(t *testing.T) {
	capturePtermOutput(t)
	for _, actionURL := range []string{"", "https://provider.example/approval", "javascript:alert(1)", "https://user:secret@provider.example"} {
		var item kernel.VaultItemUnion
		body := strings.TrimSuffix(connectedWalletFixture, "}") + fmt.Sprintf(`,"action":{"name":"link_oauth","url":%q}}`, actionURL)
		require.NoError(t, json.Unmarshal([]byte(body), &item))
		opened := ""
		c := VaultsCmd{openURL: func(url string) error { opened = url; return nil }, prompter: interactive.NewPrompterWithTerminal(false)}
		require.NoError(t, c.showItem(&item, "", false))
		assert.Empty(t, opened)
		err := c.showItem(&item, "", true)
		if strings.HasPrefix(actionURL, "https://provider.example") {
			require.NoError(t, err)
			assert.Equal(t, actionURL, opened)
		} else if actionURL == "" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			assert.Empty(t, opened)
		}
	}
}
