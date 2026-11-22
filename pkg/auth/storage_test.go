package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func storageFixture(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
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
		{"older CLI copies compare by expiry", TokenStorage{AccessToken: "keyring", ExpiresAt: now}, TokenStorage{AccessToken: "file", ExpiresAt: now.Add(time.Hour)}, "file"},
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
