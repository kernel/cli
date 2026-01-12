package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"

	"github.com/Masterminds/semver/v3"
)

const (
	defaultReleasesAPI = "https://api.github.com/repos/kernel/cli/releases"
	userAgent          = "kernel-cli/update-check"
	cacheRelPath       = "kernel/update-check.json"
	requestTimeout     = 3 * time.Second
)

// Cache stores update-check metadata to throttle frequency and avoid
// repeating the same banner too often.
type Cache struct {
	LastChecked      time.Time `json:"last_checked"`
	LastShownVersion string    `json:"last_shown_version"`
}

// shouldCheck returns true if we should perform a network check now.
func shouldCheck(lastChecked, now time.Time, frequency time.Duration) bool {
	if lastChecked.IsZero() {
		return true
	}
	return now.Sub(lastChecked) >= frequency
}

func normalizeSemver(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		v = v[1:]
	}
	return v
}

func isSemverLike(v string) bool {
	v = normalizeSemver(v)
	if v == "" {
		return false
	}
	_, err := semver.NewVersion(v)
	return err == nil
}

// isNewerVersion reports whether latest > current using semver rules.
func isNewerVersion(current, latest string) (bool, error) {
	c := normalizeSemver(current)
	l := normalizeSemver(latest)
	if c == "" || l == "" {
		return false, errors.New("non-semver version")
	}
	cv, err := semver.NewVersion(c)
	if err != nil {
		return false, err
	}
	lv, err := semver.NewVersion(l)
	if err != nil {
		return false, err
	}
	return lv.GreaterThan(cv), nil
}

// fetchLatest queries GitHub Releases and returns the latest stable tag and URL.
// It expects that the GitHub API returns releases in descending chronological order
// (newest first), which is standard behavior.
func fetchLatest(ctx context.Context) (tag string, url string, err error) {
	apiURL := os.Getenv("KERNEL_RELEASES_URL")
	if apiURL == "" {
		apiURL = defaultReleasesAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var releases []struct {
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", "", err
	}
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		if r.TagName == "" {
			continue
		}
		return r.TagName, r.HTMLURL, nil
	}
	return "", "", errors.New("no stable releases found")
}

// printUpgradeMessage prints a concise upgrade banner.
func printUpgradeMessage(current, latest, url string) {
	cur := strings.TrimPrefix(current, "v")
	lat := strings.TrimPrefix(latest, "v")
	pterm.Println()
	pterm.Info.Printf("A new release of kernel is available: %s → %s\n", cur, lat)
	if url != "" {
		pterm.Info.Printf("Release notes: %s\n", url)
	}
	if cmd := SuggestUpgradeCommand(); cmd != "" {
		pterm.Info.Printf("To upgrade, run: %s\n", cmd)
	} else {
		pterm.Info.Println("To upgrade, visit the release page above or use your package manager.")
	}
}

// MaybeShowMessage orchestrates cache, fetch, compare, and printing.
// It is designed to be non-fatal and fast; errors are swallowed.
func MaybeShowMessage(ctx context.Context, currentVersion string, frequency time.Duration) {
	defer func() { _ = recover() }()

	if os.Getenv("KERNEL_NO_UPDATE_CHECK") == "1" {
		return
	}
	if !isSemverLike(currentVersion) {
		return
	}
	if invokedTrivialCommand() {
		return
	}

	cachePath := filepath.Join(xdgCacheDir(), cacheRelPath)
	cache, _ := loadCache(cachePath)

	// Allow env override for frequency in tests (e.g., "1h", "24h").
	effectiveFreq := frequency
	if envFreq := os.Getenv("KERNEL_UPDATE_CHECK_FREQUENCY"); envFreq != "" {
		if d, err := time.ParseDuration(envFreq); err == nil && d > 0 {
			effectiveFreq = d
		}
	}
	if !shouldCheck(cache.LastChecked, time.Now().UTC(), effectiveFreq) {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	latestTag, releaseURL, err := fetchLatest(ctx)
	if err != nil {
		cache.LastChecked = time.Now().UTC()
		_ = saveCache(cachePath, cache)
		return
	}
	isNewer, err := isNewerVersion(currentVersion, latestTag)
	if err != nil || !isNewer {
		cache.LastChecked = time.Now().UTC()
		_ = saveCache(cachePath, cache)
		return
	}

	// Note: We intentionally do not suppress by LastShownVersion so that
	// the banner reappears each frequency window until the user upgrades.
	printUpgradeMessage(currentVersion, latestTag, releaseURL)
	cache.LastChecked = time.Now().UTC()
	cache.LastShownVersion = latestTag
	_ = saveCache(cachePath, cache)
}

// xdgCacheDir returns a best-effort per-user cache directory.
func xdgCacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return d
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".cache")
	}
	return "."
}

