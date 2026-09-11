package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const linkProviderConfigFixture = `{"id":"vpc-link-1","name":"my-link-client","provider":"link","client_id":"example-client-id","created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`
const agentcardProviderConfigFixture = `{"id":"vpc-ac-1","name":"my-agentcard","provider":"agentcard","client_id":"example-client-id","test_mode":true,"created_at":"2026-09-01T00:00:00Z","updated_at":"2026-09-01T00:00:00Z"}`

func TestVaultProviderConfigCommandConstruction(t *testing.T) {
	for _, path := range []string{"provider-configs create", "provider-configs list", "provider-configs get", "provider-configs update", "provider-configs delete"} {
		t.Run(path, func(t *testing.T) {
			cmd, remaining, err := newVaultsCommand().Find(strings.Fields(path))
			require.NoError(t, err)
			require.Empty(t, remaining)
			assert.NotNil(t, cmd.RunE)
			assert.NotNil(t, cmd.PreRunE)
			assert.NotNil(t, cmd.Args)
			if cmd.Name() == "delete" {
				assert.NotNil(t, cmd.Flags().Lookup("yes"))
				return
			}
			require.NotNil(t, cmd.Flags().Lookup("output"))
			assert.Contains(t, cmd.Flags().Lookup("output").Usage, "display-safe")
		})
	}

	create, _, err := newVaultsCommand().Find([]string{"provider-configs", "create"})
	require.NoError(t, err)
	for _, flag := range []string{"name", "provider", "client-id", "client-secret", "client-secret-file"} {
		assert.NotNil(t, create.Flags().Lookup(flag), flag)
	}
	list, _, err := newVaultsCommand().Find([]string{"provider-configs", "list"})
	require.NoError(t, err)
	assert.NotNil(t, list.Flags().Lookup("page"))
	assert.NotNil(t, list.Flags().Lookup("per-page"))
	// The endpoint pages with limit/offset, but those stay an implementation detail.
	assert.Nil(t, list.Flags().Lookup("limit"))
	assert.Nil(t, list.Flags().Lookup("offset"))
	// The client ID is immutable, so update must not offer to change it.
	update, _, err := newVaultsCommand().Find([]string{"provider-configs", "update"})
	require.NoError(t, err)
	assert.Nil(t, update.Flags().Lookup("client-id"))
	assert.Nil(t, update.Flags().Lookup("provider"))
}

func TestVaultProviderConfigCreate(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		provider string
		fixture  string
		usage    string
	}{
		{"link", linkProviderConfigFixture, "not through the CLI"},
		{"agentcard", agentcardProviderConfigFixture, `--provider agentcard --spec '{"provider_config": {"name": "my-agentcard"}}'`},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/vault-provider-configs", r.URL.Path)
				body, _ := io.ReadAll(r.Body)
				assert.JSONEq(t, `{"name":"cfg-1","provider":"`+tc.provider+`","credentials":{"client_id":"example-client-id","client_secret":"s3cret"}}`, string(body))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, tc.fixture)
			})
			out, human, err := executeVaultCommand(t, client,
				"vaults", "provider-configs", "create", "--name", "cfg-1", "--provider", tc.provider,
				"--client-id", "example-client-id", "--client-secret", "s3cret", "-o", "json")
			require.NoError(t, err)
			assert.JSONEq(t, tc.fixture, out)
			assert.Empty(t, human)

			_, human, err = executeVaultCommand(t, client,
				"vaults", "provider-configs", "create", "--name", "cfg-1", "--provider", tc.provider,
				"--client-id", "example-client-id", "--client-secret", "s3cret")
			require.NoError(t, err)
			assert.Contains(t, human, "example-client-id")
			assert.NotContains(t, human, "s3cret")
			assert.Contains(t, human, tc.usage)
		})
	}
}

func TestVaultProviderConfigCreateSecretSources(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	var sent string
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Credentials struct {
				ClientSecret string `json:"client_secret"`
			} `json:"credentials"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		sent = body.Credentials.ClientSecret
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, linkProviderConfigFixture)
	})

	// A file keeps the secret out of shell history, and its trailing newline is
	// an artifact of how the file was written rather than part of the secret.
	path := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("file-secret\n"), 0o600))
	_, _, err := executeVaultCommand(t, client,
		"vaults", "provider-configs", "create", "--name", "cfg-1", "--provider", "link",
		"--client-id", "example-client-id", "--client-secret-file", path, "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, "file-secret", sent)

	_, _, err = executeVaultCommand(t, client,
		"vaults", "provider-configs", "create", "--name", "cfg-1", "--provider", "link",
		"--client-id", "example-client-id", "--client-secret", "inline-secret", "--client-secret-file", path)
	require.ErrorContains(t, err, "not both")
}

func TestVaultProviderConfigCreateValidation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
	})
	for _, tc := range []struct {
		args    []string
		message string
	}{
		{[]string{"--name", "cfg-1", "--provider", "stripe", "--client-id", "id", "--client-secret", "s"}, "--provider must be link or agentcard"},
		{[]string{"--name", "bad name", "--provider", "link", "--client-id", "id", "--client-secret", "s"}, "--name must contain"},
		{[]string{"--name", "cfg-1", "--provider", "link", "--client-id", "id"}, "--client-secret"},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "provider-configs", "create"}, tc.args...)...)
		require.ErrorContains(t, err, tc.message)
	}
}

func TestVaultProviderConfigListPagination(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	body := "[" + linkProviderConfigFixture + "," + agentcardProviderConfigFixture + "]"
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vault-provider-configs", r.URL.Path)
		// One extra item over --per-page is requested so the response itself
		// reveals whether another page exists.
		assert.Equal(t, "2", r.URL.Query().Get("limit"))
		assert.Equal(t, "1", r.URL.Query().Get("offset"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	out, _, err := executeVaultCommand(t, client, "vaults", "provider-configs", "list", "--page", "2", "--per-page", "1", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"provider_configs":[`+linkProviderConfigFixture+`],"page":2,"per_page":1,"has_more":true}`, out)

	_, human, err := executeVaultCommand(t, client, "vaults", "provider-configs", "list", "--page", "2", "--per-page", "1")
	require.NoError(t, err)
	assert.Contains(t, human, "Page: 2  Per-page: 1  Items this page: 1  Has more: yes")
	assert.Contains(t, human, "Next: kernel vaults provider-configs list --page 3 --per-page 1")
}

