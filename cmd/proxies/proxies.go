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
  kernel proxies create --type datacenter --country US --name "US Datacenter"

  # Create a custom proxy
  kernel proxies create --type custom --host proxy.example.com --port 8080 --username myuser --password mypass

  # Create a residential proxy with location
  kernel proxies create --type residential --country US --city sanfrancisco --state CA

  # Create a proxy with bypass hosts
  kernel proxies create --type datacenter --country US --bypass-host localhost,internal.service.local`,
	RunE: runProxiesCreate,
}

var proxiesDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a proxy configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runProxiesDelete,
}

var proxiesCheckCmd = &cobra.Command{
	Use:   "check <id>",
	Short: "Run a health check on a proxy",
	Long:  "Run a health check on a proxy to verify it's working and update its status.",
	Args:  cobra.ExactArgs(1),
	RunE:  runProxiesCheck,
}

func init() {
	// Add subcommands
	ProxiesCmd.AddCommand(proxiesListCmd)
	ProxiesCmd.AddCommand(proxiesGetCmd)
	ProxiesCmd.AddCommand(proxiesCreateCmd)
	ProxiesCmd.AddCommand(proxiesDeleteCmd)
	ProxiesCmd.AddCommand(proxiesCheckCmd)

	// Add output flags
	addJSONOutputFlag(proxiesListCmd)
	proxiesListCmd.Flags().Int("limit", 0, "Maximum number of proxies to return")
	proxiesListCmd.Flags().Int("offset", 0, "Number of proxies to skip (for pagination)")
	addJSONOutputFlag(proxiesGetCmd)
	addJSONOutputFlag(proxiesCreateCmd)

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

	// Custom proxy flags
	proxiesCreateCmd.Flags().String("host", "", "Proxy host address or IP")
	proxiesCreateCmd.Flags().Int("port", 0, "Proxy port")
	proxiesCreateCmd.Flags().String("username", "", "Username for proxy authentication")
	proxiesCreateCmd.Flags().String("password", "", "Password for proxy authentication")
	proxiesCreateCmd.Flags().StringSlice("bypass-host", nil, "Hostname(s) to bypass proxy and connect directly (repeat or comma-separated)")

	// Delete flags
	proxiesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Check flags
	addJSONOutputFlag(proxiesCheckCmd)
}
