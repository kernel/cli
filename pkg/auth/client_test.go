package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedClientUsesConfiguredBaseURL(t *testing.T) {
	requested := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/org/projects", r.URL.Path)
		assert.Equal(t, "Bearer local-test-key", r.Header.Get("Authorization"))
		requested <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("KERNEL_API_KEY", "local-test-key")
	t.Setenv("KERNEL_BASE_URL", server.URL)

	client, err := GetAuthenticatedClient()
	require.NoError(t, err)
	_, err = client.Projects.List(context.Background(), kernel.ProjectListParams{
		Limit: param.NewOpt[int64](1),
	})
	require.NoError(t, err)
	select {
	case <-requested:
	default:
		t.Fatal("configured Kernel base URL was not called")
	}
}

func TestRefreshRequiresLoginOnlyForRejectedCredentials(t *testing.T) {
	assert.True(t, refreshRequiresLogin(&TokenRefreshError{StatusCode: http.StatusBadRequest}))
	assert.True(t, refreshRequiresLogin(&TokenRefreshError{StatusCode: http.StatusUnauthorized}))
	assert.False(t, refreshRequiresLogin(&TokenRefreshError{StatusCode: http.StatusInternalServerError}))
	assert.False(t, refreshRequiresLogin(errors.New("network unavailable")))
}
