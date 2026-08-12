package browserimport

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverMacOSProfilesFindsChromeAndHelium(t *testing.T) {
	home := t.TempDir()
	writeBrowserFixture(t, home, "Google/Chrome", "Default", "Personal")
	writeBrowserFixture(t, home, "net.imput.helium", "Profile 1", "Work")

	profiles, err := DiscoverMacOSProfiles(home)
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, "Google Chrome / Personal", profiles[0].DisplayName())
	assert.Equal(t, "Helium / Work", profiles[1].DisplayName())
	assert.NotEqual(t, profiles[0].ID, profiles[1].ID)
}

func TestRegistrableDomainGroupsSubdomains(t *testing.T) {
	assert.Equal(t, "example.co.uk", registrableDomain("login.example.co.uk"))
	assert.Equal(t, "localhost", registrableDomain("localhost"))
}

func TestDomainMatchesSelectedSite(t *testing.T) {
	assert.True(t, domainMatchesSite(".accounts.google.com", "google.com"))
	assert.True(t, domainMatchesSite("google.com", "accounts.google.com"))
	assert.False(t, domainMatchesSite("notgoogle.com", "google.com"))
}

func TestDecryptChromiumValueSupportsHostDigest(t *testing.T) {
	key := []byte("0123456789abcdef")
	domain := ".example.com"
	digest := sha256.Sum256([]byte(domain))
	plaintext := append(digest[:], []byte("secret")...)
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	plaintext = append(plaintext, bytesRepeat(byte(padding), padding)...)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, bytesRepeat(' ', aes.BlockSize)).CryptBlocks(ciphertext, plaintext)

	decrypted, err := decryptChromiumValue(key, domain, append([]byte("v10"), ciphertext...))
	require.NoError(t, err)
	assert.Equal(t, "secret", string(decrypted))
}

func writeBrowserFixture(t *testing.T, home, rootSuffix, directory, name string) {
	t.Helper()
	root := filepath.Join(home, "Library/Application Support", rootSuffix)
	require.NoError(t, os.MkdirAll(filepath.Join(root, directory), 0o755))
	state := map[string]any{"profile": map[string]any{"info_cache": map[string]any{directory: map[string]string{"name": name}}}}
	data, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "Local State"), data, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, directory, "History"), nil, 0o600))
}

func bytesRepeat(value byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = value
	}
	return result
}
