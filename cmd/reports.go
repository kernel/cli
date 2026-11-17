package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// ReportsService defines the report SDK methods exposed by the CLI.
type ReportsService interface {
	New(ctx context.Context, body kernel.ReportNewParams, opts ...option.RequestOption) (*kernel.Report, error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Report, error)
	List(ctx context.Context, query kernel.ReportListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Report], error)
}

type ReportsCmd struct {
	reports ReportsService
}

type ReportsCreateInput struct {
	TaskID        string
	Type          string
	Data          string
	OccurredAt    string
	SchemaVersion int64
	SessionID     string
	Output        string
}

type ReportsGetInput struct {
	ID     string
	Output string
}

type ReportsListInput struct {
	TaskID  string
	Type    string
	Page    int
	PerPage int
	Output  string
}

func parseReportData(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var data map[string]any
	if err := decoder.Decode(&data); err != nil || data == nil {
		return nil, fmt.Errorf("--data must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("--data must contain exactly one JSON object")
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("--data must be a JSON object")
	}
	if len(encoded) > 16*1024 {
		return nil, fmt.Errorf("--data must encode to at most 16 KB of JSON")
	}
	return data, nil
}

func (c ReportsCmd) Create(ctx context.Context, in ReportsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return fmt.Errorf("--task-id is required")
	}
	if strings.TrimSpace(in.Type) == "" {
		return fmt.Errorf("--type is required")
	}
	data, err := parseReportData(in.Data)
	if err != nil {
		return err
	}
	params := kernel.ReportNewParams{TaskID: in.TaskID, Type: in.Type, Data: data}
	if in.OccurredAt != "" {
		occurredAt, err := parseRFC3339Flag("--occurred-at", in.OccurredAt)
		if err != nil {
			return err
		}
		params.OccurredAt = kernel.Opt(occurredAt)
	}
	if in.SchemaVersion != 0 {
		if in.SchemaVersion < 1 {
			return fmt.Errorf("--schema-version must be at least 1")
		}
		params.SchemaVersion = kernel.Opt(in.SchemaVersion)
	}
	if in.SessionID != "" {
		params.SessionID = kernel.Opt(in.SessionID)
	}

	report, err := c.reports.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(report)
	}
	pterm.Success.Printf("Submitted report: %s\n", report.ID)
	return printReportDetail(report)
}

func (c ReportsCmd) Get(ctx context.Context, in ReportsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	report, err := c.reports.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(report)
	}
	return printReportDetail(report)
}

func (c ReportsCmd) List(ctx context.Context, in ReportsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return fmt.Errorf("--task-id is required")
	}
	page, perPage := normalizePageInput(in.Page, in.PerPage)
	params := kernel.ReportListParams{
		TaskID: in.TaskID,
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}
	if in.Type != "" {
		params.Type = kernel.Opt(in.Type)
	}
	result, err := c.reports.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	var items []kernel.Report
	if result != nil {
		items = result.Items
	}
	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	if in.Output == "json" {
		return util.PrintPrettyJSONSlice(items)
	}

	if len(items) == 0 {
		pterm.Info.Println("No reports found")
	} else {
		rows := pterm.TableData{{"Report ID", "Type", "Schema Version", "Browser ID", "Occurred At", "Submitted At"}}
		for _, report := range items {
			rows = append(rows, []string{
				report.ID,
				report.Type,
				fmt.Sprint(report.SchemaVersion),
				util.OrDash(report.SessionID),
				util.FormatLocal(report.OccurredAt),
				util.FormatLocal(report.CreatedAt),
			})
		}
		PrintTableNoPad(rows, true)
	}

	next := fmt.Sprintf("kernel reports list --page %d --per-page %d --task-id %q", page+1, perPage, in.TaskID)
	if in.Type != "" {
		next += fmt.Sprintf(" --type %q", in.Type)
	}
	printListPaginationFooter(page, perPage, len(items), hasMore, next)
	return nil
}

func printReportDetail(report *kernel.Report) error {
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", report.ID},
		{"Task ID", report.TaskID},
		{"Browser ID", util.OrDash(report.SessionID)},
		{"Type", report.Type},
		{"Schema Version", fmt.Sprint(report.SchemaVersion)},
		{"Occurred At", util.FormatLocal(report.OccurredAt)},
		{"Submitted At", util.FormatLocal(report.CreatedAt)},
	}
	PrintTableNoPad(rows, true)
	data, err := json.MarshalIndent(report.Data, "", "  ")
	if err != nil {
		return err
	}
	pterm.Info.Println("Data:")
	fmt.Println(string(data))
	return nil
}

