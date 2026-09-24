package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

type transport string

const (
	stdio transport = "stdio"
	http  transport = "http"
)

type configField struct {
	name      string
	value     any
	ifMissing bool
	skipWhen  string
}

type targetSpec struct {
	target       Target
	description  string
	path         func(string) string
	section      string
	transport    transport
	fields       []configField
	clientName   string
	callbackPort int
	remove       []string
	legacyName   string
	legacyKey    string
	printOnly    bool
}

func homePath(parts ...string) func(string) string {
	return func(home string) string {
		return filepath.Join(append([]string{home}, parts...)...)
	}
}

func claudePath(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		return filepath.Join(appDataPath(home), "Claude", "claude_desktop_config.json")
	default:
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}
}

func vsCodePath(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json")
	case "windows":
		return filepath.Join(appDataPath(home), "Code", "User", "mcp.json")
	default:
		return filepath.Join(home, ".config", "Code", "User", "mcp.json")
	}
}

func appDataPath(home string) string {
	if path := os.Getenv("APPDATA"); path != "" {
		return path
	}
	return filepath.Join(home, "AppData", "Roaming")
}

var targetSpecs = []targetSpec{
	{target: TargetCursor, description: "Cursor editor", path: homePath(".cursor", "mcp.json"), section: "mcpServers", transport: http,
		fields: []configField{{name: "url", value: KernelMCPURL}}, remove: []string{"type"}},
	{target: TargetClaude, description: "Claude Desktop app", path: claudePath, section: "mcpServers", transport: stdio, clientName: "Claude Desktop", callbackPort: 46093},
	{target: TargetClaudeCode, description: "Claude Code CLI", path: homePath(".claude.json"), section: "mcpServers", transport: http,
		fields: []configField{{name: "type", value: "http"}, {name: "url", value: KernelMCPURL}}},
	{target: TargetAntigravity, description: "Google Antigravity", path: homePath(".gemini", "config", "mcp_config.json"), section: "mcpServers", transport: stdio, clientName: "Antigravity", callbackPort: 46094},
	{target: TargetWindsurf, description: "Windsurf editor", path: homePath(".codeium", "windsurf", "mcp_config.json"), section: "mcpServers", transport: stdio, clientName: "Windsurf", callbackPort: 46095},
	{target: TargetVSCode, description: "Visual Studio Code", path: vsCodePath, section: "servers", transport: http,
		legacyName: "settings.json", legacyKey: "mcp.servers",
		fields: []configField{{name: "url", value: KernelMCPURL}, {name: "type", value: "http"}}},
	{target: TargetGoose, description: "Goose AI", path: homePath(".config", "goose", "config.yaml"), transport: stdio, clientName: "Goose", callbackPort: 46096, printOnly: true},
	// Current Zed settings omit source; its settings migrator removes that old field.
	{target: TargetZed, description: "Zed editor", path: homePath(".config", "zed", "settings.json"), section: "context_servers", transport: stdio, clientName: "Zed", callbackPort: 46097,
		remove: []string{"source"}},
	{target: TargetFx, description: "fx coding agent", path: homePath(".fx", "mcp.json"), section: "mcp", transport: http,
		fields: []configField{{name: "type", value: "http"}, {name: "url", value: KernelMCPURL}, {name: "oauth", value: map[string]any{}, ifMissing: true, skipWhen: "bearer_token_env"}}},
}

func specFor(target Target) (targetSpec, bool) {
	for _, spec := range targetSpecs {
		if spec.target == target {
			return spec, true
		}
	}
	return targetSpec{}, false
}

func AllTargets() []Target {
	targets := make([]Target, 0, len(targetSpecs))
	for _, spec := range targetSpecs {
		targets = append(targets, spec.target)
	}
	return targets
}

func getConfigPath(target Target) (string, error) {
	spec, ok := specFor(target)
	if !ok {
		return "", fmt.Errorf("unsupported target: %s", target)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return spec.path(home), nil
}

func clientCacheDir(target Target) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".mcp-auth", "kernel-"+string(target)), nil
}

