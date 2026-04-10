package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// ProjectsService defines the subset of the Kernel SDK project client that we use.
type ProjectsService interface {
	New(ctx context.Context, body kernel.ProjectNewParams, opts ...option.RequestOption) (res *kernel.Project, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.Project, err error)
	Update(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (res *kernel.Project, err error)
	List(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.Project], err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) error
}

// ProjectLimitsService defines the subset of the Kernel SDK project limits client that we use.
type ProjectLimitsService interface {
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.ProjectLimits, err error)
	Update(ctx context.Context, id string, body kernel.ProjectLimitUpdateParams, opts ...option.RequestOption) (res *kernel.ProjectLimits, err error)
}

// ProjectsCmd handles project operations independent of cobra.
type ProjectsCmd struct {
	projects ProjectsService
	limits   ProjectLimitsService
}

type ProjectsListInput struct {
	Output  string
	Page    int
	PerPage int
}

type ProjectsGetInput struct {
	ID     string
	Output string
}

type ProjectsCreateInput struct {
	Name   string
	Output string
}

type ProjectsUpdateInput struct {
	ID     string
	Name   string
	Status string
	Output string
}

type ProjectsDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type ProjectLimitsGetInput struct {
	ID     string
	Output string
}

type ProjectLimitsUpdateInput struct {
	ID                       string
	MaxConcurrentInvocations Int64Flag
	MaxConcurrentSessions    Int64Flag
	MaxPersistentSessions    Int64Flag
	MaxPooledSessions        Int64Flag
	Output                   string
}

func (p ProjectsCmd) List(ctx context.Context, in ProjectsListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	page := in.Page
	perPage := in.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}

	params := kernel.ProjectListParams{
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}

	result, err := p.projects.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var items []kernel.Project
	if result != nil {
		items = result.Items
	}

	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	itemsThisPage := len(items)

	if in.Output == "json" {
		if len(items) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Println("No projects found")
		return nil
	}

	rows := pterm.TableData{{"Project ID", "Name", "Status", "Created At", "Updated At"}}
	for _, project := range items {
		rows = append(rows, []string{
			project.ID,
			project.Name,
			string(project.Status),
			util.FormatLocal(project.CreatedAt),
			util.FormatLocal(project.UpdatedAt),
		})
	}
	PrintTableNoPad(rows, true)

	pterm.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, itemsThisPage, lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		pterm.Printf("Next: kernel projects list --page %d --per-page %d\n", page+1, perPage)
	}

	return nil
}

func (p ProjectsCmd) Get(ctx context.Context, in ProjectsGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	project, err := p.projects.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(project)
	}

	rows := projectTable(project)
	PrintTableNoPad(rows, true)
	return nil
}

func (p ProjectsCmd) Create(ctx context.Context, in ProjectsCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}
	if in.Name == "" {
		return fmt.Errorf("--name is required")
	}

	project, err := p.projects.New(ctx, kernel.ProjectNewParams{
		CreateProjectRequest: kernel.CreateProjectRequestParam{Name: in.Name},
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(project)
	}

	pterm.Success.Printf("Created project: %s\n", project.ID)
	PrintTableNoPad(projectTable(project), true)
	return nil
}

func (p ProjectsCmd) Update(ctx context.Context, in ProjectsUpdateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.ProjectUpdateParams{
		UpdateProjectRequest: kernel.UpdateProjectRequestParam{},
	}
	changed := false

	if in.Name != "" {
		params.UpdateProjectRequest.Name = kernel.Opt(in.Name)
		changed = true
	}

	if in.Status != "" {
		status, err := parseProjectStatus(in.Status)
		if err != nil {
			return err
		}
		params.UpdateProjectRequest.Status = status
		changed = true
	}

	if !changed {
		return fmt.Errorf("at least one of --name or --status is required")
	}

	project, err := p.projects.Update(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(project)
	}

	pterm.Success.Printf("Updated project: %s\n", project.ID)
	PrintTableNoPad(projectTable(project), true)
	return nil
}

func (p ProjectsCmd) Delete(ctx context.Context, in ProjectsDeleteInput) error {
	if !in.SkipConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete project '%s'? The project must be empty and this cannot be undone.", in.ID)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := p.projects.Delete(ctx, in.ID); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Project '%s' not found\n", in.ID)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Deleted project: %s\n", in.ID)
	return nil
}

