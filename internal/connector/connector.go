// Package connector owns the small operating-system bridge between a
// kernel:// browser deep link and the Kernel CLI.
package connector

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	Scheme        = "kernel"
	ImportHost    = "browser-import"
	maxDeepLink   = 2048
	bundleID      = "com.onkernel.cli.connector"
	connectorName = "Kernel Connector.app"
)

var (
	projectIDPattern = regexp.MustCompile(`^[a-z0-9]{24}$`)
	importIDPattern  = regexp.MustCompile(`^bri_[a-z0-9]{8,64}$`)
)

// BrowserImportLink is the trusted, non-secret input carried by a dashboard
// deep link. The CLI still authenticates and authorizes the project itself.
type BrowserImportLink struct {
	ProjectID string
	ImportID  string
}

// ParseBrowserImportLink validates a Kernel browser-import deep link.
func ParseBrowserImportLink(raw string) (BrowserImportLink, error) {
	if raw == "" || len(raw) > maxDeepLink || strings.TrimSpace(raw) != raw {
		return BrowserImportLink{}, errors.New("invalid Kernel browser import link")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != Scheme || parsed.Host != ImportHost || (parsed.Path != "" && parsed.Path != "/") || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return BrowserImportLink{}, errors.New("invalid Kernel browser import link")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) < 1 || len(query) > 2 || len(query["project_id"]) != 1 {
		return BrowserImportLink{}, errors.New("invalid Kernel browser import link")
	}
	projectID := query.Get("project_id")
	if !projectIDPattern.MatchString(projectID) {
		return BrowserImportLink{}, errors.New("invalid Kernel project ID")
	}
	importID := query.Get("import_id")
	if len(query) == 2 && (len(query["import_id"]) != 1 || !importIDPattern.MatchString(importID)) {
		return BrowserImportLink{}, errors.New("invalid Kernel browser import ID")
	}
	return BrowserImportLink{ProjectID: projectID, ImportID: importID}, nil
}

// URL returns the canonical browser-import deep link for a project.
func URL(projectID string, importID ...string) (string, error) {
	if !projectIDPattern.MatchString(projectID) {
		return "", errors.New("invalid Kernel project ID")
	}
	query := url.Values{"project_id": []string{projectID}}
	if len(importID) > 1 || (len(importID) == 1 && !importIDPattern.MatchString(importID[0])) {
		return "", errors.New("invalid Kernel browser import ID")
	}
	if len(importID) == 1 {
		query.Set("import_id", importID[0])
	}
	return Scheme + "://" + ImportHost + "?" + query.Encode(), nil
}

type commandRunner func(context.Context, string, ...string) error

// Install registers the connector for the current user without downloading
// another executable. The macOS app delegates back to this Kernel binary.
func Install(ctx context.Context, home, executable string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("Kernel Connector installation is not yet supported on %s", runtime.GOOS)
	}
	return installMacOS(ctx, home, executable, runCommand)
}

