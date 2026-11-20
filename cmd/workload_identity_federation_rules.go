package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// WorkloadIdentityFederationRulesService is the subset of the SDK service used by the CLI.
type WorkloadIdentityFederationRulesService interface {
	New(ctx context.Context, body kernel.WorkloadIdentityFederationRuleNewParams, opts ...option.RequestOption) (*kernel.WorkloadIdentityFederationRule, error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.WorkloadIdentityFederationRule, error)
	Update(ctx context.Context, id string, body kernel.WorkloadIdentityFederationRuleUpdateParams, opts ...option.RequestOption) (*kernel.WorkloadIdentityFederationRule, error)
	List(ctx context.Context, query kernel.WorkloadIdentityFederationRuleListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.WorkloadIdentityFederationRule], error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) error
}

type WorkloadIdentityFederationRulesCmd struct {
	rules    WorkloadIdentityFederationRulesService
	prompter interactive.Prompter
}

type WorkloadIdentityFederationRulesListInput struct {
	Page             int
	PerPage          int
	IssuerID         string
	ProjectID        string
	Query            string
	ServiceAccountID string
	Status           string
	Output           string
}

type WorkloadIdentityFederationRuleGetInput struct {
	ID     string
	Output string
}

type WorkloadIdentityMatchInput struct {
	Set               bool
	Type              string
	Subject           *string
	SubjectPrefix     *string
	Claims            map[string]string
	ClaimsSet         bool
	Repository        *string
	RepositoryID      *string
	RepositoryOwnerID *string
	Workflow          *string
	WorkflowRef       *string
	JobWorkflowRef    *string
	Ref               *string
	Environment       *string
	EventNames        []string
	EventNamesSet     bool
	Namespace         *string
	ServiceAccount    *string
}

type WorkloadIdentityAuthorizationInput struct {
	Set                bool
	ProjectID          string
	Policy             string
	PolicySet          bool
	TokenAudience      string
	MaxTokenTTLSeconds Int64Flag
}

type WorkloadIdentityFederationRuleCreateInput struct {
	Name             string
	Description      string
	IssuerID         string
	ExpectedAudience string
	Enabled          BoolFlag
	Match            WorkloadIdentityMatchInput
	ServiceAccountID string
	Authorization    WorkloadIdentityAuthorizationInput
	Output           string
}

type WorkloadIdentityFederationRuleUpdateInput struct {
	ID               string
	ExpectedVersion  int64
	Name             *string
	Description      *string
	IssuerID         *string
	ExpectedAudience *string
	Enabled          BoolFlag
	Match            WorkloadIdentityMatchInput
	ServiceAccountID *string
	Authorization    WorkloadIdentityAuthorizationInput
	Output           string
}

type WorkloadIdentityFederationRuleDeleteInput struct {
	ID          string
	SkipConfirm bool
}

func (c WorkloadIdentityFederationRulesCmd) List(ctx context.Context, in WorkloadIdentityFederationRulesListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	page, perPage, err := normalizePageInput(in.Page, in.PerPage)
	if err != nil {
		return err
	}
	status, err := validateWorkloadIdentityListStatus(in.Status)
	if err != nil {
		return err
	}
	if len(in.Query) > 256 {
		return fmt.Errorf("--query must be at most 256 characters")
	}

	params := kernel.WorkloadIdentityFederationRuleListParams{
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}
	if in.IssuerID != "" {
		params.IssuerID = kernel.Opt(in.IssuerID)
	}
	if in.ProjectID != "" {
		params.ProjectID = kernel.Opt(in.ProjectID)
	}
	if in.Query != "" {
		params.Query = kernel.Opt(in.Query)
	}
	if in.ServiceAccountID != "" {
		params.ServiceAccountID = kernel.Opt(in.ServiceAccountID)
	}
	if status != "" {
		params.Status = kernel.WorkloadIdentityFederationRuleListParamsStatus(status)
	}

	result, err := c.rules.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	items := []kernel.WorkloadIdentityFederationRule{}
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
		pterm.Info.Println("No workload identity federation rules found")
	} else {
		rows := pterm.TableData{{"ID", "Name", "Issuer ID", "Service Account ID", "Project ID", "Enabled", "Status", "Last Exchange At"}}
		for _, rule := range items {
			rows = append(rows, []string{
				rule.ID,
				rule.Name,
				rule.IssuerID,
				rule.Target.ServiceAccountID,
				rule.Authorization.ProjectID,
				fmt.Sprintf("%t", rule.Enabled),
				workloadIdentityResourceStatus(rule.JSON.ArchivedAt.Valid()),
				util.FormatLocal(rule.LastExchangeAt),
			})
		}
		PrintTableNoPad(rows, true)
	}

	filters := ""
	if in.IssuerID != "" {
		filters += fmt.Sprintf(" --issuer-id %q", in.IssuerID)
	}
	if in.ProjectID != "" {
		filters += fmt.Sprintf(" --project-id %q", in.ProjectID)
	}
	if in.Query != "" {
		filters += fmt.Sprintf(" --query %q", in.Query)
	}
	if in.ServiceAccountID != "" {
		filters += fmt.Sprintf(" --service-account-id %q", in.ServiceAccountID)
	}
	if status != "" {
		filters += fmt.Sprintf(" --status %s", status)
	}
	nextCommand := fmt.Sprintf("kernel workload-identity federation-rules list --page %d --per-page %d%s", page+1, perPage, filters)
	printListPaginationFooter(page, perPage, len(items), hasMore, nextCommand)
	return nil
}

