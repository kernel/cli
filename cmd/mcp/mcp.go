package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// MCPCmd is the parent command for MCP operations
var MCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Configure Kernel MCP server for AI tools",
	Long:  "Commands for configuring the Kernel MCP server in AI development tools like Cursor, Claude, VS Code, and more.",
	Run: func(cmd *cobra.Command, args []string) {
		// If called without subcommands, show help
		_ = cmd.Help()
	},
}

// Target represents a supported MCP client target
type Target string

const (
	TargetCursor     Target = "cursor"
	TargetClaude     Target = "claude"
	TargetClaudeCode Target = "claude-code"
	TargetWindsurf   Target = "windsurf"
	TargetVSCode     Target = "vscode"
	TargetGoose      Target = "goose"
	TargetZed        Target = "zed"
)

// KernelMCPURL is the URL for the Kernel MCP server
const KernelMCPURL = "https://mcp.onkernel.com/mcp"

// AllTargets returns all supported targets
func AllTargets() []Target {
	return []Target{
		TargetCursor,
		TargetClaude,
		TargetClaudeCode,
		TargetWindsurf,
		TargetVSCode,
		TargetGoose,
		TargetZed,
	}
}

// getHomeDir returns the user's home directory
func getHomeDir() (string, error) {
	return os.UserHomeDir()
}

// getConfigPath returns the config file path for a given target
func getConfigPath(target Target) (string, error) {
	homeDir, err := getHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	switch target {
	case TargetCursor:
		return filepath.Join(homeDir, ".cursor", "mcp.json"), nil
	case TargetClaude:
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
		case "windows":
			appData := os.Getenv("APPDATA")
			if appData == "" {
				appData = filepath.Join(homeDir, "AppData", "Roaming")
			}
			return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
		default:
			// Linux - Claude Desktop doesn't officially support Linux, but use XDG config
			return filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json"), nil
		}
	case TargetClaudeCode:
		// Claude Code uses the ~/.claude.json file
		return filepath.Join(homeDir, ".claude.json"), nil
	case TargetWindsurf:
		return filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"), nil
	case TargetVSCode:
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "settings.json"), nil
		case "windows":
			appData := os.Getenv("APPDATA")
			if appData == "" {
				appData = filepath.Join(homeDir, "AppData", "Roaming")
			}
			return filepath.Join(appData, "Code", "User", "settings.json"), nil
		default:
			return filepath.Join(homeDir, ".config", "Code", "User", "settings.json"), nil
		}
	case TargetGoose:
		return filepath.Join(homeDir, ".config", "goose", "config.yaml"), nil
	case TargetZed:
		return filepath.Join(homeDir, ".config", "zed", "settings.json"), nil
	default:
		return "", fmt.Errorf("unsupported target: %s", target)
	}
}

