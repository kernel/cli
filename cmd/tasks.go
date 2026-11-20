package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// TasksService defines the task SDK methods exposed by the CLI.
type TasksService interface {
	New(ctx context.Context, body kernel.TaskNewParams, opts ...option.RequestOption) (*kernel.Task, error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Task, error)
	List(ctx context.Context, query kernel.TaskListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Task], error)
	LinkBrowsers(ctx context.Context, id string, body kernel.TaskLinkBrowsersParams, opts ...option.RequestOption) (*kernel.Task, error)
}

type TasksCmd struct {
	tasks TasksService
}

type TasksCreateInput struct {
	Description   string
	StartedAt     string
	TargetWebsite string
	Metadata      map[string]string
	SessionIDs    []string
	Output        string
}

type TasksGetInput struct {
	ID     string
	Output string
}

type TasksListInput struct {
	Page      int
	PerPage   int
	SessionID string
	Output    string
}

type TasksLinkBrowsersInput struct {
	ID         string
	SessionIDs []string
	Output     string
}

func normalizePageInput(page, perPage int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if perPage == 0 {
		perPage = 20
	}
	if page < 1 {
		return 0, 0, fmt.Errorf("--page must be at least 1")
	}
	// The CLI requests one extra item to determine whether another page exists;
	// the API's limit is 100, so the largest display page is 99.
	if perPage < 1 || perPage > 99 {
		return 0, 0, fmt.Errorf("--per-page must be between 1 and 99")
	}
	return page, perPage, nil
}

func printListPaginationFooter(page, perPage, itemsThisPage int, hasMore bool, nextCommand string) {
	pterm.Printf("\nPage: %d Per-page: %d Items this page: %d Has more: %s\n", page, perPage, itemsThisPage, lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		pterm.Printf("Next: %s\n", nextCommand)
	}
}

func parseRFC3339Flag(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be an RFC3339 date-time", name)
	}
	return parsed, nil
}

func formatStringMap(values map[string]string) string {
	if len(values) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return strings.Join(parts, ", ")
}

func (c TasksCmd) Create(ctx context.Context, in TasksCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.Description) == "" {
		return fmt.Errorf("--description is required")
	}
	if len(in.Metadata) > 20 {
		return fmt.Errorf("--metadata accepts at most 20 keys")
	}
	if len(in.SessionIDs) > 100 {
		return fmt.Errorf("--session-id accepts at most 100 browser IDs")
	}

	params := kernel.TaskNewParams{
		Description: in.Description,
		Metadata:    kernel.TaskMetadata(in.Metadata),
		SessionIDs:  in.SessionIDs,
	}
	if in.StartedAt != "" {
		startedAt, err := parseRFC3339Flag("--started-at", in.StartedAt)
		if err != nil {
			return err
		}
		params.StartedAt = kernel.Opt(startedAt)
	}
	if in.TargetWebsite != "" {
		params.TargetWebsite = kernel.Opt(in.TargetWebsite)
	}

	task, err := c.tasks.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(task)
	}
	pterm.Success.Printf("Created task: %s\n", task.ID)
	printTaskDetail(task)
	return nil
}

func (c TasksCmd) Get(ctx context.Context, in TasksGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	task, err := c.tasks.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(task)
	}
	printTaskDetail(task)
	return nil
}

func (c TasksCmd) List(ctx context.Context, in TasksListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	page, perPage, err := normalizePageInput(in.Page, in.PerPage)
	if err != nil {
		return err
	}
	params := kernel.TaskListParams{
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}
	if in.SessionID != "" {
		params.SessionID = kernel.Opt(in.SessionID)
	}

	result, err := c.tasks.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	var items []kernel.Task
	if result != nil {
		items = result.Items
	}
	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	if in.Output == "json" {
		return util.PrintPageJSON(items, page, perPage, hasMore)
	}

	if len(items) == 0 {
		pterm.Info.Println("No tasks found")
	} else {
		rows := pterm.TableData{{"Task ID", "Description", "Target Website", "Browsers", "Started At", "Created At"}}
		for _, task := range items {
			rows = append(rows, []string{
				task.ID,
				task.Description,
				util.OrDash(task.TargetWebsite),
				fmt.Sprint(len(task.SessionIDs)),
				util.FormatLocal(task.StartedAt),
				util.FormatLocal(task.CreatedAt),
			})
		}
		PrintTableNoPad(rows, true)
	}

	next := fmt.Sprintf("kernel tasks list --page %d --per-page %d", page+1, perPage)
	if in.SessionID != "" {
		next += fmt.Sprintf(" --session-id %q", in.SessionID)
	}
	printListPaginationFooter(page, perPage, len(items), hasMore, next)
	return nil
}

func (c TasksCmd) LinkBrowsers(ctx context.Context, in TasksLinkBrowsersInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if len(in.SessionIDs) == 0 {
		return fmt.Errorf("at least one --session-id is required")
	}
	if len(in.SessionIDs) > 100 {
		return fmt.Errorf("--session-id accepts at most 100 browser IDs")
	}
	task, err := c.tasks.LinkBrowsers(ctx, in.ID, kernel.TaskLinkBrowsersParams{SessionIDs: in.SessionIDs})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(task)
	}
	pterm.Success.Printf("Linked browsers to task: %s\n", task.ID)
	printTaskDetail(task)
	return nil
}

