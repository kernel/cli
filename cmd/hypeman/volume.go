package hypeman

import (
	"fmt"
	"os"
	"strings"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	hypemansdk "github.com/kernel/hypeman-go"
	"github.com/kernel/hypeman-go/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var volumeCmd = &cobra.Command{
	Use:     "volume",
	Aliases: []string{"volumes"},
	Short:   "Manage Hypeman volumes",
}

var volumeCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new empty volume",
	RunE:  runVolumeCreate,
}

var volumeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List volumes",
	RunE:  runVolumeList,
}

var volumeGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get volume details",
	Args:  cobra.ExactArgs(1),
	RunE:  runVolumeGet,
}

var volumeDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a volume",
	Args:  cobra.ExactArgs(1),
	RunE:  runVolumeDelete,
}

var volumeCreateFromArchiveCmd = &cobra.Command{
	Use:   "create-from-archive <archive-path>",
	Short: "Create a volume from a tar.gz archive",
	Args:  cobra.ExactArgs(1),
	RunE:  runVolumeCreateFromArchive,
}

func init() {
	volumeCmd.AddCommand(volumeCreateCmd)
	volumeCmd.AddCommand(volumeListCmd)
	volumeCmd.AddCommand(volumeGetCmd)
	volumeCmd.AddCommand(volumeDeleteCmd)
	volumeCmd.AddCommand(volumeCreateFromArchiveCmd)

	// volume create flags
	volumeCreateCmd.Flags().String("name", "", "Volume name (required)")
	_ = volumeCreateCmd.MarkFlagRequired("name")
	volumeCreateCmd.Flags().Int64("size-gb", 0, "Size in gigabytes (required)")
	_ = volumeCreateCmd.MarkFlagRequired("size-gb")
	volumeCreateCmd.Flags().String("id", "", "Optional custom identifier")
	volumeCreateCmd.Flags().StringP("output", "o", "", "Output format: json")

	// volume list flags
	volumeListCmd.Flags().StringP("output", "o", "", "Output format: json")

	// volume get flags
	volumeGetCmd.Flags().StringP("output", "o", "", "Output format: json")

	// volume create-from-archive flags
	volumeCreateFromArchiveCmd.Flags().String("name", "", "Volume name (required)")
	_ = volumeCreateFromArchiveCmd.MarkFlagRequired("name")
	volumeCreateFromArchiveCmd.Flags().Int64("size-gb", 0, "Maximum size in GB (required)")
	_ = volumeCreateFromArchiveCmd.MarkFlagRequired("size-gb")
	volumeCreateFromArchiveCmd.Flags().String("id", "", "Optional custom volume ID")
	volumeCreateFromArchiveCmd.Flags().StringP("output", "o", "", "Output format: json")
}

func runVolumeCreate(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	sizeGB, _ := cmd.Flags().GetInt64("size-gb")

	params := hypemansdk.VolumeNewParams{
		Name:   name,
		SizeGB: sizeGB,
	}
	if v, _ := cmd.Flags().GetString("id"); v != "" {
		params.ID = param.NewOpt(v)
	}

	volume, err := client.Volumes.New(cmd.Context(), params)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(volume)
	}

	pterm.Success.Printf("Created volume %s (%s, %dGB)\n", volume.Name, volume.ID, volume.SizeGB)
	return nil
}

func runVolumeList(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	volumes, err := client.Volumes.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}

	if output == "json" {
		if volumes == nil || len(*volumes) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*volumes)
	}

	if volumes == nil || len(*volumes) == 0 {
		pterm.Info.Println("No volumes found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Name", "Size (GB)", "Attachments", "Created At"}}
	for _, vol := range *volumes {
		attachStr := "-"
		if len(vol.Attachments) > 0 {
			var parts []string
			for _, a := range vol.Attachments {
				parts = append(parts, fmt.Sprintf("%s@%s", a.InstanceID, a.MountPath))
			}
			attachStr = strings.Join(parts, ", ")
		}
		tableData = append(tableData, []string{
			vol.ID,
			vol.Name,
			fmt.Sprintf("%d", vol.SizeGB),
			attachStr,
			util.FormatLocal(vol.CreatedAt),
		})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runVolumeGet(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	volume, err := client.Volumes.Get(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to get volume: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(volume)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", volume.ID},
		{"Name", volume.Name},
		{"Size (GB)", fmt.Sprintf("%d", volume.SizeGB)},
		{"Created At", util.FormatLocal(volume.CreatedAt)},
	}
	if len(volume.Attachments) > 0 {
		for _, a := range volume.Attachments {
			tableData = append(tableData, []string{"Attachment", fmt.Sprintf("%s@%s (readonly: %t)", a.InstanceID, a.MountPath, a.Readonly)})
		}
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runVolumeDelete(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	if err := client.Volumes.Delete(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	pterm.Success.Printf("Deleted volume %s\n", args[0])
	return nil
}

func runVolumeCreateFromArchive(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	sizeGB, _ := cmd.Flags().GetInt64("size-gb")

	archivePath := args[0]
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	params := hypemansdk.VolumeNewFromArchiveParams{
		Name:   name,
		SizeGB: sizeGB,
	}
	if v, _ := cmd.Flags().GetString("id"); v != "" {
		params.ID = param.NewOpt(v)
	}

	volume, err := client.Volumes.NewFromArchive(cmd.Context(), file, params)
	if err != nil {
		return fmt.Errorf("failed to create volume from archive: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(volume)
	}

	pterm.Success.Printf("Created volume %s from archive (%s, %dGB)\n", volume.Name, volume.ID, volume.SizeGB)
	return nil
}
