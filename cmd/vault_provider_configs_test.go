package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const providerConfigFixture = `{"id":"config-1","name":"checkout-client","provider":"agentcard","client_id":"client-1","test_mode":false,"created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`

func vaultTestSecret(t *testing.T) string {
	t.Helper()
	var data [32]byte
	_, err := rand.Read(data[:])
	require.NoError(t, err)
	return hex.EncodeToString(data[:])
}

func executeVaultInputCommand(t *testing.T, client kernel.Client, stdin string, args ...string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "kernel", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("project", "", "Project")
	root.SetContext(context.WithValue(context.Background(), util.KernelClientKey, client))
	root.SetIn(strings.NewReader(stdin))
	root.AddCommand(newVaultProviderConfigsCommand(), newVaultsCommand())
	root.SetArgs(args)
	buf := capturePtermOutput(t)
	var err error
	stdout := captureStdout(t, func() { err = root.Execute() })
	return stdout, buf.String(), err
}

func TestVaultProviderConfigCreate(t *testing.T) {
	for _, provider := range []string{"link", "agentcard"} {
		for _, source := range []string{"file", "stdin"} {
			t.Run(provider+"/"+source, func(t *testing.T) {
				secret := vaultTestSecret(t)
				input := fmt.Sprintf(`{"client_id":"client-1","client_secret":%q}`, secret)
				path, stdin := "-", input
				if source == "file" {
					path, stdin = filepath.Join(t.TempDir(), "credentials.json"), ""
					require.NoError(t, os.WriteFile(path, []byte(input), 0600))
				}
				calls := 0
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/vault-provider-configs", r.URL.Path)
					var body struct {
						Name        string            `json:"name"`
						Provider    string            `json:"provider"`
						Credentials map[string]string `json:"credentials"`
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					assert.Equal(t, "checkout-client", body.Name)
					assert.Equal(t, provider, body.Provider)
					assert.Equal(t, 2, len(body.Credentials))
					assert.True(t, body.Credentials["client_secret"] == secret, "secret must be sent unchanged")
					assert.Equal(t, "client-1", body.Credentials["client_id"])
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, strings.ReplaceAll(providerConfigFixture, "agentcard", provider))
				})
				out, human, err := executeVaultInputCommand(t, client, stdin, "vault-provider-configs", "create", "--name", "checkout-client", "--provider", provider, "--credentials-file", path, "-o", "json")
				require.NoError(t, err)
				assert.Equal(t, 1, calls)
				assert.Empty(t, human)
				assert.False(t, strings.Contains(out, secret), "output must not contain credentials")
				assert.Contains(t, out, `"id": "config-1"`)
			})
		}
	}
}

func TestVaultProviderConfigUpdates(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		for _, rename := range []bool{false, true} {
			if !rotate && !rename {
				continue
			}
			t.Run(fmt.Sprint(rotate, rename), func(t *testing.T) {
				secret := vaultTestSecret(t)
				args := []string{"vault-provider-configs", "update", "config-1", "-o", "json"}
				if rename {
					args = append(args, "--name", "renamed")
				}
				if rotate {
					args = append(args, "--credentials-file", "-")
				}
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPatch, r.Method)
					assert.Equal(t, "/vault-provider-configs/config-1", r.URL.Path)
					var body map[string]json.RawMessage
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					_, hasName := body["name"]
					_, hasCredentials := body["credentials"]
					assert.Equal(t, rename, hasName)
					assert.Equal(t, rotate, hasCredentials)
					_, hasProvider := body["provider"]
					_, hasClientID := body["client_id"]
					assert.False(t, hasProvider)
					assert.False(t, hasClientID)
					if rotate {
						var credentials map[string]string
						require.NoError(t, json.Unmarshal(body["credentials"], &credentials))
						assert.Equal(t, 1, len(credentials))
						assert.True(t, credentials["client_secret"] == secret)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, providerConfigFixture)
				})
				out, _, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"client_secret":%q}`, secret), args...)
				require.NoError(t, err)
				assert.False(t, strings.Contains(out, secret))
			})
		}
	}
}

func TestVaultProviderConfigSafeOutput(t *testing.T) {
	secret := vaultTestSecret(t)
	body := strings.TrimSuffix(providerConfigFixture, "}") + fmt.Sprintf(`,"credentials":{"client_secret":%q},"access_token":%q}`, secret, secret)
	for _, operation := range []string{"get", "show", "list"} {
		for _, output := range []string{"", "json"} {
			t.Run(operation+"/"+output, func(t *testing.T) {
				args := []string{"vault-provider-configs", operation}
				if operation != "list" {
					args = append(args, "checkout-client")
				} else {
					args = append(args, "--limit", "1", "--offset", "20")
				}
				if output != "" {
					args = append(args, "-o", output)
				}
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					w.Header().Set("Content-Type", "application/json")
					if operation == "list" {
						assert.Equal(t, "/vault-provider-configs", r.URL.Path)
						assert.Equal(t, "1", r.URL.Query().Get("limit"))
						assert.Equal(t, "20", r.URL.Query().Get("offset"))
						w.Header().Set("X-Has-More", "true")
						w.Header().Set("X-Next-Offset", "21")
						_, _ = io.WriteString(w, "["+body+"]")
					} else {
						assert.Equal(t, "/vault-provider-configs/checkout-client", r.URL.Path)
						_, _ = io.WriteString(w, body)
					}
				})
				out, human, err := executeVaultInputCommand(t, client, "", args...)
				require.NoError(t, err)
				assert.False(t, strings.Contains(out+human, secret), "secret echoed by API must not be displayed")
				assert.NotContains(t, out+human, "client_secret")
				assert.Contains(t, out+human, "false", "false test mode must not be lost")
				if operation == "list" {
					if output == "json" {
						assert.Contains(t, out, `"next_offset": 21`)
					} else {
						assert.Contains(t, human, "--limit 1 --offset 21")
					}
				}
			})
		}
	}
}

func TestVaultProviderConfigInvalidInput(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached API") })
	for _, args := range []string{
		"create", "create --name .. --provider link --credentials-file -", "create --name good --provider other --credentials-file -",
		"get ../bad", "get config -o yaml", "list --limit 0", "list --limit 101", "list --offset -1",
		"update config", "update config --name=", "update config --client-id other", "update config --provider link",
		"delete config", "create --name good --provider link --credentials-file=", "update config --credentials-file=",
	} {
		t.Run(args, func(t *testing.T) {
			_, _, err := executeVaultInputCommand(t, client, "{}", append([]string{"vault-provider-configs"}, strings.Fields(args)...)...)
			require.Error(t, err)
		})
	}
	for _, raw := range []string{"", "null", "[]", "{}", "{} {}", `{"client_id":"client-1"}`, `{"client_id":false,"client_secret":null}`} {
		_, _, err := executeVaultInputCommand(t, client, raw, "vault-provider-configs", "create", "--name", "good", "--provider", "link", "--credentials-file", "-")
		require.Error(t, err)
	}
	secret := vaultTestSecret(t)
	for _, raw := range []string{secret, fmt.Sprintf(`{"client_id":"client-1","client_secret":%q,"unexpected":true}`, secret), strings.Repeat(secret, (1<<20)/len(secret)+1)} {
		out, human, err := executeVaultInputCommand(t, client, raw, "vault-provider-configs", "create", "--name", "good", "--provider", "link", "--credentials-file", "-")
		require.Error(t, err)
		assert.False(t, strings.Contains(out+human+err.Error(), secret))
	}
	_, _, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"client_id":"other","client_secret":%q}`, secret), "vault-provider-configs", "update", "config-1", "--credentials-file", "-")
	require.Error(t, err, "rotation cannot change identity")
}

