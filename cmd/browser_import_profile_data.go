package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/pterm/pterm"
)

const (
	bookmarksChoice = "Bookmarks"
	historyChoice   = "History"
	storageChoice   = "Local storage"
)

type localProfileDataSelection struct {
	data          localbrowser.ProfileData
	bookmarkCount int
	historyCount  int
	history       bool
	storage       bool
	storageSites  []string
	storageBytes  int64
}

type profileBundleBuilder func(localbrowser.ProfileData) ([]byte, error)

type profileBundleFit struct {
	bundle                []byte
	data                  localbrowser.ProfileData
	itemCounts            map[string]int
	originalSize          int64
	limit                 int64
	skippedStorageOrigins []string
	skippedStorageRecords int
	skippedStorageBytes   int64
	skippedHistoryRecords int
}

type storageOriginGroup struct {
	origin  string
	bytes   int64
	records int
}

func fitBrowserImportBundle(data localbrowser.ProfileData, itemCounts map[string]int, build profileBundleBuilder) (profileBundleFit, error) {
	bundle, err := build(data)
	if err == nil {
		return profileBundleFit{bundle: bundle, data: data, itemCounts: cloneItemCounts(itemCounts)}, nil
	}
	var originalTooLarge *localbrowser.BundleTooLargeError
	if !errors.As(err, &originalTooLarge) {
		return profileBundleFit{}, err
	}

	groups := groupedStorageOrigins(data.Storage)
	fit, found, err := fitBrowserImportStorage(data, itemCounts, groups, false, originalTooLarge, build)
	if err != nil {
		return profileBundleFit{}, err
	}
	if !found && len(data.History) > 0 {
		fit, found, err = fitBrowserImportStorage(data, itemCounts, groups, true, nil, build)
		if err != nil {
			return profileBundleFit{}, err
		}
	}
	if !found {
		return profileBundleFit{}, fmt.Errorf("cookies and bookmarks do not fit in the browser import bundle: %w", originalTooLarge)
	}
	fit.originalSize = originalTooLarge.Size
	fit.limit = originalTooLarge.Limit
	return fit, nil
}

func fitBrowserImportStorage(data localbrowser.ProfileData, itemCounts map[string]int, groups []storageOriginGroup, dropHistory bool, pendingTooLarge *localbrowser.BundleTooLargeError, build profileBundleBuilder) (profileBundleFit, bool, error) {
	fit := profileBundleFit{data: data, itemCounts: cloneItemCounts(itemCounts)}
	if dropHistory {
		fit.skippedHistoryRecords = len(data.History)
		fit.data.History = nil
		delete(fit.itemCounts, "history")
	}
	removed := make(map[string]struct{}, len(groups))
	nextGroup := 0
	for {
		if pendingTooLarge == nil {
			bundle, err := build(fit.data)
			if err == nil {
				fit.bundle = bundle
				return fit, true, nil
			}
			if !errors.As(err, &pendingTooLarge) {
				return profileBundleFit{}, false, err
			}
		}
		if nextGroup == len(groups) {
			return profileBundleFit{}, false, nil
		}

		bytesToRemove := pendingTooLarge.Size - pendingTooLarge.Limit
		var removedBytes int64
		for nextGroup < len(groups) && removedBytes < bytesToRemove {
			group := groups[nextGroup]
			nextGroup++
			removed[group.origin] = struct{}{}
			removedBytes += group.bytes
			fit.skippedStorageOrigins = append(fit.skippedStorageOrigins, group.origin)
			fit.skippedStorageRecords += group.records
			fit.skippedStorageBytes += group.bytes
		}
		fit.data.Storage = filterStorageOrigins(data.Storage, removed)
		if len(fit.data.Storage) == 0 {
			delete(fit.itemCounts, "storage")
		} else {
			fit.itemCounts["storage"] = len(fit.data.Storage)
		}
		pendingTooLarge = nil
	}
}

