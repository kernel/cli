package proxies

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func (p ProxyCmd) Check(ctx context.Context, in ProxyCheckInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Output != "json" {
		pterm.Info.Printf("Running health check on proxy %s...\n", in.ID)
	}

	proxy, err := p.proxies.Check(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(proxy)
	}

	// Display proxy details after check
	rows := pterm.TableData{{"Property", "Value"}}

	rows = append(rows, []string{"ID", proxy.ID})

	name := proxy.Name
	if name == "" {
		name = "-"
	}
	rows = append(rows, []string{"Name", name})
	rows = append(rows, []string{"Type", string(proxy.Type)})
	rows = append(rows, []string{"Bypass Hosts", formatBypassHosts(proxy.BypassHosts)})

	// Display protocol (default to https if not set)
	protocol := string(proxy.Protocol)
	if protocol == "" {
		protocol = "https"
	}
	rows = append(rows, []string{"Protocol", protocol})

	// Display IP address if available
	if proxy.IPAddress != "" {
		rows = append(rows, []string{"IP Address", proxy.IPAddress})
	}

	// Display type-specific config details
	rows = append(rows, getProxyCheckConfigRows(proxy)...)

	// Display status with color
	status := string(proxy.Status)
	if status == "" {
		status = "-"
	} else if proxy.Status == kernel.ProxyCheckResponseStatusAvailable {
		status = pterm.Green(status)
	} else if proxy.Status == kernel.ProxyCheckResponseStatusUnavailable {
		status = pterm.Red(status)
	}
	rows = append(rows, []string{"Status", status})

	// Display last checked timestamp
	lastChecked := util.FormatLocal(proxy.LastChecked)
	rows = append(rows, []string{"Last Checked", lastChecked})

	table.PrintTableNoPad(rows, true)

	// Print a summary message
	if proxy.Status == kernel.ProxyCheckResponseStatusAvailable {
		pterm.Success.Println("Proxy health check passed")
	} else {
		pterm.Warning.Println("Proxy health check failed - proxy is unavailable")
	}

	return nil
}

func getProxyCheckConfigRows(proxy *kernel.ProxyCheckResponse) [][]string {
	var rows [][]string
	config := &proxy.Config

	switch proxy.Type {
	case kernel.ProxyCheckResponseTypeDatacenter, kernel.ProxyCheckResponseTypeIsp:
		if config.Country != "" {
			rows = append(rows, []string{"Country", config.Country})
		}
	case kernel.ProxyCheckResponseTypeResidential:
		if config.Country != "" {
			rows = append(rows, []string{"Country", config.Country})
		}
		if config.City != "" {
			rows = append(rows, []string{"City", config.City})
		}
		if config.State != "" {
			rows = append(rows, []string{"State", config.State})
		}
		if config.Zip != "" {
			rows = append(rows, []string{"ZIP", config.Zip})
		}
		if config.Asn != "" {
			rows = append(rows, []string{"ASN", config.Asn})
		}
		if config.Os != "" {
			rows = append(rows, []string{"OS", config.Os})
		}
	case kernel.ProxyCheckResponseTypeMobile:
		if config.Country != "" {
			rows = append(rows, []string{"Country", config.Country})
		}
		if config.City != "" {
			rows = append(rows, []string{"City", config.City})
		}
		if config.State != "" {
			rows = append(rows, []string{"State", config.State})
		}
		if config.Zip != "" {
			rows = append(rows, []string{"ZIP", config.Zip})
		}
		if config.Asn != "" {
			rows = append(rows, []string{"ASN", config.Asn})
		}
		if config.Carrier != "" {
			rows = append(rows, []string{"Carrier", config.Carrier})
		}
	case kernel.ProxyCheckResponseTypeCustom:
		if config.Host != "" {
			rows = append(rows, []string{"Host", config.Host})
		}
		if config.Port != 0 {
			rows = append(rows, []string{"Port", fmt.Sprintf("%d", config.Port)})
		}
		if config.Username != "" {
			rows = append(rows, []string{"Username", config.Username})
		}
		hasPassword := "No"
		if config.HasPassword {
			hasPassword = "Yes"
		}
		rows = append(rows, []string{"Has Password", hasPassword})
	}

	return rows
}

func runProxiesCheck(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	svc := client.Proxies
	p := ProxyCmd{proxies: &svc}
	return p.Check(cmd.Context(), ProxyCheckInput{ID: args[0], Output: output})
}
