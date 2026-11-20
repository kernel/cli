package cmd

import (
	"context"
	"fmt"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// WorkloadIdentityAuthenticationEventsService is the subset of the SDK service used by the CLI.
type WorkloadIdentityAuthenticationEventsService interface {
	List(ctx context.Context, query kernel.WorkloadIdentityAuthenticationEventListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.WorkloadIdentityAuthenticationEvent], error)
}

type WorkloadIdentityAuthenticationEventsCmd struct {
	events WorkloadIdentityAuthenticationEventsService
}

type WorkloadIdentityAuthenticationEventsListInput struct {
	Page             int
	PerPage          int
	IssuerID         string
	FederationRuleID string
	ServiceAccountID string
	ProjectID        string
	Outcome          string
	StartTime        string
	EndTime          string
	Output           string
}

func (c WorkloadIdentityAuthenticationEventsCmd) List(ctx context.Context, in WorkloadIdentityAuthenticationEventsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	page, perPage, err := normalizePageInput(in.Page, in.PerPage)
	if err != nil {
		return err
	}
	params := kernel.WorkloadIdentityAuthenticationEventListParams{
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}
	if in.IssuerID != "" {
		params.IssuerID = kernel.Opt(in.IssuerID)
	}
	if in.FederationRuleID != "" {
		params.FederationRuleID = kernel.Opt(in.FederationRuleID)
	}
	if in.ServiceAccountID != "" {
		params.ServiceAccountID = kernel.Opt(in.ServiceAccountID)
	}
	if in.ProjectID != "" {
		params.ProjectID = kernel.Opt(in.ProjectID)
	}
	if in.Outcome != "" {
		switch in.Outcome {
		case string(kernel.WorkloadIdentityAuthenticationEventListParamsOutcomeSuccess):
			params.Outcome = kernel.WorkloadIdentityAuthenticationEventListParamsOutcomeSuccess
		case string(kernel.WorkloadIdentityAuthenticationEventListParamsOutcomeFailure):
			params.Outcome = kernel.WorkloadIdentityAuthenticationEventListParamsOutcomeFailure
		default:
			return fmt.Errorf("invalid --outcome %q: must be one of success, failure", in.Outcome)
		}
	}
	if in.StartTime != "" {
		startTime, err := parseRFC3339Flag("--start-time", in.StartTime)
		if err != nil {
			return err
		}
		params.StartTime = kernel.Opt(startTime)
	}
	if in.EndTime != "" {
		endTime, err := parseRFC3339Flag("--end-time", in.EndTime)
		if err != nil {
			return err
		}
		params.EndTime = kernel.Opt(endTime)
	}

	result, err := c.events.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	items := []kernel.WorkloadIdentityAuthenticationEvent{}
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
		pterm.Info.Println("No workload identity authentication events found")
	} else {
		rows := pterm.TableData{{"Created At", "Outcome", "Reason", "Service Account ID", "Project ID", "Issuer ID", "Rule ID", "Credential ID", "Correlation ID"}}
		for _, event := range items {
			rows = append(rows, []string{
				util.FormatLocal(event.CreatedAt),
				string(event.Outcome),
				util.OrDash(event.Reason),
				event.ServiceAccountID,
				event.ProjectID,
				event.IssuerID,
				event.FederationRuleID,
				util.OrDash(event.CredentialID),
				event.CorrelationID,
			})
		}
		PrintTableNoPad(rows, true)
	}

	filters := ""
	if in.IssuerID != "" {
		filters += fmt.Sprintf(" --issuer-id %q", in.IssuerID)
	}
	if in.FederationRuleID != "" {
		filters += fmt.Sprintf(" --federation-rule-id %q", in.FederationRuleID)
	}
	if in.ServiceAccountID != "" {
		filters += fmt.Sprintf(" --service-account-id %q", in.ServiceAccountID)
	}
	if in.ProjectID != "" {
		filters += fmt.Sprintf(" --project-id %q", in.ProjectID)
	}
	if in.Outcome != "" {
		filters += fmt.Sprintf(" --outcome %s", in.Outcome)
	}
	if in.StartTime != "" {
		filters += fmt.Sprintf(" --start-time %q", in.StartTime)
	}
	if in.EndTime != "" {
		filters += fmt.Sprintf(" --end-time %q", in.EndTime)
	}
	nextCommand := fmt.Sprintf("kernel workload-identity authentication-events list --page %d --per-page %d%s", page+1, perPage, filters)
	printListPaginationFooter(page, perPage, len(items), hasMore, nextCommand)
	return nil
}