func groupedStorageOrigins(records []localbrowser.StorageRecord) []storageOriginGroup {
	byOrigin := make(map[string]*storageOriginGroup)
	for _, record := range records {
		group := byOrigin[record.Origin]
		if group == nil {
			group = &storageOriginGroup{origin: record.Origin}
			byOrigin[record.Origin] = group
		}
		encoded, _ := json.Marshal(record)
		group.bytes += int64(len(encoded) + 1)
		group.records++
	}
	groups := make([]storageOriginGroup, 0, len(byOrigin))
	for _, group := range byOrigin {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(left, right int) bool {
		if groups[left].bytes == groups[right].bytes {
			return groups[left].origin < groups[right].origin
		}
		return groups[left].bytes > groups[right].bytes
	})
	return groups
}

func filterStorageOrigins(records []localbrowser.StorageRecord, removed map[string]struct{}) []localbrowser.StorageRecord {
	filtered := make([]localbrowser.StorageRecord, 0, len(records))
	for _, record := range records {
		if _, skip := removed[record.Origin]; !skip {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func cloneItemCounts(counts map[string]int) map[string]int {
	cloned := make(map[string]int, len(counts))
	for category, count := range counts {
		cloned[category] = count
	}
	return cloned
}

func (c ProfilesImportLocalCmd) chooseLocalProfileData(ctx context.Context, profile localbrowser.Profile, since time.Time, includeHistory, nonInteractive, humanOutput bool) (localProfileDataSelection, error) {
	bookmarks, bookmarkCount, bookmarkErr := localbrowser.ExportBookmarks(profile)
	historyCount, historyErr := localbrowser.HistoryCount(ctx, profile, since)
	storageSites, storageErr := localbrowser.LocalStorageSites(ctx, profile)

	if humanOutput {
		warnUnavailableBrowserData("bookmarks", bookmarkErr)
		warnUnavailableBrowserData("history", historyErr)
		warnUnavailableBrowserData("local storage", storageErr)
	}

	selection := localProfileDataSelection{history: includeHistory}
	options := make([]string, 0, 3)
	defaults := make([]string, 0, 3)
	labels := make(map[string]string, 3)
	if bookmarkErr == nil && bookmarkCount > 0 {
		labels[bookmarksChoice] = fmt.Sprintf("%s — %d", bookmarksChoice, bookmarkCount)
		options = append(options, labels[bookmarksChoice])
		defaults = append(defaults, labels[bookmarksChoice])
	}
	if historyErr == nil && historyCount > 0 {
		labels[historyChoice] = fmt.Sprintf("%s — %d visits", historyChoice, historyCount)
		options = append(options, labels[historyChoice])
		if includeHistory {
			defaults = append(defaults, labels[historyChoice])
		}
	}
	if storageErr == nil && len(storageSites) > 0 {
		total := storageSiteBytes(storageSites)
		labels[storageChoice] = fmt.Sprintf("%s — %s across %d origins", storageChoice, formatBinaryBytes(total), len(storageSites))
		options = append(options, labels[storageChoice])
		defaults = append(defaults, labels[storageChoice])
	}
	chosen := defaults
	var err error
	if !nonInteractive && len(options) > 0 {
		chosen, err = c.prompter.MultiSelect("browser data", "use Space to exclude a category", "Choose browser data to import", options, defaults)
		if err != nil {
			return localProfileDataSelection{}, err
		}
	}
	for _, choice := range chosen {
		switch choice {
		case labels[bookmarksChoice]:
			selection.data.Bookmarks = &bookmarks
			selection.bookmarkCount = bookmarkCount
		case labels[historyChoice]:
			selection.history = true
			selection.historyCount = historyCount
		case labels[storageChoice]:
			selection.storage = true
			selection.storageBytes = storageSiteBytes(storageSites)
		}
	}

	if selection.storage {
		selection.storageSites, err = c.chooseLocalStorageSites(storageSites, nonInteractive)
		if err != nil {
			return localProfileDataSelection{}, err
		}
		selection.storageBytes = selectedStorageSiteBytes(storageSites, selection.storageSites)
	}
	return selection, nil
}

func warnUnavailableBrowserData(category string, err error) {
	if err != nil {
		pterm.Warning.Printf("%s could not be read and will be skipped: %v\n", category, err)
	}
}

func (c ProfilesImportLocalCmd) confirmBrowserImport(targetName string, cookies cookieImportSelection, cookieSites []localbrowser.Site, profileData localProfileDataSelection, logins pendingManagedAuth) (bool, error) {
	pterm.Println()
	pterm.Printf("Ready to import into profile %q\n\n", targetName)
	if cookies.all {
		pterm.Printf("  All cookies — %d across %d websites\n", selectedCookieCount(cookieSites, cookies.sites), len(cookies.sites))
	} else {
		pterm.Printf("  Cookies from %d selected websites\n", len(cookies.sites))
	}
	if profileData.bookmarkCount > 0 {
		pterm.Printf("  Bookmarks — %d\n", profileData.bookmarkCount)
	}
	if profileData.history {
		pterm.Printf("  History — %d visits\n", profileData.historyCount)
	}
	if profileData.storage {
		pterm.Printf("  Local storage — %s across %d origins\n", formatBinaryBytes(profileData.storageBytes), len(profileData.storageSites))
	}
	loginCount := 0
	for _, provider := range logins.providers {
		loginCount += len(provider.candidates)
	}
	if loginCount > 0 {
		pterm.Printf("  Managed Auth connections — %d\n", loginCount)
	}
	pterm.Println()
	return c.prompter.ConfirmDefault("import browser data", "Proceed?", true)
}

func (c ProfilesImportLocalCmd) confirmBundleFallback(fit profileBundleFit) (bool, error) {
	pterm.Println()
	pterm.Warning.Printf("Browser data is %s; Kernel supports %s.\n\n", formatBinaryBytes(fit.originalSize), formatBinaryBytes(fit.limit))
	pterm.Println("To fit, Kernel will skip:")
	if fit.skippedStorageRecords > 0 {
		originCount := len(fit.skippedStorageOrigins)
		pterm.Printf(
			"  Local storage — %d key%s from %d origin%s (%s)\n",
			fit.skippedStorageRecords, pluralSuffix(fit.skippedStorageRecords),
			originCount, pluralSuffix(originCount), formatBinaryBytes(fit.skippedStorageBytes),
		)
	}
	if fit.skippedHistoryRecords > 0 {
		pterm.Printf("  History — %d visit%s\n", fit.skippedHistoryRecords, pluralSuffix(fit.skippedHistoryRecords))
	}
	pterm.Println("\nCookies and selected bookmarks will remain included.")
	return c.prompter.ConfirmDefault("fit browser import", "Continue with these changes?", true)
}

func selectedCookieCount(sites []localbrowser.Site, selected []string) int {
	set := make(map[string]struct{}, len(selected))
	for _, site := range selected {
		set[site] = struct{}{}
	}
	total := 0
	for _, site := range sites {
		if _, ok := set[site.Domain]; ok {
			total += site.CookieCount
		}
	}
	return total
}

func (c ProfilesImportLocalCmd) chooseLocalStorageSites(sites []localbrowser.StorageSite, nonInteractive bool) ([]string, error) {
	all := make([]string, 0, len(sites))
	for _, site := range sites {
		all = append(all, site.Origin)
	}
	if storageSiteBytes(sites) <= localbrowser.MaxPortableStorageSize {
		return all, nil
	}
	if nonInteractive {
		return nil, fmt.Errorf("browser local storage exceeds Kernel's 64 MiB import limit; run interactively to choose websites or disable local storage")
	}

	sorted := append([]localbrowser.StorageSite(nil), sites...)
	sort.SliceStable(sorted, func(left, right int) bool {
		if sorted[left].Bytes == sorted[right].Bytes {
			return sorted[left].Origin < sorted[right].Origin
		}
		return sorted[left].Bytes > sorted[right].Bytes
	})
	labels := make([]string, 0, len(sorted))
	byLabel := make(map[string]string, len(sorted))
	defaults := make([]string, 0, len(sorted))
	var selectedBytes int64
	for index, site := range sorted {
		label := fmt.Sprintf("%d  %s — %s", index+1, compactField(site.Origin, 46), formatBinaryBytes(site.Bytes))
		labels = append(labels, label)
		byLabel[label] = site.Origin
		if selectedBytes+site.Bytes <= localbrowser.MaxPortableStorageSize {
			defaults = append(defaults, label)
			selectedBytes += site.Bytes
		}
	}
	chosen, err := c.prompter.MultiSelect("local storage", "deselect websites until the selection fits", "Choose local website data to import (64 MiB maximum)", labels, defaults)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(chosen))
	for _, label := range chosen {
		result = append(result, byLabel[label])
	}
	return result, nil
}

func buildSelectedProfileData(ctx context.Context, profile localbrowser.Profile, selection localProfileDataSelection, cookies []localbrowser.Cookie, since time.Time) (localbrowser.ProfileData, map[string]int, error) {
	data := selection.data
	data.Cookies = cookies
	counts := make(map[string]int, 5)
	if len(cookies) > 0 {
		counts["cookies"] = len(cookies)
	}
	if data.Bookmarks != nil {
		counts["bookmarks"] = selection.bookmarkCount
	}
	if selection.history {
		history, err := localbrowser.ExportHistory(ctx, profile, since)
		if err != nil {
			return localbrowser.ProfileData{}, nil, err
		}
		data.History = history
		counts["history"] = len(history)
	}
	if selection.storage {
		storage, err := localbrowser.ExportLocalStorage(ctx, profile, selection.storageSites)
		if err != nil {
			return localbrowser.ProfileData{}, nil, err
		}
		data.Storage = storage
		counts["storage"] = len(storage)
	}
	return data, counts, nil
}

func selectedProfileCategories(counts map[string]int) []string {
	order := []string{"cookies", "storage", "bookmarks", "history"}
	result := make([]string, 0, len(counts))
	for _, category := range order {
		if count, selected := counts[category]; selected && count > 0 {
			result = append(result, category)
		}
	}
	return result
}

func storageSiteBytes(sites []localbrowser.StorageSite) int64 {
	var total int64
	for _, site := range sites {
		total += site.Bytes
	}
	return total
}

func selectedStorageSiteBytes(sites []localbrowser.StorageSite, selected []string) int64 {
	set := make(map[string]struct{}, len(selected))
	for _, origin := range selected {
		set[origin] = struct{}{}
	}
	var total int64
	for _, site := range sites {
		if _, ok := set[site.Origin]; ok {
			total += site.Bytes
		}
	}
	return total
}

func importedStorageOriginCount(records []localbrowser.StorageRecord) int {
	origins := make(map[string]struct{})
	for _, record := range records {
		origins[record.Origin] = struct{}{}
	}
	return len(origins)
}

type storageImportSummary struct {
	importedOrigins int
	importedEntries int
	skippedOrigins  int
	skippedEntries  int
}

func effectiveStorageImportSummary(applied localbrowser.AppliedProfile, requestedEntries, requestedOrigins int) storageImportSummary {
	if applied.StorageEntriesImported == nil || applied.StorageOriginsImported == nil {
		return storageImportSummary{importedOrigins: requestedOrigins, importedEntries: requestedEntries}
	}
	summary := storageImportSummary{
		importedOrigins: *applied.StorageOriginsImported,
		importedEntries: *applied.StorageEntriesImported,
	}
	if applied.StorageOriginsSkipped != nil {
		summary.skippedOrigins = *applied.StorageOriginsSkipped
	}
	if applied.StorageEntriesSkipped != nil {
		summary.skippedEntries = *applied.StorageEntriesSkipped
	}
	return summary
}

func formatBinaryBytes(bytes int64) string {
	if bytes < 1<<20 {
		return fmt.Sprintf("%.1f KiB", float64(bytes)/(1<<10))
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
}
