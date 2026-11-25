package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/lib/credentials"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/zalando/go-keyring"
)

// federationStandIns are local servers standing in for the GitHub Actions
// OIDC endpoint, the Kernel token endpoint and the Kernel API.
type federationStandIns struct {
	api, auth, github *httptest.Server

	mu sync.Mutex
	// exchanges is the federation rule ID of every token request.
	exchanges []string
	// expiresIn is the lifetime the token endpoint reports, in seconds.
	expiresIn int
	// refuseExchange makes the token endpoint answer invalid_grant.
	refuseExchange bool
	// refuseIdentityToken makes the GitHub Actions endpoint answer 403.
	refuseIdentityToken bool
	identityTokens      int
	// apiRequests is "<Authorization> <body>" of every API request.
	apiRequests []string
	// apiStatus is the status the API answers a request with.
	apiStatus func(accessToken string) int
	// assertions is the identity token of every token request.
	assertions []string
}

// change updates the stand-ins' behavior between requests.
func (s *federationStandIns) change(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// rejectOnly makes the API answer 401 to one access token, as when it is
// revoked.
func rejectOnly(accessToken string) func(string) int {
	return func(got string) int {
		if got == accessToken {
			return http.StatusUnauthorized
		}
		return http.StatusOK
	}
}

func (s *federationStandIns) exchangeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.exchanges)
}

func (s *federationStandIns) seenByAPI() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.apiRequests...)
}

// federationFixture points the environment at stand-in servers and makes it
// look like a GitHub Actions job with workload identity federation set up.
func federationFixture(t *testing.T) *federationStandIns {
	t.Helper()
	storageFixture(t)
	s := &federationStandIns{expiresIn: 300, apiStatus: func(string) int { return http.StatusOK }}

	s.github = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.refuseIdentityToken || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer request-credential-") || r.URL.Query().Get("audience") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		s.identityTokens++
		_ = json.NewEncoder(w).Encode(map[string]string{"value": fmt.Sprintf("identity-token-%d", s.identityTokens)})
	}))
	t.Cleanup(s.github.Close)

	s.auth = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/token" || s.refuseExchange || r.Form.Get("assertion") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "no rule matched"})
			return
		}
		s.exchanges = append(s.exchanges, r.Form.Get("federation_rule_id"))
		s.assertions = append(s.assertions, r.Form.Get("assertion"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("access-token-%d", len(s.exchanges)),
			"token_type":   "Bearer",
			"expires_in":   s.expiresIn,
		})
	}))
	t.Cleanup(s.auth.Close)

	s.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		defer s.mu.Unlock()
		authorization := r.Header.Get("Authorization")
		s.apiRequests = append(s.apiRequests, strings.TrimSpace(authorization+" "+string(body)))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.apiStatus(strings.TrimPrefix(authorization, "Bearer ")))
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(s.api.Close)

	for name, value := range map[string]string{
		"KERNEL_API_KEY":                     "",
		"KERNEL_BASE_URL":                    s.api.URL,
		"KERNEL_AUTH_BASE_URL":               s.auth.URL,
		credentials.EnvFederationRuleID:      "wifr_test",
		credentials.EnvIdentityTokenSource:   "",
		credentials.EnvIdentityTokenFile:     "",
		credentials.EnvIdentityToken:         "",
		credentials.EnvIdentityTokenAudience: "",
		EnvFederationTokenCache:              "",
		"GITHUB_ACTIONS":                     "true",
		"RUNNER_TEMP":                        t.TempDir(),
		"ACTIONS_ID_TOKEN_REQUEST_URL":       s.github.URL + "/idtoken?api-version=2",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN":     "request-credential-1",
		"GITHUB_REPOSITORY":                  "acme/app",
		"GITHUB_RUN_ID":                      "100",
		"GITHUB_RUN_ATTEMPT":                 "1",
		"GITHUB_JOB":                         "deploy",
	} {
		t.Setenv(name, value)
	}
	return s
}

// newProcess authenticates the way a new CLI process does: nothing is carried
// over from an earlier call except what is on disk.
func newProcess(t *testing.T) *kernel.Client {
	t.Helper()
	client, err := GetAuthenticatedClient(option.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("GetAuthenticatedClient() error = %v", err)
	}
	return client
}

func callAPI(client *kernel.Client) error {
	var out map[string]any
	return client.Get(context.Background(), "ping", nil, &out)
}

func mustCallAPI(t *testing.T, client *kernel.Client) {
	t.Helper()
	if err := callAPI(client); err != nil {
		t.Fatalf("API request error = %v", err)
	}
}

