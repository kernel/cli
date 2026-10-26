package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const passkeyCredentialFixture = `{"id":"credential-1","key":"login","type":"credential","version":1,"spec":{"provider":"kernel","description":"Example","fields":[{"name":"passkey","type":"passkey","required":false,"sensitive":true}]},"state":{"provider":"kernel","status":"ready","fields":{"passkey":{"has_value":true,"type":"passkey"}}},"available_operations":[{"type":"arm_passkey","description":"Arm the stored passkey."}],"available_expansions":[]}`

func passkeyFixture(operation string) string {
	return strings.ReplaceAll(passkeyCredentialFixture, `"type":"arm_passkey","description":"Arm the stored passkey."`, `"type":"`+operation+`","description":"Use the advertised passkey operation."`)
}

func TestVaultPasskeyOperationsUseGeneratedSDKVariants(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	tests := []struct {
		operation    string
		flags        []string
		expectedBody string
		response     string
		expectedJSON string
	}{
		{
			operation:    "arm_passkey",
			flags:        []string{"--browser-id", "browser-session-id", "--field", "passkey"},
			expectedBody: `{"type":"arm_passkey","browser_id":"browser-session-id","field":"passkey"}`,
			response:     `{"type":"arm_passkey","status":"armed","rp_id":"example.com","opaque":"discard-me"}`,
			expectedJSON: `{"type":"arm_passkey","status":"armed","rp_id":"example.com"}`,
		},
		{
			operation:    "register_passkey",
			flags:        []string{"--browser-id", "browser-session-id", "--field", "passkey"},
			expectedBody: `{"type":"register_passkey","browser_id":"browser-session-id","field":"passkey"}`,
			response:     `{"type":"register_passkey","status":"armed","opaque":"discard-me"}`,
			expectedJSON: `{"type":"register_passkey","status":"armed"}`,
		},
		{
			operation:    "disarm_passkey",
			flags:        []string{"--browser-id", "browser-session-id"},
			expectedBody: `{"type":"disarm_passkey","browser_id":"browser-session-id"}`,
			response:     `{"type":"disarm_passkey","status":"disarmed","opaque":"discard-me"}`,
			expectedJSON: `{"type":"disarm_passkey","status":"disarmed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch calls {
				case 1:
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "/vaults/user-vault/items/login", r.URL.Path)
					_, _ = io.WriteString(w, passkeyFixture(tt.operation))
				case 2:
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/vaults/user-vault/items/login/operations", r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, tt.expectedBody, string(body))
					_, _ = io.WriteString(w, tt.response)
				default:
					t.Fatalf("unexpected API call %d", calls)
				}
			})
			args := []string{"vaults", "items", "invoke", "user-vault", "login", tt.operation}
			args = append(args, tt.flags...)
			args = append(args, "-o", "json")
			out, human, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
			assert.JSONEq(t, tt.expectedJSON, out)
			assert.Empty(t, human)
			assert.NotContains(t, out, "discard-me")
		})
	}
}

func TestVaultPasskeyOperationFlagsValidateBeforeAPI(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("invalid passkey operation reached API")
	})
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing browser", []string{"arm_passkey"}, "requires --browser-id with a browser session id, not a name"},
		{"empty browser", []string{"arm_passkey", "--browser-id", ""}, "non-empty browser session id"},
		{"empty field", []string{"arm_passkey", "--browser-id", "browser-id", "--field", ""}, "non-empty passkey field name"},
		{"long field", []string{"register_passkey", "--browser-id", "browser-id", "--field", strings.Repeat("x", 65)}, "at most 64 bytes"},
		{"page url", []string{"disarm_passkey", "--browser-id", "browser-id", "--page-url", "https://example.com"}, "--page-url is only supported for authorize"},
		{"params", []string{"arm_passkey", "--browser-id", "browser-id", "--params", `{"field":"passkey"}`}, "not --params or --spec-file"},
		{"open", []string{"arm_passkey", "--browser-id", "browser-id", "--open"}, "--open is only supported"},
		{"field on authorize", []string{"authorize", "--field", "passkey"}, "--field is only supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"vaults", "items", "invoke", "user-vault", "login"}
			args = append(args, tt.args...)
			_, _, err := executeVaultCommand(t, client, args...)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestVaultPasskeyOperationHumanOutputAndHints(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	fixture := strings.ReplaceAll(passkeyCredentialFixture,
		`[{"type":"arm_passkey","description":"Arm the stored passkey."}]`,
		`[{"type":"arm_passkey","description":"Arm it."},{"type":"register_passkey","description":"Register it."},{"type":"disarm_passkey","description":"Disarm it."}]`)
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"type":"arm_passkey","status":"armed","rp_id":"example.com"}`)
			return
		}
		_, _ = io.WriteString(w, fixture)
	})
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "user-vault", "login")
	require.NoError(t, err)
	assert.Contains(t, human, "--browser-id '<browser-session-id>' --field '<passkey-field>' -- user-vault login arm_passkey")
	assert.Contains(t, human, "--browser-id '<browser-session-id>' --field '<passkey-field>' -- user-vault login register_passkey")
	assert.Contains(t, human, "--browser-id '<browser-session-id>' -- user-vault login disarm_passkey")

	_, human, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "user-vault", "login", "arm_passkey", "--browser-id", "browser-session-id")
	require.NoError(t, err)
	assert.Contains(t, human, "relying party")
	assert.Contains(t, human, "example.com")
	assert.Contains(t, human, "does not mean a page has asked yet")
	assert.Equal(t, 3, calls)
}

func TestVaultPasskeyHelpMatchesSpec(t *testing.T) {
	cmd, _, err := newVaultsCommand().Find([]string{"items", "invoke"})
	require.NoError(t, err)
	require.NotNil(t, cmd.Flags().Lookup("field"))
	assert.Contains(t, cmd.Flags().Lookup("browser-id").Usage, "session id, not a reusable name")
	assert.Contains(t, cmd.Flags().Lookup("page-url").Usage, "checkout url")
	assert.Contains(t, cmd.Flags().Lookup("field").Usage, "disarm omits it to affect every field")
	for _, text := range []string{
		"browser session id, not a reusable browser name",
		"private key never enters the browser",
		"there is no time limit",
		"reload it",
		"without user interaction",
		"only ES256 passkeys are created",
		"without --field it affects every passkey field",
	} {
		assert.Contains(t, cmd.Long, text)
	}
	credentials := newVaultCredentialsCommand()
	assert.Contains(t, credentials.Long, "totp, and passkey")
	assert.Contains(t, credentials.Long, "FIDO Credential Exchange Format")
	assert.Contains(t, credentials.Long, "PKCS#8 ECDSA P-256")
	assert.Contains(t, credentials.Long, "passkeys are never filled")
}
