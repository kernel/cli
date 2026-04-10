package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/param"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Manage projects",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var projectsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		projects, err := client.Projects.List(ctx, kernel.ProjectListParams{})
		if err != nil {
			pterm.Error.Println("Failed to list projects:", err)
			return nil
		}

		if len(projects.Items) == 0 {
			pterm.Info.Println("No projects found")
			return nil
		}

		table := pterm.TableData{{"ID", "Name", "Status", "Created At"}}
		for _, p := range projects.Items {
			table = append(table, []string{p.ID, p.Name, string(p.Status), p.CreatedAt.String()})
		}
		_ = pterm.DefaultTable.WithHasHeader(true).WithData(table).Render()
		return nil
	},
}

var projectsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		project, err := client.Projects.New(ctx, kernel.ProjectNewParams{
			CreateProjectRequest: kernel.CreateProjectRequestParam{
				Name: args[0],
			},
		})
		if err != nil {
			pterm.Error.Println("Failed to create project:", err)
			return nil
		}

		pterm.Success.Printf("Created project: %s (ID: %s)\n", project.Name, project.ID)
		return nil
	},
}

var projectsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a project by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		project, err := client.Projects.Get(ctx, args[0])
		if err != nil {
			pterm.Error.Println("Failed to get project:", err)
			return nil
		}

		table := pterm.TableData{
			{"Field", "Value"},
			{"ID", project.ID},
			{"Name", project.Name},
			{"Status", string(project.Status)},
			{"Created At", project.CreatedAt.String()},
			{"Updated At", project.UpdatedAt.String()},
		}
		_ = pterm.DefaultTable.WithHasHeader(true).WithData(table).Render()
		return nil
	},
}

var projectsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		err := client.Projects.Delete(ctx, args[0])
		if err != nil {
			pterm.Error.Println("Failed to delete project:", err)
			return nil
		}

		pterm.Success.Printf("Deleted project: %s\n", args[0])
		return nil
	},
}

var projectsLimitsGetCmd = &cobra.Command{
	Use:   "get-limits <project-id>",
	Short: "Get project limit overrides",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		limits, err := client.Projects.Limits.Get(ctx, args[0])
		if err != nil {
			pterm.Error.Println("Failed to get project limits:", err)
			return nil
		}

		out, _ := json.MarshalIndent(limits, "", "  ")
		fmt.Println(string(out))
		return nil
	},
}

var projectsLimitsSetCmd = &cobra.Command{
	Use:   "set-limits <project-id>",
	Short: "Set project limit overrides",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		inner := kernel.UpdateProjectLimitsRequestParam{}
		if v, _ := cmd.Flags().GetInt64("max-concurrent-sessions"); v >= 0 && cmd.Flags().Changed("max-concurrent-sessions") {
			inner.MaxConcurrentSessions = param.NewOpt(v)
		}
		if v, _ := cmd.Flags().GetInt64("max-persistent-sessions"); v >= 0 && cmd.Flags().Changed("max-persistent-sessions") {
			inner.MaxPersistentSessions = param.NewOpt(v)
		}
		if v, _ := cmd.Flags().GetInt64("max-concurrent-invocations"); v >= 0 && cmd.Flags().Changed("max-concurrent-invocations") {
			inner.MaxConcurrentInvocations = param.NewOpt(v)
		}
		if v, _ := cmd.Flags().GetInt64("max-pooled-sessions"); v >= 0 && cmd.Flags().Changed("max-pooled-sessions") {
			inner.MaxPooledSessions = param.NewOpt(v)
		}
		params := kernel.ProjectLimitUpdateParams{
			UpdateProjectLimitsRequest: inner,
		}

		limits, err := client.Projects.Limits.Update(ctx, args[0], params)
		if err != nil {
			pterm.Error.Println("Failed to set project limits:", err)
			return nil
		}

		out, _ := json.MarshalIndent(limits, "", "  ")
		pterm.Success.Println("Project limits updated:")
		fmt.Println(string(out))
		return nil
	},
}

func init() {
	projectsLimitsSetCmd.Flags().Int64("max-concurrent-sessions", 0, "Maximum concurrent browser sessions (0 to remove cap)")
	projectsLimitsSetCmd.Flags().Int64("max-persistent-sessions", 0, "Maximum persistent browser sessions (0 to remove cap)")
	projectsLimitsSetCmd.Flags().Int64("max-concurrent-invocations", 0, "Maximum concurrent app invocations (0 to remove cap)")
	projectsLimitsSetCmd.Flags().Int64("max-pooled-sessions", 0, "Maximum pooled sessions capacity (0 to remove cap)")

	projectsCmd.AddCommand(projectsListCmd)
	projectsCmd.AddCommand(projectsCreateCmd)
	projectsCmd.AddCommand(projectsGetCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectsLimitsGetCmd)
	projectsCmd.AddCommand(projectsLimitsSetCmd)
}
