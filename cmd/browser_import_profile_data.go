package cmd

import (
	"context"
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

func formatBinaryBytes(bytes int64) string {
	if bytes < 1<<20 {
		return fmt.Sprintf("%.1f KiB", float64(bytes)/(1<<10))
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
}
