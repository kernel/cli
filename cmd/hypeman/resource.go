package hypeman

import (
	"fmt"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var resourceCmd = &cobra.Command{
	Use:     "resource",
	Aliases: []string{"resources"},
	Short:   "View Hypeman host resources",
}

var resourceGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get current host resource capacity and allocation status",
	RunE:  runResourceGet,
}

func init() {
	resourceCmd.AddCommand(resourceGetCmd)

	resourceGetCmd.Flags().StringP("output", "o", "", "Output format: json")
}

func runResourceGet(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	resources, err := client.Resources.Get(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get resources: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(resources)
	}

	// Resource summary table
	tableData := pterm.TableData{
		{"Resource", "Capacity", "Effective Limit", "Allocated", "Available", "Oversub Ratio"},
		{
			resources.CPU.Type,
			fmt.Sprintf("%d", resources.CPU.Capacity),
			fmt.Sprintf("%d", resources.CPU.EffectiveLimit),
			fmt.Sprintf("%d", resources.CPU.Allocated),
			fmt.Sprintf("%d", resources.CPU.Available),
			fmt.Sprintf("%.2f", resources.CPU.OversubRatio),
		},
		{
			resources.Memory.Type,
			fmt.Sprintf("%d", resources.Memory.Capacity),
			fmt.Sprintf("%d", resources.Memory.EffectiveLimit),
			fmt.Sprintf("%d", resources.Memory.Allocated),
			fmt.Sprintf("%d", resources.Memory.Available),
			fmt.Sprintf("%.2f", resources.Memory.OversubRatio),
		},
		{
			resources.Disk.Type,
			fmt.Sprintf("%d", resources.Disk.Capacity),
			fmt.Sprintf("%d", resources.Disk.EffectiveLimit),
			fmt.Sprintf("%d", resources.Disk.Allocated),
			fmt.Sprintf("%d", resources.Disk.Available),
			fmt.Sprintf("%.2f", resources.Disk.OversubRatio),
		},
		{
			resources.Network.Type,
			fmt.Sprintf("%d", resources.Network.Capacity),
			fmt.Sprintf("%d", resources.Network.EffectiveLimit),
			fmt.Sprintf("%d", resources.Network.Allocated),
			fmt.Sprintf("%d", resources.Network.Available),
			fmt.Sprintf("%.2f", resources.Network.OversubRatio),
		},
	}
	table.PrintTableNoPad(tableData, true)

	// Allocations
	if len(resources.Allocations) > 0 {
		pterm.Println()
		pterm.Info.Println("Allocations:")
		allocData := pterm.TableData{{"Instance", "Name", "CPU", "Memory (bytes)", "Disk (bytes)"}}
		for _, a := range resources.Allocations {
			allocData = append(allocData, []string{
				a.InstanceID,
				a.InstanceName,
				fmt.Sprintf("%d", a.CPU),
				fmt.Sprintf("%d", a.MemoryBytes),
				fmt.Sprintf("%d", a.DiskBytes),
			})
		}
		table.PrintTableNoPad(allocData, true)
	}

	return nil
}