func (c WorkloadIdentityFederationRulesCmd) Get(ctx context.Context, in WorkloadIdentityFederationRuleGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	rule, err := c.rules.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(rule)
	}
	printWorkloadIdentityFederationRuleDetail(rule)
	return nil
}

func (c WorkloadIdentityFederationRulesCmd) Create(ctx context.Context, in WorkloadIdentityFederationRuleCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	name, err := validateWorkloadIdentityName(in.Name, "name")
	if err != nil {
		return err
	}
	if err := validateWorkloadIdentityDescription(in.Description); err != nil {
		return err
	}
	if err := validateRequiredBoundedString("--issuer-id", in.IssuerID, 128); err != nil {
		return err
	}
	if err := validateRequiredBoundedString("--expected-audience", in.ExpectedAudience, 2048); err != nil {
		return err
	}
	if err := validateRequiredBoundedString("--service-account-id", in.ServiceAccountID, 128); err != nil {
		return err
	}
	match, set, err := buildWorkloadIdentityMatch(in.Match)
	if err != nil {
		return err
	}
	if !set {
		return fmt.Errorf("--match-type is required")
	}
	authorization, set, err := buildWorkloadIdentityAuthorization(in.Authorization)
	if err != nil {
		return err
	}
	if !set {
		return fmt.Errorf("--project-id, --policy, and --token-audience are required")
	}

	config := kernel.WorkloadIdentityFederationRuleConfigParam{
		Name:             name,
		IssuerID:         in.IssuerID,
		ExpectedAudience: in.ExpectedAudience,
		Match:            match,
		Target: kernel.WorkloadIdentityTargetParam{
			Type:             kernel.WorkloadIdentityTargetTypeServiceAccount,
			ServiceAccountID: in.ServiceAccountID,
		},
		Authorization: authorization,
	}
	if in.Description != "" {
		config.Description = kernel.Opt(in.Description)
	}
	if in.Enabled.Set {
		config.Enabled = kernel.Opt(in.Enabled.Value)
	}

	rule, err := c.rules.New(ctx, kernel.WorkloadIdentityFederationRuleNewParams{WorkloadIdentityFederationRuleConfig: config})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(rule)
	}
	pterm.Success.Printf("Created workload identity federation rule: %s\n", rule.ID)
	printWorkloadIdentityFederationRuleDetail(rule)
	return nil
}

func (c WorkloadIdentityFederationRulesCmd) Update(ctx context.Context, in WorkloadIdentityFederationRuleUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.ExpectedVersion < 1 {
		return fmt.Errorf("--expected-version must be at least 1")
	}
	params := kernel.WorkloadIdentityFederationRuleUpdateParams{ExpectedVersion: in.ExpectedVersion}
	changed := false
	if in.Name != nil {
		name, err := validateWorkloadIdentityName(*in.Name, "name")
		if err != nil {
			return err
		}
		params.Name = kernel.Opt(name)
		changed = true
	}
	if in.Description != nil {
		if err := validateWorkloadIdentityDescription(*in.Description); err != nil {
			return err
		}
		params.Description = kernel.Opt(*in.Description)
		changed = true
	}
	if in.IssuerID != nil {
		if err := validateRequiredBoundedString("--issuer-id", *in.IssuerID, 128); err != nil {
			return err
		}
		params.IssuerID = kernel.Opt(*in.IssuerID)
		changed = true
	}
	if in.ExpectedAudience != nil {
		if err := validateRequiredBoundedString("--expected-audience", *in.ExpectedAudience, 2048); err != nil {
			return err
		}
		params.ExpectedAudience = kernel.Opt(*in.ExpectedAudience)
		changed = true
	}
	if in.Enabled.Set {
		params.Enabled = kernel.Opt(in.Enabled.Value)
		changed = true
	}
	match, set, err := buildWorkloadIdentityMatch(in.Match)
	if err != nil {
		return err
	}
	if set {
		params.Match = match
		changed = true
	}
	if in.ServiceAccountID != nil {
		if err := validateRequiredBoundedString("--service-account-id", *in.ServiceAccountID, 128); err != nil {
			return err
		}
		params.Target = kernel.WorkloadIdentityTargetParam{
			Type:             kernel.WorkloadIdentityTargetTypeServiceAccount,
			ServiceAccountID: *in.ServiceAccountID,
		}
		changed = true
	}
	authorization, set, err := buildWorkloadIdentityAuthorization(in.Authorization)
	if err != nil {
		return err
	}
	if set {
		params.Authorization = authorization
		changed = true
	}
	if !changed {
		return fmt.Errorf("nothing to update: pass at least one mutable field")
	}

	rule, err := c.rules.Update(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(rule)
	}
	pterm.Success.Printf("Updated workload identity federation rule: %s\n", rule.ID)
	printWorkloadIdentityFederationRuleDetail(rule)
	return nil
}