// MCPServerConfig represents the configuration for an MCP server
type MCPServerConfig struct {
	URL     string   `json:"url,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Type    string   `json:"type,omitempty"`
}

// stripJSONComments removes single-line (//) and multi-line (/* */) comments from JSON
// It properly handles strings to avoid removing // or /* */ that appear inside string literals
func stripJSONComments(data []byte) []byte {
	content := string(data)
	var result strings.Builder
	i := 0
	inString := false
	inMultiLineComment := false
	escapeNext := false

	for i < len(content) {
		char := content[i]

		if escapeNext {
			result.WriteByte(char)
			escapeNext = false
			i++
			continue
		}

		// Check multi-line comment first - skip all content including quotes
		if inMultiLineComment {
			if i+1 < len(content) && char == '*' && content[i+1] == '/' {
				inMultiLineComment = false
				i += 2
				continue
			}
			i++
			continue
		}

		if char == '\\' && inString {
			escapeNext = true
			result.WriteByte(char)
			i++
			continue
		}

		if char == '"' {
			inString = !inString
			result.WriteByte(char)
			i++
			continue
		}

		if inString {
			result.WriteByte(char)
			i++
			continue
		}

		if i+1 < len(content) && char == '/' && content[i+1] == '/' {
			// Single-line comment - skip to end of line
			for i < len(content) && content[i] != '\n' {
				i++
			}
			if i < len(content) {
				result.WriteByte('\n')
				i++
			}
			continue
		}

		if i+1 < len(content) && char == '/' && content[i+1] == '*' {
			inMultiLineComment = true
			i += 2
			continue
		}

		result.WriteByte(char)
		i++
	}

	return []byte(result.String())
}

// readJSONFile reads and parses a JSON config file
func readJSONFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}

	// Handle empty files
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}

	// Strip comments to support JSON5 format (used by Zed)
	data = stripJSONComments(data)

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return config, nil
}

// writeJSONFile writes a config map to a JSON file with proper formatting
func writeJSONFile(path string, config map[string]interface{}) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// installForCursor installs MCP config for Cursor
func installForCursor(configPath string) error {
	config, err := readJSONFile(configPath)
	if err != nil {
		return err
	}

	// Get or create mcpServers section
	mcpServers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		mcpServers = make(map[string]interface{})
	}

	// Add kernel server config
	mcpServers["kernel"] = map[string]interface{}{
		"url": KernelMCPURL,
	}
	config["mcpServers"] = mcpServers

	return writeJSONFile(configPath, config)
}

// installForClaude installs MCP config for Claude Desktop
func installForClaude(configPath string) error {
	config, err := readJSONFile(configPath)
	if err != nil {
		return err
	}

	// Get or create mcpServers section
	mcpServers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		mcpServers = make(map[string]interface{})
	}

	// Claude Desktop uses stdio transport via mcp-remote
	mcpServers["kernel"] = map[string]interface{}{
		"command": "npx",
		"args":    []string{"-y", "mcp-remote", KernelMCPURL},
	}
	config["mcpServers"] = mcpServers

	return writeJSONFile(configPath, config)
}

// installForClaudeCode installs MCP config for Claude Code CLI
func installForClaudeCode(configPath string) error {
	config, err := readJSONFile(configPath)
	if err != nil {
		return err
	}

	// Get or create mcpServers section
	mcpServers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		mcpServers = make(map[string]interface{})
	}

	// Claude Code uses HTTP transport
	mcpServers["kernel"] = map[string]interface{}{
		"type": "http",
		"url":  KernelMCPURL,
	}
	config["mcpServers"] = mcpServers

	return writeJSONFile(configPath, config)
}

// installForWindsurf installs MCP config for Windsurf
func installForWindsurf(configPath string) error {
	config, err := readJSONFile(configPath)
	if err != nil {
		return err
	}

	// Get or create mcpServers section
	mcpServers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		mcpServers = make(map[string]interface{})
	}

	// Windsurf uses stdio transport via mcp-remote
	mcpServers["kernel"] = map[string]interface{}{
		"command": "npx",
		"args":    []string{"-y", "mcp-remote", KernelMCPURL},
	}
	config["mcpServers"] = mcpServers

	return writeJSONFile(configPath, config)
}

// installForVSCode installs MCP config for VS Code
func installForVSCode(configPath string) error {
	config, err := readJSONFile(configPath)
	if err != nil {
		return err
	}

	// Get or create mcp.servers section (VS Code uses dot notation in settings)
	mcpServers, ok := config["mcp.servers"].(map[string]interface{})
	if !ok {
		mcpServers = make(map[string]interface{})
	}

	// VS Code uses HTTP transport
	mcpServers["kernel"] = map[string]interface{}{
		"url":  KernelMCPURL,
		"type": "http",
	}
	config["mcp.servers"] = mcpServers

	return writeJSONFile(configPath, config)
}

// installForGoose installs MCP config for Goose (YAML format)
func installForGoose(configPath string) error {
	// For Goose, we'll output instructions since it uses YAML format
	// and we don't want to add a YAML dependency
	pterm.Info.Println("Goose uses YAML configuration. Add the following to your Goose config:")
	pterm.Println()
	fmt.Println(`extensions:
  kernel:
    name: Kernel
    type: stdio
    cmd: npx
    args:
      - -y
      - mcp-remote
      - ` + KernelMCPURL)
	pterm.Println()
	pterm.Info.Printf("Config file location: %s\n", configPath)
	return nil
}

// installForZed installs MCP config for Zed
func installForZed(configPath string) error {
	config, err := readJSONFile(configPath)
	if err != nil {
		return err
	}

	// Get or create context_servers section
	contextServers, ok := config["context_servers"].(map[string]interface{})
	if !ok {
		contextServers = make(map[string]interface{})
	}

	// Zed uses context_servers with custom source
	contextServers["kernel"] = map[string]interface{}{
		"source":  "custom",
		"command": "npx",
		"args":    []string{"-y", "mcp-remote", KernelMCPURL},
	}
	config["context_servers"] = contextServers

	return writeJSONFile(configPath, config)
}

// Install configures the MCP server for the specified target
func Install(target Target) error {
	configPath, err := getConfigPath(target)
	if err != nil {
		return err
	}

	switch target {
	case TargetCursor:
		return installForCursor(configPath)
	case TargetClaude:
		return installForClaude(configPath)
	case TargetClaudeCode:
		return installForClaudeCode(configPath)
	case TargetWindsurf:
		return installForWindsurf(configPath)
	case TargetVSCode:
		return installForVSCode(configPath)
	case TargetGoose:
		return installForGoose(configPath)
	case TargetZed:
		return installForZed(configPath)
	default:
		return fmt.Errorf("unsupported target: %s", target)
	}
}

// GetConfigPath returns the config path for a target (exported for display)
func GetConfigPath(target Target) (string, error) {
	return getConfigPath(target)
}
