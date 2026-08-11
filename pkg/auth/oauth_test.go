package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

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

func TestOAuthCodeExchangeUsesAuthoritativeProjectScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got, want := r.Form.Get("org_id"), "org_from_state"; got != want {
			t.Fatalf("org_id = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"org_id":        "org_authoritative",
			"access_scope":  "project",
			"project_id":    "proj_1",
		})
	}))
	defer server.Close()

	config := &OAuthConfig{
		Config: &oauth2.Config{
			ClientID:    "client-id",
			RedirectURL: "http://localhost/callback",
			Endpoint: oauth2.Endpoint{
				TokenURL:  server.URL,
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		Verifier:      "verifier",
		AuthBaseURL:   server.URL,
		OAuthClientID: "client-id",
	}

	tokens, err := config.exchangeCodeForTokens(
		context.Background(),
		"code",
		"org_from_state",
		"organization",
		"",
	)
	if err != nil {
		t.Fatalf("exchangeCodeForTokens() error = %v", err)
	}
	if got, want := tokens.OrgID, "org_authoritative"; got != want {
		t.Fatalf("OrgID = %q, want %q", got, want)
	}
	if got, want := tokens.AccessScope, "project"; got != want {
		t.Fatalf("AccessScope = %q, want %q", got, want)
	}
	if got, want := tokens.ProjectID, "proj_1"; got != want {
		t.Fatalf("ProjectID = %q, want %q", got, want)
	}
}

func TestRefreshTokensPreservesProjectScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got, want := r.Form.Get("refresh_token"), "refresh-old"; got != want {
			t.Fatalf("refresh_token = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-new",
			"refresh_token": "refresh-new",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"org_id":        "org_1",
			"access_scope":  "project",
			"project_id":    "proj_1",
		})
	}))
	defer server.Close()

	tokens, err := RefreshTokens(context.Background(), &TokenStorage{
		RefreshToken:  "refresh-old",
		ExpiresAt:     time.Now().Add(-time.Hour),
		OrgID:         "org_1",
		AccessScope:   "project",
		ProjectID:     "proj_1",
		AuthBaseURL:   server.URL,
		OAuthClientID: "client-id",
	})
	if err != nil {
		t.Fatalf("RefreshTokens() error = %v", err)
	}
	if tokens.AccessScope != "project" || tokens.ProjectID != "proj_1" {
		t.Fatalf("RefreshTokens() scope = %q project = %q", tokens.AccessScope, tokens.ProjectID)
	}
}

func TestRefreshLegacyTokensRemainOrganizationWide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-new",
			"refresh_token": "refresh-new",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	tokens, err := RefreshTokens(context.Background(), &TokenStorage{
		RefreshToken:  "refresh-old",
		OrgID:         "org_legacy",
		AuthBaseURL:   server.URL,
		OAuthClientID: "client-id",
	})
	if err != nil {
		t.Fatalf("RefreshTokens() error = %v", err)
	}
	if got, want := tokens.AccessScope, "organization"; got != want {
		t.Fatalf("AccessScope = %q, want %q", got, want)
	}
	if tokens.ProjectID != "" {
		t.Fatalf("ProjectID = %q, want empty", tokens.ProjectID)
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