func saveLogin(t *testing.T) {
	t.Helper()
	if err := SaveTokens(&TokenStorage{AccessToken: "login-token", RefreshToken: "login-refresh", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
}

func requireLoginUntouched(t *testing.T) {
	t.Helper()
	loaded, err := LoadTokens()
	if err != nil || loaded.AccessToken != "login-token" || loaded.RefreshToken != "login-refresh" {
		t.Fatalf("saved login = %+v, %v, want it untouched", loaded, err)
	}
}

func TestAuthenticationPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name                string
		apiKey, rule, login bool
		want                string
		wantExchanges       int
	}{
		{name: "API key beats federation and a saved login", apiKey: true, rule: true, login: true, want: "Bearer api-key"},
		{name: "API key alone", apiKey: true, want: "Bearer api-key"},
		{name: "federation beats a saved login", rule: true, login: true, want: "Bearer access-token-1", wantExchanges: 1},
		{name: "federation alone", rule: true, want: "Bearer access-token-1", wantExchanges: 1},
		{name: "saved login", login: true, want: "Bearer login-token"},
		{name: "nothing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := federationFixture(t)
			if tc.apiKey {
				t.Setenv("KERNEL_API_KEY", "api-key")
			}
			if !tc.rule {
				t.Setenv(credentials.EnvFederationRuleID, "")
			}
			if tc.login {
				saveLogin(t)
			}

			client, err := GetAuthenticatedClient(option.WithMaxRetries(0))
			if tc.want == "" {
				if err == nil || !strings.Contains(err.Error(), "no authentication available") {
					t.Fatalf("GetAuthenticatedClient() error = %v, want no authentication available", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetAuthenticatedClient() error = %v", err)
			}
			mustCallAPI(t, client)
			if got := s.seenByAPI(); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("API saw %q, want %q", got, tc.want)
			}
			if got := s.exchangeCount(); got != tc.wantExchanges {
				t.Fatalf("token exchanges = %d, want %d", got, tc.wantExchanges)
			}
			if tc.login {
				requireLoginUntouched(t)
			}
		})
	}
}

func TestFederationFailureDoesNotFallBackToTheSavedLogin(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, s *federationStandIns)
		want  string
	}{
		{"no identity token source", func(t *testing.T, s *federationStandIns) {
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
		}, "no identity token source could be configured"},
		{"identity provider refuses", func(t *testing.T, s *federationStandIns) {
			s.change(func() { s.refuseIdentityToken = true })
		}, "could not get an identity token"},
		{"exchange refused", func(t *testing.T, s *federationStandIns) {
			s.change(func() { s.refuseExchange = true })
		}, "invalid_grant"},
		{"authorization server unreachable", func(t *testing.T, s *federationStandIns) {
			s.auth.Close()
		}, "token request failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := federationFixture(t)
			saveLogin(t)
			tc.setup(t, s)

			_, err := GetAuthenticatedClient()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("GetAuthenticatedClient() error = %v, want %q", err, tc.want)
			}
			if got := s.seenByAPI(); len(got) != 0 {
				t.Fatalf("API saw %q, want no request", got)
			}
			requireLoginUntouched(t)
		})
	}
}

func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "federation-token-*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestFederationTokenIsReusedByLaterProcesses(t *testing.T) {
	s := federationFixture(t)
	for range 3 {
		client := newProcess(t)
		mustCallAPI(t, client)
		mustCallAPI(t, client)
	}
	// One exchange also shows that the SDK's own federation support, which
	// reads the same environment, did not get a token of its own.
	if got := s.exchangeCount(); got != 1 {
		t.Fatalf("token exchanges = %d, want 1", got)
	}
	for _, seen := range s.seenByAPI() {
		if seen != "Bearer access-token-1" {
			t.Fatalf("API saw %q, want the cached token on every request", s.seenByAPI())
		}
	}

	// In a GitHub Actions job the token is kept in the job's temporary
	// directory only, readable by the owner.
	files := cacheFiles(t, filepath.Join(os.Getenv("RUNNER_TEMP"), "kernel-cli"))
	if len(files) != 1 {
		t.Fatalf("cache files in RUNNER_TEMP = %q, want one", files)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(files[0])
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("cache file mode = %v, %v, want 0600", info.Mode().Perm(), err)
		}
	}
	configDir, _ := getConfigDir()
	if files := cacheFiles(t, configDir); len(files) != 0 {
		t.Fatalf("cache files in the config directory = %q, want none", files)
	}
	cache, _ := federationCacheFromEnv("wifr_test")
	if _, err := keyring.Get(KeyringService, cache.name); err != keyring.ErrNotFound {
		t.Fatalf("keychain entry error = %v, want none stored", err)
	}
}

