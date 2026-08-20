package passwordmanager

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

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

func TestBitwardenCandidateDiscoveryDoesNotSync(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "commands")
	path := filepath.Join(t.TempDir(), "bw")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$BW_TEST_LOG"
case "$1" in
  status) printf '{"status":"unlocked"}' ;;
  list) printf '[]' ;;
  sync) exit 99 ;;
  *) exit 1 ;;
esac
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	t.Setenv("BW_TEST_LOG", logPath)
	provider := &bitwardenProvider{path: path}

	candidates, err := provider.Candidates(context.Background(), []string{"github.com"})
	require.NoError(t, err)
	assert.Empty(t, candidates)
	commands, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.NotContains(t, string(commands), "sync")
	assert.Contains(t, string(commands), "list items --url github.com")
}

func TestBitwardenCandidateQueriesAreBoundedOrderedAndCanceled(t *testing.T) {
	sites := []string{"one.com", "two.com", "three.com", "four.com", "five.com", "six.com"}
	var active, peak atomic.Int32
	results, err := fetchBitwardenCandidateSites(t.Context(), sites, func(ctx context.Context, site string) ([]bitwardenCandidateItem, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(time.Duration(len(sites)-stringIndex(sites, site)) * time.Millisecond)
		return []bitwardenCandidateItem{{ID: site}}, nil
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, peak.Load(), int32(4))
	for index, site := range sites {
		assert.Equal(t, site, results[index][0].ID)
	}

	canceled := make(chan struct{}, 1)
	_, err = fetchBitwardenCandidateSites(t.Context(), []string{"fail", "slow"}, func(ctx context.Context, site string) ([]bitwardenCandidateItem, error) {
		if site == "fail" {
			time.Sleep(10 * time.Millisecond)
			return nil, assert.AnError
		}
		<-ctx.Done()
		canceled <- struct{}{}
		return nil, ctx.Err()
	})
	require.Error(t, err)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("sibling query was not canceled")
	}
}

func TestBitwardenApprovedItemReadsAreBoundedOrderedDeduplicatedAndCanceled(t *testing.T) {
	selected := []Candidate{
		{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}, {ID: "three", Name: "Three"},
		{ID: "four", Name: "Four"}, {ID: "five", Name: "Five"}, {ID: "one", Name: "One again"},
	}
	var active, peak, oneReads atomic.Int32
	items, err := fetchBitwardenApprovedItems(t.Context(), selected, func(_ context.Context, candidate Candidate) (bitwardenItem, error) {
		current := active.Add(1)
		defer active.Add(-1)
		if candidate.ID == "one" {
			oneReads.Add(1)
		}
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(time.Duration(len(selected)-stringIndex([]string{"one", "two", "three", "four", "five"}, candidate.ID)) * time.Millisecond)
		return bitwardenItem{ID: candidate.ID}, nil
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, peak.Load(), int32(4))
	assert.Equal(t, int32(1), oneReads.Load())
	for index, candidate := range selected {
		assert.Equal(t, candidate.ID, items[index].ID)
	}

	canceled := make(chan struct{}, 1)
	_, err = fetchBitwardenApprovedItems(t.Context(), []Candidate{{ID: "fail"}, {ID: "slow"}}, func(ctx context.Context, candidate Candidate) (bitwardenItem, error) {
		if candidate.ID == "fail" {
			time.Sleep(10 * time.Millisecond)
			return bitwardenItem{}, assert.AnError
		}
		<-ctx.Done()
		canceled <- struct{}{}
		return bitwardenItem{}, ctx.Err()
	})
	require.Error(t, err)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("sibling approved-item read was not canceled")
	}
}

func stringIndex(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
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
