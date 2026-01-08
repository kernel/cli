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

func (p ProxyCmd) Get(ctx context.Context, in ProxyGetInput) error {
	item, err := p.proxies.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	// Display proxy details
	rows := pterm.TableData{{"Property", "Value"}}

	rows = append(rows, []string{"ID", item.ID})

	name := item.Name
	if name == "" {
		name = "-"
	}
	rows = append(rows, []string{"Name", name})
	rows = append(rows, []string{"Type", string(item.Type)})

	// Display protocol (default to https if not set)
	protocol := string(item.Protocol)
	if protocol == "" {
		protocol = "https"
	}
	rows = append(rows, []string{"Protocol", protocol})

	// Display type-specific config details
	rows = append(rows, getProxyConfigRows(item)...)

	// Display status with color
	status := string(item.Status)
	if status == "" {
		status = "-"
	} else if status == "available" {
		status = pterm.Green(status)
	} else if status == "unavailable" {
		status = pterm.Red(status)
	}
	rows = append(rows, []string{"Status", status})

	// Display last checked timestamp
	lastChecked := util.FormatLocal(item.LastChecked)
	rows = append(rows, []string{"Last Checked", lastChecked})

	table.PrintTableNoPad(rows, true)
	return nil
}

func getProxyConfigRows(proxy *kernel.ProxyGetResponse) [][]string {
	var rows [][]string
	config := &proxy.Config

	switch proxy.Type {
	case kernel.ProxyGetResponseTypeDatacenter, kernel.ProxyGetResponseTypeIsp:
		if config.Country != "" {
			rows = append(rows, []string{"Country", config.Country})
		}
	case kernel.ProxyGetResponseTypeResidential:
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
	case kernel.ProxyGetResponseTypeMobile:
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
	case kernel.ProxyGetResponseTypeCustom:
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

func runProxiesGet(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	svc := client.Proxies
	p := ProxyCmd{proxies: &svc}
	return p.Get(cmd.Context(), ProxyGetInput{ID: args[0]})
}