func TestCachedTokenIsReusedOnlyWithMoreThanSixtySecondsLeft(t *testing.T) {
	for _, tc := range []struct {
		left          time.Duration
		wantExchanges int
	}{
		{-time.Minute, 1},
		{0, 1},
		{55 * time.Second, 1},
		{90 * time.Second, 0},
		{5 * time.Minute, 0},
	} {
		t.Run(tc.left.String(), func(t *testing.T) {
			s := federationFixture(t)
			cache, _ := federationCacheFromEnv("wifr_test")
			if err := cache.save(&credentials.Token{AccessToken: "cached-token", ExpiresAt: time.Now().Add(tc.left)}); err != nil {
				t.Fatal(err)
			}
			mustCallAPI(t, newProcess(t))
			if got := s.exchangeCount(); got != tc.wantExchanges {
				t.Fatalf("token exchanges = %d, want %d", got, tc.wantExchanges)
			}
			want := "Bearer cached-token"
			if tc.wantExchanges == 1 {
				want = "Bearer access-token-1"
			}
			if got := s.seenByAPI(); len(got) != 1 || got[0] != want {
				t.Fatalf("API saw %q, want %q", got, want)
			}
		})
	}
}

// A token loaded with just over a minute left is not used once it has less.
func TestCachedTokenIsNotSentWithLessThanSixtySecondsLeft(t *testing.T) {
	s := federationFixture(t)
	cache, _ := federationCacheFromEnv("wifr_test")
	if err := cache.save(&credentials.Token{AccessToken: "cached-token", ExpiresAt: time.Now().Add(61 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	client := newProcess(t)
	mustCallAPI(t, client)
	time.Sleep(1200 * time.Millisecond)
	mustCallAPI(t, client)
	want := []string{"Bearer cached-token", "Bearer access-token-1"}
	if got := s.seenByAPI(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("API saw %q, want %q", got, want)
	}
}

// The token file can be replaced between the cache lookup and the exchange.
// The new token is then cached for the identity that was exchanged, not for
// the one the lookup saw.
func TestTokenIsCachedForTheIdentityThatWasExchanged(t *testing.T) {
	s := federationFixture(t)
	path := useTokenFile(t, "identity-token-a")
	f, err := newFederation()
	if err != nil {
		t.Fatal(err)
	}
	exchange := f.exchange
	f.exchange = func(ctx context.Context, force bool) (*credentials.Token, error) {
		if err := os.WriteFile(path, []byte("identity-token-b\n"), 0600); err != nil {
			t.Error(err)
		}
		return exchange(ctx, force)
	}
	if _, err := f.tokens.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.change(func() {
		if got := s.assertions; len(got) != 1 || got[0] != "identity-token-b" {
			t.Errorf("exchanged identity tokens = %q, want identity-token-b", got)
		}
	})

	cache, _ := federationCacheFromEnv("wifr_test")
	if cache.load() == nil {
		t.Fatal("the workload that exchanged the token cannot load it")
	}
	if err := os.WriteFile(path, []byte("identity-token-a"), 0600); err != nil {
		t.Fatal(err)
	}
	cache, _ = federationCacheFromEnv("wifr_test")
	if cache.load() != nil {
		t.Fatal("the workload the lookup saw can load a token exchanged for another identity")
	}
}

// leaveGitHubActions makes the environment a machine that is not running a
// GitHub Actions job.
func leaveGitHubActions(t *testing.T) {
	t.Helper()
	for _, name := range []string{"GITHUB_ACTIONS", "RUNNER_TEMP", "ACTIONS_ID_TOKEN_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"} {
		t.Setenv(name, "")
	}
}

// useTokenFile takes the identity token from a file, outside GitHub Actions.
func useTokenFile(t *testing.T, content string) string {
	t.Helper()
	leaveGitHubActions(t)
	path := filepath.Join(t.TempDir(), "identity-token")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(credentials.EnvIdentityTokenFile, path)
	return path
}

func fromTokenFile(t *testing.T) { useTokenFile(t, "identity-token-from-file") }

// Each case changes one thing about the environment after a token was cached.
// The changed environment must exchange a token of its own.
func TestCachedTokenIsNotSharedAcrossRulesServersOrWorkloads(t *testing.T) {
	for _, tc := range []struct {
		name string
		// source replaces the GitHub Actions job with another identity token
		// source before the first token is cached.
		source func(t *testing.T)
		change func(t *testing.T, s *federationStandIns)
		// sameEntry is set when the change leaves the entry's name alone, so
		// the new workload's token replaces the first one instead of sitting
		// beside it.
		sameEntry bool
	}{
		{name: "federation rule", change: func(t *testing.T, s *federationStandIns) {
			t.Setenv(credentials.EnvFederationRuleID, "wifr_other")
		}},
		{name: "authorization server", change: func(t *testing.T, s *federationStandIns) {
			other := httptest.NewServer(s.auth.Config.Handler)
			t.Cleanup(other.Close)
			t.Setenv("KERNEL_AUTH_BASE_URL", other.URL)
		}},
		{name: "API origin", change: func(t *testing.T, s *federationStandIns) {
			other := httptest.NewServer(s.api.Config.Handler)
			t.Cleanup(other.Close)
			t.Setenv("KERNEL_BASE_URL", other.URL)
		}},
		{name: "identity token audience", change: func(t *testing.T, s *federationStandIns) {
			t.Setenv(credentials.EnvIdentityTokenAudience, "another-audience")
		}},
		{name: "repository", change: func(t *testing.T, s *federationStandIns) {
			t.Setenv("GITHUB_REPOSITORY", "other/app")
		}},
		{name: "workflow run", change: func(t *testing.T, s *federationStandIns) {
			t.Setenv("GITHUB_RUN_ID", "101")
		}},
		{name: "run attempt", change: func(t *testing.T, s *federationStandIns) {
			t.Setenv("GITHUB_RUN_ATTEMPT", "2")
		}},
		{name: "job", change: func(t *testing.T, s *federationStandIns) {
			t.Setenv("GITHUB_JOB", "test")
		}},
		// Two legs of a matrix job share the repository, run, attempt and
		// job name, and can still run as different identities.
		{name: "job instance", sameEntry: true, change: func(t *testing.T, s *federationStandIns) {
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "request-credential-2")
		}},
		{name: "identity token file path", source: fromTokenFile, change: func(t *testing.T, s *federationStandIns) {
			other := filepath.Join(t.TempDir(), "identity-token")
			if err := os.WriteFile(other, []byte("identity-token-from-file"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(credentials.EnvIdentityTokenFile, other)
		}},
		{name: "identity token in the file", source: fromTokenFile, sameEntry: true, change: func(t *testing.T, s *federationStandIns) {
			if err := os.WriteFile(os.Getenv(credentials.EnvIdentityTokenFile), []byte("identity-token-of-another-workload"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "identity token in the environment", sameEntry: true, source: func(t *testing.T) {
			leaveGitHubActions(t)
			// The SDK lets a process exchange a literal token once, so each
			// run of the test needs its own.
			t.Setenv(credentials.EnvIdentityToken, fmt.Sprintf("identity-token-from-the-environment-%d", time.Now().UnixNano()))
		}, change: func(t *testing.T, s *federationStandIns) {
			t.Setenv(credentials.EnvIdentityToken, fmt.Sprintf("identity-token-of-another-workload-%d", time.Now().UnixNano()))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := federationFixture(t)
			if tc.source != nil {
				tc.source(t)
			}
			before := map[string]string{}
			for _, entry := range os.Environ() {
				name, value, _ := strings.Cut(entry, "=")
				before[name] = value
			}
			firstCache, _ := federationCacheFromEnv(os.Getenv(credentials.EnvFederationRuleID))

			mustCallAPI(t, newProcess(t))
			mustCallAPI(t, newProcess(t))
			if got := s.exchangeCount(); got != 1 {
				t.Fatalf("token exchanges before the change = %d, want 1", got)
			}

			tc.change(t, s)
			secondCache, _ := federationCacheFromEnv(os.Getenv(credentials.EnvFederationRuleID))
			if sameEntry := firstCache.name == secondCache.name; sameEntry != tc.sameEntry {
				t.Fatalf("same cache entry = %v, want %v", sameEntry, tc.sameEntry)
			}
			if secondCache.load() != nil {
				t.Fatal("the changed environment can load the first token")
			}
			mustCallAPI(t, newProcess(t))
			mustCallAPI(t, newProcess(t))
			if got := s.exchangeCount(); got != 2 {
				t.Fatalf("token exchanges after the change = %d, want 2: the first token must not be reused", got)
			}
			if got := s.seenByAPI(); got[len(got)-1] != "Bearer access-token-2" {
				t.Fatalf("API saw %q last, want the second token", got[len(got)-1])
			}
			if tc.sameEntry {
				return
			}

			// Both tokens stay cached, each for its own environment.
			for name, value := range before {
				if os.Getenv(name) != value {
					t.Setenv(name, value)
				}
			}
			mustCallAPI(t, newProcess(t))
			if got := s.exchangeCount(); got != 2 {
				t.Fatalf("token exchanges after changing back = %d, want 2: the first token is still cached", got)
			}
			if got := s.seenByAPI(); got[len(got)-1] != "Bearer access-token-1" {
				t.Fatalf("API saw %q last, want the first token", got[len(got)-1])
			}
		})
	}
}

func TestTokenFromAFileSourceIsCachedLikeALogin(t *testing.T) {
	s := federationFixture(t)
	useTokenFile(t, "identity-token-from-file")
	saveLogin(t)

	mustCallAPI(t, newProcess(t))
	mustCallAPI(t, newProcess(t))
	if got := s.exchangeCount(); got != 1 {
		t.Fatalf("token exchanges = %d, want 1", got)
	}
	cache, _ := federationCacheFromEnv("wifr_test")
	if cache.name == KeyringUser {
		t.Fatal("the federation token shares the saved login's keychain entry")
	}
	if _, err := keyring.Get(KeyringService, cache.name); err != nil {
		t.Fatalf("keychain entry error = %v, want the token stored there", err)
	}
	configDir, _ := getConfigDir()
	if files := cacheFiles(t, configDir); len(files) != 0 {
		t.Fatalf("cache files = %q, want none while the keychain works", files)
	}

	// Without a keychain the token goes to a file only the owner can read.
	_ = keyring.Delete(KeyringService, cache.name)
	setKeyring = func(string, string, string) error { return keyring.ErrSetDataTooBig }
	mustCallAPI(t, newProcess(t))
	mustCallAPI(t, newProcess(t))
	if got := s.exchangeCount(); got != 2 {
		t.Fatalf("token exchanges = %d, want 2", got)
	}
	files := cacheFiles(t, configDir)
	if len(files) != 1 {
		t.Fatalf("cache files = %q, want one", files)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(files[0]); err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("cache file mode = %v, %v, want 0600", info.Mode().Perm(), err)
		}
	}
	requireLoginUntouched(t)
}

func TestTokensAreNotCachedWithoutAWorkloadScope(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{"AWS", func(t *testing.T) { t.Setenv(credentials.EnvIdentityTokenSource, credentials.SourceAWSSTS) }, "aws_sts"},
		{"Google Cloud", func(t *testing.T) { t.Setenv(credentials.EnvIdentityTokenSource, credentials.SourceGCPMetadata) }, "gcp_metadata"},
		{"GitHub Actions without a job directory", func(t *testing.T) { t.Setenv("RUNNER_TEMP", "") }, "RUNNER_TEMP"},
		{"GitHub Actions token outside a job", func(t *testing.T) { t.Setenv("GITHUB_ACTIONS", "") }, "RUNNER_TEMP"},
		{"unreadable token file", func(t *testing.T) {
			t.Setenv(credentials.EnvIdentityTokenFile, filepath.Join(t.TempDir(), "missing"))
		}, "identity token file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			federationFixture(t)
			tc.setup(t)
			cache, reason := federationCacheFromEnv("wifr_test")
			if cache != nil || !strings.Contains(reason, tc.want) {
				t.Fatalf("federationCacheFromEnv() = %+v, %q, want no cache because of %s", cache, reason, tc.want)
			}
		})
	}

	// Without a cache every process exchanges, and nothing is written.
	s := federationFixture(t)
	runnerTemp := os.Getenv("RUNNER_TEMP")
	t.Setenv("GITHUB_ACTIONS", "")
	mustCallAPI(t, newProcess(t))
	mustCallAPI(t, newProcess(t))
	if got := s.exchangeCount(); got != 2 {
		t.Fatalf("token exchanges = %d, want 2", got)
	}
	requireNothingCached(t, runnerTemp)
}

func requireNothingCached(t *testing.T, runnerTemp string) {
	t.Helper()
	configDir, _ := getConfigDir()
	for _, dir := range []string{configDir, filepath.Join(runnerTemp, "kernel-cli")} {
		if files := cacheFiles(t, dir); len(files) != 0 {
			t.Fatalf("cache files = %q, want none", files)
		}
	}
}

func TestCacheCanBeTurnedOff(t *testing.T) {
	for _, value := range []string{"off", "OFF", "false", "0", "no"} {
		t.Run(value, func(t *testing.T) {
			s := federationFixture(t)
			t.Setenv(EnvFederationTokenCache, value)
			for range 2 {
				client := newProcess(t)
				mustCallAPI(t, client)
				mustCallAPI(t, client)
			}
			if got := s.exchangeCount(); got != 2 {
				t.Fatalf("token exchanges = %d, want one per process", got)
			}
			requireNothingCached(t, os.Getenv("RUNNER_TEMP"))
			if status := DescribeFederation(); status == nil || !strings.Contains(status.NotCached, EnvFederationTokenCache) {
				t.Fatalf("DescribeFederation() = %+v, want the cache reported off", status)
			}
		})
	}
}

func TestCacheTurnedOffIgnoresAnExistingEntry(t *testing.T) {
	s := federationFixture(t)
	mustCallAPI(t, newProcess(t))
	t.Setenv(EnvFederationTokenCache, "off")
	mustCallAPI(t, newProcess(t))
	if got := s.exchangeCount(); got != 2 {
		t.Fatalf("token exchanges = %d, want 2", got)
	}
	if got := s.seenByAPI(); got[1] != "Bearer access-token-2" {
		t.Fatalf("API saw %q, want a new token with the cache off", got)
	}
}

func TestRejectedCachedTokenIsDroppedAndTheRequestRetriedOnce(t *testing.T) {
	s := federationFixture(t)
	mustCallAPI(t, newProcess(t))

	// The API stops accepting the cached token, as when it is revoked.
	s.change(func() {
		s.apiStatus = rejectOnly("access-token-1")
		s.apiRequests = nil
	})

	client := newProcess(t)
	var out map[string]any
	if err := client.Post(context.Background(), "things", map[string]string{"name": "thing"}, &out); err != nil {
		t.Fatalf("API request error = %v, want the retry to succeed", err)
	}
	want := []string{`Bearer access-token-1 {"name":"thing"}`, `Bearer access-token-2 {"name":"thing"}`}
	if got := s.seenByAPI(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("API saw %q, want %q", got, want)
	}
	if got := s.exchangeCount(); got != 2 {
		t.Fatalf("token exchanges = %d, want 2", got)
	}

	// The replacement is what later requests and processes use.
	mustCallAPI(t, client)
	mustCallAPI(t, newProcess(t))
	if got := s.exchangeCount(); got != 2 {
		t.Fatalf("token exchanges = %d, want the replacement cached", got)
	}
	if got := s.seenByAPI(); got[len(got)-1] != "Bearer access-token-2" {
		t.Fatalf("API saw %q last, want the replacement", got[len(got)-1])
	}
}

func TestSecondRejectionFailsTheCommand(t *testing.T) {
	s := federationFixture(t)
	mustCallAPI(t, newProcess(t))
	s.change(func() {
		s.apiStatus = func(string) int { return http.StatusUnauthorized }
		s.apiRequests = nil
	})

	err := callAPI(newProcess(t))
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("API request error = %v, want the 401", err)
	}
	want := []string{"Bearer access-token-1", "Bearer access-token-2"}
	if got := s.seenByAPI(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("API saw %q, want %q: one retry, with a new token", got, want)
	}
	if got := s.exchangeCount(); got != 2 {
		t.Fatalf("token exchanges = %d, want 2", got)
	}
	cache, _ := federationCacheFromEnv("wifr_test")
	if cache.load() != nil {
		t.Fatal("the replacement the API rejected is still cached")
	}
}

// The request is sent a second time only once: the SDK's own retries, which
// are on by default, must not send it a third time.
func TestRetryAfterRejectionIsFinal(t *testing.T) {
	s := federationFixture(t)
	mustCallAPI(t, newProcess(t))
	s.change(func() {
		s.apiStatus = func(accessToken string) int {
			if accessToken == "access-token-1" {
				return http.StatusUnauthorized
			}
			return http.StatusServiceUnavailable
		}
		s.apiRequests = nil
	})

	client, err := GetAuthenticatedClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := callAPI(client); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("API request error = %v, want the 503", err)
	}
	want := []string{"Bearer access-token-1", "Bearer access-token-2"}
	if got := s.seenByAPI(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("API saw %q, want %q", got, want)
	}
}

// Vault writes take away what would let a request be sent twice: the
// replayable body, and for a request without a body the idempotency key. A
// rejected token is dropped and the 401 returned.
func TestRejectionOfARequestThatMustNotBeReplayedIsNotRetried(t *testing.T) {
	noReplay := option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		req.GetBody = nil
		if req.Body == nil || req.Body == http.NoBody {
			req.Header.Del("Idempotency-Key")
			req.Header.Del("X-Idempotency-Key")
		}
		return next(req)
	})
	for _, tc := range []struct {
		name string
		send func(client *kernel.Client) error
	}{
		{"write with a body", func(client *kernel.Client) error {
			return client.Post(context.Background(), "things", map[string]string{"name": "thing"}, nil, noReplay)
		}},
		{"write without a body", func(client *kernel.Client) error {
			return client.Delete(context.Background(), "things/thing", nil, nil, noReplay)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := federationFixture(t)
			mustCallAPI(t, newProcess(t))
			s.change(func() {
				s.apiStatus = rejectOnly("access-token-1")
				s.apiRequests = nil
			})

			if err := tc.send(newProcess(t)); err == nil || !strings.Contains(err.Error(), "401") {
				t.Fatalf("API request error = %v, want the 401", err)
			}
			if got := s.seenByAPI(); len(got) != 1 {
				t.Fatalf("API saw %q, want the request sent once", got)
			}
			mustCallAPI(t, newProcess(t))
			if got := s.seenByAPI(); got[len(got)-1] != "Bearer access-token-2" {
				t.Fatalf("API saw %q last, want the next command to use a new token", got[len(got)-1])
			}
		})
	}
}