func (c WorkloadIdentityFederationRulesCmd) Delete(ctx context.Context, in WorkloadIdentityFederationRuleDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("archive workload identity federation rule '%s'", in.ID),
			fmt.Sprintf("Archive workload identity federation rule '%s'? Authentication history is retained, but the rule can no longer issue credentials.", in.ID),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Archive cancelled")
			return nil
		}
	}
	if err := c.rules.Delete(ctx, in.ID); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Archived workload identity federation rule: %s\n", in.ID)
	return nil
}

func buildWorkloadIdentityMatch(in WorkloadIdentityMatchInput) (kernel.WorkloadIdentityMatchParam, bool, error) {
	if !in.Set {
		return kernel.WorkloadIdentityMatchParam{}, false, nil
	}
	matchType, err := parseWorkloadIdentityMatchType(in.Type)
	if err != nil {
		return kernel.WorkloadIdentityMatchParam{}, false, err
	}
	result := kernel.WorkloadIdentityMatchParam{Type: matchType}
	if in.Subject != nil {
		if err := validateOptionalMatchString("--match-subject", *in.Subject, 1024, true); err != nil {
			return result, false, err
		}
		result.Subject = kernel.Opt(*in.Subject)
	}
	if in.SubjectPrefix != nil {
		if err := validateOptionalMatchString("--match-subject-prefix", *in.SubjectPrefix, 1024, true); err != nil {
			return result, false, err
		}
		result.SubjectPrefix = kernel.Opt(*in.SubjectPrefix)
	}
	if in.ClaimsSet {
		if len(in.Claims) > 32 {
			return result, false, fmt.Errorf("--match-claim may be repeated at most 32 times")
		}
		for pointer, value := range in.Claims {
			if len(value) < 1 || len(value) > 1024 {
				return result, false, fmt.Errorf("--match-claim value for %q must contain 1-1024 characters", pointer)
			}
		}
		result.Claims = in.Claims
	}
	if in.Repository != nil {
		if err := validateOptionalMatchString("--match-repository", *in.Repository, 256, false); err != nil {
			return result, false, err
		}
		result.Repository = kernel.Opt(*in.Repository)
	}
	if in.RepositoryID != nil {
		if err := validateNumericMatchID("--match-repository-id", *in.RepositoryID); err != nil {
			return result, false, err
		}
		result.RepositoryID = kernel.Opt(*in.RepositoryID)
	}
	if in.RepositoryOwnerID != nil {
		if err := validateNumericMatchID("--match-repository-owner-id", *in.RepositoryOwnerID); err != nil {
			return result, false, err
		}
		result.RepositoryOwnerID = kernel.Opt(*in.RepositoryOwnerID)
	}
	if in.Workflow != nil {
		if err := validateOptionalMatchString("--match-workflow", *in.Workflow, 512, false); err != nil {
			return result, false, err
		}
		result.Workflow = kernel.Opt(*in.Workflow)
	}
	if in.WorkflowRef != nil {
		if err := validateOptionalMatchString("--match-workflow-ref", *in.WorkflowRef, 1024, false); err != nil {
			return result, false, err
		}
		result.WorkflowRef = kernel.Opt(*in.WorkflowRef)
	}
	if in.JobWorkflowRef != nil {
		if err := validateOptionalMatchString("--match-job-workflow-ref", *in.JobWorkflowRef, 1024, false); err != nil {
			return result, false, err
		}
		result.JobWorkflowRef = kernel.Opt(*in.JobWorkflowRef)
	}
	if in.Ref != nil {
		if err := validateOptionalMatchString("--match-ref", *in.Ref, 512, false); err != nil {
			return result, false, err
		}
		result.Ref = kernel.Opt(*in.Ref)
	}
	if in.Environment != nil {
		if err := validateOptionalMatchString("--match-environment", *in.Environment, 256, false); err != nil {
			return result, false, err
		}
		result.Environment = kernel.Opt(*in.Environment)
	}
	if in.EventNamesSet {
		if len(in.EventNames) < 1 || len(in.EventNames) > 20 {
			return result, false, fmt.Errorf("--match-event-name must be repeated between 1 and 20 times")
		}
		for _, eventName := range in.EventNames {
			if len(eventName) < 1 || len(eventName) > 128 {
				return result, false, fmt.Errorf("each --match-event-name must contain 1-128 characters")
			}
		}
		result.EventNames = in.EventNames
	}
	if in.Namespace != nil {
		if err := validateOptionalMatchString("--match-namespace", *in.Namespace, 63, false); err != nil {
			return result, false, err
		}
		result.Namespace = kernel.Opt(*in.Namespace)
	}
	if in.ServiceAccount != nil {
		if err := validateOptionalMatchString("--match-service-account", *in.ServiceAccount, 253, false); err != nil {
			return result, false, err
		}
		result.ServiceAccount = kernel.Opt(*in.ServiceAccount)
	}
	return result, true, nil
}

