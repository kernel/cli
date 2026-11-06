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
- isp: ISP proxies (supported countries: US, GB, FR, DE, SG, KR)
- custom: Your own proxy server

Country targeting:
- isp defaults to US when --country and --ip-address are omitted
- an ISP --ip-address uses that reserved IP's country unless --country is also set
- residential and mobile use the global pool without country targeting when --country is omitted

Use kernel proxy-ips list to find active reserved IPs. Omit --ip-address to let Kernel choose an ISP exit IP.

Examples:
  # Create a custom proxy
  kernel proxies create --type custom --host proxy.example.com --port 8080 --username myuser --password mypass --name "My Custom Proxy"

  # Create a custom TLS-terminating proxy with a CA bundle
  kernel proxies create --type custom --host proxy.example.com --port 8080 --ca-bundle ./proxy-ca.pem --name "My TLS Proxy"

  # Create an ISP proxy in Germany
  kernel proxies create --type isp --country DE --name "DE ISP"

  # Pin an ISP proxy to an active reserved IP
  kernel proxies create --type isp --ip-address 203.0.113.10 --name "Pinned ISP"

  # Create a residential proxy with location
  kernel proxies create --type residential --country US --city sanfrancisco --state CA --name "SF Residential"

  # Create a proxy with bypass hosts
  kernel proxies create --type isp --country US --bypass-host localhost,internal.service.local --name "Internal Services"`,
	RunE: runProxiesCreate,
}

var proxiesUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update a proxy configuration",
	Long: `Update a proxy's name, move an ISP proxy to another reserved exit IP, or do both.

Proxy names do not need to be unique. Moving with --ip-address is available only for ISP proxies in organizations with a reserved ISP allocation. The IP must be active and in the proxy's country. Kernel probes the new endpoint before changing the proxy, so a failed probe leaves it unchanged. Live browsers use the new IP for new connections within a few seconds while established connections drain on the old IP.

If the IP is outside the organization's allocation, the update is rejected. If the IP cannot currently be assigned or the proxy changed concurrently, inspect it with kernel proxies get and retry.`,
	Args: cobra.ExactArgs(1),
	RunE: runProxiesUpdate,
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
	Long:  "Run a health check on a proxy to verify it's working and update its status. Optionally test against a specific public URL.",
	Args:  cobra.ExactArgs(1),
	RunE:  runProxiesCheck,
}

func init() {
	// Add subcommands
	ProxiesCmd.AddCommand(proxiesListCmd)
	ProxiesCmd.AddCommand(proxiesGetCmd)
	ProxiesCmd.AddCommand(proxiesCreateCmd)
	ProxiesCmd.AddCommand(proxiesUpdateCmd)
	ProxiesCmd.AddCommand(proxiesDeleteCmd)
	ProxiesCmd.AddCommand(proxiesCheckCmd)

	// Add output flags
	addJSONOutputFlag(proxiesListCmd)
	proxiesListCmd.Flags().String("name", "", "Filter by exact proxy name")
	proxiesListCmd.Flags().String("query", "", "Search proxies by name, host, or IP address (IDs match by exact value)")
	proxiesListCmd.Flags().String("ip-address", "", "Filter ISP proxies by an exact primary exit IPv4 address")
	proxiesListCmd.Flags().Int("limit", 0, "Maximum number of proxies to return")
	proxiesListCmd.Flags().Int("offset", 0, "Number of proxies to skip (for pagination)")
	addJSONOutputFlag(proxiesGetCmd)
	addJSONOutputFlag(proxiesCreateCmd)

	// Add flags for create command
	proxiesCreateCmd.Flags().String("name", "", "Proxy configuration name (required)")
	_ = proxiesCreateCmd.MarkFlagRequired("name")
	proxiesCreateCmd.Flags().String("type", "", "Proxy type (isp|residential|mobile|custom)")
	_ = proxiesCreateCmd.MarkFlagRequired("type")
	proxiesCreateCmd.Flags().String("protocol", "https", "Protocol to use for the proxy connection (http|https)")
	proxiesCreateCmd.Flags().String("ip-address", "", "Active reserved exit IP to pin an ISP proxy to; must match --country when both are set")

	// Location flags (isp, residential, mobile)
	proxiesCreateCmd.Flags().String("country", "", "ISO 3166 country code or EU (ISP supports US, GB, FR, DE, SG, KR and defaults to US, or to --ip-address's country when set; residential and mobile use the global pool when omitted)")
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
	proxiesCreateCmd.Flags().String("ca-bundle", "", "Path to a PEM-encoded CA certificate bundle for a TLS-terminating custom proxy")
	proxiesCreateCmd.Flags().StringSlice("bypass-host", nil, "Hostname(s) to bypass proxy and connect directly (repeat or comma-separated)")

	// Update flags
	addJSONOutputFlag(proxiesUpdateCmd)
	proxiesUpdateCmd.Flags().String("name", "", "New proxy name; duplicate names are allowed")
	proxiesUpdateCmd.Flags().String("ip-address", "", "Active reserved exit IP to move an ISP proxy to")
	proxiesUpdateCmd.MarkFlagsOneRequired("name", "ip-address")

	// Delete flags
	proxiesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Check flags
	proxiesCheckCmd.Flags().String("url", "", "Optional public HTTP or HTTPS URL to test reachability against")
	addJSONOutputFlag(proxiesCheckCmd)
}
