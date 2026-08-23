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
