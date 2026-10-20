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
	Use:   "get <id-or-name>",
	Short: "Get proxy configuration by ID or exact name",
	Long: `Retrieve a proxy in the resolved project by ID or exact name using the
database collation. IDs take precedence when a value matches both an ID and a
name. Proxy names are not unique; an ambiguous name returns a 400 response
with code ambiguous_proxy_name.`,
	Args: cobra.ExactArgs(1),
	RunE: runProxiesGet,
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
- isp defaults to US when --country is omitted
- residential and mobile use the global pool without country targeting when --country is omitted

Examples:
  # Create a custom proxy
  kernel proxies create --type custom --host proxy.example.com --port 8080 --username myuser --password mypass --name "My Custom Proxy"

  # Create a custom TLS-terminating proxy with a CA bundle
  kernel proxies create --type custom --host proxy.example.com --port 8080 --ca-bundle ./proxy-ca.pem --name "My TLS Proxy"

  # Create an ISP proxy in Germany
  kernel proxies create --type isp --country DE --name "DE ISP"

  # Create a residential proxy with location
  kernel proxies create --type residential --country US --city sanfrancisco --state CA --name "SF Residential"

  # Create a proxy with bypass hosts
  kernel proxies create --type isp --country US --bypass-host localhost,internal.service.local --name "Internal Services"`,
	RunE: runProxiesCreate,
}

var proxiesUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Rename a proxy configuration by ID or exact name",
	Long:  "Rename a proxy configuration by ID or exact name. Proxy names are not unique and duplicate names are allowed; an ambiguous name is rejected. Only the name can be changed; recreate the proxy to change its type or config.",
	Args:  cobra.ExactArgs(1),
	RunE:  runProxiesUpdate,
}

var proxiesDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a proxy configuration by ID or exact name",
	Long:  "Soft-delete a proxy by ID or exact name. Session records referencing it are not modified. With egress binding polling enabled, active-session tunnels using it terminate within one polling interval and later connections through it are rejected.",
	Args:  cobra.ExactArgs(1),
	RunE:  runProxiesDelete,
}

var proxiesCheckCmd = &cobra.Command{
	Use:   "check <id-or-name>",
	Short: "Run a health check on a proxy by ID or exact name",
	Long:  "Run a health check on a proxy by ID or exact name to verify it is working. Optionally test a public URL. For ISP proxies this reliably tests reachability from the stable exit IP; residential and mobile exit nodes vary, so their result validates configuration and connectivity rather than guaranteeing site-specific reachability.",
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

	// Location flags (isp, residential, mobile)
	proxiesCreateCmd.Flags().String("country", "", "ISO 3166 country code or EU (isp proxies support US, GB, FR, DE, SG, KR; isp defaults to US, residential and mobile use the global pool without country targeting)")
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
	proxiesUpdateCmd.Flags().String("name", "", "New proxy name (required)")
	_ = proxiesUpdateCmd.MarkFlagRequired("name")

	// Delete flags
	proxiesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Check flags
	proxiesCheckCmd.Flags().String("url", "", "Optional public HTTP or HTTPS URL to test reachability against")
	addJSONOutputFlag(proxiesCheckCmd)
}