func (p ProjectsCmd) GetLimits(ctx context.Context, in ProjectLimitsGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	limits, err := p.limits.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(limits)
	}

	PrintTableNoPad(projectLimitsTable(limits), true)
	return nil
}

func (p ProjectsCmd) UpdateLimits(ctx context.Context, in ProjectLimitsUpdateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.ProjectLimitUpdateParams{
		UpdateProjectLimitsRequest: kernel.UpdateProjectLimitsRequestParam{},
	}
	changed := false

	if in.MaxConcurrentInvocations.Set {
		params.UpdateProjectLimitsRequest.MaxConcurrentInvocations = kernel.Opt(in.MaxConcurrentInvocations.Value)
		changed = true
	}
	if in.MaxConcurrentSessions.Set {
		params.UpdateProjectLimitsRequest.MaxConcurrentSessions = kernel.Opt(in.MaxConcurrentSessions.Value)
		changed = true
	}
	if in.MaxPersistentSessions.Set {
		params.UpdateProjectLimitsRequest.MaxPersistentSessions = kernel.Opt(in.MaxPersistentSessions.Value)
		changed = true
	}
	if in.MaxPooledSessions.Set {
		params.UpdateProjectLimitsRequest.MaxPooledSessions = kernel.Opt(in.MaxPooledSessions.Value)
		changed = true
	}

	if !changed {
		return fmt.Errorf("at least one limit flag is required")
	}

	limits, err := p.limits.Update(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(limits)
	}

	pterm.Success.Printf("Updated project limits for: %s\n", in.ID)
	PrintTableNoPad(projectLimitsTable(limits), true)
	return nil
}

func parseProjectStatus(status string) (kernel.UpdateProjectRequestStatus, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return kernel.UpdateProjectRequestStatusActive, nil
	case "archived":
		return kernel.UpdateProjectRequestStatusArchived, nil
	default:
		return "", fmt.Errorf("invalid --status value: %s (must be 'active' or 'archived')", status)
	}
}

func projectTable(project *kernel.Project) pterm.TableData {
	return pterm.TableData{
		{"Property", "Value"},
		{"ID", project.ID},
		{"Name", project.Name},
		{"Status", string(project.Status)},
		{"Created At", util.FormatLocal(project.CreatedAt)},
		{"Updated At", util.FormatLocal(project.UpdatedAt)},
	}
}

func projectLimitsTable(limits *kernel.ProjectLimits) pterm.TableData {
	return pterm.TableData{
		{"Property", "Value"},
		{"Max Concurrent Invocations", formatProjectLimit(limits.MaxConcurrentInvocations)},
		{"Max Concurrent Sessions", formatProjectLimit(limits.MaxConcurrentSessions)},
		{"Max Persistent Sessions", formatProjectLimit(limits.MaxPersistentSessions)},
		{"Max Pooled Sessions", formatProjectLimit(limits.MaxPooledSessions)},
	}
}

func formatProjectLimit(value int64) string {
	if value == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

var projectsCmd = &cobra.Command{
	Use:     "projects",
	Aliases: []string{"project"},
	Short:   "Manage organization projects",
	Long:    "Commands for managing Kernel projects and project-level resource limits",
}

var projectsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	Args:  cobra.NoArgs,
	RunE:  runProjectsList,
}

var projectsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a project by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsGet,
}

var projectsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project",
	Args:  cobra.NoArgs,
	RunE:  runProjectsCreate,
}

var projectsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a project's name or status",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsUpdate,
}

var projectsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectsDelete,
}

var projectLimitsCmd = &cobra.Command{
	Use:   "limits",
	Short: "Manage project resource limits",
}

var projectLimitsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get project resource limits",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectLimitsGet,
}

var projectLimitsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update project resource limits",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectLimitsUpdate,
}