// A write that keeps its idempotency key is sent again after a rejection,
// with or without a body.
func TestRejectedWriteWithoutABodyIsRetried(t *testing.T) {
	s := federationFixture(t)
	mustCallAPI(t, newProcess(t))
	s.change(func() {
		s.apiStatus = rejectOnly("access-token-1")
		s.apiRequests = nil
	})
	if err := newProcess(t).Delete(context.Background(), "things/thing", nil, nil); err != nil {
		t.Fatalf("API request error = %v, want the retry to succeed", err)
	}
	want := []string{"Bearer access-token-1", "Bearer access-token-2"}
	if got := s.seenByAPI(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("API saw %q, want %q", got, want)
	}
}

func TestTokenIsSentToTheAPIOnly(t *testing.T) {
	s := federationFixture(t)
	seen := make(chan string, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer other.Close()

	client := newProcess(t)
	var out map[string]any
	if err := client.Get(context.Background(), other.URL+"/elsewhere", nil, &out); err != nil {
		t.Fatal(err)
	}
	if got := <-seen; got != "" {
		t.Fatalf("another host saw Authorization %q, want none", got)
	}
	mustCallAPI(t, client)
	if got := s.seenByAPI(); len(got) != 1 || got[0] != "Bearer access-token-1" {
		t.Fatalf("API saw %q, want the access token", got)
	}
}

func TestRejectedTokenIsDroppedWhenTheExchangeFails(t *testing.T) {
	s := federationFixture(t)
	mustCallAPI(t, newProcess(t))
	s.change(func() {
		s.apiStatus = func(string) int { return http.StatusUnauthorized }
		s.refuseExchange = true
	})

	client := newProcess(t)
	if err := callAPI(client); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("API request error = %v, want the refused exchange", err)
	}
	cache, _ := federationCacheFromEnv("wifr_test")
	if entry := cache.load(); entry != nil {
		t.Fatal("the rejected token is still cached")
	}
	if _, err := GetAuthenticatedClient(); err == nil {
		t.Fatal("GetAuthenticatedClient() succeeded, want the refused exchange")
	}
}

