package mcp

import (
	"fmt"

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
	TargetCursor      Target = "cursor"
	TargetClaude      Target = "claude"
	TargetClaudeCode  Target = "claude-code"
	TargetAntigravity Target = "antigravity"
	TargetWindsurf    Target = "windsurf"
	TargetVSCode      Target = "vscode"
	TargetGoose       Target = "goose"
	TargetZed         Target = "zed"
	TargetFx          Target = "fx"
)

// KernelMCPURL is the URL for the Kernel MCP server
const KernelMCPURL = "https://mcp.onkernel.com/mcp"

// MCPServerConfig represents the configuration for an MCP server.
type MCPServerConfig struct {
	URL     string   `json:"url,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Type    string   `json:"type,omitempty"`
}

// installForGoose installs MCP config for Goose (YAML format)
func installForGoose(configPath string, spec targetSpec) error {
	cacheDir, err := clientCacheDir(spec.target)
	if err != nil {
		return err
	}
	// For Goose, we'll output instructions since it uses YAML format
	// and we don't want to add a YAML dependency
	pterm.Info.Println("Goose uses YAML configuration. Add the following to your Goose config:")
	pterm.Println()
	fmt.Println(gooseConfig(spec.clientName, cacheDir))
	pterm.Println()
	pterm.Info.Printf("Config file location: %s\n", configPath)
	return nil
}

func gooseConfig(clientName, cacheDir string) string {
	return `extensions:
  kernel:
    name: Kernel
    type: stdio
    enabled: true
    cmd: npx
    args:
      - -y
      - mcp-remote
      - ` + KernelMCPURL + `
      - --static-oauth-client-metadata
      - '` + clientMetadata(clientName) + `'
    envs:
      MCP_REMOTE_CONFIG_DIR: ` + fmt.Sprintf("%q", cacheDir)
}