var workloadIdentityAuthenticationEventsCmd = &cobra.Command{
	Use:     "authentication-events",
	Aliases: []string{"authentication-event", "events", "event"},
	Short:   "Inspect workload identity authentication outcomes",
	Long: "List sanitized workload identity authentication outcomes. Assertions, tokens, and arbitrary claims are never returned. " +
		"Any signed-in organization member can read these events; Kernel API keys cannot.",
	Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var workloadIdentityAuthenticationEventsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sanitized workload authentication outcomes",
	Long:  "List authentication events without assertions, tokens, or arbitrary claims. Any signed-in organization member can read this; Kernel API keys cannot.",
	Args:  cobra.NoArgs,
	RunE:  runWorkloadIdentityAuthenticationEventsList,
}

func init() {
	addJSONOutputFlag(workloadIdentityAuthenticationEventsListCmd)
	workloadIdentityAuthenticationEventsListCmd.Flags().Int("page", 1, "Page number (1-based)")
	workloadIdentityAuthenticationEventsListCmd.Flags().Int("per-page", 20, "Items per page (1-99; default 20)")
	workloadIdentityAuthenticationEventsListCmd.Flags().String("issuer-id", "", "Filter by exact issuer ID")
	workloadIdentityAuthenticationEventsListCmd.Flags().String("federation-rule-id", "", "Filter by exact federation rule ID")
	workloadIdentityAuthenticationEventsListCmd.Flags().String("service-account-id", "", "Filter by exact service account ID")
	workloadIdentityAuthenticationEventsListCmd.Flags().String("project-id", "", "Filter by exact project ID")
	workloadIdentityAuthenticationEventsListCmd.Flags().String("outcome", "", "Filter by outcome: success or failure")
	workloadIdentityAuthenticationEventsListCmd.Flags().String("start-time", "", "Include events at or after this RFC3339 date-time")
	workloadIdentityAuthenticationEventsListCmd.Flags().String("end-time", "", "Include events at or before this RFC3339 date-time")

	workloadIdentityAuthenticationEventsCmd.AddCommand(workloadIdentityAuthenticationEventsListCmd)
	workloadIdentityCmd.AddCommand(workloadIdentityAuthenticationEventsCmd)
}

func getWorkloadIdentityAuthenticationEventsHandler(cmd *cobra.Command) WorkloadIdentityAuthenticationEventsCmd {
	client := getKernelClient(cmd)
	return WorkloadIdentityAuthenticationEventsCmd{events: &client.WorkloadIdentityAuthenticationEvents}
}

func runWorkloadIdentityAuthenticationEventsList(cmd *cobra.Command, args []string) error {
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	issuerID, _ := cmd.Flags().GetString("issuer-id")
	federationRuleID, _ := cmd.Flags().GetString("federation-rule-id")
	serviceAccountID, _ := cmd.Flags().GetString("service-account-id")
	projectID, _ := cmd.Flags().GetString("project-id")
	outcome, _ := cmd.Flags().GetString("outcome")
	startTime, _ := cmd.Flags().GetString("start-time")
	endTime, _ := cmd.Flags().GetString("end-time")
	output, _ := cmd.Flags().GetString("output")
	return getWorkloadIdentityAuthenticationEventsHandler(cmd).List(cmd.Context(), WorkloadIdentityAuthenticationEventsListInput{
		Page: page, PerPage: perPage, IssuerID: issuerID, FederationRuleID: federationRuleID,
		ServiceAccountID: serviceAccountID, ProjectID: projectID, Outcome: outcome,
		StartTime: startTime, EndTime: endTime, Output: output,
	})
}
