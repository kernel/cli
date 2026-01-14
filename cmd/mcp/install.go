package mcp

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Kernel MCP server configuration for an AI tool",
	Long: `Install Kernel MCP server configuration for a supported AI development tool.

This command modifies the configuration file for the specified target to add
the Kernel MCP server, enabling browser automation capabilities in your AI tool.

Supported targets:
  cursor      - Cursor editor
  claude      - Claude Desktop app
  claude-code - Claude Code CLI
  windsurf    - Windsurf editor
  vscode      - Visual Studio Code
  goose       - Goose AI
  zed         - Zed editor

Examples:
  # Install for Cursor
  kernel mcp install --target cursor

  # Install for Claude Desktop
  kernel mcp install --target claude

  # Install for VS Code
  kernel mcp install --target vscode`,
	RunE: runInstall,
}

func init() {
	MCPCmd.AddCommand(installCmd)

	// Build target list for help text
	targets := AllTargets()
	targetStrs := make([]string, len(targets))
	for i, t := range targets {
		targetStrs[i] = string(t)
	}

	installCmd.Flags().String("target", "", fmt.Sprintf("Target AI tool (%s)", strings.Join(targetStrs, ", ")))
	_ = installCmd.MarkFlagRequired("target")
}

func runInstall(cmd *cobra.Command, args []string) error {
	targetStr, _ := cmd.Flags().GetString("target")
	target := Target(strings.ToLower(targetStr))

	// Validate target
	validTarget := false
	for _, t := range AllTargets() {
		if target == t {
			validTarget = true
			break
		}
	}

	if !validTarget {
		targets := AllTargets()
		targetStrs := make([]string, len(targets))
		for i, t := range targets {
			targetStrs[i] = string(t)
		}
		return fmt.Errorf("invalid target '%s'. Supported targets: %s", targetStr, strings.Join(targetStrs, ", "))
	}

	// Get the config path for display
	configPath, err := GetConfigPath(target)
	if err != nil {
		return fmt.Errorf("failed to determine config path: %w", err)
	}

	// Install the MCP configuration
	if err := Install(target); err != nil {
		return fmt.Errorf("failed to install MCP configuration: %w", err)
	}

	// For Goose, the install function already printed instructions
	if target == TargetGoose {
		return nil
	}

	pterm.Success.Printf("MCP server successfully configured for %s at %s\n", target, configPath)

	// Print post-install instructions based on target
	printPostInstallInstructions(target)

	return nil
}

func printPostInstallInstructions(target Target) {
	pterm.Println()

	switch target {
	case TargetCursor:
		pterm.Info.Println("Next steps:")
		pterm.Println("  1. Restart Cursor or reload the window")
		pterm.Println("  2. The Kernel MCP server will appear in your tools")
		pterm.Println("  3. You'll be prompted to authenticate when first using Kernel tools")

	case TargetClaude:
		pterm.Info.Println("Next steps:")
		pterm.Println("  1. Restart Claude Desktop")
		pterm.Println("  2. The Kernel tools will be available in your conversations")
		pterm.Println("  3. You'll be prompted to authenticate when first using Kernel tools")

	case TargetClaudeCode:
		pterm.Info.Println("Next steps:")
		pterm.Println("  1. Run '/mcp' in the Claude Code REPL to authenticate")
		pterm.Println("  2. The Kernel tools will then be available")

	case TargetWindsurf:
		pterm.Info.Println("Next steps:")
		pterm.Println("  1. Open Windsurf settings and navigate to MCP servers")
		pterm.Println("  2. Click 'Refresh' to load the Kernel MCP server")
		pterm.Println("  3. You'll be prompted to authenticate when first using Kernel tools")

	case TargetVSCode:
		pterm.Info.Println("Next steps:")
		pterm.Println("  1. Restart VS Code or reload the window")
		pterm.Println("  2. The Kernel MCP server will be available")
		pterm.Println("  3. You'll be prompted to authenticate when first using Kernel tools")

	case TargetZed:
		pterm.Info.Println("Next steps:")
		pterm.Println("  1. Restart Zed")
		pterm.Println("  2. The Kernel context server will be available")
		pterm.Println("  3. You'll be prompted to authenticate when first using Kernel tools")
	}
}