func buildWorkloadIdentityAuthorization(in WorkloadIdentityAuthorizationInput) (kernel.WorkloadIdentityAuthorizationParam, bool, error) {
	if !in.Set {
		return kernel.WorkloadIdentityAuthorizationParam{}, false, nil
	}
	if err := validateRequiredBoundedString("--project-id", in.ProjectID, 128); err != nil {
		return kernel.WorkloadIdentityAuthorizationParam{}, false, fmt.Errorf("authorization replacement requires --project-id: %w", err)
	}
	if !in.PolicySet {
		return kernel.WorkloadIdentityAuthorizationParam{}, false, fmt.Errorf("authorization replacement requires --policy")
	}
	if err := validateRequiredBoundedString("--token-audience", in.TokenAudience, 2048); err != nil {
		return kernel.WorkloadIdentityAuthorizationParam{}, false, fmt.Errorf("authorization replacement requires --token-audience: %w", err)
	}
	policy, err := parseAPIKeyPolicy[kernel.WorkloadIdentityAuthorizationPolicyParam](in.Policy)
	if err != nil {
		return kernel.WorkloadIdentityAuthorizationParam{}, false, err
	}
	result := kernel.WorkloadIdentityAuthorizationParam{
		ProjectID:     in.ProjectID,
		Policy:        policy,
		TokenAudience: in.TokenAudience,
	}
	if in.MaxTokenTTLSeconds.Set {
		if in.MaxTokenTTLSeconds.Value < 1 || in.MaxTokenTTLSeconds.Value > 600 {
			return result, false, fmt.Errorf("--max-token-ttl-seconds must be between 1 and 600")
		}
		result.MaxTokenTtlSeconds = kernel.Opt(in.MaxTokenTTLSeconds.Value)
	}
	return result, true, nil
}

func validateRequiredBoundedString(flagName, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", flagName)
	}
	if len(value) > maxLength {
		return fmt.Errorf("%s must be at most %d characters", flagName, maxLength)
	}
	return nil
}

func parseWorkloadIdentityMatchType(value string) (kernel.WorkloadIdentityMatchType, error) {
	switch value {
	case string(kernel.WorkloadIdentityMatchTypeGitHubActions):
		return kernel.WorkloadIdentityMatchTypeGitHubActions, nil
	case string(kernel.WorkloadIdentityMatchTypeAws):
		return kernel.WorkloadIdentityMatchTypeAws, nil
	case string(kernel.WorkloadIdentityMatchTypeGoogleCloud):
		return kernel.WorkloadIdentityMatchTypeGoogleCloud, nil
	case string(kernel.WorkloadIdentityMatchTypeMicrosoftEntra):
		return kernel.WorkloadIdentityMatchTypeMicrosoftEntra, nil
	case string(kernel.WorkloadIdentityMatchTypeKubernetes):
		return kernel.WorkloadIdentityMatchTypeKubernetes, nil
	case string(kernel.WorkloadIdentityMatchTypeCustomOidc):
		return kernel.WorkloadIdentityMatchTypeCustomOidc, nil
	case "":
		return "", fmt.Errorf("--match-type is required when setting match fields")
	default:
		return "", fmt.Errorf("invalid --match-type %q: must be one of github_actions, aws, google_cloud, microsoft_entra, kubernetes, custom_oidc", value)
	}
}

