package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallForFx(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	configPath := filepath.Join(home, ".fx", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{
  "mcp": {
    "existing": {
      "type": "http",
      "url": "https://example.com/mcp"
    }
  },
  "setting": "preserved"
}`
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Install(TargetFx); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["setting"] != "preserved" {
		t.Fatalf("setting = %v, want preserved", config["setting"])
	}

	servers, ok := config["mcp"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp = %#v, want object", config["mcp"])
	}
	if _, ok := servers["existing"]; !ok {
		t.Fatal("existing MCP server was removed")
	}

	kernel, ok := servers["kernel"].(map[string]interface{})
	if !ok {
		t.Fatalf("kernel = %#v, want object", servers["kernel"])
	}
	if kernel["type"] != "http" {
		t.Fatalf("type = %v, want http", kernel["type"])
	}
	if kernel["url"] != KernelMCPURL {
		t.Fatalf("url = %v, want %s", kernel["url"], KernelMCPURL)
	}
	if oauth, ok := kernel["oauth"].(map[string]interface{}); !ok || len(oauth) != 0 {
		t.Fatalf("oauth = %#v, want empty object", kernel["oauth"])
	}

	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := fileInfo.Mode().Perm(); got != 0600 {
			t.Fatalf("config permissions = %o, want 600", got)
		}

		dirInfo, err := os.Stat(filepath.Dir(configPath))
		if err != nil {
			t.Fatal(err)
		}
		if got := dirInfo.Mode().Perm(); got != 0755 {
			t.Fatalf("config directory permissions = %o, want preserved 755", got)
		}
	}
}

func TestInstallForFxClean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := Install(TargetFx); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		return
	}

	configPath := filepath.Join(home, ".fx", "mcp.json")
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(configPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("config directory permissions = %o, want 700", got)
	}
}

func assertMCPRemoteShape(t *testing.T, kernel map[string]interface{}) {
	t.Helper()

	if kernel["command"] != "npx" {
		t.Fatalf("command = %v, want npx", kernel["command"])
	}
	args, ok := kernel["args"].([]interface{})
	if !ok {
		t.Fatalf("args = %#v, want array", kernel["args"])
	}
	want := []string{"-y", "mcp-remote", KernelMCPURL, "--static-oauth-client-metadata", `{"client_name":"Antigravity"}`}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i, w := range want {
		if args[i] != w {
			t.Fatalf("args[%d] = %v, want %s", i, args[i], w)
		}
	}
	// A leftover remote-transport key would misrepresent how the server is reached
	for _, key := range []string{"serverUrl", "url", "httpUrl"} {
		if _, ok := kernel[key]; ok {
			t.Fatalf("kernel = %#v, want %s removed", kernel, key)
		}
	}
}

func TestInstallForAntigravity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	configPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{
  "mcpServers": {
    "existing": {
      "serverUrl": "https://example.com/mcp"
    }
  },
  "setting": "preserved"
}`
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Install(TargetAntigravity); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["setting"] != "preserved" {
		t.Fatalf("setting = %v, want preserved", config["setting"])
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers = %#v, want object", config["mcpServers"])
	}
	if _, ok := servers["existing"]; !ok {
		t.Fatal("existing MCP server was removed")
	}

	kernel, ok := servers["kernel"].(map[string]interface{})
	if !ok {
		t.Fatalf("kernel = %#v, want object", servers["kernel"])
	}
	assertMCPRemoteShape(t, kernel)
}

func TestInstallForAntigravityClean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := Install(TargetAntigravity); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".gemini", "config", "mcp_config.json"))
	if err != nil {
		t.Fatal(err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers = %#v, want object", config["mcpServers"])
	}
	kernel, ok := servers["kernel"].(map[string]interface{})
	if !ok {
		t.Fatalf("kernel = %#v, want object", servers["kernel"])
	}
	assertMCPRemoteShape(t, kernel)
}

func TestInstallForAntigravityPreservesExistingKernelFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	configPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}

	// headers carries the documented API-key workaround, and url is a stale key
	// from an earlier install that Antigravity ignores.
	existing := `{
  "mcpServers": {
    "kernel": {
      "serverUrl": "https://mcp.onkernel.com/stale",
      "url": "https://mcp.onkernel.com/stale",
      "httpUrl": "https://mcp.onkernel.com/stale",
      "headers": {
        "Authorization": "Bearer sk-test"
      }
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Install(TargetAntigravity); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers = %#v, want object", config["mcpServers"])
	}
	kernel, ok := servers["kernel"].(map[string]interface{})
	if !ok {
		t.Fatalf("kernel = %#v, want object", servers["kernel"])
	}

	headers, ok := kernel["headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("headers = %#v, want the existing block preserved", kernel["headers"])
	}
	if headers["Authorization"] != "Bearer sk-test" {
		t.Fatalf("Authorization = %v, want the existing value", headers["Authorization"])
	}

	assertMCPRemoteShape(t, kernel)

	// The preserved headers block can hold an API key, so the file must not stay
	// world-readable after a reinstall.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("config permissions = %o, want 600", got)
		}
	}
}