func init() {
	projectsCmd.AddCommand(projectsListCmd)
	projectsCmd.AddCommand(projectsGetCmd)
	projectsCmd.AddCommand(projectsCreateCmd)
	projectsCmd.AddCommand(projectsUpdateCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectLimitsCmd)

	projectLimitsCmd.AddCommand(projectLimitsGetCmd)
	projectLimitsCmd.AddCommand(projectLimitsUpdateCmd)

	projectsListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	projectsListCmd.Flags().Int("per-page", 20, "Items per page (default 20)")
	projectsListCmd.Flags().Int("page", 1, "Page number (1-based)")

	projectsGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	projectsCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	projectsCreateCmd.Flags().String("name", "", "Project name")
	_ = projectsCreateCmd.MarkFlagRequired("name")

	projectsUpdateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	projectsUpdateCmd.Flags().String("name", "", "New project name")
	projectsUpdateCmd.Flags().String("status", "", "New project status: active or archived")

	projectsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	projectLimitsGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	projectLimitsUpdateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	projectLimitsUpdateCmd.Flags().Int64("max-concurrent-invocations", 0, "Maximum concurrent app invocations for this project; set to 0 to remove the cap")
	projectLimitsUpdateCmd.Flags().Int64("max-concurrent-sessions", 0, "Maximum concurrent browser sessions for this project; set to 0 to remove the cap")
	projectLimitsUpdateCmd.Flags().Int64("max-persistent-sessions", 0, "Maximum persistent browser sessions for this project; set to 0 to remove the cap")
	projectLimitsUpdateCmd.Flags().Int64("max-pooled-sessions", 0, "Maximum pooled browser sessions for this project; set to 0 to remove the cap")
}

func runProjectsList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")

	projectSvc := client.Projects
	limitSvc := client.Projects.Limits
	p := ProjectsCmd{projects: &projectSvc, limits: &limitSvc}
	return p.List(cmd.Context(), ProjectsListInput{
		Output:  output,
		Page:    page,
		PerPage: perPage,
	})
}

func runProjectsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	projectSvc := client.Projects
	limitSvc := client.Projects.Limits
	p := ProjectsCmd{projects: &projectSvc, limits: &limitSvc}
	return p.Get(cmd.Context(), ProjectsGetInput{
		ID:     args[0],
		Output: output,
	})
}

func runProjectsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")

	projectSvc := client.Projects
	limitSvc := client.Projects.Limits
	p := ProjectsCmd{projects: &projectSvc, limits: &limitSvc}
	return p.Create(cmd.Context(), ProjectsCreateInput{
		Name:   name,
		Output: output,
	})
}

func runProjectsUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	status, _ := cmd.Flags().GetString("status")

	projectSvc := client.Projects
	limitSvc := client.Projects.Limits
	p := ProjectsCmd{projects: &projectSvc, limits: &limitSvc}
	return p.Update(cmd.Context(), ProjectsUpdateInput{
		ID:     args[0],
		Name:   name,
		Status: status,
		Output: output,
	})
}

func runProjectsDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	projectSvc := client.Projects
	limitSvc := client.Projects.Limits
	p := ProjectsCmd{projects: &projectSvc, limits: &limitSvc}
	return p.Delete(cmd.Context(), ProjectsDeleteInput{
		ID:          args[0],
		SkipConfirm: skipConfirm,
	})
}

func runProjectLimitsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	projectSvc := client.Projects
	limitSvc := client.Projects.Limits
	p := ProjectsCmd{projects: &projectSvc, limits: &limitSvc}
	return p.GetLimits(cmd.Context(), ProjectLimitsGetInput{
		ID:     args[0],
		Output: output,
	})
}

func runProjectLimitsUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	maxConcurrentInvocations, _ := cmd.Flags().GetInt64("max-concurrent-invocations")
	maxConcurrentSessions, _ := cmd.Flags().GetInt64("max-concurrent-sessions")
	maxPersistentSessions, _ := cmd.Flags().GetInt64("max-persistent-sessions")
	maxPooledSessions, _ := cmd.Flags().GetInt64("max-pooled-sessions")

	projectSvc := client.Projects
	limitSvc := client.Projects.Limits
	p := ProjectsCmd{projects: &projectSvc, limits: &limitSvc}
	return p.UpdateLimits(cmd.Context(), ProjectLimitsUpdateInput{
		ID: args[0],
		MaxConcurrentInvocations: Int64Flag{
			Set:   cmd.Flags().Changed("max-concurrent-invocations"),
			Value: maxConcurrentInvocations,
		},
		MaxConcurrentSessions: Int64Flag{
			Set:   cmd.Flags().Changed("max-concurrent-sessions"),
			Value: maxConcurrentSessions,
		},
		MaxPersistentSessions: Int64Flag{
			Set:   cmd.Flags().Changed("max-persistent-sessions"),
			Value: maxPersistentSessions,
		},
		MaxPooledSessions: Int64Flag{
			Set:   cmd.Flags().Changed("max-pooled-sessions"),
			Value: maxPooledSessions,
		},
		Output: output,
	})
}
