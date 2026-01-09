package proxies

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func (p ProxyCmd) List(ctx context.Context, in ProxyListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Output != "json" {
		pterm.Info.Println("Fetching proxy configurations...")
	}

	items, err := p.proxies.List(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if items == nil || len(*items) == 0 {
			fmt.Println("[]")
			return nil
		}
		bs, err := json.MarshalIndent(*items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	if items == nil || len(*items) == 0 {
		pterm.Info.Println("No proxy configurations found")
		return nil
	}

	// Prepare table data
	tableData := pterm.TableData{
		{"ID", "Name", "Type", "Protocol", "Config", "Status", "Last Checked"},
	}

	for _, proxy := range *items {
		name := proxy.Name
		if name == "" {
			name = "-"
		}

		// Get protocol (default to https if not set, since that's the default)
		protocol := string(proxy.Protocol)
		if protocol == "" {
			protocol = "https"
		}

		// Format config based on type
		configStr := formatProxyConfig(&proxy)

		// Format status with color
		status := string(proxy.Status)
		if status == "" {
			status = "-"
		} else if status == "available" {
			status = pterm.Green(status)
		} else if status == "unavailable" {
			status = pterm.Red(status)
		}

		// Format last checked timestamp
		lastChecked := util.FormatLocal(proxy.LastChecked)

		tableData = append(tableData, []string{
			proxy.ID,
			name,
			string(proxy.Type),
			protocol,
			configStr,
			status,
			lastChecked,
		})
	}

	table.PrintTableNoPad(tableData, true)
	return nil
}

func formatProxyConfig(proxy *kernel.ProxyListResponse) string {
	config := &proxy.Config
	switch proxy.Type {
	case kernel.ProxyListResponseTypeDatacenter, kernel.ProxyListResponseTypeIsp:
		if config.Country != "" {
			return fmt.Sprintf("Country: %s", config.Country)
		}
	case kernel.ProxyListResponseTypeResidential:
		parts := []string{}
		if config.Country != "" {
			parts = append(parts, fmt.Sprintf("Country: %s", config.Country))
		}
		if config.City != "" {
			parts = append(parts, fmt.Sprintf("City: %s", config.City))
		}
		if config.State != "" {
			parts = append(parts, fmt.Sprintf("State: %s", config.State))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	case kernel.ProxyListResponseTypeMobile:
		parts := []string{}
		if config.Country != "" {
			parts = append(parts, fmt.Sprintf("Country: %s", config.Country))
		}
		if config.Carrier != "" {
			parts = append(parts, fmt.Sprintf("Carrier: %s", config.Carrier))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	case kernel.ProxyListResponseTypeCustom:
		if config.Host != "" {
			auth := ""
			if config.Username != "" {
				auth = fmt.Sprintf(", Auth: %s", config.Username)
			}
			return fmt.Sprintf("%s:%d%s", config.Host, config.Port, auth)
		}
	}
	return "-"
}

func runProxiesList(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	svc := client.Proxies
	p := ProxyCmd{proxies: &svc}
	return p.List(cmd.Context(), ProxyListInput{Output: output})
}
