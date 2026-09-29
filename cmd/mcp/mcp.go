package mcp

import (
	"fmt"
	"os"
	"os/exec"
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
	TargetCursor      Target = "cursor"
	TargetClaude      Target = "claude"
	TargetClaudeCode  Target = "claude-code"
	TargetAntigravity Target = "antigravity"
	TargetWindsurf    Target = "windsurf"
	TargetVSCode      Target = "vscode"
	TargetGoose       Target = "goose"
	TargetZed         Target = "zed"
	TargetFx          Target = "fx"
	TargetCodex       Target = "codex"
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
	fmt.Println(gooseConfig(spec, cacheDir))
	pterm.Println()
	pterm.Info.Printf("Config file location: %s\n", configPath)
	return nil
}

func gooseConfig(spec targetSpec, cacheDir string) string {
	var config strings.Builder
	config.WriteString(`extensions:
  kernel:
    name: Kernel
    type: stdio
    enabled: true
    cmd: npx
    args:
`)
	for _, arg := range stdioArgs(spec) {
		fmt.Fprintf(&config, "      - %q\n", arg)
	}
	fmt.Fprintf(&config, "    envs:\n      MCP_REMOTE_CONFIG_DIR: %q", cacheDir)
	return config.String()
}

// installForCodex delegates to `codex mcp add`, which edits config.toml in
// place and starts the OAuth flow. Without the Codex CLI on PATH (e.g. IDE
// extension only), print the TOML to add by hand.
func installForCodex(configPath string, spec targetSpec) error {
	codex, err := exec.LookPath("codex")
	if err != nil {
		pterm.Info.Println("Codex CLI not found on PATH. Add the following to your Codex config:")
		pterm.Println()
		fmt.Println(codexConfig())
		pterm.Println()
		pterm.Info.Printf("Config file location: %s\n", configPath)
		pterm.Info.Println("Then run 'codex mcp login kernel' to authenticate")
		return nil
	}
	cmd := exec.Command(codex, "mcp", "add", "kernel", "--url", KernelMCPURL)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex mcp add failed: %w", err)
	}
	pterm.Success.Printf("MCP server successfully configured for %s at %s\n", spec.target, configPath)
	pterm.Println()
	pterm.Info.Println("Next steps:")
	pterm.Println("  1. Restart Codex")
	pterm.Println("  2. Run '/mcp' in Codex to verify that Kernel is connected")
	pterm.Println("  3. If Kernel isn't authenticated, run 'codex mcp login kernel'")
	return nil
}

func codexConfig() string {
	return fmt.Sprintf("[mcp_servers.kernel]\nurl = %q", KernelMCPURL)
}