func TestVaultProviderConfigErrorsAndDelete(t *testing.T) {
	for _, operation := range []string{"create", "get", "list", "update", "delete"} {
		for _, status := range []int{204, 400, 403, 404, 409, 429, 500} {
			if status == 204 && operation != "delete" {
				continue
			}
			for _, plain := range []bool{false, true} {
				t.Run(fmt.Sprint(operation, status, plain), func(t *testing.T) {
					secret := vaultTestSecret(t)
					calls := 0
					client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
						calls++
						if !plain {
							w.Header().Set("Content-Type", "application/json")
						}
						w.WriteHeader(status)
						if status != 204 {
							if plain {
								_, _ = io.WriteString(w, secret)
							} else {
								_, _ = fmt.Fprintf(w, `{"code":%q,"message":%q}`, secret, secret)
							}
						}
					})
					args := []string{"vault-provider-configs", operation}
					switch operation {
					case "create":
						args = append(args, "--name", "good", "--provider", "link", "--credentials-file", "-")
					case "get":
						args = append(args, "config-1")
					case "update":
						args = append(args, "config-1", "--name", "renamed")
					case "delete":
						args = append(args, "config-1", "--yes")
					}
					out, human, err := executeVaultInputCommand(t, client, fmt.Sprintf(`{"client_id":"client-1","client_secret":%q}`, secret), args...)
					assert.Equal(t, 1, calls, "SDK retries must be disabled")
					if operation == "delete" && (status == 204 || status == 404) {
						require.NoError(t, err)
						assert.Contains(t, human, "Deleted or not found")
					} else {
						require.Error(t, err)
						assert.Contains(t, err.Error(), fmt.Sprint(status))
						assert.False(t, strings.Contains(util.CleanedUpSdkError{Err: err}.Error(), secret), "root formatting must not expose response")
						assert.NotContains(t, human, "Deleted")
					}
					assert.False(t, strings.Contains(out+human, secret))
				})
			}
		}
	}
}

func TestVaultCredentialTransportAndFileErrorsAreSafe(t *testing.T) {
	secret := vaultTestSecret(t)
	err := vaultCredentialError(fmt.Errorf("transport: %w", errors.New(secret)))
	assert.False(t, strings.Contains(util.CleanedUpSdkError{Err: err}.Error(), secret))

	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unreadable input reached API") })
	out, human, err := executeVaultInputCommand(t, client, "", "vault-provider-configs", "update", "config-1", "--credentials-file", filepath.Join(t.TempDir(), secret))
	require.Error(t, err)
	assert.False(t, strings.Contains(out+human+err.Error(), secret), "do not echo paths supplied to secret input flags")
}

func TestVaultProviderConfigEmptyAndInvalidPagination(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Has-More", "false")
			if invalid {
				w.Header().Set("X-Has-More", "true")
				w.Header().Set("X-Next-Offset", "bad")
			}
			_, _ = io.WriteString(w, "[]")
		})
		out, _, err := executeVaultInputCommand(t, client, "", "vault-provider-configs", "list", "-o", "json")
		if invalid {
			require.ErrorContains(t, err, "pagination")
			assert.Empty(t, out)
		} else {
			require.NoError(t, err)
			assert.JSONEq(t, `{"vault_provider_configs":[]}`, out)
		}
	}
}
