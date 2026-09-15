package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialUpdateIdentityPrecondition(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		name, expectedID string
		setID            bool
		status           int
	}{
		{name: "omitted", status: 200},
		{name: "bound", expectedID: "credential-original", setID: true, status: 200},
		{name: "replacement conflict", expectedID: "credential-original", setID: true, status: 409},
		{name: "empty", setID: true, status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "PATCH", r.Method)
				var body map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.JSONEq(t, `7`, string(body["version"]))
				if tc.setID {
					assert.JSONEq(t, `"credential-original"`, string(body["expected_item_id"]))
				} else {
					assert.NotContains(t, body, "expected_item_id")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					fmt.Fprint(w, credentialFixture)
				} else {
					fmt.Fprint(w, `{"message":"conflict"}`)
				}
			})
			args := []string{"vaults", "credentials", "update", "user-123", "login", "--version", "7", "--spec-file", credentialSpecFile(t, `{"fields":{"password":{"value":""}}}`), "-o", "json"}
			if tc.setID {
				args = append(args, "--expected-item-id", tc.expectedID)
			}
			_, _, err := executeVaultCommand(t, client, args...)
			if tc.name == "empty" {
				require.Error(t, err)
				assert.Zero(t, calls)
			} else {
				assert.Equal(t, 1, calls)
				if tc.status == 409 {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestCredentialInitialValuesWithReleasedSDK(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	spec := `{"description":"Example","fields":{"username":{"type":"text","sensitive":false,"value":"synthetic-user"},"email":{"type":"email","sensitive":false,"value":"test@example.com"},"password":{"type":"password","sensitive":true,"value":"synthetic-password"},"otp":{"type":"totp","sensitive":true,"value":"JBSWY3DPEHPK3PXP"}}}`
	calls := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "PUT", r.Method)
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, spec, string(body["spec"]))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"credential-1","key":"login","type":"credential","version":1,"spec":%s,"state":{"status":"ready","fields":{"otp":{"has_value":true,"value":"JBSWY3DPEHPK3PXP"}}},"available_operations":[],"available_expansions":[]}`, spec)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "credentials", "create", "user-123", "login", "--spec-file", credentialSpecFile(t, spec), "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	for _, value := range []string{"synthetic-user", "test@example.com", "synthetic-password", "JBSWY3DPEHPK3PXP"} {
		assert.NotContains(t, out, value)
	}
	assert.Contains(t, out, `"has_value": true`)
}

func TestVaultPreparationApprovalURLIsPrintedInFull(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	approvalURL := "https://approve.example/" + strings.Repeat("long-token", 40)
	fixture := strings.Replace(preparationCardFixture, `"action":{"name":"spend_approval","url":"https://approve.example/prepare"},`, "", 1)
	fixture = strings.Replace(fixture, "https://approve.example/prepare", approvalURL, 1)
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, fixture)
	})
	_, text, err := executeVaultCommand(t, client, "vaults", "items", "get", "user-123", "order-1")
	require.NoError(t, err)
	assert.Contains(t, text, "Approval URL:\n"+approvalURL+"\n")
	assert.NotContains(t, text, "Preparation approval URL")
}

func TestVaultFileValidationUsesFieldNames(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input must not reach the API")
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "user-123", "login", "fill", "--spec-file", credentialSpecFile(t, `{"browser_id":"","fields":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "browser_id")
	assert.NotContains(t, err.Error(), "--params")
}

func TestVaultPreparationEventsAreProjected(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"event-1","name":"preparation_ready","created_at":"2026-09-15T00:00:00Z","browser_id":"browser-1","data":{"preparation_id":"prep-1","reason":"ready","provider_secret":"never-print"}}]`)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "events", "user-123", "order-1", "-o", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"preparation_id": "prep-1"`)
	assert.NotContains(t, out, "never-print")
}
