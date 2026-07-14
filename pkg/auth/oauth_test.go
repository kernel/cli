package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCallbackPageDoesNotClaimAuthenticationComplete(t *testing.T) {
	t.Parallel()

	if strings.Contains(authorizationReceivedHTML, "authentication successful") {
		t.Fatal("callback page must not claim authentication succeeded before token exchange completes")
	}
	if !strings.Contains(authorizationReceivedHTML, "authorization received") {
		t.Fatal("callback page should tell the user browser authorization was received")
	}
}

func TestTokenExchangeTimesOut(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	cfg, err := NewOAuthConfig()
	if err != nil {
		t.Fatalf("NewOAuthConfig() error = %v", err)
	}
	cfg.Config.Endpoint.TokenURL = server.URL

	done := make(chan error, 1)
	go func() {
		_, err := cfg.exchangeCodeForTokens(context.Background(), "code", "org", 25*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "token exchange timed out") || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("exchange error = %v, want token exchange timeout", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("token exchange did not honor its timeout")
	}
}

func TestNewOAuthConfigUsesAuthOverrides(t *testing.T) {
	t.Setenv("KERNEL_AUTH_BASE_URL", "https://auth.dev.onkernel.com/")
	t.Setenv("KERNEL_OAUTH_CLIENT_ID", "staging-client-id")

	cfg, err := NewOAuthConfig()
	if err != nil {
		t.Fatalf("NewOAuthConfig() error = %v", err)
	}

	if got, want := cfg.AuthBaseURL, "https://auth.dev.onkernel.com"; got != want {
		t.Fatalf("AuthBaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Config.Endpoint.AuthURL, "https://auth.dev.onkernel.com/authorize"; got != want {
		t.Fatalf("AuthURL = %q, want %q", got, want)
	}
	if got, want := cfg.Config.Endpoint.TokenURL, "https://auth.dev.onkernel.com/token"; got != want {
		t.Fatalf("TokenURL = %q, want %q", got, want)
	}
	if got, want := cfg.Config.ClientID, "staging-client-id"; got != want {
		t.Fatalf("ClientID = %q, want %q", got, want)
	}
}

func TestTokenRefreshConfigPrefersStoredValues(t *testing.T) {
	t.Setenv("KERNEL_AUTH_BASE_URL", "https://auth.dev.onkernel.com")
	t.Setenv("KERNEL_OAUTH_CLIENT_ID", "staging-client-id")

	tokens := &TokenStorage{
		AuthBaseURL:   "https://auth.saved.onkernel.com/",
		OAuthClientID: "saved-client-id",
	}

	if got, want := tokenAuthBaseURL(tokens), "https://auth.saved.onkernel.com"; got != want {
		t.Fatalf("tokenAuthBaseURL = %q, want %q", got, want)
	}
	if got, want := tokenOAuthClientID(tokens), "saved-client-id"; got != want {
		t.Fatalf("tokenOAuthClientID = %q, want %q", got, want)
	}
}

func TestLegacyTokenRefreshConfigUsesProdDefaults(t *testing.T) {
	t.Setenv("KERNEL_AUTH_BASE_URL", "https://auth.dev.onkernel.com")
	t.Setenv("KERNEL_OAUTH_CLIENT_ID", "staging-client-id")

	tokens := &TokenStorage{}

	if got, want := tokenAuthBaseURL(tokens), DefaultAuthBaseURL; got != want {
		t.Fatalf("tokenAuthBaseURL = %q, want %q", got, want)
	}
	if got, want := tokenOAuthClientID(tokens), DefaultClientID; got != want {
		t.Fatalf("tokenOAuthClientID = %q, want %q", got, want)
	}
}
