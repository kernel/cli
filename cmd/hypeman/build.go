package hypeman

import (
	"fmt"
	"os"

	"github.com/kernel/cli/pkg/table"
	"github.com/kernel/cli/pkg/util"
	hypemansdk "github.com/kernel/hypeman-go"
	"github.com/kernel/hypeman-go/packages/param"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:     "build",
	Aliases: []string{"builds"},
	Short:   "Manage Hypeman builds",
}

var buildCreateCmd = &cobra.Command{
	Use:   "create <source-tarball>",
	Short: "Create a new build job from a source tarball",
	Args:  cobra.ExactArgs(1),
	RunE:  runBuildCreate,
}

var buildListCmd = &cobra.Command{
	Use:   "list",
	Short: "List builds",
	RunE:  runBuildList,
}

var buildGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get build details",
	Args:  cobra.ExactArgs(1),
	RunE:  runBuildGet,
}

var buildCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Cancel a build",
	Args:  cobra.ExactArgs(1),
	RunE:  runBuildCancel,
}

var buildEventsCmd = &cobra.Command{
	Use:   "events <id>",
	Short: "Stream build events",
	Args:  cobra.ExactArgs(1),
	RunE:  runBuildEvents,
}

func init() {
	buildCmd.AddCommand(buildCreateCmd)
	buildCmd.AddCommand(buildListCmd)
	buildCmd.AddCommand(buildGetCmd)
	buildCmd.AddCommand(buildCancelCmd)
	buildCmd.AddCommand(buildEventsCmd)

	// build create flags
	buildCreateCmd.Flags().String("dockerfile", "", "Dockerfile content (if not in source tarball)")
	buildCreateCmd.Flags().String("base-image-digest", "", "Optional pinned base image digest")
	buildCreateCmd.Flags().String("cache-scope", "", "Tenant-specific cache key prefix")
	buildCreateCmd.Flags().String("global-cache-key", "", "Global cache identifier (e.g., 'node', 'python')")
	buildCreateCmd.Flags().String("is-admin-build", "", "Set to 'true' for admin builds with global cache push access")
	buildCreateCmd.Flags().String("secrets", "", "JSON array of secret references to inject during build")
	buildCreateCmd.Flags().Int64("timeout-seconds", 0, "Build timeout in seconds (default 600)")
	buildCreateCmd.Flags().StringP("output", "o", "", "Output format: json")

	// build list flags
	buildListCmd.Flags().StringP("output", "o", "", "Output format: json")

	// build get flags
	buildGetCmd.Flags().StringP("output", "o", "", "Output format: json")

	// build events flags
	buildEventsCmd.Flags().BoolP("follow", "f", false, "Continue streaming new events")
}

func runBuildCreate(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	sourcePath := args[0]
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source tarball: %w", err)
	}
	defer file.Close()

	params := hypemansdk.BuildNewParams{
		Source: file,
	}

	if v, _ := cmd.Flags().GetString("dockerfile"); v != "" {
		params.Dockerfile = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("base-image-digest"); v != "" {
		params.BaseImageDigest = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("cache-scope"); v != "" {
		params.CacheScope = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("global-cache-key"); v != "" {
		params.GlobalCacheKey = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("is-admin-build"); v != "" {
		params.IsAdminBuild = param.NewOpt(v)
	}
	if v, _ := cmd.Flags().GetString("secrets"); v != "" {
		params.Secrets = param.NewOpt(v)
	}
	if cmd.Flags().Changed("timeout-seconds") {
		v, _ := cmd.Flags().GetInt64("timeout-seconds")
		params.TimeoutSeconds = param.NewOpt(v)
	}

	build, err := client.Builds.New(cmd.Context(), params)
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(build)
	}

	pterm.Success.Printf("Created build %s (status: %s)\n", build.ID, build.Status)
	printBuildDetail(build)
	return nil
}

func runBuildList(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	builds, err := client.Builds.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list builds: %w", err)
	}

	if output == "json" {
		if builds == nil || len(*builds) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*builds)
	}

	if builds == nil || len(*builds) == 0 {
		pterm.Info.Println("No builds found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Status", "Image Ref", "Duration (ms)", "Created At"}}
	for _, b := range *builds {
		durationStr := "-"
		if b.DurationMs > 0 {
			durationStr = fmt.Sprintf("%d", b.DurationMs)
		}
		tableData = append(tableData, []string{
			b.ID,
			string(b.Status),
			util.OrDash(b.ImageRef),
			durationStr,
			util.FormatLocal(b.CreatedAt),
		})
	}
	table.PrintTableNoPad(tableData, true)
	return nil
}

func runBuildGet(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}
	output, _ := cmd.Flags().GetString("output")

	build, err := client.Builds.Get(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to get build: %w", err)
	}

	if output == "json" {
		return util.PrintPrettyJSON(build)
	}

	printBuildDetail(build)
	return nil
}

func runBuildCancel(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	if err := client.Builds.Cancel(cmd.Context(), args[0]); err != nil {
		return fmt.Errorf("failed to cancel build: %w", err)
	}

	pterm.Success.Printf("Cancelled build %s\n", args[0])
	return nil
}

func runBuildEvents(cmd *cobra.Command, args []string) error {
	client, err := mustGetClient(cmd)
	if err != nil {
		return err
	}

	follow, _ := cmd.Flags().GetBool("follow")

	params := hypemansdk.BuildEventsParams{
		Follow: param.NewOpt(follow),
	}

	stream := client.Builds.EventsStreaming(cmd.Context(), args[0], params)
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case hypemansdk.BuildEventTypeLog:
			fmt.Print(event.Content)
		case hypemansdk.BuildEventTypeStatus:
			pterm.Info.Printf("Build status: %s\n", event.Status)
		case hypemansdk.BuildEventTypeHeartbeat:
			// ignore heartbeats
		}
	}
	if stream.Err() != nil {
		return fmt.Errorf("event stream error: %w", stream.Err())
	}
	return nil
}

func printBuildDetail(b *hypemansdk.Build) {
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", b.ID},
		{"Status", string(b.Status)},
		{"Image Ref", util.OrDash(b.ImageRef)},
		{"Image Digest", util.OrDash(b.ImageDigest)},
		{"Duration (ms)", fmt.Sprintf("%d", b.DurationMs)},
		{"Created At", util.FormatLocal(b.CreatedAt)},
	}
	if b.Error != "" {
		tableData = append(tableData, []string{"Error", b.Error})
	}
	if b.BuilderInstanceID != "" {
		tableData = append(tableData, []string{"Builder Instance", b.BuilderInstanceID})
	}
	table.PrintTableNoPad(tableData, true)
}