func validateOptionalMatchString(flagName, value string, maxLength int, requireNonEmpty bool) error {
	if requireNonEmpty && value == "" {
		return fmt.Errorf("%s must not be empty", flagName)
	}
	if len(value) > maxLength {
		return fmt.Errorf("%s must be at most %d characters", flagName, maxLength)
	}
	return nil
}

var numericWorkloadIdentityID = regexp.MustCompile(`^[0-9]+$`)

func validateNumericMatchID(flagName, value string) error {
	if len(value) > 32 || !numericWorkloadIdentityID.MatchString(value) {
		return fmt.Errorf("%s must contain 1-32 decimal digits", flagName)
	}
	return nil
}

func printWorkloadIdentityFederationRuleDetail(rule *kernel.WorkloadIdentityFederationRule) {
	if rule == nil {
		pterm.Info.Println("Workload identity federation rule not found")
		return
	}
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", rule.ID},
		{"Name", rule.Name},
		{"Description", util.OrDash(rule.Description)},
		{"Status", workloadIdentityResourceStatus(rule.JSON.ArchivedAt.Valid())},
		{"Enabled", fmt.Sprintf("%t", rule.Enabled)},
		{"Issuer ID", rule.IssuerID},
		{"Expected Audience", rule.ExpectedAudience},
		{"Match Type", string(rule.Match.Type)},
		{"Match Subject", util.OrDash(rule.Match.Subject)},
		{"Match Subject Prefix", util.OrDash(rule.Match.SubjectPrefix)},
		{"Match Claims", formatStringMap(rule.Match.Claims)},
		{"Match Repository", util.OrDash(rule.Match.Repository)},
		{"Match Repository ID", util.OrDash(rule.Match.RepositoryID)},
		{"Match Repository Owner ID", util.OrDash(rule.Match.RepositoryOwnerID)},
		{"Match Workflow", util.OrDash(rule.Match.Workflow)},
		{"Match Workflow Ref", util.OrDash(rule.Match.WorkflowRef)},
		{"Match Job Workflow Ref", util.OrDash(rule.Match.JobWorkflowRef)},
		{"Match Ref", util.OrDash(rule.Match.Ref)},
		{"Match Environment", util.OrDash(rule.Match.Environment)},
		{"Match Event Names", util.OrDash(strings.Join(rule.Match.EventNames, ", "))},
		{"Match Namespace", util.OrDash(rule.Match.Namespace)},
		{"Match Service Account", util.OrDash(rule.Match.ServiceAccount)},
		{"Target Type", string(rule.Target.Type)},
		{"Target Service Account ID", rule.Target.ServiceAccountID},
		{"Authorization Project ID", rule.Authorization.ProjectID},
		{"Authorization Token Audience", rule.Authorization.TokenAudience},
		{"Authorization Max Token TTL Seconds", formatOptionalPositiveInt(rule.Authorization.MaxTokenTtlSeconds)},
		{"Authorization Policy Statements", fmt.Sprintf("%d", len(rule.Authorization.Policy.Statements))},
		{"Security Version", fmt.Sprintf("%d", rule.SecurityVersion)},
		{"Created By User ID", util.OrDash(rule.CreatedByUserID)},
		{"Last Exchange At", util.FormatLocal(rule.LastExchangeAt)},
		{"Archived At", util.FormatLocal(rule.ArchivedAt)},
		{"Created At", util.FormatLocal(rule.CreatedAt)},
		{"Updated At", util.FormatLocal(rule.UpdatedAt)},
	}
	PrintTableNoPad(rows, true)
}

var workloadIdentityFederationRulesCmd = &cobra.Command{
	Use:     "federation-rules",
	Aliases: []string{"federation-rule", "rules", "rule"},
	Short:   "Manage workload identity federation rules",
	Long: "Manage rules that map trusted issuer claims to a service account and a project-scoped authorization policy. " +
		"Reading requires a signed-in organization member; mutations require credential administrator access.",
	Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var workloadIdentityFederationRulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workload identity federation rule records",
	Long: "List federation rules. Search matches name or description case-insensitively and matches an exact rule ID. " +
		"Any signed-in organization member can read this; Kernel API keys cannot.",
	Args: cobra.NoArgs,
	RunE: runWorkloadIdentityFederationRulesList,
}

var workloadIdentityFederationRulesGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a workload identity federation rule",
	Long:  "Get a federation rule by its stable resource ID. Any signed-in organization member can read this; Kernel API keys cannot.",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkloadIdentityFederationRulesGet,
}

