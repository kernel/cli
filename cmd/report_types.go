package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// ReportTypesService defines the report type SDK methods exposed by the CLI.
type ReportTypesService interface {
	Get(ctx context.Context, typeName string, opts ...option.RequestOption) (*kernel.ReportType, error)
	List(ctx context.Context, query kernel.ReportTypeListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ReportTypeSummary], error)
}

type ReportTypeVersionsService interface {
	Get(ctx context.Context, version int64, query kernel.ReportTypeVersionGetParams, opts ...option.RequestOption) (*kernel.ReportTypeVersion, error)
}

type ReportTypesCmd struct {
	reportTypes ReportTypesService
	versions    ReportTypeVersionsService
}

type ReportTypesListInput struct {
	TaskID  string
	Page    int
	PerPage int
	Output  string
}

type ReportTypesGetInput struct {
	Type   string
	Output string
}

type ReportTypeVersionsGetInput struct {
	Type    string
	Version int64
	Output  string
}

func (c ReportTypesCmd) List(ctx context.Context, in ReportTypesListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return fmt.Errorf("--task-id is required")
	}
	page, perPage, err := normalizePageInput(in.Page, in.PerPage)
	if err != nil {
		return err
	}
	result, err := c.reportTypes.List(ctx, kernel.ReportTypeListParams{
		TaskID: in.TaskID,
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	var items []kernel.ReportTypeSummary
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
		pterm.Info.Println("No report types found")
	} else {
		rows := pterm.TableData{{"Type", "Latest Version", "Description"}}
		for _, reportType := range items {
			rows = append(rows, []string{reportType.Type, fmt.Sprint(reportType.LatestVersion), reportType.Description})
		}
		PrintTableNoPad(rows, true)
	}

	next := fmt.Sprintf("kernel report-types list --page %d --per-page %d --task-id %q", page+1, perPage, in.TaskID)
	printListPaginationFooter(page, perPage, len(items), hasMore, next)
	return nil
}

func (c ReportTypesCmd) Get(ctx context.Context, in ReportTypesGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	reportType, err := c.reportTypes.Get(ctx, in.Type)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(reportType)
	}
	return printReportTypeDetail(reportType)
}

func (c ReportTypesCmd) GetVersion(ctx context.Context, in ReportTypeVersionsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Version < 1 {
		return fmt.Errorf("version must be at least 1")
	}
	version, err := c.versions.Get(ctx, in.Version, kernel.ReportTypeVersionGetParams{Type: in.Type})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(version)
	}
	rows := pterm.TableData{
		{"Property", "Value"},
		{"Type", version.Type},
		{"Version", fmt.Sprint(version.Version)},
		{"Retired", strconv.FormatBool(version.Retired)},
	}
	PrintTableNoPad(rows, true)
	return printReportSchema(version.Schema)
}

func printReportTypeDetail(reportType *kernel.ReportType) error {
	versions := make([]string, 0, len(reportType.Versions))
	for _, version := range reportType.Versions {
		label := fmt.Sprint(version.Version)
		if version.Retired {
			label += " (retired)"
		}
		versions = append(versions, label)
	}
	rows := pterm.TableData{
		{"Property", "Value"},
		{"Type", reportType.Type},
		{"Description", reportType.Description},
		{"Latest Version", fmt.Sprint(reportType.LatestVersion)},
		{"Versions", util.OrDash(strings.Join(versions, ", "))},
	}
	PrintTableNoPad(rows, true)
	return printReportSchema(reportType.Schema)
}

func printReportSchema(schema map[string]any) error {
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	pterm.Info.Println("Schema:")
	fmt.Println(string(data))
	return nil
}

var reportTypesCmd = &cobra.Command{
	Use:     "report-types",
	Aliases: []string{"report-type"},
	Short:   "Discover report types and their schemas",
	Long:    "Discover the report types available for a task and inspect each immutable JSON Schema version before submitting reports.",
}

var reportTypesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List report types available for a task",
	Long:  "List the report types an agent can submit for a task. Use kernel report-types get to inspect the latest JSON Schema for a type.",
	Args:  cobra.NoArgs,
	RunE:  runReportTypesList,
}

var reportTypesGetCmd = &cobra.Command{
	Use:   "get <type>",
	Short: "Get a report type and its latest schema",
	Long:  "Get a report type, all of its versions, and the JSON Schema for its latest non-retired version.",
	Args:  cobra.ExactArgs(1),
	RunE:  runReportTypesGet,
}

var reportTypeVersionsCmd = &cobra.Command{
	Use:   "versions",
	Short: "Inspect immutable report type schema versions",
}

var reportTypeVersionsGetCmd = &cobra.Command{
	Use:   "get <type> <version>",
	Short: "Get one report type schema version",
	Long:  "Get one immutable JSON Schema version of a report type, including a retired version.",
	Args:  cobra.ExactArgs(2),
	RunE:  runReportTypeVersionsGet,
}

func init() {
	reportTypesCmd.AddCommand(reportTypesListCmd, reportTypesGetCmd, reportTypeVersionsCmd)
	reportTypeVersionsCmd.AddCommand(reportTypeVersionsGetCmd)

	reportTypesListCmd.Flags().String("task-id", "", "Task the agent is finding a report type for (required)")
	reportTypesListCmd.Flags().Int("page", 1, "Page number (1-based)")
	reportTypesListCmd.Flags().Int("per-page", 20, "Items per page (1-99, default 20)")
	_ = reportTypesListCmd.MarkFlagRequired("task-id")
	addJSONOutputFlag(reportTypesListCmd)
	addJSONOutputFlag(reportTypesGetCmd)
	addJSONOutputFlag(reportTypeVersionsGetCmd)
}

func runReportTypesList(cmd *cobra.Command, args []string) error {
	taskID, _ := cmd.Flags().GetString("task-id")
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (ReportTypesCmd{reportTypes: &client.ReportTypes}).List(cmd.Context(), ReportTypesListInput{TaskID: taskID, Page: page, PerPage: perPage, Output: output})
}

func runReportTypesGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (ReportTypesCmd{reportTypes: &client.ReportTypes}).Get(cmd.Context(), ReportTypesGetInput{Type: args[0], Output: output})
}

func runReportTypeVersionsGet(cmd *cobra.Command, args []string) error {
	version, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("version must be an integer of at least 1")
	}
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (ReportTypesCmd{versions: &client.ReportTypes.Versions}).GetVersion(cmd.Context(), ReportTypeVersionsGetInput{Type: args[0], Version: version, Output: output})
}
