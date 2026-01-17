package proxies

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func (p ProxyCmd) Check(ctx context.Context, in ProxyCheckInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Output != "json" {
		pterm.Info.Printf("Checking proxy %s...\n", in.ID)
	}

	proxy, err := p.proxies.Check(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(proxy)
	}

	// Display check result
	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", proxy.ID})

	name := proxy.Name
	if name == "" {
		name = "-"
	}
	rows = append(rows, []string{"Name", name})
	rows = append(rows, []string{"Type", string(proxy.Type)})
	rows = append(rows, []string{"Status", string(proxy.Status)})
	rows = append(rows, []string{"IP Address", proxy.IPAddress})

	protocol := string(proxy.Protocol)
	if protocol == "" {
		protocol = "https"
	}
	rows = append(rows, []string{"Protocol", protocol})
	rows = append(rows, []string{"Last Checked", util.FormatLocal(proxy.LastChecked)})

	table.PrintTableNoPad(rows, true)

	// Show status message
	if proxy.Status == "available" {
		pterm.Success.Println("Proxy is available and working")
	} else {
		pterm.Warning.Println("Proxy is unavailable")
	}

	return nil
}

func runProxiesCheck(cmd *cobra.Command, args []string) error {
	client := util.GetKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Proxies
	p := ProxyCmd{proxies: &svc}
	return p.Check(cmd.Context(), ProxyCheckInput{
		ID:     args[0],
		Output: output,
	})
}
