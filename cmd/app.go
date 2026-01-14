package cmd

import (
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var appCmd = &cobra.Command{
	Use:     "app",
	Aliases: []string{"apps"},
	Short:   "Manage deployed applications",
	Long:    "Commands for managing deployed Kernel applications",
}

// --- app list subcommand
var appListCmd = &cobra.Command{
	Use:   "list",
	Short: "List deployed application versions",
	RunE:  runAppList,
}

// --- app history subcommand (scaffold)
var appHistoryCmd = &cobra.Command{
	Use:   "history <app_name>",
	Short: "Show deployment history for an application",
	Args:  cobra.ExactArgs(1),
	RunE:  runAppHistory,
}

func init() {
	// register subcommands under app
	appCmd.AddCommand(appListCmd)
	appCmd.AddCommand(appHistoryCmd)

	// Add optional filters for list
	appListCmd.Flags().String("name", "", "Filter by application name")
	appListCmd.Flags().String("version", "", "Filter by version label")
	appListCmd.Flags().Int("limit", 20, "Max apps to return (default 20)")
	appListCmd.Flags().Int("per-page", 20, "Items per page (alias of --limit)")
	appListCmd.Flags().Int("page", 1, "Page number (1-based)")
	appListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Limit rows returned for app history (0 = all)
	appHistoryCmd.Flags().Int("limit", 20, "Max deployments to return (default 20)")
	appHistoryCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
}

func runAppList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	appName, _ := cmd.Flags().GetString("name")
	version, _ := cmd.Flags().GetString("version")
	lim, _ := cmd.Flags().GetInt("limit")
	perPage, _ := cmd.Flags().GetInt("per-page")
	page, _ := cmd.Flags().GetInt("page")
	output, _ := cmd.Flags().GetString("output")

	if output != "" && output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	// Determine pagination inputs: prefer page/per-page if provided; else map legacy --limit
	usePager := cmd.Flags().Changed("per-page") || cmd.Flags().Changed("page")
	if !usePager && cmd.Flags().Changed("limit") {
		if lim < 0 {
			lim = 0
		}
		perPage = lim
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if page <= 0 {
		page = 1
	}

	if output != "json" {
		pterm.Debug.Println("Fetching deployed applications...")
	}

	params := kernel.AppListParams{}
	if appName != "" {
		params.AppName = kernel.Opt(appName)
	}
	if version != "" {
		params.Version = kernel.Opt(version)
	}
	// Apply server-side pagination (request one extra to detect hasMore)
	params.Limit = kernel.Opt(int64(perPage + 1))
	params.Offset = kernel.Opt(int64((page - 1) * perPage))

	apps, err := client.Apps.List(cmd.Context(), params)
	if err != nil {
		pterm.Error.Printf("Failed to list applications: %v\n", err)
		return nil
	}

	if output == "json" {
		if apps == nil || len(apps.Items) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(apps.Items)
	}

	if apps == nil || len(apps.Items) == 0 {
		pterm.Info.Println("No applications found")
		return nil
	}

	// Determine hasMore using +1 item trick and keep only perPage items for display
	items := apps.Items
	hasMore := false
	if len(items) > perPage {
		hasMore = true
		items = items[:perPage]
	}
	itemsThisPage := len(items)

	// Prepare table data
	tableData := pterm.TableData{
		{"App Name", "Version", "App Version ID", "Region", "Actions", "Env Vars"},
	}

	rows := 0
	for _, app := range items {
		// Format env vars
		envVarsStr := "-"
		if len(app.EnvVars) > 0 {
			envVarsStr = strings.Join(lo.Keys(app.EnvVars), ", ")
		}

		actionsStr := "-"
		if len(app.Actions) > 0 {
			actionsStr = strings.Join(lo.Map(app.Actions, func(a kernel.AppAction, _ int) string {
				return a.Name
			}), ", ")
		}

		tableData = append(tableData, []string{
			app.AppName,
			app.Version,
			app.ID,
			string(app.Region),
			actionsStr,
			envVarsStr,
		})
		rows++
	}

	PrintTableNoPad(tableData, true)

	// Footer with pagination details and next command suggestion
	fmt.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, itemsThisPage, lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		nextPage := page + 1
		nextCmd := fmt.Sprintf("kernel app list --page %d --per-page %d", nextPage, perPage)
		if appName != "" {
			nextCmd += fmt.Sprintf(" --name %s", appName)
		}
		if version != "" {
			nextCmd += fmt.Sprintf(" --version %s", version)
		}
		fmt.Printf("Next: %s\n", nextCmd)
	}
	// Concise notes when user-specified per-page/limit/page are outside API-allowed range
	if cmd.Flags().Changed("per-page") {
		if v, _ := cmd.Flags().GetInt("per-page"); v > 100 {
			pterm.Warning.Printfln("Requested --per-page %d; capped to 100.", v)
		} else if v < 1 {
			if cmd.Flags().Changed("page") {
				if p, _ := cmd.Flags().GetInt("page"); p < 1 {
					pterm.Warning.Println("Requested --per-page <1 and --page <1; using per-page=20, page=1.")
				} else {
					pterm.Warning.Println("Requested --per-page <1; using per-page=20.")
				}
			} else {
				pterm.Warning.Println("Requested --per-page <1; using per-page=20.")
			}
		}
	} else if !usePager && cmd.Flags().Changed("limit") {
		if lim > 100 {
			pterm.Warning.Printfln("Requested --limit %d; capped to 100.", lim)
		} else if lim < 1 {
			if cmd.Flags().Changed("page") {
				if p, _ := cmd.Flags().GetInt("page"); p < 1 {
					pterm.Warning.Println("Requested --limit <1 and --page <1; using per-page=20, page=1.")
				} else {
					pterm.Warning.Println("Requested --limit <1; using per-page=20.")
				}
			} else {
				pterm.Warning.Println("Requested --limit <1; using per-page=20.")
			}
		}
	} else if cmd.Flags().Changed("page") {
		if p, _ := cmd.Flags().GetInt("page"); p < 1 {
			pterm.Warning.Println("Requested --page <1; using page=1.")
		}
	}
	return nil
}

func runAppHistory(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	appName := args[0]
	lim, _ := cmd.Flags().GetInt("limit")
	output, _ := cmd.Flags().GetString("output")

	if output != "" && output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if output != "json" {
		pterm.Debug.Printf("Fetching deployment history for app '%s'...\n", appName)
	}

	params := kernel.DeploymentListParams{}
	if appName != "" {
		params.AppName = kernel.Opt(appName)
	}

	deployments, err := client.Deployments.List(cmd.Context(), params)
	if err != nil {
		pterm.Error.Printf("Failed to list deployments: %v\n", err)
		return nil
	}

	if output == "json" {
		if deployments == nil || len(deployments.Items) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(deployments.Items)
	}

	if deployments == nil || len(deployments.Items) == 0 {
		pterm.Info.Println("No deployments found for this application")
		return nil
	}

	tableData := pterm.TableData{
		{"Deployment ID", "Created At", "Region", "Status", "Entrypoint", "Reason"},
	}

	rows := 0
	for _, dep := range deployments.Items {
		created := util.FormatLocal(dep.CreatedAt)
		status := string(dep.Status)

		tableData = append(tableData, []string{
			dep.ID,
			created,
			string(dep.Region),
			status,
			dep.EntrypointRelPath,
			dep.StatusReason,
		})

		rows++
		if lim > 0 && rows >= lim {
			break
		}
	}

	PrintTableNoPad(tableData, true)
	return nil
}
