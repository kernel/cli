package mcp

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Display information about the Kernel MCP server",
	Long: `Display information about the Kernel MCP server.

The Kernel MCP server is hosted remotely and does not need to be started locally.
This command provides connection details and documentation links.

For local development or debugging, you can connect to the MCP server at:
  ` + KernelMCPURL + `

The server supports both HTTP transport (recommended) and stdio via mcp-remote.`,
	Run: runServer,
}

func init() {
	MCPCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) {
	pterm.DefaultHeader.Println("Kernel MCP Server")
	pterm.Println()

	pterm.Info.Println("The Kernel MCP server is hosted remotely and does not need to be started locally.")
	pterm.Println()

	pterm.DefaultSection.Println("Connection Details")

	rows := pterm.TableData{
		{"Transport", "URL / Command"},
		{"HTTP (recommended)", KernelMCPURL},
		{"stdio (via mcp-remote)", "npx -y mcp-remote " + KernelMCPURL},
	}
	_ = pterm.DefaultTable.WithHasHeader().WithData(rows).Render()

	pterm.Println()
	pterm.DefaultSection.Println("Quick Install")
	pterm.Println("  kernel mcp install --target cursor")
	pterm.Println("  kernel mcp install --target claude")
	pterm.Println("  kernel mcp install --target vscode")

	pterm.Println()
	pterm.DefaultSection.Println("Documentation")
	pterm.Println("  https://www.kernel.sh/docs/reference/mcp-server")

	pterm.Println()
	pterm.Info.Println("Use 'kernel mcp install --target <tool>' to configure your AI tool automatically.")
}
