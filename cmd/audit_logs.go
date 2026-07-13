package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type AuditLogsService interface {
	ListAutoPaging(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry]
}

type AuditLogsCmd struct {
	auditLogs AuditLogsService
}

type AuditLogsSearchInput struct {
	Start         string
	End           string
	Search        string
	Method        string
	ExcludeMethod string
	IncludeGet    bool
	Service       string
	AuthStrategy  string
	UserIDs       []string
	Limit         int
	Output        string
}

const auditLogsMaxPageSize = 100

func (c AuditLogsCmd) Search(ctx context.Context, in AuditLogsSearchInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Limit < 1 {
		return fmt.Errorf("--limit must be positive")
	}
	var err error
	start := time.Now().UTC().Add(-24 * time.Hour)
	if in.Start != "" {
		start, err = parseAuditLogTime(in.Start)
		if err != nil {
			return fmt.Errorf("--start: %w", err)
		}
	}
	end := time.Now().UTC()
	if in.End != "" {
		end, err = parseAuditLogTime(in.End)
		if err != nil {
			return fmt.Errorf("--end: %w", err)
		}
	}
	if !start.Before(end) {
		return fmt.Errorf("--start must be before --end")
	}

	params := kernel.AuditLogListParams{Start: start, End: end}
	if in.Search != "" {
		params.Search = kernel.String(in.Search)
	}
	if in.Method != "" {
		params.Method = kernel.String(in.Method)
	}
	excludeMethods := auditLogExcludeMethods(in.Method, in.ExcludeMethod, in.IncludeGet)
	if len(excludeMethods) > 0 {
		params.ExcludeMethod = excludeMethods
	}
	if in.Service != "" {
		params.Service = kernel.String(in.Service)
	}
	if in.AuthStrategy != "" {
		params.AuthStrategy = kernel.String(in.AuthStrategy)
	}
	if len(in.UserIDs) > 0 {
		params.SearchUserID = in.UserIDs
	}
	params.Limit = kernel.Int(int64(min(in.Limit, auditLogsMaxPageSize)))

	entries := make([]kernel.AuditLogEntry, 0)
	hasMore := false
	pager := c.auditLogs.ListAutoPaging(ctx, params)
	for pager.Next() {
		entries = append(entries, pager.Current())
		if len(entries) >= in.Limit {
			hasMore = pager.Next()
			break
		}
	}
	if err := pager.Err(); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSONSlice(entries)
	}

	if len(entries) == 0 {
		pterm.Info.Println("No audit log entries found")
		return nil
	}

	table := pterm.TableData{{"Timestamp", "Method", "Status", "Path", "User", "Duration (ms)", "Client IP"}}
	for _, entry := range entries {
		table = append(table, []string{
			util.FormatLocal(entry.Timestamp),
			entry.Method,
			strconv.FormatInt(entry.Status, 10),
			entry.Path,
			formatAuditLogUser(entry),
			strconv.FormatInt(entry.DurationMs, 10),
			entry.ClientIP,
		})
	}
	PrintTableNoPad(table, true)

	if hasMore {
		pterm.Info.Printf("Showing first %d results; increase --limit or narrow the search window\n", in.Limit)
	}
	return nil
}

func auditLogExcludeMethods(method, excludeMethod string, includeGet bool) []string {
	if method != "" || includeGet {
		if excludeMethod == "" {
			return nil
		}
		return []string{excludeMethod}
	}
	if excludeMethod == "" || strings.EqualFold(excludeMethod, "GET") {
		return []string{"GET"}
	}
	return []string{"GET", excludeMethod}
}

func parseAuditLogTime(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (expected RFC3339 like 2026-07-01T15:04:05Z or a date like 2026-07-01)", value)
}

func formatAuditLogUser(entry kernel.AuditLogEntry) string {
	if entry.Email != "" {
		return entry.Email
	}
	if entry.UserID != "" {
		return entry.UserID
	}
	return "-"
}

func getAuditLogsHandler(cmd *cobra.Command) AuditLogsCmd {
	client := getKernelClient(cmd)
	return AuditLogsCmd{auditLogs: &client.AuditLogs}
}

func runAuditLogsSearch(cmd *cobra.Command, args []string) error {
	c := getAuditLogsHandler(cmd)
	start, _ := cmd.Flags().GetString("start")
	end, _ := cmd.Flags().GetString("end")
	search, _ := cmd.Flags().GetString("search")
	method, _ := cmd.Flags().GetString("method")
	excludeMethod, _ := cmd.Flags().GetString("exclude-method")
	includeGet, _ := cmd.Flags().GetBool("include-get")
	service, _ := cmd.Flags().GetString("service")
	authStrategy, _ := cmd.Flags().GetString("auth-strategy")
	userIDs, _ := cmd.Flags().GetStringArray("user-id")
	limit, _ := cmd.Flags().GetInt("limit")
	output, _ := cmd.Flags().GetString("output")

	return c.Search(cmd.Context(), AuditLogsSearchInput{
		Start:         start,
		End:           end,
		Search:        search,
		Method:        method,
		ExcludeMethod: excludeMethod,
		IncludeGet:    includeGet,
		Service:       service,
		AuthStrategy:  authStrategy,
		UserIDs:       userIDs,
		Limit:         limit,
		Output:        output,
	})
}

var auditLogsCmd = &cobra.Command{
	Use:     "audit-logs",
	Aliases: []string{"audit-log", "auditlogs", "auditlog"},
	Short:   "Search audit logs",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var auditLogsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search audit logs within a time window",
	Long:  "Search audit logs within a bounded time window.\n\nGET requests are excluded by default; pass --include-get to include them, or --method GET to see only them.\n\nThe API limits searches to a 30-day window and returns up to 100 records per page. Not recommended for bulk export.",
	Args:  cobra.NoArgs,
	RunE:  runAuditLogsSearch,
}

func init() {
	addJSONOutputFlag(auditLogsSearchCmd)
	auditLogsSearchCmd.Flags().String("start", "", "Start of the search window, RFC3339 or YYYY-MM-DD (default: 24 hours ago)")
	auditLogsSearchCmd.Flags().String("end", "", "Exclusive end of the search window, RFC3339 or YYYY-MM-DD (default: now)")
	auditLogsSearchCmd.Flags().String("search", "", "Free-text search")
	auditLogsSearchCmd.Flags().String("method", "", "Filter by HTTP method (e.g. GET)")
	auditLogsSearchCmd.Flags().String("exclude-method", "", "Exclude an HTTP method")
	auditLogsSearchCmd.Flags().Bool("include-get", false, "Include GET requests, which are excluded by default")
	auditLogsSearchCmd.Flags().String("service", "", "Filter by service")
	auditLogsSearchCmd.Flags().String("auth-strategy", "", "Filter by authentication strategy")
	auditLogsSearchCmd.Flags().StringArray("user-id", nil, "Filter by user ID (repeatable)")
	auditLogsSearchCmd.Flags().Int("limit", 100, "Maximum number of results to return")

	auditLogsCmd.AddCommand(auditLogsSearchCmd)
	rootCmd.AddCommand(auditLogsCmd)
}
