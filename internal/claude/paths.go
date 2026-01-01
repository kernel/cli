package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// GetChromeExtensionPath returns the path to the Claude extension directory for the given Chrome profile.
// It automatically detects the OS and returns the appropriate path.
func GetChromeExtensionPath(profile string) (string, error) {
	if profile == "" {
		profile = "Default"
	}

	extensionsDir, err := getChromeExtensionsDir(profile)
	if err != nil {
		return "", err
	}

	extDir := filepath.Join(extensionsDir, ExtensionID)
	if _, err := os.Stat(extDir); os.IsNotExist(err) {
		return "", fmt.Errorf("Claude extension not found at %s", extDir)
	}

	// Find the latest version directory
	versionDir, err := findLatestVersionDir(extDir)
	if err != nil {
		return "", fmt.Errorf("failed to find extension version: %w", err)
	}

	return versionDir, nil
}

// GetChromeAuthStoragePath returns the path to the Claude extension's auth storage (LevelDB).
func GetChromeAuthStoragePath(profile string) (string, error) {
	if profile == "" {
		profile = "Default"
	}

	userDataDir, err := getChromeUserDataDir()
	if err != nil {
		return "", err
	}

	// Extension local storage is stored in "Local Extension Settings/<extension-id>"
	authPath := filepath.Join(userDataDir, profile, "Local Extension Settings", ExtensionID)
	if _, err := os.Stat(authPath); os.IsNotExist(err) {
		return "", fmt.Errorf("Claude extension auth storage not found at %s", authPath)
	}

	return authPath, nil
}

// getChromeUserDataDir returns the Chrome user data directory for the current OS.
func getChromeUserDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	var userDataDir string
	switch runtime.GOOS {
	case "darwin":
		userDataDir = filepath.Join(homeDir, "Library", "Application Support", "Google", "Chrome")
	case "linux":
		userDataDir = filepath.Join(homeDir, ".config", "google-chrome")
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(homeDir, "AppData", "Local")
		}
		userDataDir = filepath.Join(localAppData, "Google", "Chrome", "User Data")
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if _, err := os.Stat(userDataDir); os.IsNotExist(err) {
		return "", fmt.Errorf("Chrome user data directory not found at %s", userDataDir)
	}

	return userDataDir, nil
}

// getChromeExtensionsDir returns the extensions directory for the given profile.
func getChromeExtensionsDir(profile string) (string, error) {
	userDataDir, err := getChromeUserDataDir()
	if err != nil {
		return "", err
	}

	extensionsDir := filepath.Join(userDataDir, profile, "Extensions")
	if _, err := os.Stat(extensionsDir); os.IsNotExist(err) {
		return "", fmt.Errorf("Chrome extensions directory not found at %s", extensionsDir)
	}

	return extensionsDir, nil
}

// findLatestVersionDir finds the latest version directory within an extension directory.
// Chrome stores extensions in subdirectories named by version (e.g., "1.0.0_0").
func findLatestVersionDir(extDir string) (string, error) {
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return "", fmt.Errorf("failed to read extension directory: %w", err)
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "" && entry.Name()[0] != '.' {
			versions = append(versions, entry.Name())
		}
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no version directories found in %s", extDir)
	}

	// Sort versions and pick the latest (lexicographic sort works for semver-like versions)
	sort.Strings(versions)
	latestVersion := versions[len(versions)-1]

	return filepath.Join(extDir, latestVersion), nil
}

// ListChromeProfiles returns a list of available Chrome profiles.
func ListChromeProfiles() ([]string, error) {
	userDataDir, err := getChromeUserDataDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(userDataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read Chrome user data directory: %w", err)
	}

	var profiles []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Chrome profiles are named "Default", "Profile 1", "Profile 2", etc.
		if name == "Default" || (len(name) > 8 && name[:8] == "Profile ") {
			// Check if it's actually a profile by looking for a Preferences file
			prefsPath := filepath.Join(userDataDir, name, "Preferences")
			if _, err := os.Stat(prefsPath); err == nil {
				profiles = append(profiles, name)
			}
		}
	}

	return profiles, nil
}
