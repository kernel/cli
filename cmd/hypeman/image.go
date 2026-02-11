package hypeman

import (
	"fmt"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	hypemansdk "github.com/kernel/hypeman-go"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	Use:     "image",
	Aliases: []string{"images"},
	Short:   "Manage Hypeman images",
}

var imageCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Pull and convert an OCI image",
	RunE:  runImageCreate,
}

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List images",
	RunE:  runImageList,
}

var imageGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get image details",
	Args:  cobra.ExactArgs(1),
	RunE:  runImageGet,
}

var imageDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an image",
	Args:  cobra.ExactArgs(1),
	RunE:  runImageDelete,
}

func init() {
	imageCmd.AddCommand(imageCreateCmd)
	imageCmd.AddCommand(imageListCmd)
	imageCmd.AddCommand(imageGetCmd)
	imageCmd.AddCommand(imageDeleteCmd)

	imageCreateCmd.Flags().String("name", "", "OCI image reference (e.g., docker.io/library/nginx:latest) (required)")
	_ = imageCreateCmd.MarkFlagRequired("name")
	imageCreateCmd.Flags().StringP("output", "o", "", "Output format: json")

	imageListCmd.Flags().StringP("output", "o", "", "Output format: json")
	imageGetCmd.Flags().StringP("output", "o", "", "Output format: json")
}

func runImageCreate(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")

	image, err := client.Images.New(cmd.Context(), hypemansdk.ImageNewParams{
		Name: name,
	})
	if err != nil {
		return fmt.Errorf("failed to create image: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(image)
	}

	pterm.Success.Printf("Created image %s (status: %s)\n", image.Name, image.Status)
	printImageDetail(image)
	return nil
}

func runImageList(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	images, err := client.Images.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	if output == "json" {
		if images == nil || len(*images) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*images)
	}

	if images == nil || len(*images) == 0 {
		pterm.Info.Println("No images found")
		return nil
	}

	tableData := pterm.TableData{{"Name", "Status", "Digest", "Size (bytes)", "Created At"}}
	for _, img := range *images {
		sizeStr := "-"
		if img.SizeBytes > 0 {
			sizeStr = fmt.Sprintf("%d", img.SizeBytes)
		}
		tableData = append(tableData, []string{
			img.Name,
			string(img.Status),
			truncate(img.Digest, 20),
			sizeStr,
			util.FormatLocal(img.CreatedAt),
		})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runImageGet(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	image, err := client.Images.Get(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to get image: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(image)
	}

	printImageDetail(image)
	return nil
}

func runImageDelete(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	if err := client.Images.Delete(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	pterm.Success.Printf("Deleted image %s\n", args[0])
	return nil
}

func printImageDetail(img *hypemansdk.Image) {
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Name", img.Name},
		{"Status", string(img.Status)},
		{"Digest", img.Digest},
		{"Size (bytes)", fmt.Sprintf("%d", img.SizeBytes)},
		{"Created At", util.FormatLocal(img.CreatedAt)},
	}
	if img.Error != "" {
		tableData = append(tableData, []string{"Error", img.Error})
	}
	if img.WorkingDir != "" {
		tableData = append(tableData, []string{"Working Dir", img.WorkingDir})
	}
	table.PrintTableNoPad(tableData, true)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
