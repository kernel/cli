package cmd

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type ProjectListService interface {
	List(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.Project], err error)
}

type ProjectsService interface {
	ProjectListService
	New(ctx context.Context, body kernel.ProjectNewParams, opts ...option.RequestOption) (res *kernel.Project, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.Project, err error)
	Update(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (res *kernel.Project, err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
}

type ProjectLimitsService interface {
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.ProjectLimits, err error)
	Update(ctx context.Context, id string, body kernel.ProjectLimitUpdateParams, opts ...option.RequestOption) (res *kernel.ProjectLimits, err error)
}

type ProjectsCmd struct {
	projects ProjectsService
	limits   ProjectLimitsService
}

type ProjectsListInput struct{}

type ProjectsCreateInput struct {
	Name string
}

type ProjectsGetInput struct {
	Identifier string
}

type ProjectsUpdateInput struct {
	Identifier string
	Name       string
	NameSet    bool
	Status     string
	StatusSet  bool
}

type ProjectsDeleteInput struct {
	Identifier string
}

type ProjectsLimitsGetInput struct {
	Identifier string
	Output     string
}

type ProjectsLimitsSetInput struct {
	Identifier               string
	MaxConcurrentSessions    Int64Flag
	MaxPersistentSessions    Int64Flag
	MaxConcurrentInvocations Int64Flag
	MaxPooledSessions        Int64Flag
	Output                   string
}

// resolveProjectArg resolves a positional project argument that may be an ID or
// a name. If it looks like a cuid2 ID it is returned as-is; otherwise we list
// projects and find the matching name (case-insensitive).
func resolveProjectArg(ctx context.Context, projects ProjectListService, val string) (string, error) {
	if cuidRegex.MatchString(val) {
		return val, nil
	}
	resolved, err := resolveProjectByName(ctx, projects, val)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func (c ProjectsCmd) List(ctx context.Context, in ProjectsListInput) error {
	projects, err := c.projects.List(ctx, kernel.ProjectListParams{})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if projects == nil || len(projects.Items) == 0 {
		pterm.Info.Println("No projects found")
		return nil
	}

	table := pterm.TableData{{"ID", "Name", "Status", "Created At"}}
	for _, p := range projects.Items {
		table = append(table, []string{p.ID, p.Name, string(p.Status), util.FormatLocal(p.CreatedAt)})
	}
	PrintTableNoPad(table, true)
	return nil
}

func (c ProjectsCmd) Create(ctx context.Context, in ProjectsCreateInput) error {
	project, err := c.projects.New(ctx, kernel.ProjectNewParams{
		CreateProjectRequest: kernel.CreateProjectRequestParam{
			Name: in.Name,
		},
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Created project: %s (ID: %s)\n", project.Name, project.ID)
	return nil
}

func (c ProjectsCmd) Get(ctx context.Context, in ProjectsGetInput) error {
	projectID, err := resolveProjectArg(ctx, c.projects, in.Identifier)
	if err != nil {
		return err
	}

	project, err := c.projects.Get(ctx, projectID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	table := pterm.TableData{
		{"Field", "Value"},
		{"ID", project.ID},
		{"Name", project.Name},
		{"Status", string(project.Status)},
		{"Created At", util.FormatLocal(project.CreatedAt)},
		{"Updated At", util.FormatLocal(project.UpdatedAt)},
	}
	PrintTableNoPad(table, true)
	return nil
}

func (c ProjectsCmd) Update(ctx context.Context, in ProjectsUpdateInput) error {
	if !in.NameSet && !in.StatusSet {
		return fmt.Errorf("must provide at least one of --name or --status")
	}

	projectID, err := resolveProjectArg(ctx, c.projects, in.Identifier)
	if err != nil {
		return err
	}

	inner := kernel.UpdateProjectRequestParam{}
	if in.NameSet {
		inner.Name = param.NewOpt(in.Name)
	}
	if in.StatusSet {
		switch in.Status {
		case "active":
			inner.Status = kernel.UpdateProjectRequestStatusActive
		case "archived":
			inner.Status = kernel.UpdateProjectRequestStatusArchived
		default:
			return fmt.Errorf("--status must be one of: active, archived (got %q)", in.Status)
		}
	}

	project, err := c.projects.Update(ctx, projectID, kernel.ProjectUpdateParams{
		UpdateProjectRequest: inner,
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Updated project: %s (ID: %s)\n", project.Name, project.ID)
	table := pterm.TableData{
		{"Field", "Value"},
		{"ID", project.ID},
		{"Name", project.Name},
		{"Status", string(project.Status)},
		{"Updated At", util.FormatLocal(project.UpdatedAt)},
	}
	PrintTableNoPad(table, true)
	return nil
}

func (c ProjectsCmd) Delete(ctx context.Context, in ProjectsDeleteInput) error {
	projectID, err := resolveProjectArg(ctx, c.projects, in.Identifier)
	if err != nil {
		return err
	}

	err = c.projects.Delete(ctx, projectID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Deleted project: %s\n", projectID)
	return nil
}

func (c ProjectsCmd) LimitsGet(ctx context.Context, in ProjectsLimitsGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	projectID, err := resolveProjectArg(ctx, c.projects, in.Identifier)
	if err != nil {
		return err
	}

	limits, err := c.limits.Get(ctx, projectID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if limits == nil {
			fmt.Println("null")
			return nil
		}
		return util.PrintPrettyJSON(limits)
	}

	renderProjectLimits(limits)
	return nil
}

func (c ProjectsCmd) LimitsSet(ctx context.Context, in ProjectsLimitsSetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	projectID, err := resolveProjectArg(ctx, c.projects, in.Identifier)
	if err != nil {
		return err
	}

	inner := kernel.UpdateProjectLimitsRequestParam{}

	if in.MaxConcurrentSessions.Set {
		if in.MaxConcurrentSessions.Value < 0 {
			return fmt.Errorf("--max-concurrent-sessions must be non-negative (got %d); use 0 to remove the cap", in.MaxConcurrentSessions.Value)
		}
		inner.MaxConcurrentSessions = param.NewOpt(in.MaxConcurrentSessions.Value)
	}
	if in.MaxPersistentSessions.Set {
		if in.MaxPersistentSessions.Value < 0 {
			return fmt.Errorf("--max-persistent-sessions must be non-negative (got %d); use 0 to remove the cap", in.MaxPersistentSessions.Value)
		}
		inner.MaxPersistentSessions = param.NewOpt(in.MaxPersistentSessions.Value)
	}
	if in.MaxConcurrentInvocations.Set {
		if in.MaxConcurrentInvocations.Value < 0 {
			return fmt.Errorf("--max-concurrent-invocations must be non-negative (got %d); use 0 to remove the cap", in.MaxConcurrentInvocations.Value)
		}
		inner.MaxConcurrentInvocations = param.NewOpt(in.MaxConcurrentInvocations.Value)
	}
	if in.MaxPooledSessions.Set {
		if in.MaxPooledSessions.Value < 0 {
			return fmt.Errorf("--max-pooled-sessions must be non-negative (got %d); use 0 to remove the cap", in.MaxPooledSessions.Value)
		}
		inner.MaxPooledSessions = param.NewOpt(in.MaxPooledSessions.Value)
	}

	params := kernel.ProjectLimitUpdateParams{
		UpdateProjectLimitsRequest: inner,
	}

	limits, err := c.limits.Update(ctx, projectID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if limits == nil {
			fmt.Println("null")
			return nil
		}
		return util.PrintPrettyJSON(limits)
	}

	pterm.Success.Println("Project limits updated:")
	renderProjectLimits(limits)
	return nil
}

func renderProjectLimits(limits *kernel.ProjectLimits) {
	if limits == nil {
		pterm.Info.Println("No project limit overrides found")
		return
	}

	rows := pterm.TableData{
		{"Limit", "Value"},
		{"Max Concurrent Sessions", formatProjectLimitValue(limits.MaxConcurrentSessions, limits.JSON.MaxConcurrentSessions)},
		{"Max Persistent Sessions", formatProjectLimitValue(limits.MaxPersistentSessions, limits.JSON.MaxPersistentSessions)},
		{"Max Concurrent Invocations", formatProjectLimitValue(limits.MaxConcurrentInvocations, limits.JSON.MaxConcurrentInvocations)},
		{"Max Pooled Sessions", formatProjectLimitValue(limits.MaxPooledSessions, limits.JSON.MaxPooledSessions)},
	}
	PrintTableNoPad(rows, true)
}

func formatProjectLimitValue(value int64, field respjson.Field) string {
	if !field.Valid() {
		return "unlimited"
	}
	return fmt.Sprintf("%d", value)
}

func getProjectsHandler(cmd *cobra.Command) ProjectsCmd {
	client := getKernelClient(cmd)
	return ProjectsCmd{
		projects: &client.Projects,
		limits:   &client.Projects.Limits,
	}
}

func runProjectsList(cmd *cobra.Command, args []string) error {
	c := getProjectsHandler(cmd)
	return c.List(cmd.Context(), ProjectsListInput{})
}

func runProjectsCreate(cmd *cobra.Command, args []string) error {
	c := getProjectsHandler(cmd)
	return c.Create(cmd.Context(), ProjectsCreateInput{Name: args[0]})
}

func runProjectsGet(cmd *cobra.Command, args []string) error {
	c := getProjectsHandler(cmd)
	return c.Get(cmd.Context(), ProjectsGetInput{Identifier: args[0]})
}

func runProjectsUpdate(cmd *cobra.Command, args []string) error {
	c := getProjectsHandler(cmd)
	name, _ := cmd.Flags().GetString("name")
	status, _ := cmd.Flags().GetString("status")
	return c.Update(cmd.Context(), ProjectsUpdateInput{
		Identifier: args[0],
		Name:       name,
		NameSet:    cmd.Flags().Changed("name"),
		Status:     status,
		StatusSet:  cmd.Flags().Changed("status"),
	})
}

func runProjectsDelete(cmd *cobra.Command, args []string) error {
	c := getProjectsHandler(cmd)
	return c.Delete(cmd.Context(), ProjectsDeleteInput{Identifier: args[0]})
}

func runProjectsLimitsGet(cmd *cobra.Command, args []string) error {
	c := getProjectsHandler(cmd)
	output, _ := cmd.Flags().GetString("output")
	return c.LimitsGet(cmd.Context(), ProjectsLimitsGetInput{
		Identifier: args[0],
		Output:     output,
	})
}

func runProjectsLimitsSet(cmd *cobra.Command, args []string) error {
	c := getProjectsHandler(cmd)
	maxConcurrentSessions, _ := cmd.Flags().GetInt64("max-concurrent-sessions")
	maxPersistentSessions, _ := cmd.Flags().GetInt64("max-persistent-sessions")
	maxConcurrentInvocations, _ := cmd.Flags().GetInt64("max-concurrent-invocations")
	maxPooledSessions, _ := cmd.Flags().GetInt64("max-pooled-sessions")
	output, _ := cmd.Flags().GetString("output")

	return c.LimitsSet(cmd.Context(), ProjectsLimitsSetInput{
		Identifier: args[0],
		MaxConcurrentSessions: Int64Flag{
			Set:   cmd.Flags().Changed("max-concurrent-sessions"),
			Value: maxConcurrentSessions,
		},
		MaxPersistentSessions: Int64Flag{
			Set:   cmd.Flags().Changed("max-persistent-sessions"),
			Value: maxPersistentSessions,
		},
		MaxConcurrentInvocations: Int64Flag{
			Set:   cmd.Flags().Changed("max-concurrent-invocations"),
			Value: maxConcurrentInvocations,
		},
		MaxPooledSessions: Int64Flag{
			Set:   cmd.Flags().Changed("max-pooled-sessions"),
			Value: maxPooledSessions,
		},
		Output: output,
	})
}

func addProjectsLimitsOutputFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
}

func addProjectsLimitsSetFlags(cmd *cobra.Command) {
	cmd.Flags().Int64("max-concurrent-sessions", 0, "Maximum concurrent browser sessions (0 to remove cap)")
	cmd.Flags().Int64("max-persistent-sessions", 0, "Maximum persistent browser sessions (0 to remove cap)")
	cmd.Flags().Int64("max-concurrent-invocations", 0, "Maximum concurrent app invocations (0 to remove cap)")
	cmd.Flags().Int64("max-pooled-sessions", 0, "Maximum pooled sessions capacity (0 to remove cap)")
	addProjectsLimitsOutputFlag(cmd)
}

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
	RunE:  runProjectsList,
}

var projectsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsCreate,
}

var projectsGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get a project by ID or name",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsGet,
}

var projectsUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update a project (rename or change status)",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsUpdate,
}

var projectsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsDelete,
}

var projectsLimitsCmd = &cobra.Command{
	Use:   "limits",
	Short: "Manage project limit overrides",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var projectsLimitsGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get project limit overrides",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsLimitsGet,
}

var projectsLimitsSetCmd = &cobra.Command{
	Use:   "set <id-or-name>",
	Short: "Set project limit overrides",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsLimitsSet,
}

var projectsGetLimitsCompatCmd = &cobra.Command{
	Use:    "get-limits <id-or-name>",
	Short:  "Get project limit overrides",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE:   runProjectsLimitsGet,
}

var projectsSetLimitsCompatCmd = &cobra.Command{
	Use:    "set-limits <id-or-name>",
	Short:  "Set project limit overrides",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE:   runProjectsLimitsSet,
}

func init() {
	addProjectsLimitsOutputFlag(projectsLimitsGetCmd)
	addProjectsLimitsSetFlags(projectsLimitsSetCmd)
	addProjectsLimitsOutputFlag(projectsGetLimitsCompatCmd)
	addProjectsLimitsSetFlags(projectsSetLimitsCompatCmd)

	projectsLimitsCmd.AddCommand(projectsLimitsGetCmd)
	projectsLimitsCmd.AddCommand(projectsLimitsSetCmd)

	projectsUpdateCmd.Flags().String("name", "", "New project name")
	projectsUpdateCmd.Flags().String("status", "", "New project status (active, archived)")

	projectsCmd.AddCommand(projectsListCmd)
	projectsCmd.AddCommand(projectsCreateCmd)
	projectsCmd.AddCommand(projectsGetCmd)
	projectsCmd.AddCommand(projectsUpdateCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectsLimitsCmd)
	projectsCmd.AddCommand(projectsGetLimitsCompatCmd)
	projectsCmd.AddCommand(projectsSetLimitsCompatCmd)
}
