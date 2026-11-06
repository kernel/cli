package proxies

import (
	"context"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// ProxyIPsCmd is the parent command for reserved ISP proxy IP operations.
var ProxyIPsCmd = &cobra.Command{
	Use:     "proxy-ips",
	Aliases: []string{"proxy-ip"},
	Short:   "Inspect reserved ISP proxy IPs",
	Long:    "List and inspect the IP addresses reserved for your organization's ISP proxies.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var proxyIPsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List reserved ISP IPs",
	Long: `List the IP addresses reserved for the organization's ISP proxies, including each IP's country and whether it can currently be assigned.

The list comes from the organization's allocation, so it includes IPs that no proxy currently uses. Excluded IPs remain listed but cannot be pinned or moved to; proxies already using them continue working. This command is available only to organizations with a reserved ISP allocation and requires project-wide or organization-wide proxy read access.`,
	Args: cobra.NoArgs,
	RunE: runProxyIPsList,
}

var proxyIPsGetCmd = &cobra.Command{
	Use:   "get <ip-address>",
	Short: "Get a reserved ISP IP",
	Long: `Get one reserved ISP IP and the ISP proxies using it as their primary exit.

The proxy count covers the entire organization, while the proxy details include only proxies in the current project. An excluded IP cannot be pinned or moved to, though proxies already using it continue working. This command is available only to organizations with a reserved ISP allocation and requires project-wide or organization-wide proxy read access.`,
	Args: cobra.ExactArgs(1),
	RunE: runProxyIPsGet,
}

func init() {
	ProxyIPsCmd.AddCommand(proxyIPsListCmd, proxyIPsGetCmd)

	proxyIPsListCmd.Flags().Int("page", 1, "Page number (1-based)")
	proxyIPsListCmd.Flags().Int("per-page", 20, "Items per page")
	proxyIPsListCmd.Flags().String("country", "", "Filter by an ISP-supported ISO 3166 country code")
	proxyIPsListCmd.Flags().String("status", "", "Filter by assignability (active|excluded)")
	addJSONOutputFlag(proxyIPsListCmd)
	addJSONOutputFlag(proxyIPsGetCmd)
}

// ProxyIPCmd handles reserved ISP proxy IP operations independently of cobra.
type ProxyIPCmd struct {
	proxyIPs ProxyIPService
}

func (p ProxyIPCmd) List(ctx context.Context, in ProxyIPListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	page := in.Page
	perPage := in.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}

	params := kernel.ProxyIPListParams{
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}
	if in.Country != "" {
		params.Country = kernel.Opt(in.Country)
	}
	if in.Status != "" {
		switch in.Status {
		case string(kernel.ProxyIPListParamsStatusActive), string(kernel.ProxyIPListParamsStatusExcluded):
			params.Status = kernel.ProxyIPListParamsStatus(in.Status)
		default:
			return fmt.Errorf("invalid --status %q: must be active or excluded", in.Status)
		}
	}

	if in.Output != "json" {
		pterm.Info.Println("Fetching reserved ISP IPs...")
	}

	result, err := p.proxyIPs.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var items []kernel.ProxyIP
	if result != nil {
		items = result.Items
	}
	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}

	if in.Output == "json" {
		if len(items) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Println("No reserved ISP IPs found")
	} else {
		rows := pterm.TableData{{"IP Address", "Country", "Status"}}
		for _, item := range items {
			rows = append(rows, []string{
				item.IPAddress,
				item.Country,
				formatProxyIPStatus(string(item.Status)),
			})
		}
		table.PrintTableNoPad(rows, true)
	}

	pterm.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, len(items), lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		next := fmt.Sprintf("kernel proxy-ips list --page %d --per-page %d", page+1, perPage)
		if in.Country != "" {
			next += fmt.Sprintf(" --country %q", in.Country)
		}
		if in.Status != "" {
			next += fmt.Sprintf(" --status %q", in.Status)
		}
		pterm.Printf("Next: %s\n", next)
	}

	return nil
}

func (p ProxyIPCmd) Get(ctx context.Context, in ProxyIPGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	item, err := p.proxyIPs.Get(ctx, in.IPAddress)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"IP Address", item.IPAddress})
	rows = append(rows, []string{"Country", item.Country})
	rows = append(rows, []string{"Status", formatProxyIPStatus(string(item.Status))})
	rows = append(rows, []string{"Organization Proxy Count", fmt.Sprintf("%d", item.ProxyCount)})
	rows = append(rows, []string{"Current Project Proxies", formatProxyIPProxies(item.Proxies)})
	table.PrintTableNoPad(rows, true)
	return nil
}

func runProxyIPsList(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	country, _ := cmd.Flags().GetString("country")
	status, _ := cmd.Flags().GetString("status")
	output, _ := cmd.Flags().GetString("output")
	svc := client.ProxyIPs
	return ProxyIPCmd{proxyIPs: &svc}.List(cmd.Context(), ProxyIPListInput{
		Country: country,
		Status:  status,
		Page:    page,
		PerPage: perPage,
		Output:  output,
	})
}

func runProxyIPsGet(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	svc := client.ProxyIPs
	return ProxyIPCmd{proxyIPs: &svc}.Get(cmd.Context(), ProxyIPGetInput{IPAddress: args[0], Output: output})
}

func formatProxyIPStatus(status string) string {
	switch status {
	case "active":
		return pterm.Green(status)
	case "excluded":
		return pterm.Yellow(status)
	case "":
		return "-"
	default:
		return status
	}
}

func formatProxyIPProxies(proxies []kernel.ProxyIPDetailProxy) string {
	if len(proxies) == 0 {
		return "-"
	}
	values := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy.Name == "" {
			values = append(values, proxy.ID)
			continue
		}
		values = append(values, fmt.Sprintf("%s (%s)", proxy.Name, proxy.ID))
	}
	return strings.Join(values, ", ")
}