func GetConfigPath(target Target) (string, error) {
	return getConfigPath(target)
}

func Install(target Target) error {
	spec, ok := specFor(target)
	if !ok {
		return fmt.Errorf("unsupported target: %s", target)
	}
	path, err := getConfigPath(target)
	if err != nil {
		return err
	}
	if spec.printOnly {
		return installForGoose(path, spec)
	}
	return installConfig(path, spec)
}

func installConfig(path string, spec targetSpec) error {
	data, mode, err := readConfig(path)
	if err != nil {
		return err
	}
	var legacy map[string]json.RawMessage
	var legacyPath string
	var legacyMode os.FileMode
	if spec.legacyName != "" {
		legacyPath = filepath.Join(filepath.Dir(path), spec.legacyName)
		legacy, legacyMode, err = readLegacyKernel(legacyPath, spec.legacyKey)
		if err != nil {
			return err
		}
	}
	updated, err := mergeConfig(data, spec, legacy)
	if err != nil {
		return err
	}
	privateMode := mode & 0600
	if !bytes.Equal(data, updated) || mode != privateMode {
		if err := writeConfigAtomic(path, updated, privateMode); err != nil {
			return err
		}
	}
	if legacy != nil && legacyMode != legacyMode&0600 {
		if err := os.Chmod(legacyPath, legacyMode&0600); err != nil {
			return fmt.Errorf("failed to secure legacy config: %w", err)
		}
	}
	return nil
}

func readLegacyKernel(path, sectionKey string) (map[string]json.RawMessage, os.FileMode, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("legacy config is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, 0, fmt.Errorf("legacy config is empty: %s", path)
	}
	root, err := hujson.Parse(data)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse legacy config: %w", err)
	}
	if err := validateConfigKeys(&root, ""); err != nil {
		return nil, 0, err
	}
	obj, err := objectAt(&root, "")
	if err != nil {
		return nil, 0, err
	}
	if _, exists := member(obj, sectionKey); !exists {
		return nil, 0, nil
	}
	section, err := objectAt(&root, "/"+sectionKey)
	if err != nil {
		return nil, 0, err
	}
	if _, exists := member(section, "kernel"); !exists {
		return nil, 0, nil
	}
	kernel, err := objectAt(&root, "/"+sectionKey+"/kernel")
	if err != nil {
		return nil, 0, err
	}
	fields := make(map[string]json.RawMessage, len(kernel.Members))
	for _, item := range kernel.Members {
		name := item.Name.Value.(hujson.Literal).String()
		value := item.Value.Clone()
		value.Standardize()
		fields[name] = value.Pack()
	}
	return fields, info.Mode().Perm(), nil
}

func readConfig(path string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return []byte("{}\n"), 0600, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("config is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, 0, fmt.Errorf("config is empty: %s", path)
	}
	return data, info.Mode().Perm(), nil
}