func TestParallelProcessesNeverReadAPartialEntry(t *testing.T) {
	for _, store := range []string{"job directory", "config directory"} {
		t.Run(store, func(t *testing.T) {
			federationFixture(t)
			if store == "config directory" {
				useTokenFile(t, "identity-token-from-file")
				setKeyring = func(string, string, string) error { return keyring.ErrSetDataTooBig }
			}
			cache, _ := federationCacheFromEnv("wifr_test")
			path, err := cache.filePath()
			if err != nil {
				t.Fatal(err)
			}
			// Tokens large enough that a write in place would be seen half done.
			tokens := make([]string, 8)
			for i := range tokens {
				tokens[i] = strings.Repeat(fmt.Sprint(i), 256<<10)
			}
			expiresAt := time.Now().Add(5 * time.Minute)

			var writers, readers sync.WaitGroup
			done := make(chan struct{})
			for _, token := range tokens {
				writers.Add(1)
				go func() {
					defer writers.Done()
					for range 20 {
						if err := cache.save(&credentials.Token{AccessToken: token, ExpiresAt: expiresAt}); err != nil {
							t.Errorf("save() error = %v", err)
						}
					}
				}()
			}
			for range 4 {
				readers.Add(1)
				go func() {
					defer readers.Done()
					for {
						select {
						case <-done:
							return
						default:
						}
						data, err := os.ReadFile(path)
						if os.IsNotExist(err) {
							continue
						}
						var entry federationCacheEntry
						if err != nil || json.Unmarshal(data, &entry) != nil || len(entry.AccessToken) != 256<<10 ||
							strings.Trim(entry.AccessToken, entry.AccessToken[:1]) != "" {
							t.Errorf("read a partial cache entry (%d bytes, error %v)", len(data), err)
							return
						}
						if got := cache.load(); got == nil || len(got.AccessToken) != 256<<10 {
							t.Errorf("load() = %v, want a complete entry", got != nil)
							return
						}
					}
				}()
			}
			writers.Wait()
			close(done)
			readers.Wait()

			if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp")); len(leftovers) != 0 {
				t.Fatalf("temporary files left behind: %q", leftovers)
			}
		})
	}
}

