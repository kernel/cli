package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// refreshServer answers every token request with status and an OAuth error
// body, and counts the requests.
func refreshServer(t *testing.T, status int, code string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		// Slow enough that concurrent commands queue on the credentials lock.
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func saveExpiredTokens(t *testing.T, authBaseURL string) {
	t.Helper()
	if err := SaveTokens(&TokenStorage{
		AccessToken: "access-0", RefreshToken: "refresh-0", ExpiresAt: time.Now().Add(-time.Minute),
		OrgID: "org", AuthBaseURL: authBaseURL, OAuthClientID: "client-id",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRejectedRefreshTokenIsNotPresentedAgain(t *testing.T) {
	storageFixture(t)
	t.Setenv("KERNEL_API_KEY", "")
	server, requests := refreshServer(t, http.StatusBadRequest, "invalid_grant")
	saveExpiredTokens(t, server.URL)

	if _, err := GetAuthenticatedClient(); !errors.Is(err, errRefreshTokenRejected) {
		t.Fatalf("first GetAuthenticatedClient() error = %v, want the rejection", err)
	}
	loaded, err := LoadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RefreshToken != "" {
		t.Fatalf("saved refresh token = %q, want it cleared", loaded.RefreshToken)
	}
	if loaded.OrgID != "org" || loaded.AuthBaseURL != server.URL {
		t.Fatalf("saved credentials = %+v, want the rest kept", loaded)
	}

	for range 3 {
		_, err := GetAuthenticatedClient()
		if !errors.Is(err, errSessionEnded) {
			t.Fatalf("later GetAuthenticatedClient() error = %v, want errSessionEnded", err)
		}
		if !strings.Contains(err.Error(), "kernel login") {
			t.Fatalf("error %q does not tell the user to run kernel login", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1: later commands must not call the server", got)
	}
}

// Commands waiting on the credentials lock while another clears a rejected
// refresh token must stop too, not use the expired access token.
func TestConcurrentCommandsStopAfterRejection(t *testing.T) {
	storageFixture(t)
	t.Setenv("KERNEL_API_KEY", "")
	server, requests := refreshServer(t, http.StatusBadRequest, "invalid_grant")
	saveExpiredTokens(t, server.URL)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := GetAuthenticatedClient()
			if !errors.Is(err, errRefreshTokenRejected) && !errors.Is(err, errSessionEnded) {
				t.Errorf("GetAuthenticatedClient() error = %v, want the rejection or errSessionEnded", err)
			}
		}()
	}
	wg.Wait()
	if got := requests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
}

// A command that waited on the lock while another cleared the refresh token
// re-reads expired credentials without one and must end the session.
func TestRefreshAfterTheTokenWasClearedEndsTheSession(t *testing.T) {
	storageFixture(t)
	if err := SaveTokens(&TokenStorage{AccessToken: "access-0", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	tokens, err := refreshStoredTokens(context.Background())
	if !errors.Is(err, errSessionEnded) || tokens != nil {
		t.Fatalf("refreshStoredTokens() = %+v, %v, want errSessionEnded", tokens, err)
	}
}

func TestFailedRefreshKeepsTheRefreshToken(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
	}{
		{"server error", http.StatusInternalServerError, "server_error"},
		{"temporarily unavailable", http.StatusServiceUnavailable, "temporarily_unavailable"},
		{"other client error", http.StatusBadRequest, "invalid_request"},
		{"unauthorized client", http.StatusUnauthorized, "invalid_client"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storageFixture(t)
			t.Setenv("KERNEL_API_KEY", "")
			server, requests := refreshServer(t, tc.status, tc.code)
			saveExpiredTokens(t, server.URL)

			for range 2 {
				_, err := GetAuthenticatedClient()
				if err == nil || errors.Is(err, errRefreshTokenRejected) || errors.Is(err, errSessionEnded) {
					t.Fatalf("GetAuthenticatedClient() error = %v, want a retryable refresh failure", err)
				}
			}
			if got := requests.Load(); got != 2 {
				t.Fatalf("token requests = %d, want 2: a transient failure is retried", got)
			}
			loaded, err := LoadTokens()
			if err != nil || loaded.RefreshToken != "refresh-0" {
				t.Fatalf("LoadTokens() = %+v, %v, want the refresh token kept", loaded, err)
			}
		})
	}
}

// A rejection only clears the token that was rejected: credentials saved by
// another process in the meantime are kept.
func TestForgetRefreshTokenKeepsReplacedCredentials(t *testing.T) {
	storageFixture(t)
	if err := SaveTokens(&TokenStorage{AccessToken: "access-1", RefreshToken: "refresh-1"}); err != nil {
		t.Fatal(err)
	}
	if err := withCredentialsLock(func() error {
		forgetRefreshToken("refresh-0")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTokens()
	if err != nil || loaded.RefreshToken != "refresh-1" {
		t.Fatalf("LoadTokens() = %+v, %v, want refresh-1 kept", loaded, err)
	}
}

func TestIsExpiredRefreshesFiveMinutesEarly(t *testing.T) {
	cases := []struct {
		left time.Duration
		want bool
	}{
		{-time.Second, true},
		{time.Minute, true},
		{4 * time.Minute, true},
		{6 * time.Minute, false},
	}
	for _, tc := range cases {
		tokens := &TokenStorage{ExpiresAt: time.Now().Add(tc.left)}
		if got := tokens.IsExpired(); got != tc.want {
			t.Errorf("IsExpired() with %s left = %v, want %v", tc.left, got, tc.want)
		}
	}
}