// Patch the syntax tree so comments and formatting outside edited fields survive.
// Comments inside replaced values or on removed fields may be lost.
func mergeConfig(data []byte, spec targetSpec, legacy map[string]json.RawMessage) ([]byte, error) {
	root, err := hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if err := validateConfigKeys(&root, ""); err != nil {
		return nil, err
	}
	section, err := objectAt(&root, "")
	if err != nil {
		return nil, err
	}
	sectionPath := "/" + spec.section
	if _, ok := member(section, spec.section); !ok {
		if err := patch(&root, "add", sectionPath, map[string]any{}); err != nil {
			return nil, err
		}
	}
	section, err = objectAt(&root, sectionPath)
	if err != nil {
		return nil, err
	}
	if _, ok := member(section, "kernel"); !ok {
		if err := patch(&root, "add", sectionPath+"/kernel", map[string]any{}); err != nil {
			return nil, err
		}
	}
	kernelPath := sectionPath + "/kernel"
	kernel, err := objectAt(&root, kernelPath)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(legacy))
	for name := range legacy {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, exists := member(kernel, name); !exists {
			if err := patch(&root, "add", kernelPath+"/"+pointerName(name), legacy[name]); err != nil {
				return nil, err
			}
		}
	}
	kernel, err = objectAt(&root, kernelPath)
	if err != nil {
		return nil, err
	}

	remove := []string{"serverUrl", "httpUrl"}
	if spec.transport == stdio {
		remove = append(remove, "url", "type")
	} else {
		remove = append(remove, "command", "args", "source")
	}
	remove = append(remove, spec.remove...)
	for _, name := range remove {
		if _, ok := member(kernel, name); ok {
			if err := patch(&root, "remove", kernelPath+"/"+name, nil); err != nil {
				return nil, err
			}
		}
	}
	fields := spec.fields
	if spec.transport == stdio {
		args, err := mergeStdioArgs(kernel, spec)
		if err != nil {
			return nil, err
		}
		fields = append([]configField{{name: "command", value: "npx"}, {name: "args", value: args}}, fields...)
	}
	for _, field := range fields {
		kernel, err = objectAt(&root, kernelPath)
		if err != nil {
			return nil, err
		}
		current, exists := member(kernel, field.name)
		if field.skipWhen != "" {
			if _, found := member(kernel, field.skipWhen); found {
				if exists {
					if err := patch(&root, "remove", kernelPath+"/"+field.name, nil); err != nil {
						return nil, err
					}
				}
				continue
			}
		}
		if field.ifMissing && exists {
			continue
		}
		wanted, err := json.Marshal(field.value)
		if err != nil {
			return nil, err
		}
		if exists {
			normalized := current.Clone()
			normalized.Standardize()
			var compact bytes.Buffer
			if err := json.Compact(&compact, normalized.Pack()); err != nil {
				return nil, err
			}
			if bytes.Equal(compact.Bytes(), wanted) {
				continue
			}
		}
		if err := patch(&root, "add", kernelPath+"/"+field.name, field.value); err != nil {
			return nil, err
		}
	}
	if spec.transport == stdio {
		if err := addClientCacheDir(&root, kernelPath, spec.target); err != nil {
			return nil, err
		}
	}
	return root.Pack(), nil
}

func addClientCacheDir(root *hujson.Value, kernelPath string, target Target) error {
	kernel, err := objectAt(root, kernelPath)
	if err != nil {
		return err
	}
	if _, exists := member(kernel, "env"); !exists {
		if err := patch(root, "add", kernelPath+"/env", map[string]any{}); err != nil {
			return err
		}
	}
	env, err := objectAt(root, kernelPath+"/env")
	if err != nil {
		return err
	}
	current, exists := member(env, "MCP_REMOTE_CONFIG_DIR")
	if exists {
		normalized := current.Clone()
		normalized.Standardize()
		var path string
		if err := json.Unmarshal(normalized.Pack(), &path); err != nil {
			return fmt.Errorf("invalid MCP_REMOTE_CONFIG_DIR: %w", err)
		}
		if path != "" {
			return nil
		}
	}
	cacheDir, err := clientCacheDir(target)
	if err != nil {
		return err
	}
	return patch(root, "add", kernelPath+"/env/MCP_REMOTE_CONFIG_DIR", cacheDir)
}

func stdioArgs(spec targetSpec) []string {
	return []string{"-y", "mcp-remote", KernelMCPURL, strconv.Itoa(spec.callbackPort), "--static-oauth-client-metadata", clientMetadata(spec.clientName)}
}

func clientMetadata(clientName string) string {
	return fmt.Sprintf(`{"client_name":%q}`, clientName)
}