var reportsCmd = &cobra.Command{
	Use:     "reports",
	Aliases: []string{"report"},
	Short:   "Submit and inspect typed task reports",
	Long:    "Submit immutable, schema-validated reports against tasks and list the reports already submitted.",
}

var reportsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Submit a typed report against a task",
	Long:  "Submit a report whose --data matches the selected report type schema. A validation error identifies each invalid field, and a task can have at most 500 reports.",
	Args:  cobra.NoArgs,
	RunE:  runReportsCreate,
}

var reportsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a report",
	Args:  cobra.ExactArgs(1),
	RunE:  runReportsGet,
}

var reportsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List a task's reports, oldest first",
	Args:  cobra.NoArgs,
	RunE:  runReportsList,
}

func init() {
	reportsCmd.AddCommand(reportsCreateCmd, reportsGetCmd, reportsListCmd)

	reportsCreateCmd.Flags().String("task-id", "", "ID of the task to report against (required)")
	reportsCreateCmd.Flags().String("type", "", "Report type name from kernel report-types list (required; maximum 128 characters)")
	reportsCreateCmd.Flags().String("data", "", "Report data as a JSON object matching the report type schema (maximum 16 KB encoded; required unless --data-file is used)")
	reportsCreateCmd.Flags().String("data-file", "", "Read report data as a JSON object from a file, or '-' for stdin (maximum 16 KB encoded)")
	reportsCreateCmd.Flags().String("occurred-at", "", "When the reported event happened as RFC3339; defaults to submission time and must be within 30 days past to 5 minutes future")
	reportsCreateCmd.Flags().Int64("schema-version", 0, "Report type schema version; defaults to the latest non-retired version")
	reportsCreateCmd.Flags().String("session-id", "", "Browser ID where the event was observed; deleted browsers are accepted and are linked to the task if needed")
	reportsCreateCmd.MarkFlagsOneRequired("data", "data-file")
	reportsCreateCmd.MarkFlagsMutuallyExclusive("data", "data-file")
	_ = reportsCreateCmd.MarkFlagRequired("task-id")
	_ = reportsCreateCmd.MarkFlagRequired("type")
	addJSONOutputFlag(reportsCreateCmd)

	addJSONOutputFlag(reportsGetCmd)

	reportsListCmd.Flags().String("task-id", "", "Task whose reports to list (required)")
	reportsListCmd.Flags().String("type", "", "Only return reports of this type")
	reportsListCmd.Flags().Int("page", 1, "Page number (1-based)")
	reportsListCmd.Flags().Int("per-page", 20, "Items per page (default 20)")
	_ = reportsListCmd.MarkFlagRequired("task-id")
	addJSONOutputFlag(reportsListCmd)
}

func runReportsCreate(cmd *cobra.Command, args []string) error {
	taskID, _ := cmd.Flags().GetString("task-id")
	typeName, _ := cmd.Flags().GetString("type")
	data, _ := cmd.Flags().GetString("data")
	if cmd.Flags().Changed("data-file") {
		path, _ := cmd.Flags().GetString("data-file")
		var contents []byte
		var err error
		if path == "-" {
			contents, err = io.ReadAll(cmd.InOrStdin())
		} else {
			contents, err = os.ReadFile(path)
		}
		if err != nil {
			return fmt.Errorf("failed to read report data file: %w", err)
		}
		data = string(contents)
	}
	occurredAt, _ := cmd.Flags().GetString("occurred-at")
	schemaVersion, _ := cmd.Flags().GetInt64("schema-version")
	sessionID, _ := cmd.Flags().GetString("session-id")
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (ReportsCmd{reports: &client.Reports}).Create(cmd.Context(), ReportsCreateInput{
		TaskID: taskID, Type: typeName, Data: data, OccurredAt: occurredAt,
		SchemaVersion: schemaVersion, SessionID: sessionID, Output: output,
	})
}

func runReportsGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (ReportsCmd{reports: &client.Reports}).Get(cmd.Context(), ReportsGetInput{ID: args[0], Output: output})
}

func runReportsList(cmd *cobra.Command, args []string) error {
	taskID, _ := cmd.Flags().GetString("task-id")
	typeName, _ := cmd.Flags().GetString("type")
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	output, _ := cmd.Flags().GetString("output")
	client := getKernelClient(cmd)
	return (ReportsCmd{reports: &client.Reports}).List(cmd.Context(), ReportsListInput{TaskID: taskID, Type: typeName, Page: page, PerPage: perPage, Output: output})
}