var workloadIdentityFederationRulesCreateCmd = &cobra.Command{
	Use:   "create --name <name> --issuer-id <id> --match-type <type> --expected-audience <audience> --service-account-id <id> --project-id <id> --policy <json> --token-audience <audience>",
	Short: "Create a workload identity federation rule",
	Long: "Create a disabled-by-default rule that maps incoming issuer claims to a service account. Organization credential administrator access is required; " +
		"no external namespace-ownership or staff approval is required. All configured match conditions are ANDed. The policy uses scoped API-key policy syntax, " +
		"but API-key administration actions are not allowed. Every action and resource not listed is denied.",
	Args: cobra.NoArgs,
	RunE: runWorkloadIdentityFederationRulesCreate,
}

var workloadIdentityFederationRulesUpdateCmd = &cobra.Command{
	Use:   "update <id> --expected-version <version>",
	Short: "Update a workload identity federation rule",
	Long: "Update a rule using optimistic concurrency. Match and authorization are replaced as whole objects: when changing any match field, pass --match-type and every match condition to retain; " +
		"when changing authorization, pass --project-id, --policy, and --token-audience together.",
	Args: cobra.ExactArgs(1),
	RunE: runWorkloadIdentityFederationRulesUpdate,
}

var workloadIdentityFederationRulesDeleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"archive"},
	Short:   "Archive a workload identity federation rule",
	Long:    "Archive a federation rule. This retains history and invalidates associated authority within the documented status bound; it does not delete browser resources created through the rule.",
	Args:    cobra.ExactArgs(1),
	RunE:    runWorkloadIdentityFederationRulesDelete,
}

func addWorkloadIdentityFederationRuleMutationFlags(cmd *cobra.Command, update bool) {
	addJSONOutputFlag(cmd)
	cmd.Flags().String("name", "", "Rule name; starts with a letter or number and contains only letters, numbers, and hyphens")
	cmd.Flags().String("description", "", "Description (maximum 4096 characters); on update, pass an empty string to clear it")
	cmd.Flags().String("issuer-id", "", "Trusted workload identity issuer ID")
	cmd.Flags().String("expected-audience", "", "Exact audience required in the incoming token (maximum 2048 characters)")
	cmd.Flags().Bool("enabled", false, "Whether this rule may issue credentials")
	cmd.Flags().String("match-type", "", "Issuer preset type: github_actions, aws, google_cloud, microsoft_entra, kubernetes, or custom_oidc")
	cmd.Flags().String("match-subject", "", "Exact incoming subject (1-1024 characters)")
	cmd.Flags().String("match-subject-prefix", "", "Required incoming subject prefix (1-1024 characters)")
	cmd.Flags().StringArray("match-claim", nil, "Exact RFC 6901 JSON Pointer claim condition as POINTER=VALUE; all conditions are ANDed (repeatable, maximum 32)")
	cmd.Flags().String("match-repository", "", "GitHub repository display name (maximum 256 characters); immutable IDs define identity")
	cmd.Flags().String("match-repository-id", "", "GitHub repository ID (1-32 decimal digits)")
	cmd.Flags().String("match-repository-owner-id", "", "GitHub repository owner ID (1-32 decimal digits)")
	cmd.Flags().String("match-workflow", "", "GitHub workflow name (maximum 512 characters)")
	cmd.Flags().String("match-workflow-ref", "", "GitHub workflow reference (maximum 1024 characters)")
	cmd.Flags().String("match-job-workflow-ref", "", "GitHub reusable job workflow reference (maximum 1024 characters)")
	cmd.Flags().String("match-ref", "", "Git reference (maximum 512 characters)")
	cmd.Flags().String("match-environment", "", "GitHub environment (maximum 256 characters)")
	cmd.Flags().StringArray("match-event-name", nil, "Allowed GitHub event name (repeatable, 1-20 values of at most 128 characters)")
	cmd.Flags().String("match-namespace", "", "Kubernetes namespace (maximum 63 characters)")
	cmd.Flags().String("match-service-account", "", "Kubernetes service account name (maximum 253 characters)")
	cmd.Flags().String("service-account-id", "", "Target Kernel service account ID")
	cmd.Flags().String("project-id", "", "One project to which issued credentials are limited")
	cmd.Flags().String("policy", "", "JSON allow policy in scoped API-key format; API-key administration actions are not allowed")
	cmd.Flags().String("token-audience", "", "Audience placed in the issued credential (maximum 2048 characters)")
	cmd.Flags().Int64("max-token-ttl-seconds", 0, "Maximum issued credential lifetime, from 1 to 600 seconds (default 300)")
	if update {
		cmd.Flags().Int64("expected-version", 0, "Current security_version used for optimistic concurrency (required)")
		_ = cmd.MarkFlagRequired("expected-version")
	} else {
		for _, flag := range []string{"name", "issuer-id", "expected-audience", "match-type", "service-account-id", "project-id", "policy", "token-audience"} {
			_ = cmd.MarkFlagRequired(flag)
		}
	}
}