func TestVaultProviderConfigListFooterWithoutMorePages(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "["+agentcardProviderConfigFixture+"]")
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "provider-configs", "list")
	require.NoError(t, err)
	assert.Contains(t, human, "Page: 1  Per-page: 20  Items this page: 1  Has more: no")
	assert.NotContains(t, human, "Next:")
	// Only AgentCard configurations report a provider-introspected mode.
	assert.Contains(t, human, "sandbox")
}

func TestVaultProviderConfigGetAndUpdate(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vault-provider-configs/my-link-client", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			assert.JSONEq(t, `{"name":"renamed","credentials":{"client_secret":"rotated"}}`, string(body))
		} else {
			assert.Equal(t, http.MethodGet, r.Method)
		}
		_, _ = io.WriteString(w, linkProviderConfigFixture)
	})

	out, _, err := executeVaultCommand(t, client, "vaults", "provider-configs", "get", "my-link-client", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, linkProviderConfigFixture, out)

	_, human, err := executeVaultCommand(t, client, "vaults", "provider-configs", "update", "my-link-client",
		"--name", "renamed", "--client-secret", "rotated")
	require.NoError(t, err)
	assert.Contains(t, human, "Updated provider configuration")
	assert.NotContains(t, human, "rotated")
	assert.Contains(t, human, "does not rebind existing wallets")
}

func TestVaultProviderConfigUpdateRequiresAField(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "provider-configs", "update", "my-link-client")
	require.ErrorContains(t, err, "nothing to update")
	// An empty secret would otherwise read as "clear it", which the API cannot do.
	_, _, err = executeVaultCommand(t, client, "vaults", "provider-configs", "update", "my-link-client", "--client-secret", "")
	require.ErrorContains(t, err, "must not be empty")
}

func TestVaultProviderConfigDelete(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/vault-provider-configs/my-link-client", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "provider-configs", "delete", "my-link-client", "-y")
	require.NoError(t, err)
	assert.Contains(t, human, "Deleted provider configuration: my-link-client")
	assert.Contains(t, human, "still exists")
}

func TestVaultProviderConfigDeleteConflictIsSurfaced(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"code":"conflict","message":"vault items still reference this configuration"}`)
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "provider-configs", "delete", "my-link-client", "-y")
	require.ErrorContains(t, err, "vault items still reference this configuration")
}

func TestVaultProviderConfigNotFoundDeleteIsQuiet(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"not_found","message":"no such configuration"}`)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "provider-configs", "delete", "gone", "-y")
	require.NoError(t, err)
	assert.Contains(t, human, "not found")
}

// Wallet specs select a configuration by id or name only; the JSON projection
// must keep that reference and still drop unknown provider data around it.
func TestVaultWalletProviderConfigIsDisplayed(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	const fixture = `{"id":"wallet-id","key":"wallet-1","type":"wallet","spec":{"provider":"agentcard","user_id":"usr_1","provider_config":{"id":"vpc-ac-1","name":"my-agentcard","opaque":"drop-me"}},"state":{"provider":"agentcard","status":"connected"},"available_operations":[],"available_expansions":[]}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "wallet-1", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"vpc-ac-1"`)
	assert.Contains(t, out, `"my-agentcard"`)
	assert.NotContains(t, out, "drop-me")
}

func TestVaultWalletSpecHelpDocumentsProviderConfig(t *testing.T) {
	cmd, _, err := newVaultsCommand().Find([]string{"wallets", "create"})
	require.NoError(t, err)
	assert.Contains(t, cmd.Long, "provider_config?:")
	assert.Contains(t, cmd.Long, "vaults provider-configs")
	// Importing a Link grant means handling OAuth tokens, which this surface
	// never accepts.
	assert.Contains(t, cmd.Long, "must never be passed to the CLI")
	assert.NotContains(t, cmd.Long, "access_token")
}
