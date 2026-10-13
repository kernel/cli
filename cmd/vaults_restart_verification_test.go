package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const restartVerificationCardFixture = `{"id":"item-1","key":"visa-order","type":"card","spec":{"provider":"kernel","wallet":"wallet-1","amount":1234,"currency":"usd"},"state":{"provider":"kernel","status":"recovery_required","status_reason":"Initial attestation failed"},"available_operations":[{"type":"restart_verification","description":"Prepare a fresh verification link."}],"available_expansions":[]}`

func TestVaultRestartVerificationDispatch(t *testing.T) {
	pending := strings.Replace(strings.Replace(restartVerificationCardFixture, `"recovery_required"`, `"pending_authorization"`, 1), `"available_operations":[{"type":"restart_verification","description":"Prepare a fresh verification link."}]`, `"action":{"name":"spend_approval","url":"https://approve.example/verify"},"available_operations":[]`, 1)
	var posts int
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			body, _ := io.ReadAll(r.Body)
			assert.JSONEq(t, `{"type":"restart_verification"}`, string(body))
			_, _ = io.WriteString(w, pending)
			return
		}
		_, _ = io.WriteString(w, restartVerificationCardFixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "visa-order", "restart_verification", "-o", "json")
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	assert.Contains(t, out, "pending_authorization")
}

func TestVaultRestartVerificationOnlyOperationInRecovery(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Replace(restartVerificationCardFixture, `]`, `,{"type":"authorize","description":"stale"}]`, 1))
	})
	_, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "visa-order", "authorize")
	require.ErrorContains(t, err, "only restart_verification is available")

	client = vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Replace(restartVerificationCardFixture, `"restart_verification"`, `"authorize"`, 1))
	})
	_, _, err = executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "visa-order", "restart_verification")
	require.ErrorContains(t, err, "reconcile the original operation")
}

func TestVaultRestartVerificationGuidance(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, restartVerificationCardFixture)
	})
	_, human, err := executeVaultInputCommand(t, client, "", "vaults", "items", "get", "checkout", "visa-order")
	require.NoError(t, err)
	assert.Contains(t, human, "available operation: restart_verification")
	assert.Contains(t, human, "does not approve spending")
	assert.NotContains(t, human, "no reset operation exists")
}
