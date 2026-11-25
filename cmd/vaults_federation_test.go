package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kernel/cli/pkg/auth"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With workload identity federation, a request the API answers 401 is sent
// again with a new token. Vault writes are the exception: they are sent once.
func TestVaultWritesAreNotReplayedAfterARejectedFederationToken(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	issued := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			issued++
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("access-token-%d", issued), "token_type": "Bearer", "expires_in": 300})
			return
		}
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized","message":"invalid token"}`))
	}))
	defer server.Close()

	tokenFile := filepath.Join(t.TempDir(), "identity-token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("identity-token"), 0600))
	for name, value := range map[string]string{
		"KERNEL_API_KEY": "", "KERNEL_BASE_URL": server.URL, "KERNEL_AUTH_BASE_URL": server.URL,
		"KERNEL_FEDERATION_RULE_ID": "wifr_vaults", "KERNEL_IDENTITY_TOKEN_SOURCE": "", "KERNEL_IDENTITY_TOKEN": "",
		"KERNEL_IDENTITY_TOKEN_FILE": tokenFile, auth.EnvFederationTokenCache: "off",
	} {
		t.Setenv(name, value)
	}
	client, err := auth.GetAuthenticatedClient()
	require.NoError(t, err)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		send func(opts ...option.RequestOption) error
	}{
		{"with a body", func(opts ...option.RequestOption) error {
			_, err := client.Vaults.Upsert(ctx, kernel.VaultUpsertParams{Name: "user"}, opts...)
			return err
		}},
		{"without a body", func(opts ...option.RequestOption) error {
			return client.Vaults.Delete(ctx, "user", opts...)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sent := func(send func() error) int {
				mu.Lock()
				requests = nil
				mu.Unlock()
				require.Error(t, send())
				mu.Lock()
				defer mu.Unlock()
				return len(requests)
			}
			assert.Equal(t, 1, sent(func() error { return tc.send(vaultWriteRequestOptions()...) }), "a vault write is sent once")
			assert.Equal(t, 2, sent(func() error { return tc.send(option.WithMaxRetries(0)) }), "the same request without the vault options is sent again")
		})
	}
}
