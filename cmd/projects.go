package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

type ProjectsListInput struct {
	Limit  int
	Offset int
	Name   string
	Query  string
	Output string
}

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
	Output     string
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Limit < 1 || in.Limit > 100 {
		return fmt.Errorf("--limit must be between 1 and 100")
	}
	if in.Offset < 0 {
		return fmt.Errorf("--offset must be non-negative")
	}

	params := kernel.ProjectListParams{
		Limit:  param.NewOpt(int64(in.Limit)),
		Offset: param.NewOpt(int64(in.Offset)),
	}
	if in.Name != "" {
		params.Name = param.NewOpt(in.Name)
	}
	if in.Query != "" {
		params.Query = param.NewOpt(in.Query)
	}

	var response *http.Response
	projects, err := c.projects.List(ctx, params, option.WithResponseInto(&response))
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	items := make([]kernel.Project, 0)
	if projects != nil {
		items = projects.Items
	}
	pagination, err := parseProjectListPagination(response)
	if err != nil {
		return err
	}

	if in.Output == "json" {
		data, err := marshalProjectsListJSON(projects, pagination.NextOffset)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(items) == 0 {
		pterm.Info.Println("No projects found")
		return nil
	}

	table := pterm.TableData{{"ID", "Name", "Status", "Created At", "idx"}}
	for i, p := range items {
		table = append(table, []string{
			p.ID,
			p.Name,
			string(p.Status),
			util.FormatLocal(p.CreatedAt),
			strconv.Itoa(in.Offset + i),
		})
	}
	PrintTableNoPad(table, true)

	if pagination.HasMore {
		filters := ""
		if in.Name != "" {
			filters += fmt.Sprintf(" --name %q", in.Name)
		}
		if in.Query != "" {
			filters += fmt.Sprintf(" --query %q", in.Query)
		}
		pterm.Warning.Printfln(
			"Output truncated after index %d. Continue with: kernel projects list --limit %d --offset %d%s",
			in.Offset+len(items)-1, in.Limit, pagination.NextOffset, filters,
		)
	}
	return nil
}

type projectListPagination struct {
	HasMore    bool
	NextOffset int
}

func parseProjectListPagination(response *http.Response) (projectListPagination, error) {
	if response == nil {
		return projectListPagination{}, fmt.Errorf("project list response is missing pagination headers")
	}

	hasMoreValue := response.Header.Get("X-Has-More")
	hasMore, err := strconv.ParseBool(hasMoreValue)
	if err != nil {
		return projectListPagination{}, fmt.Errorf("invalid X-Has-More header %q", hasMoreValue)
	}

	nextOffsetValue := response.Header.Get("X-Next-Offset")
	nextOffset, err := strconv.Atoi(nextOffsetValue)
	if err != nil || nextOffset < 0 {
		return projectListPagination{}, fmt.Errorf("invalid X-Next-Offset header %q", nextOffsetValue)
	}
	if hasMore && nextOffset == 0 {
		return projectListPagination{}, fmt.Errorf("X-Has-More is true but X-Next-Offset is not positive")
	}
	if !hasMore && nextOffset != 0 {
		return projectListPagination{}, fmt.Errorf("X-Has-More is false but X-Next-Offset is %d", nextOffset)
	}

	return projectListPagination{HasMore: hasMore, NextOffset: nextOffset}, nil
}

