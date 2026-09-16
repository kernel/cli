package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const publicCredentialFixture = `{"id":"credential-1","key":"login","type":"credential","version":2,"spec":{"description":"Example","fields":{"username":{"type":"text","sensitive":false},"email":{"type":"email","sensitive":false},"password":{"type":"password"},"otp":{"type":"totp","sensitive":true}}},"state":{"status":"ready","fields":{"username":{"has_value":true,"value":"user-123"},"email":{"has_value":true,"value":"user@example.com"},"password":{"has_value":true,"value":"private-password"},"otp":{"has_value":true,"value":"private-seed"}}},"action":{"name":"collect","url":"https://vault.example/collect#token=user-123.token"},"available_operations":[{"type":"collect","description":"Open form"}],"available_expansions":[]}`

func TestVaultPublicValuesAcrossCommands(t *testing.T) {
	spec := credentialSpecFile(t, `{"fields":{"username":{"type":"text","sensitive":false,"value":"user-123"}}}`)
	update := credentialSpecFile(t, `{"fields":{"username":{"value":"user-123"}}}`)
	for _, args := range [][]string{
		{"vaults", "credentials", "create", "user-123", "login", "--spec-file", spec},
		{"vaults", "credentials", "update", "user-123", "login", "--version", "2", "--spec-file", update},
		{"vaults", "items", "get", "user-123", "login"},
		{"vaults", "items", "list", "user-123"},
		{"vaults", "items", "invoke", "user-123", "login", "collect"},
	} {
		t.Run(strings.Join(args[:3], " "), func(t *testing.T) {
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if args[2] == "list" {
					io.WriteString(w, "["+publicCredentialFixture+"]")
					return
				}
				io.WriteString(w, publicCredentialFixture)
			})
			out, _, err := executeVaultCommand(t, client, append(args, "-o", "json")...)
			require.NoError(t, err)
			assert.Contains(t, out, `"value": "user-123"`)
			assert.Contains(t, out, `"value": "user@example.com"`)
			assert.Contains(t, out, "https://vault.example/collect#token=user-123.token")
			assert.NotContains(t, out, "private-password")
			assert.NotContains(t, out, "private-seed")
		})
	}
}

func TestVaultPublicValueBoundary(t *testing.T) {
	for _, tc := range []struct {
		kind, sensitive   string
		hasValue, visible bool
	}{
		{"text", "false", true, true}, {"email", "false", true, true},
		{"text", "true", true, false}, {"text", "null", true, false},
		{"password", "false", true, false}, {"totp", "false", true, false},
		{"text", "false", false, false},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			raw := fmt.Sprintf(`{"type":"credential","spec":{"fields":{"field":{"type":%q,"sensitive":%s}}},"state":{"fields":{"field":{"has_value":%t,"value":"test-value"}}}}`, tc.kind, tc.sensitive, tc.hasValue)
			out, err := filterVaultJSON(json.RawMessage(raw), vaultItemFields)
			require.NoError(t, err)
			assert.Equal(t, tc.visible, strings.Contains(string(out), "test-value"))
		})
	}
}

func TestVaultFillActionableErrors(t *testing.T) {
	for _, tc := range []struct {
		status        int
		code, message string
	}{
		{400, "ambiguous_selector", "multiple targets"},
		{403, "destination_denied", "not authorized"},
		{404, "not_found", "identifiers"},
		{409, "conflict", "not ready"},
		{400, "field_unavailable", "no usable stored value"},
		{400, "private-unknown-code", "fill request failed"},
	} {
		t.Run(fmt.Sprint(tc.status, tc.code), func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					io.WriteString(w, readyFillCredentialFixture)
					return
				}
				require.Equal(t, http.MethodPost, r.Method)
				posts++
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"code":%q,"message":"private-upstream-detail"}`, tc.code)
			})
			out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user-123", "login", "fill", "--params", `{"browser_id":"browser-1","fields":[{"field":"missing","selector":"#field"}]}`, "-o", "json")
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("HTTP %d", tc.status))
			assert.Contains(t, err.Error(), tc.message)
			assert.Contains(t, err.Error(), "no fields were written")
			assert.NotContains(t, err.Error(), "may have been written")
			assert.NotContains(t, err.Error(), "private-")
			assert.Empty(t, out)
			assert.Equal(t, 1, posts)
		})
	}
}
