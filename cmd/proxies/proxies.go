package proxies

import (
	"github.com/spf13/cobra"
)

// ProxiesCmd is the parent command for proxy operations
var ProxiesCmd = &cobra.Command{
	Use:     "proxies",
	Aliases: []string{"proxy"},
	Short:   "Manage proxy configurations",
	Long:    "Commands for managing proxy configurations for browser sessions",
	Run: func(cmd *cobra.Command, args []string) {
		// If called without subcommands, show help
		_ = cmd.Help()
	},
}

var proxiesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List proxy configurations",
	RunE:  runProxiesList,
}

var proxiesGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get proxy configuration by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runProxiesGet,
}

var proxiesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new proxy configuration",
	Long: `Create a new proxy configuration for browser sessions.

Proxy types (from best to worst for bot detection):
- mobile: Mobile carrier proxies
- residential: Residential IP proxies  
- isp: ISP proxies
- datacenter: Datacenter proxies
- custom: Your own proxy server

Examples:
  # Create a datacenter proxy
  kernel beta proxies create --type datacenter --country US --name "US Datacenter"

  # Create a custom proxy
  kernel beta proxies create --type custom --host proxy.example.com --port 8080 --username myuser --password mypass

  # Create a residential proxy with location
  kernel beta proxies create --type residential --country US --city sanfrancisco --state CA`,
	RunE: runProxiesCreate,
}

var proxiesDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a proxy configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runProxiesDelete,
}

func init() {
	// Add subcommands
	ProxiesCmd.AddCommand(proxiesListCmd)
	ProxiesCmd.AddCommand(proxiesGetCmd)
	ProxiesCmd.AddCommand(proxiesCreateCmd)
	ProxiesCmd.AddCommand(proxiesDeleteCmd)

	// Add output flags
	proxiesListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	proxiesGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	proxiesCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Add flags for create command
	proxiesCreateCmd.Flags().String("name", "", "Proxy configuration name")
	proxiesCreateCmd.Flags().String("type", "", "Proxy type (datacenter|isp|residential|mobile|custom)")
	_ = proxiesCreateCmd.MarkFlagRequired("type")
	proxiesCreateCmd.Flags().String("protocol", "https", "Protocol to use for the proxy connection (http|https)")

	// Location flags (datacenter, isp, residential, mobile)
	proxiesCreateCmd.Flags().String("country", "", "ISO 3166 country code or EU")
	proxiesCreateCmd.Flags().String("city", "", "City name (no spaces, e.g. sanfrancisco)")
	proxiesCreateCmd.Flags().String("state", "", "Two-letter state code")
	proxiesCreateCmd.Flags().String("zip", "", "US ZIP code")
	proxiesCreateCmd.Flags().String("asn", "", "Autonomous system number (e.g. AS15169)")

	// OS flag (residential)
	proxiesCreateCmd.Flags().String("os", "", "Operating system (windows|macos|android)")

	// Carrier flag (mobile)
	proxiesCreateCmd.Flags().String("carrier", "", "Mobile carrier (see help for full list)")

	// Custom proxy flags
	proxiesCreateCmd.Flags().String("host", "", "Proxy host address or IP")
	proxiesCreateCmd.Flags().Int("port", 0, "Proxy port")
	proxiesCreateCmd.Flags().String("username", "", "Username for proxy authentication")
	proxiesCreateCmd.Flags().String("password", "", "Password for proxy authentication")

	// Delete flags
	proxiesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}
