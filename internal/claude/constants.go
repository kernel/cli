// Package claude provides utilities for working with the Claude for Chrome extension
// in Kernel browsers.
package claude

const (
	// ExtensionID is the Chrome Web Store ID for Claude for Chrome
	ExtensionID = "fcoeoabgfenejglbffodgkkbkcdhcgfn"

	// ExtensionName is the human-readable name of the extension
	ExtensionName = "Claude for Chrome"

	// KernelUserDataPath is the path to Chrome's user data directory in Kernel browsers
	KernelUserDataPath = "/home/kernel/user-data"

	// KernelDefaultProfilePath is the path to the Default profile in Kernel browsers
	KernelDefaultProfilePath = "/home/kernel/user-data/Default"

	// KernelExtensionSettingsPath is where Chrome stores extension LocalStorage/LevelDB data
	KernelExtensionSettingsPath = "/home/kernel/user-data/Default/Local Extension Settings"

	// KernelUser is the username that owns the user-data directory in Kernel browsers
	KernelUser = "kernel"

	// SidePanelURL is the URL to open the Claude extension sidepanel in window mode
	SidePanelURL = "chrome-extension://" + ExtensionID + "/sidepanel.html?mode=window"

	// DefaultBundleName is the default filename for the extension bundle
	DefaultBundleName = "claude-bundle.zip"

	// BundleExtensionDir is the directory name for the extension within the bundle
	BundleExtensionDir = "extension"

	// BundleAuthStorageDir is the directory name for auth storage within the bundle
	BundleAuthStorageDir = "auth-storage"
)
