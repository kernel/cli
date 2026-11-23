package auth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func storageFixture(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	keyring.MockInit()
	t.Cleanup(func() { setKeyring = keyring.Set })
}

func credentialsFile(t *testing.T) string {
	t.Helper()
	dir, err := getConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "credentials")
}

func TestSaveTokensFallbackReplacesStaleKeyring(t *testing.T) {
	storageFixture(t)
	if err := SaveTokens(&TokenStorage{AccessToken: "old"}); err != nil {
		t.Fatalf("SaveTokens(old) error = %v", err)
	}

	// The keychain refuses the next save, as Windows does above 2,560 bytes.
	setKeyring = func(string, string, string) error { return keyring.ErrSetDataTooBig }
	if err := SaveTokens(&TokenStorage{AccessToken: "new", Projects: []string{"proj_a"}}); err != nil {
		t.Fatalf("SaveTokens(new) error = %v", err)
	}
	if _, err := keyring.Get(KeyringService, KeyringUser); err != keyring.ErrNotFound {
		t.Fatalf("stale keychain entry remains: err = %v", err)
	}
	loaded, err := LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens() error = %v", err)
	}
	if loaded.AccessToken != "new" {
		t.Fatalf("LoadTokens() = %q, want the credentials saved to disk", loaded.AccessToken)
	}

	// Once the keychain accepts credentials again, the file is removed.
	setKeyring = keyring.Set
	if err := SaveTokens(&TokenStorage{AccessToken: "newest"}); err != nil {
		t.Fatalf("SaveTokens(newest) error = %v", err)
	}
	if _, err := os.Stat(credentialsFile(t)); !os.IsNotExist(err) {
		t.Fatalf("credentials file remains after a keychain save: err = %v", err)
	}
	if loaded, err = LoadTokens(); err != nil || loaded.AccessToken != "newest" {
		t.Fatalf("LoadTokens() = %+v, %v", loaded, err)
	}
}

func TestLoadTokensPrefersNewerCopy(t *testing.T) {
	now := time.Now().UTC()
	write := func(t *testing.T, keyringTokens, fileTokens TokenStorage) {
		t.Helper()
		data, _ := json.Marshal(keyringTokens)
		if err := keyring.Set(KeyringService, KeyringUser, string(data)); err != nil {
			t.Fatal(err)
		}
		data, _ = json.Marshal(fileTokens)
		if err := saveTokensToFile(data); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name                string
		keyringTokens, file TokenStorage
		want                string
	}{
		{"file saved later", TokenStorage{AccessToken: "keyring", SavedAt: now.Add(-time.Hour)}, TokenStorage{AccessToken: "file", SavedAt: now}, "file"},
		{"keychain saved later", TokenStorage{AccessToken: "keyring", SavedAt: now}, TokenStorage{AccessToken: "file", SavedAt: now.Add(-time.Hour)}, "keyring"},
		// An older CLI left a long-lived token for one organization on disk and
		// saved a shorter-lived one for another in the keychain, which it used.
		{"neither stamped keeps the keychain", TokenStorage{AccessToken: "keyring", OrgID: "org_b", ExpiresAt: now}, TokenStorage{AccessToken: "file", OrgID: "org_a", ExpiresAt: now.Add(24 * time.Hour)}, "keyring"},
		{"only the file stamped", TokenStorage{AccessToken: "keyring", ExpiresAt: now.Add(time.Hour)}, TokenStorage{AccessToken: "file", SavedAt: now}, "file"},
		{"only the keychain stamped", TokenStorage{AccessToken: "keyring", SavedAt: now}, TokenStorage{AccessToken: "file", ExpiresAt: now.Add(time.Hour)}, "keyring"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storageFixture(t)
			write(t, tc.keyringTokens, tc.file)
			loaded, err := LoadTokens()
			if err != nil {
				t.Fatalf("LoadTokens() error = %v", err)
			}
			if loaded.AccessToken != tc.want {
				t.Fatalf("LoadTokens() = %q, want %q", loaded.AccessToken, tc.want)
			}
		})
	}
}

func TestConcurrentSavesKeepOneCopy(t *testing.T) {
	storageFixture(t)
	// The keychain refuses large credentials and accepts small ones, so the
	// two saves below take different paths and each cleans up the other store.
	setKeyring = func(service, user, data string) error {
		if strings.Contains(data, "large") {
			return keyring.ErrSetDataTooBig
		}
		return keyring.Set(service, user, data)
	}
	for round := range 25 {
		var wg sync.WaitGroup
		for _, token := range []string{"large", "small"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := SaveTokens(&TokenStorage{AccessToken: token}); err != nil {
					t.Errorf("SaveTokens(%s) error = %v", token, err)
				}
			}()
		}
		wg.Wait()
		loaded, err := LoadTokens()
		if err != nil {
			t.Fatalf("round %d: LoadTokens() error = %v", round, err)
		}
		if loaded.AccessToken != "large" && loaded.AccessToken != "small" {
			t.Fatalf("round %d: LoadTokens() = %q", round, loaded.AccessToken)
		}
	}
}

func TestConcurrentCommandsRefreshOnce(t *testing.T) {
	storageFixture(t)
	t.Setenv("KERNEL_API_KEY", "")
	var mu sync.Mutex
	valid, generation, requests := "refresh-0", 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("refresh_token") != valid {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		generation++
		valid = fmt.Sprintf("refresh-%d", generation)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("access-%d", generation), "refresh_token": valid,
			"token_type": "Bearer", "expires_in": 600, "access_scope": "organization",
		})
	}))
	defer server.Close()
	if err := SaveTokens(&TokenStorage{
		AccessToken: "access-0", RefreshToken: "refresh-0", ExpiresAt: time.Now().Add(-time.Minute),
		AuthBaseURL: server.URL, OAuthClientID: "client-id",
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := GetAuthenticatedClient(); err != nil {
				t.Errorf("GetAuthenticatedClient() error = %v", err)
			}
		}()
	}
	wg.Wait()
	if requests != 1 {
		t.Fatalf("token requests = %d, want 1: commands that start together refresh once", requests)
	}
	loaded, err := LoadTokens()
	if err != nil || loaded.RefreshToken != "refresh-1" {
		t.Fatalf("LoadTokens() = %+v, %v", loaded, err)
	}
}

// TestCredentialsLockExcludesOtherProcesses holds the lock in a child process
// and checks that this process waits for it.
func TestCredentialsLockExcludesOtherProcesses(t *testing.T) {
	if os.Getenv("KERNEL_TEST_HOLD_CREDENTIALS_LOCK") == "1" {
		_ = withCredentialsLock(func() error {
			fmt.Println("locked")
			time.Sleep(500 * time.Millisecond)
			return nil
		})
		return
	}
	storageFixture(t)
	child := exec.Command(os.Args[0], "-test.run=^TestCredentialsLockExcludesOtherProcesses$")
	child.Env = append(os.Environ(), "KERNEL_TEST_HOLD_CREDENTIALS_LOCK=1")
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("child did not take the lock: %q, %v", line, err)
	}
	start := time.Now()
	if err := withCredentialsLock(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 300*time.Millisecond {
		t.Fatalf("took the lock after %s while another process held it", waited)
	}
}
