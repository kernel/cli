package hypeman

import (
	"fmt"
	"os"

	hypemansdk "github.com/kernel/hypeman-go"
	"github.com/kernel/hypeman-go/option"
	"github.com/spf13/cobra"
)

// HypemanCmd is the top-level command for Hypeman operations.
var HypemanCmd = &cobra.Command{
	Use:   "hypeman",
	Short: "Manage Hypeman instances, images, volumes, devices, ingresses, builds, and resources",
	Long:  "Commands for interacting with the Hypeman API for VM instance management",
}

func init() {
	HypemanCmd.AddCommand(instanceCmd)
	HypemanCmd.AddCommand(imageCmd)
	HypemanCmd.AddCommand(volumeCmd)
	HypemanCmd.AddCommand(deviceCmd)
	HypemanCmd.AddCommand(ingressCmd)
	HypemanCmd.AddCommand(resourceCmd)
	HypemanCmd.AddCommand(buildCmd)
}

// getHypemanClient creates a Hypeman SDK client.
// It reads HYPEMAN_API_KEY and HYPEMAN_BASE_URL from the environment.
func getHypemanClient() (hypemansdk.Client, error) {
	apiKey := os.Getenv("HYPEMAN_API_KEY")
	if apiKey == "" {
		return hypemansdk.Client{}, fmt.Errorf("HYPEMAN_API_KEY environment variable is required")
	}
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL := os.Getenv("HYPEMAN_BASE_URL"); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return hypemansdk.NewClient(opts...), nil
}

// mustGetClient is a helper that returns the client or sets the error on the command.
func mustGetClient(cmd *cobra.Command) (hypemansdk.Client, error) {
	return getHypemanClient()
}
