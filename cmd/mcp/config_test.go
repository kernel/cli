package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
)

func testHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func readTestConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	standard, err := hujson.Standardize(data)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(standard, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestInstallPreservesTargetConfigs(t *testing.T) {
	targets := []struct {
		target  Target
		section string
		stdio   bool
		name    string
		port    string
	}{
		{TargetCursor, "mcpServers", false, "", ""},
		{TargetClaude, "mcpServers", true, "Claude Desktop", "46093"},
		{TargetClaudeCode, "mcpServers", false, "", ""},
		{TargetAntigravity, "mcpServers", true, "Antigravity", "46094"},
		{TargetWindsurf, "mcpServers", true, "Windsurf", "46095"},
		{TargetVSCode, "servers", false, "", ""},
		{TargetZed, "context_servers", true, "Zed", "46097"},
		{TargetFx, "mcp", false, "", ""},
	}
	if got := AllTargets(); len(got) != len(targets)+1 {
		t.Fatalf("registered targets = %d, want %d including Goose", len(got), len(targets)+1)
	}
	for _, tc := range targets {
		t.Run(string(tc.target), func(t *testing.T) {
			testHome(t)
			path, err := GetConfigPath(tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			seed := map[string]any{
				"unrelated": map[string]any{"nested": true},
				tc.section: map[string]any{
					"other": map[string]any{"command": "other"},
					"kernel": map[string]any{
						"headers":   map[string]any{"Authorization": "Bearer example"},
						"custom":    []any{"leave", "alone"},
						"url":       "https://old.example",
						"serverUrl": "https://old.example",
						"httpUrl":   "https://old.example",
						"command":   "old",
						"args":      []any{"old"},
						"type":      "stdio",
					},
				},
			}
			if tc.target == TargetFx {
				seed[tc.section].(map[string]any)["kernel"].(map[string]any)["oauth"] = map[string]any{"clientId": "custom"}
			}
			if tc.target == TargetZed {
				seed[tc.section].(map[string]any)["kernel"].(map[string]any)["source"] = "custom"
			}
			input, err := json.Marshal(seed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, input, 0644); err != nil {
				t.Fatal(err)
			}
			if err := Install(tc.target); err != nil {
				t.Fatal(err)
			}
			config := readTestConfig(t, path)
			if !reflect.DeepEqual(config["unrelated"], seed["unrelated"]) {
				t.Fatalf("unrelated config changed: %#v", config["unrelated"])
			}
			servers := config[tc.section].(map[string]any)
			if !reflect.DeepEqual(servers["other"], seed[tc.section].(map[string]any)["other"]) {
				t.Fatalf("other server changed: %#v", servers["other"])
			}
			kernel := servers["kernel"].(map[string]any)
			if !reflect.DeepEqual(kernel["headers"], seed[tc.section].(map[string]any)["kernel"].(map[string]any)["headers"]) {
				t.Fatalf("headers changed: %#v", kernel["headers"])
			}
			if !reflect.DeepEqual(kernel["custom"], []any{"leave", "alone"}) {
				t.Fatalf("custom field changed: %#v", kernel["custom"])
			}
			for _, stale := range []string{"serverUrl", "httpUrl"} {
				if _, ok := kernel[stale]; ok {
					t.Errorf("stale field %s retained", stale)
				}
			}
			if tc.stdio {
				if kernel["command"] != "npx" || kernel["url"] != nil {
					t.Fatalf("stdio entry = %#v", kernel)
				}
				args := kernel["args"].([]any)
				if args[0] != "-y" || args[1] != "mcp-remote" || args[2] != KernelMCPURL {
					t.Fatalf("stdio args = %#v", args)
				}
				if len(args) != 6 || args[3] != tc.port || args[4] != "--static-oauth-client-metadata" {
					t.Fatalf("stdio metadata args = %#v", args)
				}
				var metadata map[string]string
				if err := json.Unmarshal([]byte(args[5].(string)), &metadata); err != nil || metadata["client_name"] != tc.name {
					t.Fatalf("stdio client metadata = %#v: %v", metadata, err)
				}
				cacheDir := kernel["env"].(map[string]any)["MCP_REMOTE_CONFIG_DIR"]
				wantDir := filepath.Join(os.Getenv("HOME"), ".mcp-auth", "kernel-"+string(tc.target))
				if cacheDir != wantDir {
					t.Fatalf("auth cache = %v, want %s", cacheDir, wantDir)
				}
				if tc.target == TargetZed {
					if _, exists := kernel["source"]; exists {
						t.Fatal("obsolete Zed source field retained")
					}
				}
			} else {
				if kernel["url"] != KernelMCPURL || kernel["command"] != nil || kernel["args"] != nil {
					t.Fatalf("HTTP entry = %#v", kernel)
				}
				if tc.target == TargetCursor {
					if _, exists := kernel["type"]; exists {
						t.Fatal("stale Cursor transport type retained")
					}
				} else if kernel["type"] != "http" {
					t.Fatalf("HTTP transport type = %#v", kernel["type"])
				}
				if tc.target == TargetFx {
					if !reflect.DeepEqual(kernel["oauth"], map[string]any{"clientId": "custom"}) {
						t.Fatalf("fx oauth = %#v", kernel["oauth"])
					}
				}
			}
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := Install(tc.target); err != nil {
				t.Fatal(err)
			}
			second, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Fatal("second install changed config bytes")
			}
			if runtime.GOOS != "windows" {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 {
					t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
				}
			}
		})
	}
}

func TestInstallFreshAndStrictModes(t *testing.T) {
	for _, target := range AllTargets() {
		if target == TargetGoose {
			continue
		}
		t.Run(string(target), func(t *testing.T) {
			testHome(t)
			path, err := GetConfigPath(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := Install(target); err != nil {
				t.Fatal(err)
			}
			config := readTestConfig(t, path)
			spec, _ := specFor(target)
			if _, ok := config[spec.section].(map[string]any)["kernel"]; !ok {
				t.Fatal("missing kernel entry")
			}
			if runtime.GOOS != "windows" {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 {
					t.Fatalf("new file mode = %o", info.Mode().Perm())
				}
				dir, err := os.Stat(filepath.Dir(path))
				if err != nil {
					t.Fatal(err)
				}
				if filepath.Dir(path) != os.Getenv("HOME") && dir.Mode().Perm() != 0700 {
					t.Fatalf("new directory mode = %o", dir.Mode().Perm())
				}
				if err := os.WriteFile(path, []byte("{\"unrelated\":true}"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0400); err != nil {
					t.Fatal(err)
				}
				if err := Install(target); err != nil {
					t.Fatal(err)
				}
				info, err = os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0400 {
					t.Fatalf("stricter file mode = %o", info.Mode().Perm())
				}
				if !readTestConfig(t, path)["unrelated"].(bool) {
					t.Fatal("existing config lost")
				}
			}
		})
	}
}

func TestInstallPreservesCommentsAndFormatting(t *testing.T) {
	for _, target := range []Target{TargetVSCode, TargetZed} {
		t.Run(string(target), func(t *testing.T) {
			testHome(t)
			path, err := GetConfigPath(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			spec, _ := specFor(target)
			seed := "{\n  // retain this setting\n  \"editor.fontSize\": 14,\n  \"" + spec.section + "\": {\n    // retain this server\n    \"other\": {\"command\": \"other\"},\n    \"kernel\": {\n      // retain this field\n      \"custom\": true,\n    },\n  },\n}\n"
			if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
				t.Fatal(err)
			}
			if err := Install(target); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, keep := range []string{"// retain this setting", "// retain this server", "// retain this field", "\"editor.fontSize\": 14"} {
				if !strings.Contains(string(data), keep) {
					t.Fatalf("lost %s in %s", keep, data)
				}
			}
			readTestConfig(t, path)
		})
	}
}

func TestInstallRejectsBadConfigWithoutChangingIt(t *testing.T) {
	for _, target := range AllTargets() {
		if target == TargetGoose {
			continue
		}
		t.Run(string(target), func(t *testing.T) {
			testHome(t)
			path, err := GetConfigPath(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			spec, _ := specFor(target)
			bad := []string{
				"",
				"{",
				"null",
				"{\"" + spec.section + "\": []}",
				"{\"" + spec.section + "\": {\"kernel\": []}}",
				"{\"" + spec.section + "\": {}, \"" + spec.section + "\": {}}",
			}
			for _, seed := range bad {
				if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
					t.Fatal(err)
				}
				if err := Install(target); err == nil {
					t.Fatalf("accepted malformed config %q", seed)
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != seed {
					t.Fatalf("modified malformed config %q", seed)
				}
				matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".kernel-mcp-*"))
				if err != nil || len(matches) != 0 {
					t.Fatalf("temporary files after failure: %v, %v", matches, err)
				}
			}
		})
	}
}

func TestInstallRejectsNestedDuplicateKeys(t *testing.T) {
	for _, tc := range []struct{ name, config string }{
		{"kernel headers", `{"mcpServers":{"kernel":{"headers":{"Authorization":"first","Authorization":"second"}}}}`},
		{"object in array", `{"other":[{"Authorization":"first","Authorization":"second"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testHome(t)
			path, err := GetConfigPath(TargetCursor)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			if err := Install(TargetCursor); err == nil || !strings.Contains(err.Error(), `duplicate config key "Authorization"`) {
				t.Fatalf("install error = %v, want nested duplicate key", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.config {
				t.Fatal("duplicate-key config changed")
			}
		})
	}
}

func TestInstallWriteFailureLeavesExistingConfig(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetCursor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	seed := []byte("{\"mcpServers\": {}}")
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	// An invalid parent path fails before a temporary file can replace the original.
	badPath := filepath.Join(path, "nested.json")
	spec, _ := specFor(TargetCursor)
	if err := installConfig(badPath, spec); err == nil {
		t.Fatal("expected write failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, seed) {
		t.Fatal("existing config changed after failure")
	}
	failurePath := filepath.Join(t.TempDir(), "config-dir")
	if err := os.Mkdir(failurePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigAtomic(failurePath, seed, 0600); err == nil {
		t.Fatal("expected replacement failure")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(failurePath), ".kernel-mcp-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files after failed replacement: %v, %v", matches, err)
	}
}

func TestInstallSecuresUnchangedConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	testHome(t)
	path, err := GetConfigPath(TargetCursor)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetCursor); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetCursor); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("unchanged config was reformatted")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestInstallDoesNotRewriteMatchingConfig(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetCursor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	seed := []byte("{\n  \"mcpServers\": {\n    \"kernel\": { \"url\": \"" + KernelMCPURL + "\" }\n  }\n}\n")
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetCursor); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("matching config was replaced")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, seed) {
		t.Fatal("matching config was reformatted")
	}
}

func TestInstallVSCodeMigratesLegacyKernelFields(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetVSCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(filepath.Dir(path), "settings.json")
	legacy := []byte("{\n  // leave this file untouched\n  \"editor.fontSize\": 14,\n  \"mcp.servers\": {\"kernel\": {\"url\": \"https://old.example\", \"headers\": {\"Authorization\": \"placeholder\"}, \"custom\": \"old\"}}\n}\n")
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"servers\":{\"other\":{\"command\":\"other\"},\"kernel\":{\"custom\":\"new\"}}}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetVSCode); err != nil {
		t.Fatal(err)
	}
	config := readTestConfig(t, path)
	servers := config["servers"].(map[string]any)
	kernel := servers["kernel"].(map[string]any)
	if kernel["custom"] != "new" || kernel["url"] != KernelMCPURL {
		t.Fatalf("kernel = %#v", kernel)
	}
	if kernel["headers"].(map[string]any)["Authorization"] != "placeholder" {
		t.Fatalf("headers = %#v", kernel["headers"])
	}
	if _, ok := servers["other"]; !ok {
		t.Fatal("other server removed")
	}
	gotLegacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotLegacy, legacy) {
		t.Fatal("legacy settings content changed")
	}
	if runtime.GOOS != "windows" {
		for _, file := range []string{path, legacyPath} {
			info, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0600 {
				t.Fatalf("%s mode = %o", file, info.Mode().Perm())
			}
		}
	}
}

func TestInstallVSCodeRejectsMalformedLegacyConfig(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetVSCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(filepath.Dir(path), "settings.json")
	if err := os.WriteFile(legacyPath, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetVSCode); err == nil {
		t.Fatal("expected invalid legacy settings error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("new config changed after failure: %v", err)
	}
}

func TestInstallVSCodeRejectsNestedDuplicateLegacyKeys(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetVSCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(filepath.Dir(path), "settings.json")
	legacy := []byte(`{"mcp.servers":{"kernel":{"headers":{"Authorization":"first","Authorization":"second"}}}}`)
	if err := os.WriteFile(legacyPath, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetVSCode); err == nil || !strings.Contains(err.Error(), `duplicate config key "Authorization"`) {
		t.Fatalf("install error = %v, want nested duplicate key", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("destination changed after failure: %v", err)
	}
	got, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, legacy) {
		t.Fatal("legacy config changed after failure")
	}
}

func TestInstallVSCodeWriteFailureDoesNotChmodLegacy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions vary on Windows")
	}
	testHome(t)
	path, err := GetConfigPath(TargetVSCode)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(dir, "settings.json")
	legacy := []byte(`{"mcp.servers":{"kernel":{"headers":{"Authorization":"placeholder"}}}}`)
	if err := os.WriteFile(legacyPath, legacy, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Error(err)
		}
	})
	probe, err := os.CreateTemp(dir, "probe-*")
	if err == nil {
		probe.Close()
		os.Remove(probe.Name())
		t.Skip("directory permissions do not prevent writes")
	}
	if !os.IsPermission(err) {
		t.Fatalf("permission probe failed: %v", err)
	}
	if err := Install(TargetVSCode); err == nil {
		t.Fatal("expected destination write failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("destination changed after failure: %v", err)
	}
	got, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, legacy) {
		t.Fatal("legacy config content changed")
	}
	info, err := os.Stat(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("legacy config mode = %o, want 644", info.Mode().Perm())
	}
}

func TestInstallPreservesMCPRemoteOptions(t *testing.T) {
	for target, port := range map[Target]string{
		TargetClaude: "46093", TargetAntigravity: "46094", TargetWindsurf: "46095", TargetZed: "46097",
	} {
		t.Run(string(target), func(t *testing.T) {
			testHome(t)
			path, err := GetConfigPath(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			spec, _ := specFor(target)
			seed := map[string]any{spec.section: map[string]any{"kernel": map[string]any{
				"command": "npx",
				"args":    []string{"-y", "mcp-remote@latest", "https://old.example", "--header-file", "/private/headers.txt", "--static-oauth-client-metadata", `{"client_name":"Old","scope":"read","token_endpoint_auth_method":"none"}`},
				"env":     map[string]string{"CUSTOM": "keep"},
			}}}
			data, err := json.Marshal(seed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := Install(target); err != nil {
				t.Fatal(err)
			}
			config := readTestConfig(t, path)
			args := config[spec.section].(map[string]any)["kernel"].(map[string]any)["args"].([]any)
			if len(args) != 8 || args[1] != "mcp-remote@latest" || args[2] != KernelMCPURL || args[3] != port || args[4] != "--header-file" || args[5] != "/private/headers.txt" {
				t.Fatalf("custom args lost: %#v", args)
			}
			if args[6] != "--static-oauth-client-metadata" {
				t.Fatalf("client metadata missing: %#v", args)
			}
			var metadata map[string]any
			if err := json.Unmarshal([]byte(args[7].(string)), &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["client_name"] != spec.clientName || metadata["scope"] != "read" || metadata["token_endpoint_auth_method"] != "none" {
				t.Fatalf("client metadata changed: %#v", metadata)
			}
			env := config[spec.section].(map[string]any)["kernel"].(map[string]any)["env"].(map[string]any)
			if env["CUSTOM"] != "keep" || env["MCP_REMOTE_CONFIG_DIR"] == nil {
				t.Fatalf("stdio environment changed: %#v", env)
			}
		})
	}
}

func TestInstallPreservesExplicitCallbackPort(t *testing.T) {
	for _, target := range []Target{TargetClaude, TargetAntigravity, TargetWindsurf, TargetZed} {
		t.Run(string(target), func(t *testing.T) {
			testHome(t)
			path, err := GetConfigPath(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := Install(target); err != nil {
				t.Fatal(err)
			}
			config := readTestConfig(t, path)
			spec, _ := specFor(target)
			kernel := config[spec.section].(map[string]any)["kernel"].(map[string]any)
			args := []any{"-y", "mcp-remote@latest", KernelMCPURL, "54321", "--host", "127.0.0.1", "--static-oauth-client-metadata", clientMetadata(spec.clientName)}
			kernel["args"] = args
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := Install(target); err != nil {
				t.Fatal(err)
			}
			got := readTestConfig(t, path)[spec.section].(map[string]any)["kernel"].(map[string]any)["args"]
			if !reflect.DeepEqual(got, args) {
				t.Fatalf("custom callback args = %#v, want %#v", got, args)
			}
		})
	}
}

func TestInstallPreservesCustomClientCache(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	seed := []byte(`{"mcpServers":{"kernel":{"env":{"MCP_REMOTE_CONFIG_DIR":"/custom/cache"}}}}`)
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetClaude); err != nil {
		t.Fatal(err)
	}
	env := readTestConfig(t, path)["mcpServers"].(map[string]any)["kernel"].(map[string]any)["env"].(map[string]any)
	if env["MCP_REMOTE_CONFIG_DIR"] != "/custom/cache" {
		t.Fatalf("custom auth cache changed: %#v", env)
	}
}

func TestInstallReplacesEmptyClientCache(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	seed := []byte(`{"mcpServers":{"kernel":{"env":{"MCP_REMOTE_CONFIG_DIR":""}}}}`)
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetClaude); err != nil {
		t.Fatal(err)
	}
	env := readTestConfig(t, path)["mcpServers"].(map[string]any)["kernel"].(map[string]any)["env"].(map[string]any)
	want, err := clientCacheDir(TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	if env["MCP_REMOTE_CONFIG_DIR"] != want {
		t.Fatalf("auth cache = %v, want %s", env["MCP_REMOTE_CONFIG_DIR"], want)
	}
}

func TestInstallRejectsExternalClientMetadata(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	seed := []byte(`{"mcpServers":{"kernel":{"args":["-y","mcp-remote","https://old.example","--static-oauth-client-metadata","@/custom/metadata.json"]}}}`)
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetClaude); err == nil {
		t.Fatal("expected external metadata to require manual update")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, seed) {
		t.Fatal("external metadata config changed")
	}
}

func TestInstallRejectsNonObjectEnvironment(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetClaude)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	seed := []byte(`{"mcpServers":{"kernel":{"env":"invalid"}}}`)
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetClaude); err == nil {
		t.Fatal("expected invalid environment to fail")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, seed) {
		t.Fatal("invalid config changed")
	}
}

func TestInstallFxKeepsBearerTokenAuth(t *testing.T) {
	testHome(t)
	path, err := GetConfigPath(TargetFx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	seed := []byte("{\"mcp\":{\"kernel\":{\"type\":\"http\",\"url\":\"https://old.example\",\"oauth\":{},\"bearer_token_env\":\"KERNEL_API_KEY\"}}}")
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetFx); err != nil {
		t.Fatal(err)
	}
	kernel := readTestConfig(t, path)["mcp"].(map[string]any)["kernel"].(map[string]any)
	if kernel["bearer_token_env"] != "KERNEL_API_KEY" || kernel["url"] != KernelMCPURL {
		t.Fatalf("fx auth changed: %#v", kernel)
	}
	if _, exists := kernel["oauth"]; exists {
		t.Fatal("OAuth was retained alongside bearer token auth")
	}
}

func TestInstallRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	testHome(t)
	path, err := GetConfigPath(TargetCursor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other.json")
	seed := []byte("{}")
	if err := os.WriteFile(other, seed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path); err != nil {
		t.Fatal(err)
	}
	if err := Install(TargetCursor); err == nil {
		t.Fatal("expected symlink rejection")
	}
	got, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, seed) {
		t.Fatal("symlink target changed")
	}
}