func TestProcessesStartingTogetherAllAuthenticate(t *testing.T) {
	s := federationFixture(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := GetAuthenticatedClient(option.WithMaxRetries(0))
			if err != nil {
				t.Errorf("GetAuthenticatedClient() error = %v", err)
				return
			}
			if err := callAPI(client); err != nil {
				t.Errorf("API request error = %v", err)
			}
		}()
	}
	wg.Wait()
	if got := s.exchangeCount(); got < 1 || got > 8 {
		t.Fatalf("token exchanges = %d, want between 1 and 8", got)
	}
	for _, seen := range s.seenByAPI() {
		if !strings.HasPrefix(seen, "Bearer access-token-") {
			t.Fatalf("API saw %q, want a complete token", seen)
		}
	}
	// Whichever process wrote last, the next one finds a usable token.
	before := s.exchangeCount()
	mustCallAPI(t, newProcess(t))
	if got := s.exchangeCount(); got != before {
		t.Fatalf("token exchanges = %d, want %d: a token is cached", got, before)
	}
}

func TestDescribeFederation(t *testing.T) {
	s := federationFixture(t)
	status := DescribeFederation()
	if status == nil || status.RuleID != "wifr_test" || status.Err != nil || status.NotCached != "" || !status.CachedTokenExpiresAt.IsZero() {
		t.Fatalf("DescribeFederation() = %+v, want the rule with nothing cached yet", status)
	}
	if got := s.exchangeCount(); got != 0 {
		t.Fatalf("token exchanges = %d: describing must not exchange", got)
	}

	mustCallAPI(t, newProcess(t))
	status = DescribeFederation()
	if left := time.Until(status.CachedTokenExpiresAt); left < 4*time.Minute || left > 5*time.Minute {
		t.Fatalf("cached token expires in %s, want the five minutes the exchange returned", left)
	}

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	if status = DescribeFederation(); status.Err == nil {
		t.Fatalf("DescribeFederation() = %+v, want the missing identity token source reported", status)
	}
	t.Setenv(credentials.EnvFederationRuleID, "")
	if status = DescribeFederation(); status != nil {
		t.Fatalf("DescribeFederation() = %+v, want nil without a rule", status)
	}
}

