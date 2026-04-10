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

// resolveProjectArg resolves a positional project argument that may be an ID or
// a name. If it looks like a cuid2 ID it is returned as-is; otherwise we list
// projects and find the matching name (case-insensitive).
func resolveProjectArg(cmd *cobra.Command, client kernel.Client, val string) (string, error) {
	if cuidRegex.MatchString(val) {
		return val, nil
	}
	resolved, err := resolveProjectByName(cmd.Context(), client, val)
	if err != nil {
		return "", err
	}
	return resolved, nil
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
	Use:   "get <id-or-name>",
	Short: "Get a project by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		projectID, err := resolveProjectArg(cmd, client, args[0])
		if err != nil {
			return err
		}

		project, err := client.Projects.Get(ctx, projectID)
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
	Use:   "delete <id-or-name>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		projectID, err := resolveProjectArg(cmd, client, args[0])
		if err != nil {
			return err
		}

		err = client.Projects.Delete(ctx, projectID)
		if err != nil {
			pterm.Error.Println("Failed to delete project:", err)
			return nil
		}

		pterm.Success.Printf("Deleted project: %s\n", projectID)
		return nil
	},
}

var projectsLimitsGetCmd = &cobra.Command{
	Use:   "get-limits <id-or-name>",
	Short: "Get project limit overrides",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		projectID, err := resolveProjectArg(cmd, client, args[0])
		if err != nil {
			return err
		}

		limits, err := client.Projects.Limits.Get(ctx, projectID)
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
	Use:   "set-limits <id-or-name>",
	Short: "Set project limit overrides",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getKernelClient(cmd)
		ctx := cmd.Context()

		projectID, err := resolveProjectArg(cmd, client, args[0])
		if err != nil {
			return err
		}

		inner := kernel.UpdateProjectLimitsRequestParam{}
		limitFlags := []string{
			"max-concurrent-sessions",
			"max-persistent-sessions",
			"max-concurrent-invocations",
			"max-pooled-sessions",
		}
		for _, name := range limitFlags {
			if cmd.Flags().Changed(name) {
				v, _ := cmd.Flags().GetInt64(name)
				if v < 0 {
					return fmt.Errorf("--%s must be non-negative (got %d); use 0 to remove the cap", name, v)
				}
			}
		}
		if cmd.Flags().Changed("max-concurrent-sessions") {
			v, _ := cmd.Flags().GetInt64("max-concurrent-sessions")
			inner.MaxConcurrentSessions = param.NewOpt(v)
		}
		if cmd.Flags().Changed("max-persistent-sessions") {
			v, _ := cmd.Flags().GetInt64("max-persistent-sessions")
			inner.MaxPersistentSessions = param.NewOpt(v)
		}
		if cmd.Flags().Changed("max-concurrent-invocations") {
			v, _ := cmd.Flags().GetInt64("max-concurrent-invocations")
			inner.MaxConcurrentInvocations = param.NewOpt(v)
		}
		if cmd.Flags().Changed("max-pooled-sessions") {
			v, _ := cmd.Flags().GetInt64("max-pooled-sessions")
			inner.MaxPooledSessions = param.NewOpt(v)
		}
		params := kernel.ProjectLimitUpdateParams{
			UpdateProjectLimitsRequest: inner,
		}

		limits, err := client.Projects.Limits.Update(ctx, projectID, params)
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
