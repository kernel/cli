package cmd

import (
	"errors"
	"fmt"
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

func TestSelectedProfileCategoriesUsePortableApplyOrder(t *testing.T) {
	categories := selectedProfileCategories(map[string]int{
		"bookmarks": 2,
		"cookies":   3,
		"history":   4,
		"storage":   5,
	})

	require.Equal(t, []string{"cookies", "storage", "bookmarks", "history"}, categories)
}

func TestProfilesImportLocalDefaultsHistoryOn(t *testing.T) {
	flag := profilesImportLocalCmd.Flags().Lookup("history")
	require.NotNil(t, flag)
	require.Equal(t, "true", flag.DefValue)
}

func TestFitBrowserImportBundleRemovesLargestStorageOriginsFirst(t *testing.T) {
	data := localbrowser.ProfileData{
		Storage: []localbrowser.StorageRecord{
			{Origin: "https://large.example", Key: "one", Value: "a much larger local storage value"},
			{Origin: "https://small.example", Key: "two", Value: "x"},
		},
		History: []localbrowser.HistoryRecord{{URL: "https://example.com"}},
	}
	counts := map[string]int{"cookies": 2, "storage": 2, "history": 1}
	builder := sizedBundleBuilder(50, 5, map[string]int64{
		"https://large.example": 10,
		"https://small.example": 2,
	})

	result, err := fitBrowserImportBundle(data, counts, builder)
	require.NoError(t, err)
	require.Equal(t, []string{"https://large.example"}, result.skippedStorageOrigins)
	require.Equal(t, 1, result.skippedStorageRecords)
	require.Zero(t, result.skippedHistoryRecords)
	require.Len(t, result.data.Storage, 1)
	require.Equal(t, "https://small.example", result.data.Storage[0].Origin)
	require.Equal(t, 1, result.itemCounts["storage"])
	require.Equal(t, 1, result.itemCounts["history"])
}

func TestFitBrowserImportBundleDropsHistoryThenRestoresStorageThatFits(t *testing.T) {
	data := localbrowser.ProfileData{
		Storage: []localbrowser.StorageRecord{
			{Origin: "https://large.example", Key: "one", Value: "a much larger local storage value"},
			{Origin: "https://small.example", Key: "two", Value: "x"},
		},
		History: []localbrowser.HistoryRecord{{URL: "https://example.com"}, {URL: "https://other.example"}},
	}
	counts := map[string]int{"cookies": 2, "storage": 2, "history": 2}
	builder := sizedBundleBuilder(60, 10, map[string]int64{
		"https://large.example": 3,
		"https://small.example": 2,
	})

	result, err := fitBrowserImportBundle(data, counts, builder)
	require.NoError(t, err)
	require.Equal(t, []string{"https://large.example"}, result.skippedStorageOrigins)
	require.Equal(t, 1, result.skippedStorageRecords)
	require.Equal(t, 2, result.skippedHistoryRecords)
	require.Len(t, result.data.Storage, 1)
	require.Empty(t, result.data.History)
	require.NotContains(t, result.itemCounts, "history")
}

func TestFitBrowserImportBundleLeavesBundleAloneWhenItFits(t *testing.T) {
	data := localbrowser.ProfileData{Storage: []localbrowser.StorageRecord{{Origin: "https://example.com", Key: "one", Value: "value"}}}
	counts := map[string]int{"cookies": 2, "storage": 1}

	result, err := fitBrowserImportBundle(data, counts, sizedBundleBuilder(60, 0, map[string]int64{"https://example.com": 3}))
	require.NoError(t, err)
	require.Zero(t, result.originalSize)
	require.Equal(t, data.Storage, result.data.Storage)
	require.Equal(t, counts, result.itemCounts)
}

func TestFitBrowserImportBundleRejectsOversizedRequiredData(t *testing.T) {
	_, err := fitBrowserImportBundle(localbrowser.ProfileData{}, map[string]int{"cookies": 2}, sizedBundleBuilder(65, 0, nil))
	require.ErrorContains(t, err, "cookies and bookmarks do not fit")
	require.True(t, errors.Is(err, localbrowser.ErrBundleTooLarge))
}

func sizedBundleBuilder(base, history int64, storage map[string]int64) profileBundleBuilder {
	return func(data localbrowser.ProfileData) ([]byte, error) {
		size := base
		if len(data.History) > 0 {
			size += history
		}
		seen := make(map[string]struct{})
		for _, record := range data.Storage {
			if _, ok := seen[record.Origin]; ok {
				continue
			}
			seen[record.Origin] = struct{}{}
			size += storage[record.Origin]
		}
		if size > 64 {
			return nil, &localbrowser.BundleTooLargeError{Size: size, Limit: 64}
		}
		return []byte(fmt.Sprintf("%d", size)), nil
	}
}