func TestTokensAreNeverPrinted(t *testing.T) {
	s := federationFixture(t)
	var output bytes.Buffer
	pterm.EnableDebugMessages()
	debugWriter := pterm.Debug.Writer
	pterm.Debug.Writer = &output
	t.Cleanup(func() {
		pterm.DisableDebugMessages()
		pterm.Debug.Writer = debugWriter
	})

	mustCallAPI(t, newProcess(t))
	mustCallAPI(t, newProcess(t))
	s.change(func() { s.apiStatus = rejectOnly("access-token-1") })
	mustCallAPI(t, newProcess(t))
	s.change(func() {
		s.apiStatus = func(string) int { return http.StatusUnauthorized }
		s.refuseExchange = true
	})
	apiErr := callAPI(newProcess(t))
	_, authErr := GetAuthenticatedClient()
	if apiErr == nil || authErr == nil {
		t.Fatalf("errors = %v, %v, want both to fail", apiErr, authErr)
	}
	fmt.Fprintln(&output, apiErr, authErr)

	if !strings.Contains(output.String(), "Using the cached workload identity token") {
		t.Fatalf("debug output = %q, want it to say the cached token was used", output.String())
	}
	for _, secret := range []string{"access-token-", "identity-token-", "request-credential-"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("output contains %q:\n%s", secret, output.String())
		}
	}
}