func printTaskDetail(task *kernel.Task) {
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", task.ID},
		{"Description", task.Description},
		{"Target Website", util.OrDash(task.TargetWebsite)},
		{"Metadata", formatStringMap(task.Metadata)},
		{"Browser IDs", util.OrDash(strings.Join(task.SessionIDs, ", "))},
		{"Started At", util.FormatLocal(task.StartedAt)},
		{"Created At", util.FormatLocal(task.CreatedAt)},
	}
	PrintTableNoPad(rows, true)
}

var tasksCmd = &cobra.Command{
	Use:     "tasks",
	Aliases: []string{"task"},
	Short:   "Define tasks and link the browsers used for them",
	Long:    "Define goals that agents work toward, link the browsers they use, and retrieve task records for reporting.",
}

var tasksCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a task",
	Long:  "Define a task before or while an agent works on it. Browsers may be linked now with --session-id or later with kernel tasks link-browsers. Submit reports against the returned task ID.",
	Args:  cobra.NoArgs,
	RunE:  runTasksCreate,
}

var tasksGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a task and its linked browser IDs",
	Args:  cobra.ExactArgs(1),
	RunE:  runTasksGet,
}

var tasksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks, newest first",
	Long:  "List tasks in the selected project, newest first. Use --session-id to show only tasks linked to one browser.",
	Args:  cobra.NoArgs,
	RunE:  runTasksList,
}

var tasksLinkBrowsersCmd = &cobra.Command{
	Use:   "link-browsers <id>",
	Short: "Link browsers to a task",
	Long:  "Link one or more browsers to an existing task. Browsers already linked are ignored, so the command is safe to retry. A task can have at most 100 linked browsers.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTasksLinkBrowsers,
}

func init() {
	tasksCmd.AddCommand(tasksCreateCmd, tasksGetCmd, tasksListCmd, tasksLinkBrowsersCmd)

	tasksCreateCmd.Flags().String("description", "", "What the task is meant to accomplish, at most 4 KB (required)")
	tasksCreateCmd.Flags().String("started-at", "", "When work started as RFC3339; defaults to creation time and must be within 30 days past to 5 minutes future")
	tasksCreateCmd.Flags().String("target-website", "", "Target website hostname without a scheme or path (maximum 253 characters)")
	tasksCreateCmd.Flags().StringArray("metadata", nil, "Grouping metadata as KEY=VALUE (repeatable; at most 20 keys, keys 1-64 characters, values at most 256 characters, 4 KB total)")
	tasksCreateCmd.Flags().StringSlice("session-id", nil, "Browser ID to link, including a deleted browser (repeatable; at most 100)")
	_ = tasksCreateCmd.MarkFlagRequired("description")
	addJSONOutputFlag(tasksCreateCmd)

	addJSONOutputFlag(tasksGetCmd)

	tasksListCmd.Flags().Int("page", 1, "Page number (1-based)")
	tasksListCmd.Flags().Int("per-page", 20, "Items per page (1-99, default 20)")
	tasksListCmd.Flags().String("session-id", "", "Only return tasks linked to this browser ID")
	addJSONOutputFlag(tasksListCmd)

	tasksLinkBrowsersCmd.Flags().StringSlice("session-id", nil, "Browser ID to link, including a deleted browser (repeatable; 1-100 required)")
	_ = tasksLinkBrowsersCmd.MarkFlagRequired("session-id")
	addJSONOutputFlag(tasksLinkBrowsersCmd)
}

func runTasksCreate(cmd *cobra.Command, args []string) error {
	description, _ := cmd.Flags().GetString("description")
	startedAt, _ := cmd.Flags().GetString("started-at")
	targetWebsite, _ := cmd.Flags().GetString("target-website")
	metadataValues, _ := cmd.Flags().GetStringArray("metadata")
	metadata, err := parseStringMapFlag(metadataValues, "--metadata")
	if err != nil {
		return err
	}
	sessionIDs, _ := cmd.Flags().GetStringSlice("session-id")
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (TasksCmd{tasks: &client.Tasks}).Create(cmd.Context(), TasksCreateInput{
		Description: description, StartedAt: startedAt, TargetWebsite: targetWebsite,
		Metadata: metadata, SessionIDs: sessionIDs, Output: output,
	})
}

func runTasksGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (TasksCmd{tasks: &client.Tasks}).Get(cmd.Context(), TasksGetInput{ID: args[0], Output: output})
}

func runTasksList(cmd *cobra.Command, args []string) error {
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	sessionID, _ := cmd.Flags().GetString("session-id")
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (TasksCmd{tasks: &client.Tasks}).List(cmd.Context(), TasksListInput{Page: page, PerPage: perPage, SessionID: sessionID, Output: output})
}

func runTasksLinkBrowsers(cmd *cobra.Command, args []string) error {
	sessionIDs, _ := cmd.Flags().GetStringSlice("session-id")
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (TasksCmd{tasks: &client.Tasks}).LinkBrowsers(cmd.Context(), TasksLinkBrowsersInput{ID: args[0], SessionIDs: sessionIDs, Output: output})
}
