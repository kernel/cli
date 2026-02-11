package hypeman

import (
	"fmt"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	hypemansdk "github.com/kernel/hypeman-go"
	"github.com/kernel/hypeman-go/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var deviceCmd = &cobra.Command{
	Use:     "device",
	Aliases: []string{"devices"},
	Short:   "Manage Hypeman devices",
}

var deviceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Register a device for passthrough",
	RunE:  runDeviceCreate,
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered devices",
	RunE:  runDeviceList,
}

var deviceGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get device details",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeviceGet,
}

var deviceDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Unregister a device",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeviceDelete,
}

var deviceListAvailableCmd = &cobra.Command{
	Use:   "list-available",
	Short: "Discover passthrough-capable devices on host",
	RunE:  runDeviceListAvailable,
}

func init() {
	deviceCmd.AddCommand(deviceCreateCmd)
	deviceCmd.AddCommand(deviceListCmd)
	deviceCmd.AddCommand(deviceGetCmd)
	deviceCmd.AddCommand(deviceDeleteCmd)
	deviceCmd.AddCommand(deviceListAvailableCmd)

	// device create flags
	deviceCreateCmd.Flags().String("pci-address", "", "PCI address of the device (required, e.g., '0000:a2:00.0')")
	_ = deviceCreateCmd.MarkFlagRequired("pci-address")
	deviceCreateCmd.Flags().String("name", "", "Optional device name")
	deviceCreateCmd.Flags().StringP("output", "o", "", "Output format: json")

	// device list flags
	deviceListCmd.Flags().StringP("output", "o", "", "Output format: json")

	// device get flags
	deviceGetCmd.Flags().StringP("output", "o", "", "Output format: json")

	// device list-available flags
	deviceListAvailableCmd.Flags().StringP("output", "o", "", "Output format: json")
}

func runDeviceCreate(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	pciAddress, _ := cmd.Flags().GetString("pci-address")

	params := hypemansdk.DeviceNewParams{
		PciAddress: pciAddress,
	}
	if v, _ := cmd.Flags().GetString("name"); v != "" {
		params.Name = param.NewOpt(v)
	}

	device, err := client.Devices.New(cmd.Context(), params)
	if err != nil {
		return fmt.Errorf("failed to register device: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(device)
	}

	pterm.Success.Printf("Registered device %s (%s, type: %s)\n", device.Name, device.ID, device.Type)
	return nil
}

func runDeviceList(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	devices, err := client.Devices.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	if output == "json" {
		if devices == nil || len(*devices) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*devices)
	}

	if devices == nil || len(*devices) == 0 {
		pterm.Info.Println("No devices found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Name", "Type", "PCI Address", "VFIO Bound", "Attached To", "Created At"}}
	for _, dev := range *devices {
		tableData = append(tableData, []string{
			dev.ID,
			util.OrDash(dev.Name),
			string(dev.Type),
			dev.PciAddress,
			fmt.Sprintf("%t", dev.BoundToVfio),
			util.OrDash(dev.AttachedTo),
			util.FormatLocal(dev.CreatedAt),
		})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runDeviceGet(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	device, err := client.Devices.Get(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(device)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", device.ID},
		{"Name", util.OrDash(device.Name)},
		{"Type", string(device.Type)},
		{"PCI Address", device.PciAddress},
		{"Vendor ID", device.VendorID},
		{"Device ID", device.DeviceID},
		{"IOMMU Group", fmt.Sprintf("%d", device.IommuGroup)},
		{"VFIO Bound", fmt.Sprintf("%t", device.BoundToVfio)},
		{"Attached To", util.OrDash(device.AttachedTo)},
		{"Created At", util.FormatLocal(device.CreatedAt)},
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runDeviceDelete(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	if err := client.Devices.Delete(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("failed to unregister device: %w", err)
	}

	pterm.Success.Printf("Unregistered device %s\n", args[0])
	return nil
}

func runDeviceListAvailable(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	devices, err := client.Devices.ListAvailable(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list available devices: %w", err)
	}

	if output == "json" {
		if devices == nil || len(*devices) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*devices)
	}

	if devices == nil || len(*devices) == 0 {
		pterm.Info.Println("No available devices found")
		return nil
	}

	tableData := pterm.TableData{{"PCI Address", "Vendor", "Device", "IOMMU Group", "Current Driver"}}
	for _, dev := range *devices {
		tableData = append(tableData, []string{
			dev.PciAddress,
			fmt.Sprintf("%s (%s)", util.OrDash(dev.VendorName), dev.VendorID),
			fmt.Sprintf("%s (%s)", util.OrDash(dev.DeviceName), dev.DeviceID),
			fmt.Sprintf("%d", dev.IommuGroup),
			util.OrDash(dev.CurrentDriver),
		})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}
