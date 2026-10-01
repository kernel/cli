package passwordmanager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectedDomainMatchesOnlyApprovedSites(t *testing.T) {
	assert.Equal(t, "github.com", selectedDomain("https://api.github.com/login", []string{"github.com"}))
	assert.Empty(t, selectedDomain("https://notgithub.com", []string{"github.com"}))
}

func TestNormalizeTOTPAcceptsSupportedSecret(t *testing.T) {
	assert.Equal(t, "JBSWY3DPEHPK3PXP", normalizeTOTP("otpauth://totp/Test?secret=JBSWY3DPEHPK3PXP&algorithm=SHA1"))
	assert.Empty(t, normalizeTOTP("otpauth://totp/Test?secret=JBSWY3DPEHPK3PXP&algorithm=SHA256"))
}

func TestDecodeOnePasswordItemsAcceptsArrayAndStream(t *testing.T) {
	array, err := decodeOnePasswordItems([]byte(`[{"id":"one"},{"id":"two"}]`))
	require.NoError(t, err)
	require.Len(t, array, 2)
	stream, err := decodeOnePasswordItems([]byte("{\"id\":\"one\"}\n{\"id\":\"two\"}\n"))
	require.NoError(t, err)
	require.Len(t, stream, 2)
}

func TestDecodeOnePasswordVaultsAcceptsBulkStream(t *testing.T) {
	vaults, err := decodeOnePasswordVaults([]byte("{\"id\":\"personal\",\"type\":\"PERSONAL\"}\n{\"id\":\"shared\",\"type\":\"BUSINESS\"}\n"))
	require.NoError(t, err)
	require.Len(t, vaults, 2)
	assert.Equal(t, "PERSONAL", vaults[0].Type)
}

func TestDeduplicateRejectsSharedOrEmptyRecords(t *testing.T) {
	records := deduplicate([]Record{
		{Provider: "bitwarden", ID: "one", Domain: "example.com", Username: "me"},
		{Provider: "bitwarden", ID: "one", Domain: "example.com", Username: "me"},
		{Provider: "bitwarden", ID: "two", Domain: "", Username: "me"},
	})
	require.Len(t, records, 1)
	assert.Equal(t, "one", records[0].ID)
}

func TestDeduplicatePreservesOneItemForMultipleDomains(t *testing.T) {
	records := deduplicate([]Record{
		{Provider: "bitwarden", ID: "one", Domain: "accounts.example.com", Username: "me"},
		{Provider: "bitwarden", ID: "one", Domain: "app.example.com", Username: "me"},
	})
	require.Len(t, records, 2)
}

func TestBitwardenCandidateWireTypeCannotRetainSecrets(t *testing.T) {
	login, ok := reflect.TypeOf(bitwardenCandidateItem{}).FieldByName("Login")
	require.True(t, ok)
	loginType := login.Type.Elem()
	_, hasPassword := loginType.FieldByName("Password")
	_, hasTOTP := loginType.FieldByName("TOTP")
	assert.False(t, hasPassword)
	assert.False(t, hasTOTP)
}

func TestBitwardenUnlockKeepsSessionInProviderMemory(t *testing.T) {
	t.Setenv("BW_SESSION", "")
	path := filepath.Join(t.TempDir(), "bw")
	script := `#!/bin/sh
case "$1" in
  status)
    if [ "$BW_SESSION" = "temporary-session" ]; then
      printf '{"status":"unlocked"}'
    else
      printf '{"status":"locked"}'
    fi
    ;;
  unlock)
    printf 'temporary-session'
    ;;
  *) exit 1 ;;
esac
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	provider := &bitwardenProvider{path: path}

	required, err := provider.AuthorizationRequired(t.Context())
	require.NoError(t, err)
	require.True(t, required)
	require.NoError(t, provider.Authorize(t.Context()))
	required, err = provider.AuthorizationRequired(t.Context())
	require.NoError(t, err)
	assert.False(t, required)
	assert.Empty(t, os.Getenv("BW_SESSION"))
}

func TestOnePasswordLongSummaryCanMatchWithoutItemReveal(t *testing.T) {
	var summary onePasswordSummary
	summary.ID = "item"
	summary.Title = "Example"
	summary.Vault.ID = "personal"
	summary.URLs = append(summary.URLs, struct {
		Href string `json:"href"`
	}{Href: "https://login.example.com"})

	candidates := onePasswordSummaryCandidates(summary, []string{"example.com"})
	require.Len(t, candidates, 1)
	assert.Equal(t, "item", candidates[0].ID)
	assert.Equal(t, "example.com", candidates[0].Domain)
}