func installMacOS(ctx context.Context, home, executable string, run commandRunner) (string, error) {
	if home == "" || !filepath.IsAbs(home) || executable == "" || !filepath.IsAbs(executable) {
		return "", errors.New("Kernel Connector requires absolute home and executable paths")
	}
	applications := filepath.Join(home, "Applications")
	if err := os.MkdirAll(applications, 0o755); err != nil {
		return "", fmt.Errorf("create Applications directory: %w", err)
	}
	temporaryRoot, err := os.MkdirTemp(applications, ".kernel-connector-")
	if err != nil {
		return "", fmt.Errorf("prepare Kernel Connector: %w", err)
	}
	removeTemporaryRoot := true
	defer func() {
		if removeTemporaryRoot {
			_ = os.RemoveAll(temporaryRoot)
		}
	}()
	temporaryApp := filepath.Join(temporaryRoot, connectorName)
	if err := run(ctx, "/usr/bin/osacompile", "-o", temporaryApp, "-e", macOSAppleScript(executable)); err != nil {
		return "", fmt.Errorf("build Kernel Connector app: %w", err)
	}
	plist := filepath.Join(temporaryApp, "Contents", "Info.plist")
	commands := [][]string{
		{"-replace", "CFBundleIdentifier", "-string", bundleID, plist},
		{"-insert", "LSUIElement", "-bool", "YES", plist},
		{"-insert", "CFBundleURLTypes", "-json", `[{"CFBundleURLName":"Kernel Browser Import","CFBundleURLSchemes":["kernel"]}]`, plist},
	}
	for _, args := range commands {
		if err := run(ctx, "/usr/bin/plutil", args...); err != nil {
			return "", fmt.Errorf("configure Kernel Connector app: %w", err)
		}
	}
	if err := run(ctx, "/usr/bin/codesign", "--force", "--sign", "-", temporaryApp); err != nil {
		return "", fmt.Errorf("sign Kernel Connector app: %w", err)
	}
	if err := run(ctx, "/usr/bin/codesign", "--verify", "--strict", temporaryApp); err != nil {
		return "", fmt.Errorf("verify Kernel Connector app: %w", err)
	}
	destination := filepath.Join(applications, connectorName)
	backup := filepath.Join(temporaryRoot, "previous.app")
	if _, err := os.Lstat(destination); err == nil {
		owned, ownershipErr := isKernelConnector(destination)
		if ownershipErr != nil {
			return "", fmt.Errorf("inspect existing Kernel Connector app: %w", ownershipErr)
		}
		if !owned {
			return "", fmt.Errorf("refusing to replace app not owned by Kernel: %s", destination)
		}
		if err := os.Rename(destination, backup); err != nil {
			return "", fmt.Errorf("replace Kernel Connector app: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(temporaryApp, destination); err != nil {
		if _, backupErr := os.Lstat(backup); backupErr == nil {
			if restoreErr := os.Rename(backup, destination); restoreErr != nil {
				removeTemporaryRoot = false
				return "", fmt.Errorf("install Kernel Connector app: %w; restoring previous app failed: %v; backup preserved at %s", err, restoreErr, backup)
			}
		}
		return "", fmt.Errorf("install Kernel Connector app: %w", err)
	}
	register := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	if err := run(ctx, register, "-f", destination); err != nil {
		if removeErr := os.RemoveAll(destination); removeErr != nil {
			removeTemporaryRoot = false
			return "", fmt.Errorf("register kernel:// handler: %w; remove failed installation: %v; previous app preserved at %s", err, removeErr, backup)
		}
		if _, backupErr := os.Lstat(backup); backupErr == nil {
			if restoreErr := os.Rename(backup, destination); restoreErr != nil {
				removeTemporaryRoot = false
				return "", fmt.Errorf("register kernel:// handler: %w; restoring previous app failed: %v; backup preserved at %s", err, restoreErr, backup)
			}
		}
		return "", fmt.Errorf("register kernel:// handler: %w", err)
	}
	return destination, nil
}

func isKernelConnector(app string) (bool, error) {
	info, err := os.Lstat(app)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	plist, err := os.Open(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer plist.Close()
	const maxPlistSize = 1 << 20
	contents, err := io.ReadAll(io.LimitReader(plist, maxPlistSize+1))
	if err != nil {
		return false, err
	}
	if len(contents) > maxPlistSize {
		return false, errors.New("existing app Info.plist is too large")
	}
	identifier, err := bundleIdentifier(contents)
	if err != nil {
		return false, err
	}
	return identifier == bundleID, nil
}

func bundleIdentifier(contents []byte) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(contents)))
	wantValue := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse Info.plist: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "key" {
			var key string
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return "", fmt.Errorf("parse Info.plist key: %w", err)
			}
			wantValue = key == "CFBundleIdentifier"
			continue
		}
		if wantValue && start.Name.Local == "string" {
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return "", fmt.Errorf("parse CFBundleIdentifier: %w", err)
			}
			return value, nil
		}
		wantValue = false
	}
	return "", errors.New("existing app has no CFBundleIdentifier")
}

func macOSAppleScript(executable string) string {
	return `on «event GURLGURL» incomingURL
set kernelExecutable to "` + appleScriptString(executable) + `"
set commandText to "for variable in KERNEL_BASE_URL KERNEL_API_KEY KERNEL_AUTH_BASE_URL; do value=$(/bin/launchctl getenv \"$variable\"); if [[ -n \"$value\" ]]; then export \"$variable=$value\"; fi; done; exec " & quoted form of kernelExecutable & " connector open " & quoted form of incomingURL
set scriptPath to «event sysoexec» "/usr/bin/mktemp /tmp/kernel-connector.XXXXXX"
set successPath to scriptPath & ".success"
set scriptFile to «event rdwropen» POSIX file scriptPath with «class perm»
«event rdwrwrit» "#!/bin/zsh" & linefeed & "rm -f " & quoted form of scriptPath & " " & quoted form of successPath & linefeed & "if [[ ! -x " & quoted form of kernelExecutable & " ]]; then echo 'Kernel CLI was removed. Reinstall it with: brew install kernel/tap/kernel'; read -k 1 '?Press any key to close'; exit 1; fi" & linefeed & "/bin/zsh -lic " & quoted form of commandText & linefeed & "commandStatus=$?" & linefeed & "if [[ $commandStatus -eq 0 ]]; then /usr/bin/touch " & quoted form of successPath & "; fi" & linefeed & "exit $commandStatus" & linefeed given «class refn»:scriptFile
«event rdwrclos» scriptFile
«event sysoexec» "/bin/chmod 700 " & quoted form of scriptPath
tell application "Terminal"
activate
set connectorTab to do script (quoted form of scriptPath)
end tell
repeat
try
tell application "Terminal" to set connectorBusy to busy of connectorTab
on error
return
end try
if not connectorBusy then exit repeat
delay 0.2
end repeat
try
«event sysoexec» "/usr/bin/test -f " & quoted form of successPath
tell application "Terminal" to close connectorTab
«event sysoexec» "/bin/rm -f " & quoted form of successPath
end try
end «event GURLGURL»`
}

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
