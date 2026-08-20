package connector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProjectID = "ibdzi0e8019dpsonqwixsbfe"

func testPlist(identifier string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + identifier + `</string></dict></plist>`)
}

func TestParseBrowserImportLink(t *testing.T) {
	t.Parallel()
	link, err := ParseBrowserImportLink("kernel://browser-import?project_id=" + testProjectID)
	require.NoError(t, err)
	assert.Equal(t, testProjectID, link.ProjectID)
}

func TestParseBrowserImportLinkRejectsUntrustedShapes(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"https://browser-import?project_id=" + testProjectID,
		"kernel://evil?project_id=" + testProjectID,
		"kernel://browser-import/path?project_id=" + testProjectID,
		"kernel://browser-import?project_id=" + testProjectID + "&next=https://evil.test",
		"kernel://browser-import?project_id=" + testProjectID + "&project_id=" + testProjectID,
		"kernel://browser-import?project_id=not-a-project",
		"kernel://browser-import?project_id=" + testProjectID + "#fragment",
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			_, err := ParseBrowserImportLink(raw)
			require.Error(t, err)
		})
	}
}

func TestURLRoundTrip(t *testing.T) {
	t.Parallel()
	raw, err := URL(testProjectID)
	require.NoError(t, err)
	link, err := ParseBrowserImportLink(raw)
	require.NoError(t, err)
	assert.Equal(t, testProjectID, link.ProjectID)
}

func TestInstallMacOSBuildsAndRegistersUserApp(t *testing.T) {
	home := t.TempDir()
	executable := filepath.Join(home, "bin", "kernel")
	var calls [][]string
	runner := func(_ context.Context, name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		if name == "/usr/bin/osacompile" {
			app := args[1]
			require.NoError(t, os.MkdirAll(filepath.Join(app, "Contents"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("plist"), 0o600))
		}
		return nil
	}

	installed, err := installMacOS(t.Context(), home, executable, runner)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "Applications", connectorName), installed)
	require.NotEmpty(t, calls)
	assert.Equal(t, "/usr/bin/osacompile", calls[0][0])
	assert.Contains(t, calls[0][4], executable)
	last := calls[len(calls)-1]
	assert.Contains(t, last[0], "lsregister")
	assert.Equal(t, []string{"-f", installed}, last[1:])
}

func TestInstallMacOSRefusesToReplaceForeignApp(t *testing.T) {
	home := t.TempDir()
	executable := filepath.Join(home, "bin", "kernel")
	destination := filepath.Join(home, "Applications", connectorName)
	require.NoError(t, os.MkdirAll(filepath.Join(destination, "Contents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(destination, "Contents", "Info.plist"), testPlist("example.foreign."+bundleID), 0o600))
	runner := func(_ context.Context, name string, args ...string) error {
		if name == "/usr/bin/osacompile" {
			app := args[1]
			require.NoError(t, os.MkdirAll(filepath.Join(app, "Contents"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), testPlist(bundleID), 0o600))
		}
		return nil
	}

	_, err := installMacOS(t.Context(), home, executable, runner)
	require.ErrorContains(t, err, "not owned by Kernel")
	contents, readErr := os.ReadFile(filepath.Join(destination, "Contents", "Info.plist"))
	require.NoError(t, readErr)
	assert.Equal(t, testPlist("example.foreign."+bundleID), contents)
}

func TestInstallMacOSRestoresPreviousAppWhenRegistrationFails(t *testing.T) {
	home := t.TempDir()
	executable := filepath.Join(home, "bin", "kernel")
	destination := filepath.Join(home, "Applications", connectorName)
	require.NoError(t, os.MkdirAll(filepath.Join(destination, "Contents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(destination, "Contents", "Info.plist"), testPlist(bundleID), 0o600))
	runner := func(_ context.Context, name string, args ...string) error {
		if name == "/usr/bin/osacompile" {
			app := args[1]
			require.NoError(t, os.MkdirAll(filepath.Join(app, "Contents"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), testPlist(bundleID), 0o600))
		}
		if strings.Contains(name, "lsregister") {
			return errors.New("registration failed")
		}
		return nil
	}

	_, err := installMacOS(t.Context(), home, executable, runner)
	require.ErrorContains(t, err, "register kernel:// handler")
	contents, readErr := os.ReadFile(filepath.Join(destination, "Contents", "Info.plist"))
	require.NoError(t, readErr)
	assert.Equal(t, testPlist(bundleID), contents)
}

func TestMacOSAppleScriptShellQuotesURLAtRuntime(t *testing.T) {
	t.Parallel()
	script := macOSAppleScript(`/opt/homebrew/bin/kernel`)
	assert.Contains(t, script, `quoted form of incomingURL`)
	assert.Contains(t, script, `" connector open "`)
	assert.Contains(t, script, `/usr/bin/open -a Terminal`)
	assert.Contains(t, script, `/bin/zsh -lic`)
	assert.Contains(t, script, `/usr/bin/mktemp /tmp/kernel-connector.XXXXXX`)
	assert.NotContains(t, script, `XXXXXX.command`)
	assert.Contains(t, script, `Kernel CLI was removed`)
	assert.Contains(t, script, `«event GURLGURL»`)
	assert.NotContains(t, script, `tell application "Terminal"`)
	assert.NotContains(t, script, "project_id=")

	escaped := macOSAppleScript(`/Applications/a\"b/kernel`)
	assert.True(t, strings.Contains(escaped, `a\\\"b`))
}

func TestBundleIdentifierRequiresExactValidPlistValue(t *testing.T) {
	t.Parallel()
	identifier, err := bundleIdentifier(testPlist(bundleID))
	require.NoError(t, err)
	assert.Equal(t, bundleID, identifier)

	identifier, err = bundleIdentifier([]byte(`<?xml version="1.0"?><plist><dict><key>Comment</key><string>` + bundleID + `</string><key>CFBundleIdentifier</key><string>example.foreign</string></dict></plist>`))
	require.NoError(t, err)
	assert.Equal(t, "example.foreign", identifier)

	_, err = bundleIdentifier([]byte("not a plist"))
	require.Error(t, err)
}
