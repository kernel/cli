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

func (p ProxyCmd) Update(ctx context.Context, in ProxyUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Name == "" {
		return fmt.Errorf("--name is required")
	}

	item, err := p.proxies.Update(ctx, in.ID, kernel.ProxyUpdateParams{Name: in.Name})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(item)
	}

	pterm.Success.Printf("Renamed proxy %s to %s\n", item.ID, item.Name)

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", item.ID})
	rows = append(rows, []string{"Name", util.OrDash(item.Name)})
	rows = append(rows, []string{"Type", string(item.Type)})
	rows = append(rows, []string{"Bypass Hosts", formatBypassHosts(item.BypassHosts)})

	protocol := string(item.Protocol)
	if protocol == "" {
		protocol = "https"
	}
	rows = append(rows, []string{"Protocol", protocol})

	status := string(item.Status)
	switch item.Status {
	case kernel.ProxyUpdateResponseStatusAvailable:
		status = pterm.Green(status)
	case kernel.ProxyUpdateResponseStatusUnavailable:
		status = pterm.Red(status)
	default:
		if status == "" {
			status = "-"
		}
	}
	rows = append(rows, []string{"Status", status})
	rows = append(rows, []string{"Last Checked", util.FormatLocal(item.LastChecked)})

	table.PrintTableNoPad(rows, true)
	return nil
}

func runProxiesUpdate(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	svc := client.Proxies
	p := ProxyCmd{proxies: &svc}
	return p.Update(cmd.Context(), ProxyUpdateInput{ID: args[0], Name: name, Output: output})
}