func init() {
	addJSONOutputFlag(workloadIdentityFederationRulesListCmd)
	workloadIdentityFederationRulesListCmd.Flags().Int("page", 1, "Page number (1-based)")
	workloadIdentityFederationRulesListCmd.Flags().Int("per-page", 20, "Items per page (1-99; default 20)")
	workloadIdentityFederationRulesListCmd.Flags().String("issuer-id", "", "Filter by exact issuer ID")
	workloadIdentityFederationRulesListCmd.Flags().String("project-id", "", "Filter by exact authorization project ID")
	workloadIdentityFederationRulesListCmd.Flags().String("query", "", "Case-insensitive search by name or description; an exact rule ID also matches (maximum 256 characters)")
	workloadIdentityFederationRulesListCmd.Flags().String("service-account-id", "", "Filter by exact target service account ID")
	workloadIdentityFederationRulesListCmd.Flags().String("status", "", "Filter by lifecycle status: active, archived, or all")

	addJSONOutputFlag(workloadIdentityFederationRulesGetCmd)
	addWorkloadIdentityFederationRuleMutationFlags(workloadIdentityFederationRulesCreateCmd, false)
	addWorkloadIdentityFederationRuleMutationFlags(workloadIdentityFederationRulesUpdateCmd, true)
	workloadIdentityFederationRulesDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	workloadIdentityFederationRulesCmd.AddCommand(
		workloadIdentityFederationRulesListCmd,
		workloadIdentityFederationRulesGetCmd,
		workloadIdentityFederationRulesCreateCmd,
		workloadIdentityFederationRulesUpdateCmd,
		workloadIdentityFederationRulesDeleteCmd,
	)
	workloadIdentityCmd.AddCommand(workloadIdentityFederationRulesCmd)
}

func getWorkloadIdentityFederationRulesHandler(cmd *cobra.Command) WorkloadIdentityFederationRulesCmd {
	client := getKernelClient(cmd)
	return WorkloadIdentityFederationRulesCmd{rules: &client.WorkloadIdentityFederationRules, prompter: interactive.NewPrompter()}
}

func workloadIdentityMatchInputFromFlags(cmd *cobra.Command) (WorkloadIdentityMatchInput, error) {
	result := WorkloadIdentityMatchInput{}
	result.Type, _ = cmd.Flags().GetString("match-type")
	for flag, target := range map[string]**string{
		"match-subject":             &result.Subject,
		"match-subject-prefix":      &result.SubjectPrefix,
		"match-repository":          &result.Repository,
		"match-repository-id":       &result.RepositoryID,
		"match-repository-owner-id": &result.RepositoryOwnerID,
		"match-workflow":            &result.Workflow,
		"match-workflow-ref":        &result.WorkflowRef,
		"match-job-workflow-ref":    &result.JobWorkflowRef,
		"match-ref":                 &result.Ref,
		"match-environment":         &result.Environment,
		"match-namespace":           &result.Namespace,
		"match-service-account":     &result.ServiceAccount,
	} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			valueCopy := value
			*target = &valueCopy
		}
	}
	claimSpecs, _ := cmd.Flags().GetStringArray("match-claim")
	if cmd.Flags().Changed("match-claim") {
		claims, err := parseStringMapFlag(claimSpecs, "--match-claim")
		if err != nil {
			return result, err
		}
		result.Claims = claims
		result.ClaimsSet = true
	}
	result.EventNames, _ = cmd.Flags().GetStringArray("match-event-name")
	result.EventNamesSet = cmd.Flags().Changed("match-event-name")
	result.Set = cmd.Flags().Changed("match-type") || result.Subject != nil || result.SubjectPrefix != nil || result.ClaimsSet ||
		result.Repository != nil || result.RepositoryID != nil || result.RepositoryOwnerID != nil || result.Workflow != nil ||
		result.WorkflowRef != nil || result.JobWorkflowRef != nil || result.Ref != nil || result.Environment != nil ||
		result.EventNamesSet || result.Namespace != nil || result.ServiceAccount != nil
	return result, nil
}