func marshalProjectsListJSON(projects *pagination.OffsetPagination[kernel.Project], nextOffset int) ([]byte, error) {
	rawProjects := json.RawMessage("[]")
	if projects != nil && len(projects.Items) > 0 {
		rawProjects = json.RawMessage(projects.RawJSON())
	}
	payload := struct {
		Projects   json.RawMessage `json:"projects"`
		NextOffset int             `json:"next_offset,omitempty"`
	}{Projects: rawProjects, NextOffset: nextOffset}
	return json.MarshalIndent(payload, "", "  ")
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
	// The API resolves the GET path parameter by ID or by name (names are unique
	// within an organization), so pass the identifier straight through — no
	// client-side list-and-match needed. Delete and limits endpoints do not
	// resolve names, so those paths keep resolveProjectArg.
	project, err := c.projects.Get(ctx, in.Identifier)
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	inner := kernel.UpdateProjectRequestParam{}
	if in.NameSet {
		if strings.TrimSpace(in.Name) == "" {
			return fmt.Errorf("--name must not be empty")
		}
		inner.Name = param.NewOpt(in.Name)
	}
	if in.Status != "" {
		switch in.Status {
		case string(kernel.UpdateProjectRequestStatusActive), string(kernel.UpdateProjectRequestStatusArchived):
			inner.Status = kernel.UpdateProjectRequestStatus(in.Status)
		default:
			return fmt.Errorf("invalid --status %q: must be one of active, archived", in.Status)
		}
	}
	if !in.NameSet && in.Status == "" {
		return fmt.Errorf("must provide at least one of --name or --status")
	}

	// The PATCH endpoint takes an ID only, so resolve names client-side the way
	// delete and the limits endpoints do.
	projectID, err := resolveProjectArg(ctx, c.projects, in.Identifier)
	if err != nil {
		return err
	}

	project, err := c.projects.Update(ctx, projectID, kernel.ProjectUpdateParams{
		UpdateProjectRequest: inner,
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(project)
	}

	pterm.Success.Printf("Updated project: %s (ID: %s)\n", project.Name, project.ID)
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	name, _ := cmd.Flags().GetString("name")
	query, _ := cmd.Flags().GetString("query")
	output, _ := cmd.Flags().GetString("output")
	return c.List(cmd.Context(), ProjectsListInput{
		Limit:  limit,
		Offset: offset,
		Name:   name,
		Query:  query,
		Output: output,
	})
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
	output, _ := cmd.Flags().GetString("output")
	return c.Update(cmd.Context(), ProjectsUpdateInput{
		Identifier: args[0],
		Name:       name,
		NameSet:    cmd.Flags().Changed("name"),
		Status:     status,
		Output:     output,
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
	maxConcurrentInvocations, _ := cmd.Flags().GetInt64("max-concurrent-invocations")
	maxPooledSessions, _ := cmd.Flags().GetInt64("max-pooled-sessions")
	output, _ := cmd.Flags().GetString("output")

	return c.LimitsSet(cmd.Context(), ProjectsLimitsSetInput{
		Identifier: args[0],
		MaxConcurrentSessions: Int64Flag{
			Set:   cmd.Flags().Changed("max-concurrent-sessions"),
			Value: maxConcurrentSessions,
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

func addProjectsLimitsSetFlags(cmd *cobra.Command) {
	cmd.Flags().Int64("max-concurrent-sessions", 0, "Maximum concurrent browser sessions (0 to remove cap)")
	cmd.Flags().Int64("max-concurrent-invocations", 0, "Maximum concurrent app invocations (0 to remove cap)")
	cmd.Flags().Int64("max-pooled-sessions", 0, "Maximum pooled sessions capacity (0 to remove cap)")
	addJSONOutputFlag(cmd)
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
	Short: "Update a project's name or status",
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
	projectsListCmd.Flags().Int("limit", 100, "Maximum number of projects to return (1-100)")
	projectsListCmd.Flags().Int("offset", 0, "Number of projects to skip (for pagination)")
	projectsListCmd.Flags().String("name", "", "Filter by exact project name (case- and accent-insensitive)")
	projectsListCmd.Flags().String("query", "", "Search projects by name substring (case-insensitive)")
	addJSONOutputFlag(projectsListCmd)

	projectsUpdateCmd.Flags().String("name", "", "New project name (1-255 characters)")
	projectsUpdateCmd.Flags().String("status", "", "New project status: active or archived")
	addJSONOutputFlag(projectsUpdateCmd)

	addJSONOutputFlag(projectsLimitsGetCmd)
	addProjectsLimitsSetFlags(projectsLimitsSetCmd)
	addJSONOutputFlag(projectsGetLimitsCompatCmd)
	addProjectsLimitsSetFlags(projectsSetLimitsCompatCmd)

	projectsLimitsCmd.AddCommand(projectsLimitsGetCmd)
	projectsLimitsCmd.AddCommand(projectsLimitsSetCmd)

	projectsCmd.AddCommand(projectsListCmd)
	projectsCmd.AddCommand(projectsCreateCmd)
	projectsCmd.AddCommand(projectsGetCmd)
	projectsCmd.AddCommand(projectsUpdateCmd)
	projectsCmd.AddCommand(projectsDeleteCmd)
	projectsCmd.AddCommand(projectsLimitsCmd)
	projectsCmd.AddCommand(projectsGetLimitsCompatCmd)
	projectsCmd.AddCommand(projectsSetLimitsCompatCmd)
}