func mergeStdioArgs(kernel *hujson.Object, spec targetSpec) ([]string, error) {
	defaults := stdioArgs(spec)
	value, exists := member(kernel, "args")
	if !exists {
		return defaults, nil
	}
	normalized := value.Clone()
	normalized.Standardize()
	var args []string
	if err := json.Unmarshal(normalized.Pack(), &args); err != nil {
		return nil, fmt.Errorf("invalid kernel args: %w", err)
	}
	if len(args) < 3 || args[0] != "-y" || (args[1] != "mcp-remote" && !strings.HasPrefix(args[1], "mcp-remote@")) {
		return defaults, nil
	}
	args[2] = KernelMCPURL
	if len(args) == 3 {
		args = append(args, defaults[3])
	} else if _, err := strconv.Atoi(args[3]); err != nil {
		args = slices.Insert(args, 3, defaults[3])
	}
	flagIndex := -1
	for i := 4; i < len(args); i++ {
		if args[i] != defaults[4] {
			continue
		}
		if flagIndex != -1 || i+1 >= len(args) {
			return nil, fmt.Errorf("invalid kernel args for %s", defaults[4])
		}
		flagIndex = i
	}
	if flagIndex == -1 {
		return append(args, defaults[4:]...), nil
	}
	metadata, err := mergeClientMetadata(args[flagIndex+1], spec.clientName)
	if err != nil {
		return nil, err
	}
	args[flagIndex+1] = metadata
	return args, nil
}

func mergeClientMetadata(raw, clientName string) (string, error) {
	if strings.HasPrefix(raw, "@") {
		return "", fmt.Errorf("kernel OAuth metadata is in a separate file; set client_name to %q there", clientName)
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("invalid kernel OAuth metadata: expected JSON object")
	}
	metadata, err := hujson.Parse([]byte(raw))
	if err != nil {
		return "", fmt.Errorf("invalid kernel OAuth metadata: %w", err)
	}
	if err := validateConfigKeys(&metadata, ""); err != nil {
		return "", err
	}
	obj, err := objectAt(&metadata, "")
	if err != nil {
		return "", err
	}
	if current, exists := member(obj, "client_name"); exists {
		var name string
		if err := json.Unmarshal(current.Pack(), &name); err == nil && name == clientName {
			return raw, nil
		}
	}
	if err := patch(&metadata, "add", "/client_name", clientName); err != nil {
		return "", err
	}
	return string(metadata.Pack()), nil
}

func pointerName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
}

func objectAt(root *hujson.Value, path string) (*hujson.Object, error) {
	value := root
	if path != "" {
		value = root.Find(path)
	}
	if value == nil {
		return nil, fmt.Errorf("missing config object at %s", path)
	}
	obj, ok := value.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("expected config object at %s", path)
	}
	return obj, nil
}

func validateConfigKeys(value *hujson.Value, path string) error {
	switch node := value.Value.(type) {
	case *hujson.Object:
		seen := make(map[string]bool, len(node.Members))
		for i := range node.Members {
			item := &node.Members[i]
			name := item.Name.Value.(hujson.Literal).String()
			if seen[name] {
				return fmt.Errorf("duplicate config key %q at %s", name, path)
			}
			seen[name] = true
			if err := validateConfigKeys(&item.Value, path+"/"+pointerName(name)); err != nil {
				return err
			}
		}
	case *hujson.Array:
		for i := range node.Elements {
			if err := validateConfigKeys(&node.Elements[i], path+"/"+strconv.Itoa(i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func member(obj *hujson.Object, name string) (*hujson.Value, bool) {
	for i := range obj.Members {
		if obj.Members[i].Name.Value.(hujson.Literal).String() == name {
			return &obj.Members[i].Value, true
		}
	}
	return nil, false
}

func patch(root *hujson.Value, operation, path string, value any) error {
	item := map[string]any{"op": operation, "path": path}
	if operation == "add" {
		item["value"] = value
	}
	data, err := json.Marshal([]any{item})
	if err != nil {
		return err
	}
	return root.Patch(data)
}

func writeConfigAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".kernel-mcp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary config: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("failed to write temporary config: %w", err)
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return fmt.Errorf("failed to secure temporary config: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("failed to sync temporary config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close temporary config: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("failed to replace config: %w", err)
	}
	return nil
}