func workloadIdentityAuthorizationInputFromFlags(cmd *cobra.Command) WorkloadIdentityAuthorizationInput {
	projectID, _ := cmd.Flags().GetString("project-id")
	policy, _ := cmd.Flags().GetString("policy")
	tokenAudience, _ := cmd.Flags().GetString("token-audience")
	maxTTL, _ := cmd.Flags().GetInt64("max-token-ttl-seconds")
	result := WorkloadIdentityAuthorizationInput{
		ProjectID: projectID, Policy: policy, PolicySet: cmd.Flags().Changed("policy"), TokenAudience: tokenAudience,
		MaxTokenTTLSeconds: Int64Flag{Set: cmd.Flags().Changed("max-token-ttl-seconds"), Value: maxTTL},
	}
	result.Set = cmd.Flags().Changed("project-id") || result.PolicySet || cmd.Flags().Changed("token-audience") || result.MaxTokenTTLSeconds.Set
	return result
}

func runWorkloadIdentityFederationRulesList(cmd *cobra.Command, args []string) error {
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	issuerID, _ := cmd.Flags().GetString("issuer-id")
	projectID, _ := cmd.Flags().GetString("project-id")
	query, _ := cmd.Flags().GetString("query")
	serviceAccountID, _ := cmd.Flags().GetString("service-account-id")
	status, _ := cmd.Flags().GetString("status")
	output, _ := cmd.Flags().GetString("output")
	return getWorkloadIdentityFederationRulesHandler(cmd).List(cmd.Context(), WorkloadIdentityFederationRulesListInput{
		Page: page, PerPage: perPage, IssuerID: issuerID, ProjectID: projectID, Query: query, ServiceAccountID: serviceAccountID, Status: status, Output: output,
	})
}

func runWorkloadIdentityFederationRulesGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	return getWorkloadIdentityFederationRulesHandler(cmd).Get(cmd.Context(), WorkloadIdentityFederationRuleGetInput{ID: args[0], Output: output})
}

func runWorkloadIdentityFederationRulesCreate(cmd *cobra.Command, args []string) error {
	match, err := workloadIdentityMatchInputFromFlags(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	issuerID, _ := cmd.Flags().GetString("issuer-id")
	expectedAudience, _ := cmd.Flags().GetString("expected-audience")
	serviceAccountID, _ := cmd.Flags().GetString("service-account-id")
	output, _ := cmd.Flags().GetString("output")
	return getWorkloadIdentityFederationRulesHandler(cmd).Create(cmd.Context(), WorkloadIdentityFederationRuleCreateInput{
		Name: name, Description: description, IssuerID: issuerID, ExpectedAudience: expectedAudience,
		Enabled: readBoolFlag(cmd.Flags(), "enabled"), Match: match, ServiceAccountID: serviceAccountID,
		Authorization: workloadIdentityAuthorizationInputFromFlags(cmd), Output: output,
	})
}

func runWorkloadIdentityFederationRulesUpdate(cmd *cobra.Command, args []string) error {
	match, err := workloadIdentityMatchInputFromFlags(cmd)
	if err != nil {
		return err
	}
	expectedVersion, _ := cmd.Flags().GetInt64("expected-version")
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	issuerID, _ := cmd.Flags().GetString("issuer-id")
	expectedAudience, _ := cmd.Flags().GetString("expected-audience")
	serviceAccountID, _ := cmd.Flags().GetString("service-account-id")
	output, _ := cmd.Flags().GetString("output")
	var namePtr, descriptionPtr, issuerIDPtr, expectedAudiencePtr, serviceAccountIDPtr *string
	if cmd.Flags().Changed("name") {
		namePtr = &name
	}
	if cmd.Flags().Changed("description") {
		descriptionPtr = &description
	}
	if cmd.Flags().Changed("issuer-id") {
		issuerIDPtr = &issuerID
	}
	if cmd.Flags().Changed("expected-audience") {
		expectedAudiencePtr = &expectedAudience
	}
	if cmd.Flags().Changed("service-account-id") {
		serviceAccountIDPtr = &serviceAccountID
	}
	return getWorkloadIdentityFederationRulesHandler(cmd).Update(cmd.Context(), WorkloadIdentityFederationRuleUpdateInput{
		ID: args[0], ExpectedVersion: expectedVersion, Name: namePtr, Description: descriptionPtr, IssuerID: issuerIDPtr,
		ExpectedAudience: expectedAudiencePtr, Enabled: readBoolFlag(cmd.Flags(), "enabled"), Match: match,
		ServiceAccountID: serviceAccountIDPtr, Authorization: workloadIdentityAuthorizationInputFromFlags(cmd), Output: output,
	})
}

func runWorkloadIdentityFederationRulesDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	return getWorkloadIdentityFederationRulesHandler(cmd).Delete(cmd.Context(), WorkloadIdentityFederationRuleDeleteInput{ID: args[0], SkipConfirm: yes})
}
