package auth

import "testing"

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