// loadCache reads the cache file from path. If the file doesn't exist,
// returns an empty cache and no error.
func loadCache(path string) (Cache, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Cache{}, nil
		}
		return Cache{}, err
	}
	var c Cache
	if err := json.Unmarshal(b, &c); err != nil {
		return Cache{}, err
	}
	return c, nil
}

// saveCache writes the cache to disk, creating parent directories as needed.
func saveCache(path string, c Cache) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// SuggestUpgradeCommand attempts to infer how the user installed kernel and
// returns a tailored upgrade command. Falls back to empty string on unknown.
func SuggestUpgradeCommand() string {
	// Collect candidate paths: current executable and shell-resolved binary
	candidates := []string{}
	if exe, err := os.Executable(); err == nil && exe != "" {
		if real, err2 := filepath.EvalSymlinks(exe); err2 == nil && real != "" {
			exe = real
		}
		candidates = append(candidates, exe)
	}
	if which, err := exec.LookPath("kernel"); err == nil && which != "" {
		candidates = append(candidates, which)
	}

	// Helpers
	norm := func(p string) string { return strings.ToLower(filepath.ToSlash(p)) }
	hasHomebrew := func(p string) bool {
		p = norm(p)
		return strings.Contains(p, "homebrew") || strings.Contains(p, "/cellar/")
	}
	hasBun := func(p string) bool { p = norm(p); return strings.Contains(p, "/.bun/") }
	hasPNPM := func(p string) bool {
		p = norm(p)
		return strings.Contains(p, "/pnpm/") || strings.Contains(p, "/.pnpm/")
	}
	hasNPM := func(p string) bool {
		p = norm(p)
		return strings.Contains(p, "/npm/") || strings.Contains(p, "/node_modules/.bin/")
	}

	type rule struct {
		check   func(string) bool
		envKeys []string
		cmd     string
	}

	rules := []rule{
		{hasHomebrew, nil, ""}, // Homebrew handled specially below
		{hasBun, []string{"BUN_INSTALL"}, "bun add -g @onkernel/cli@latest"},
		{hasPNPM, []string{"PNPM_HOME"}, "pnpm add -g @onkernel/cli@latest"},
		{hasNPM, []string{"NPM_CONFIG_PREFIX", "npm_config_prefix", "VOLTA_HOME"}, "npm i -g @onkernel/cli@latest"},
	}

	// Path-based detection first
	for _, c := range candidates {
		for _, r := range rules {
			if r.check != nil && r.check(c) {
				if r.cmd == "" {
					// Homebrew detected, check which tap
					return suggestHomebrewCommand(c)
				}
				return r.cmd
			}
		}
	}

	// Env-only fallbacks
	envSet := func(keys []string) bool {
		for _, k := range keys {
			if k == "" {
				continue
			}
			if os.Getenv(k) != "" {
				return true
			}
		}
		return false
	}
	for _, r := range rules {
		if len(r.envKeys) > 0 && envSet(r.envKeys) {
			return r.cmd
		}
	}

	// Default suggestion when unknown
	return "brew upgrade kernel/tap/kernel"
}

// suggestHomebrewCommand returns the appropriate brew command based on which tap
// the user has installed. If they have the old onkernel/tap, they need to uninstall
// and reinstall from the new kernel/tap.
func suggestHomebrewCommand(exePath string) string {
	// Check if the executable path indicates the old tap by looking at version.
	// The Cellar path format is: /opt/homebrew/Cellar/kernel/<version>/bin/kernel
	// Versions before 0.13.0 were published to onkernel/tap, 0.13.0+ to kernel/tap.
	if isOldTapVersion(exePath) {
		return "brew uninstall kernel && brew install kernel/tap/kernel"
	}

	return "brew upgrade kernel/tap/kernel"
}

// isOldTapVersion checks if the Homebrew Cellar path contains a version < 0.13.0,
// which indicates it was installed from the old onkernel/tap.
func isOldTapVersion(exePath string) bool {
	// Expected path format: .../Cellar/kernel/<version>/...
	normPath := strings.ToLower(filepath.ToSlash(exePath))
	if !strings.Contains(normPath, "/cellar/kernel/") {
		return false
	}

	// Extract version from path
	parts := strings.Split(normPath, "/cellar/kernel/")
	if len(parts) < 2 {
		return false
	}
	remainder := parts[1] // e.g., "0.12.4/bin/kernel"
	versionPart := strings.Split(remainder, "/")[0]
	if versionPart == "" {
		return false
	}

	// Parse and compare versions
	installed, err := semver.NewVersion(versionPart)
	if err != nil {
		return false
	}
	threshold, _ := semver.NewVersion("0.13.0")
	return installed.LessThan(threshold)
}

// invokedTrivialCommand returns true if the argv suggests a trivial invocation
// like help/completion/version-only where we can skip the update check.
func invokedTrivialCommand() bool {
	args := os.Args[1:]
	for _, a := range args {
		if a == "--version" || a == "-v" || a == "help" || a == "completion" {
			return true
		}
	}
	return false
}
