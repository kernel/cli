package cmd

import (
	"strings"
	"testing"

	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/stretchr/testify/require"
)

func TestLocalStorageSelectionUsesAllSitesWithinLimit(t *testing.T) {
	sites := []localbrowser.StorageSite{
		{Origin: "https://example.com", Bytes: 1024},
		{Origin: "https://other.example", Bytes: 2048},
	}

	selected, err := (ProfilesImportLocalCmd{}).chooseLocalStorageSites(sites, true)
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com", "https://other.example"}, selected)
}

func TestLocalStorageSelectionRequiresReviewWhenOverLimit(t *testing.T) {
	sites := []localbrowser.StorageSite{{Origin: "https://large.example", Bytes: localbrowser.MaxPortableStorageSize + 1}}

	_, err := (ProfilesImportLocalCmd{}).chooseLocalStorageSites(sites, true)
	require.ErrorContains(t, err, "run interactively to choose websites")
}

func TestNonInteractiveExtensionSelectionHonorsCapacity(t *testing.T) {
	extensions := []localbrowser.Extension{
		{ID: strings.Repeat("a", 32), Source: "chrome_web_store"},
		{ID: strings.Repeat("b", 32), Source: "chrome_web_store"},
	}

	_, err := (ProfilesImportLocalCmd{}).chooseProfileExtensions(extensions, storedExtensionCapacity{remaining: 1}, true, true)
	require.ErrorContains(t, err, "only 1 can be added")
}

func TestSelectedProfileCategoriesUsePortableApplyOrder(t *testing.T) {
	categories := selectedProfileCategories(map[string]int{
		"extensions": 1,
		"bookmarks":  2,
		"cookies":    3,
		"history":    4,
		"storage":    5,
	})

	require.Equal(t, []string{"cookies", "storage", "bookmarks", "history", "extensions"}, categories)
}

func TestProfilesImportLocalDefaultsHistoryOn(t *testing.T) {
	flag := profilesImportLocalCmd.Flags().Lookup("history")
	require.NotNil(t, flag)
	require.Equal(t, "true", flag.DefValue)
}
