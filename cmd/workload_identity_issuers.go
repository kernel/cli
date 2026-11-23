package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// WorkloadIdentityIssuersService is the subset of the SDK service used by the CLI.
type WorkloadIdentityIssuersService interface {
	New(ctx context.Context, body kernel.WorkloadIdentityIssuerNewParams, opts ...option.RequestOption) (*kernel.WorkloadIdentityIssuer, error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.WorkloadIdentityIssuer, error)
	Update(ctx context.Context, id string, body kernel.WorkloadIdentityIssuerUpdateParams, opts ...option.RequestOption) (*kernel.WorkloadIdentityIssuer, error)
	List(ctx context.Context, query kernel.WorkloadIdentityIssuerListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.WorkloadIdentityIssuer], error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) error
	Verify(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.WorkloadIdentityIssuerKeyCheck, error)
}

type WorkloadIdentityIssuersCmd struct {
	issuers  WorkloadIdentityIssuersService
	prompter interactive.Prompter
}

type WorkloadIdentityIssuersListInput struct {
	Page    int
	PerPage int
	Query   string
	Status  string
	Enabled BoolFlag
	Output  string
}

type WorkloadIdentityIssuerGetInput struct {
	ID     string
	Output string
}

type WorkloadIdentityKeySourceInput struct {
	Set              bool
	Type             string
	URL              *string
	DiscoveryBaseURL *string
	CACertPEM        *string
	KeysJSON         *string
}

type WorkloadIdentityTokenValidationInput struct {
	Set                bool
	MaxLifetimeSeconds Int64Flag
	ReplayProtection   *string
}

type WorkloadIdentityIssuerCreateInput struct {
	Name            string
	Description     string
	IssuerURL       *string
	PresetType      *string
	Enabled         BoolFlag
	JWKS            WorkloadIdentityKeySourceInput
	TokenValidation WorkloadIdentityTokenValidationInput
	Output          string
}

type WorkloadIdentityIssuerUpdateInput struct {
	ID              string
	ExpectedVersion int64
	Name            *string
	Description     *string
	IssuerURL       *string
	PresetType      *string
	Enabled         BoolFlag
	JWKS            WorkloadIdentityKeySourceInput
	TokenValidation WorkloadIdentityTokenValidationInput
	Output          string
}

type WorkloadIdentityIssuerDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type WorkloadIdentityIssuerVerifyInput struct {
	ID     string
	Output string
}

func (c WorkloadIdentityIssuersCmd) List(ctx context.Context, in WorkloadIdentityIssuersListInput) error {
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

	params := kernel.WorkloadIdentityIssuerListParams{
		Limit:  kernel.Opt(int64(perPage + 1)),
		Offset: kernel.Opt(int64((page - 1) * perPage)),
	}
	if in.Query != "" {
		params.Query = kernel.Opt(in.Query)
	}
	if status != "" {
		params.Status = kernel.WorkloadIdentityIssuerListParamsStatus(status)
	}
	if in.Enabled.Set {
		params.Enabled = kernel.Opt(in.Enabled.Value)
	}

	result, err := c.issuers.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	items := []kernel.WorkloadIdentityIssuer{}
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
		pterm.Info.Println("No workload identity issuers found")
	} else {
		rows := pterm.TableData{{"ID", "Name", "Preset", "Issuer URL", "JWKS", "Enabled", "Status", "Security Version"}}
		for _, issuer := range items {
			rows = append(rows, []string{
				issuer.ID,
				issuer.Name,
				string(issuer.PresetType),
				util.OrDash(issuer.IssuerURL),
				string(issuer.Jwks.Type),
				fmt.Sprintf("%t", issuer.Enabled),
				workloadIdentityResourceStatus(issuer.JSON.ArchivedAt.Valid()),
				fmt.Sprintf("%d", issuer.SecurityVersion),
			})
		}
		PrintTableNoPad(rows, true)
	}

	filters := ""
	if in.Query != "" {
		filters += fmt.Sprintf(" --query %q", in.Query)
	}
	if status != "" {
		filters += fmt.Sprintf(" --status %s", status)
	}
	if in.Enabled.Set {
		filters += fmt.Sprintf(" --enabled=%t", in.Enabled.Value)
	}
	nextCommand := fmt.Sprintf("kernel workload-identity issuers list --page %d --per-page %d%s", page+1, perPage, filters)
	printListPaginationFooter(page, perPage, len(items), hasMore, nextCommand)
	return nil
}

func (c WorkloadIdentityIssuersCmd) Get(ctx context.Context, in WorkloadIdentityIssuerGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	issuer, err := c.issuers.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(issuer)
	}
	printWorkloadIdentityIssuerDetail(issuer)
	return nil
}

func (c WorkloadIdentityIssuersCmd) Create(ctx context.Context, in WorkloadIdentityIssuerCreateInput) error {
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

	config := kernel.WorkloadIdentityIssuerConfigParam{Name: name}
	if in.Description != "" {
		config.Description = kernel.Opt(in.Description)
	}
	if in.IssuerURL != nil {
		if len(*in.IssuerURL) > 2048 {
			return fmt.Errorf("--issuer-url must be at most 2048 characters")
		}
		config.IssuerURL = kernel.Opt(*in.IssuerURL)
	}
	if in.PresetType != nil {
		preset, err := parseWorkloadIdentityPresetType(*in.PresetType)
		if err != nil {
			return err
		}
		config.PresetType = preset
	}
	if in.Enabled.Set {
		config.Enabled = kernel.Opt(in.Enabled.Value)
	}
	jwks, set, err := buildWorkloadIdentityKeySource(in.JWKS)
	if err != nil {
		return err
	}
	if set {
		config.Jwks = jwks
	}
	validation, set, err := buildWorkloadIdentityTokenValidation(in.TokenValidation)
	if err != nil {
		return err
	}
	if set {
		config.TokenValidation = validation
	}

	issuer, err := c.issuers.New(ctx, kernel.WorkloadIdentityIssuerNewParams{WorkloadIdentityIssuerConfig: config})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(issuer)
	}
	pterm.Success.Printf("Created workload identity issuer: %s\n", issuer.ID)
	printWorkloadIdentityIssuerDetail(issuer)
	return nil
}

func (c WorkloadIdentityIssuersCmd) Update(ctx context.Context, in WorkloadIdentityIssuerUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.ExpectedVersion < 1 {
		return fmt.Errorf("--expected-version must be at least 1")
	}
	params := kernel.WorkloadIdentityIssuerUpdateParams{ExpectedVersion: in.ExpectedVersion}
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
	if in.IssuerURL != nil {
		if len(*in.IssuerURL) > 2048 {
			return fmt.Errorf("--issuer-url must be at most 2048 characters")
		}
		params.IssuerURL = kernel.Opt(*in.IssuerURL)
		changed = true
	}
	if in.PresetType != nil {
		preset, err := parseWorkloadIdentityPresetType(*in.PresetType)
		if err != nil {
			return err
		}
		params.PresetType = preset
		changed = true
	}
	if in.Enabled.Set {
		params.Enabled = kernel.Opt(in.Enabled.Value)
		changed = true
	}
	jwks, set, err := buildWorkloadIdentityKeySource(in.JWKS)
	if err != nil {
		return err
	}
	if set {
		params.Jwks = jwks
		changed = true
	}
	validation, set, err := buildWorkloadIdentityTokenValidation(in.TokenValidation)
	if err != nil {
		return err
	}
	if set {
		params.TokenValidation = validation
		changed = true
	}
	if !changed {
		return fmt.Errorf("nothing to update: pass at least one mutable field")
	}

	issuer, err := c.issuers.Update(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(issuer)
	}
	pterm.Success.Printf("Updated workload identity issuer: %s\n", issuer.ID)
	printWorkloadIdentityIssuerDetail(issuer)
	return nil
}

func (c WorkloadIdentityIssuersCmd) Delete(ctx context.Context, in WorkloadIdentityIssuerDeleteInput) error {
	if !in.SkipConfirm {
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("archive workload identity issuer '%s'", in.ID),
			fmt.Sprintf("Archive workload identity issuer '%s'? Its rules and authentication history are retained, but it can no longer issue credentials.", in.ID),
		)
		if err != nil {
			return err
		}
		if !ok {
			pterm.Info.Println("Archive cancelled")
			return nil
		}
	}
	if err := c.issuers.Delete(ctx, in.ID); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Archived workload identity issuer: %s\n", in.ID)
	return nil
}

func (c WorkloadIdentityIssuersCmd) Verify(ctx context.Context, in WorkloadIdentityIssuerVerifyInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	result, err := c.issuers.Verify(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(result)
	}
	pterm.Success.Printf("Verified issuer keys for %s\n", result.IssuerID)
	if len(result.KeyIDs) == 0 {
		pterm.Info.Println("No key IDs were returned")
		return nil
	}
	keyIDs := append([]string(nil), result.KeyIDs...)
	sort.Strings(keyIDs)
	rows := pterm.TableData{{"Key ID"}}
	for _, keyID := range keyIDs {
		rows = append(rows, []string{keyID})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func buildWorkloadIdentityKeySource(in WorkloadIdentityKeySourceInput) (kernel.WorkloadIdentityKeySourceParam, bool, error) {
	if !in.Set {
		return kernel.WorkloadIdentityKeySourceParam{}, false, nil
	}
	var keyType kernel.WorkloadIdentityKeySourceType
	switch in.Type {
	case string(kernel.WorkloadIdentityKeySourceTypeDiscovery):
		keyType = kernel.WorkloadIdentityKeySourceTypeDiscovery
	case string(kernel.WorkloadIdentityKeySourceTypeExplicitURL):
		keyType = kernel.WorkloadIdentityKeySourceTypeExplicitURL
	case string(kernel.WorkloadIdentityKeySourceTypeInline):
		keyType = kernel.WorkloadIdentityKeySourceTypeInline
	case "":
		return kernel.WorkloadIdentityKeySourceParam{}, false, fmt.Errorf("--jwks-type is required when setting JWKS fields")
	default:
		return kernel.WorkloadIdentityKeySourceParam{}, false, fmt.Errorf("invalid --jwks-type %q: must be one of discovery, explicit_url, inline", in.Type)
	}
	result := kernel.WorkloadIdentityKeySourceParam{Type: keyType}
	if in.URL != nil {
		if len(*in.URL) > 2048 {
			return result, false, fmt.Errorf("--jwks-url must be at most 2048 characters")
		}
		result.URL = kernel.Opt(*in.URL)
	}
	if in.DiscoveryBaseURL != nil {
		if len(*in.DiscoveryBaseURL) > 2048 {
			return result, false, fmt.Errorf("--jwks-discovery-base-url must be at most 2048 characters")
		}
		result.DiscoveryBaseURL = kernel.Opt(*in.DiscoveryBaseURL)
	}
	if in.CACertPEM != nil {
		if len(*in.CACertPEM) > 65536 {
			return result, false, fmt.Errorf("--jwks-ca-cert-pem must be at most 65536 characters")
		}
		result.CaCertPem = kernel.Opt(*in.CACertPEM)
	}
	if in.KeysJSON != nil {
		var keys []map[string]any
		if err := json.Unmarshal([]byte(*in.KeysJSON), &keys); err != nil {
			return result, false, fmt.Errorf("invalid --jwks-keys JSON: expected an array of JWK objects: %w", err)
		}
		if len(keys) < 1 || len(keys) > 16 {
			return result, false, fmt.Errorf("--jwks-keys must contain between 1 and 16 JWK objects")
		}
		for i, key := range keys {
			if key == nil {
				return result, false, fmt.Errorf("--jwks-keys entry %d must be a JSON object, not null", i)
			}
		}
		result.Keys = keys
	}
	return result, true, nil
}

func buildWorkloadIdentityTokenValidation(in WorkloadIdentityTokenValidationInput) (kernel.WorkloadIdentityTokenValidationParam, bool, error) {
	if !in.Set {
		return kernel.WorkloadIdentityTokenValidationParam{}, false, nil
	}
	result := kernel.WorkloadIdentityTokenValidationParam{}
	if in.MaxLifetimeSeconds.Set {
		if in.MaxLifetimeSeconds.Value < 1 || in.MaxLifetimeSeconds.Value > 172800 {
			return result, false, fmt.Errorf("--token-max-lifetime-seconds must be between 1 and 172800")
		}
		result.MaxLifetimeSeconds = kernel.Opt(in.MaxLifetimeSeconds.Value)
	}
	if in.ReplayProtection != nil {
		switch *in.ReplayProtection {
		case string(kernel.WorkloadIdentityTokenValidationReplayProtectionSingleUse):
			result.ReplayProtection = kernel.WorkloadIdentityTokenValidationReplayProtectionSingleUse
		case string(kernel.WorkloadIdentityTokenValidationReplayProtectionAllowReuse):
			result.ReplayProtection = kernel.WorkloadIdentityTokenValidationReplayProtectionAllowReuse
		default:
			return result, false, fmt.Errorf("invalid --token-replay-protection %q: must be one of single_use, allow_reuse", *in.ReplayProtection)
		}
	}
	return result, true, nil
}

func parseWorkloadIdentityPresetType(value string) (kernel.WorkloadIdentityPresetType, error) {
	switch value {
	case string(kernel.WorkloadIdentityPresetTypeGitHubActions):
		return kernel.WorkloadIdentityPresetTypeGitHubActions, nil
	case string(kernel.WorkloadIdentityPresetTypeAws):
		return kernel.WorkloadIdentityPresetTypeAws, nil
	case string(kernel.WorkloadIdentityPresetTypeGoogleCloud):
		return kernel.WorkloadIdentityPresetTypeGoogleCloud, nil
	case string(kernel.WorkloadIdentityPresetTypeMicrosoftEntra):
		return kernel.WorkloadIdentityPresetTypeMicrosoftEntra, nil
	case string(kernel.WorkloadIdentityPresetTypeKubernetes):
		return kernel.WorkloadIdentityPresetTypeKubernetes, nil
	case string(kernel.WorkloadIdentityPresetTypeCustomOidc):
		return kernel.WorkloadIdentityPresetTypeCustomOidc, nil
	default:
		return "", fmt.Errorf("invalid --preset-type %q: must be one of github_actions, aws, google_cloud, microsoft_entra, kubernetes, custom_oidc", value)
	}
}

func printWorkloadIdentityIssuerDetail(issuer *kernel.WorkloadIdentityIssuer) {
	if issuer == nil {
		pterm.Info.Println("Workload identity issuer not found")
		return
	}
	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", issuer.ID},
		{"Name", issuer.Name},
		{"Description", util.OrDash(issuer.Description)},
		{"Status", workloadIdentityResourceStatus(issuer.JSON.ArchivedAt.Valid())},
		{"Enabled", fmt.Sprintf("%t", issuer.Enabled)},
		{"Preset Type", string(issuer.PresetType)},
		{"Issuer URL", util.OrDash(issuer.IssuerURL)},
		{"JWKS Type", string(issuer.Jwks.Type)},
		{"JWKS URL", util.OrDash(issuer.Jwks.URL)},
		{"JWKS Discovery Base URL", util.OrDash(issuer.Jwks.DiscoveryBaseURL)},
		{"Inline JWK Count", fmt.Sprintf("%d", len(issuer.Jwks.Keys))},
		{"Token Max Lifetime Seconds", formatOptionalPositiveInt(issuer.TokenValidation.MaxLifetimeSeconds)},
		{"Token Replay Protection", util.OrDash(string(issuer.TokenValidation.ReplayProtection))},
		{"Security Version", fmt.Sprintf("%d", issuer.SecurityVersion)},
		{"Created By User ID", util.OrDash(issuer.CreatedByUserID)},
		{"Archived At", util.FormatLocal(issuer.ArchivedAt)},
		{"Created At", util.FormatLocal(issuer.CreatedAt)},
		{"Updated At", util.FormatLocal(issuer.UpdatedAt)},
	}
	PrintTableNoPad(rows, true)
}

func formatOptionalPositiveInt(value int64) string {
	if value == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

var workloadIdentityIssuersCmd = &cobra.Command{
	Use:     "issuers",
	Aliases: []string{"issuer"},
	Short:   "Manage trusted workload identity issuers",
	Long: "Manage trusted external identity providers whose signed tokens Kernel can exchange for short-lived API keys. " +
		"Reading requires a signed-in organization member; mutations require credential administrator access.",
	Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var workloadIdentityIssuersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workload identity issuer records",
	Long: "List workload identity issuers. Search matches name or description case-insensitively and matches an exact issuer ID. " +
		"Any signed-in organization member can read this; Kernel API keys cannot.",
	Args: cobra.NoArgs,
	RunE: runWorkloadIdentityIssuersList,
}

var workloadIdentityIssuersGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a workload identity issuer",
	Long:  "Get a workload identity issuer by its stable resource ID. Any signed-in organization member can read this; Kernel API keys cannot.",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkloadIdentityIssuersGet,
}

var workloadIdentityIssuersCreateCmd = &cobra.Command{
	Use:   "create --name <name>",
	Short: "Create a workload identity issuer",
	Long: "Create a trusted workload identity issuer. Organization credential administrator access is required; no external namespace-ownership or staff approval is required. " +
		"GitHub Actions and Google Cloud can use their fixed issuer defaults. For discovery, explicit URL, or inline keys, set --jwks-type and the corresponding JWKS flags. " +
		"New issuers are disabled unless --enabled is set.",
	Args: cobra.NoArgs,
	RunE: runWorkloadIdentityIssuersCreate,
}

var workloadIdentityIssuersUpdateCmd = &cobra.Command{
	Use:   "update <id> --expected-version <version>",
	Short: "Update a workload identity issuer",
	Long: "Update an issuer using optimistic concurrency. Pass its current security_version as --expected-version. " +
		"Whenever any JWKS field is updated, pass --jwks-type as well so the entire key-source object is unambiguous.",
	Args: cobra.ExactArgs(1),
	RunE: runWorkloadIdentityIssuersUpdate,
}

var workloadIdentityIssuersDeleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"archive"},
	Short:   "Archive a workload identity issuer",
	Long:    "Archive an issuer. This retains history and invalidates associated authority within the documented status bound; it does not delete browser resources created through the issuer.",
	Args:    cobra.ExactArgs(1),
	RunE:    runWorkloadIdentityIssuersDelete,
}

var workloadIdentityIssuersVerifyCmd = &cobra.Command{
	Use:   "verify <id>",
	Short: "Fetch and verify an issuer's signing keys",
	Long:  "Check issuer discovery and public verification keys. This read-only operation does not enable trust, approve namespace ownership, create a grant, or issue credentials.",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkloadIdentityIssuersVerify,
}

func addWorkloadIdentityIssuerMutationFlags(cmd *cobra.Command, update bool) {
	addJSONOutputFlag(cmd)
	cmd.Flags().String("name", "", "Issuer name; starts with a letter or number and contains only letters, numbers, and hyphens")
	cmd.Flags().String("description", "", "Description (maximum 4096 characters); on update, pass an empty string to clear it")
	cmd.Flags().String("issuer-url", "", "Exact expected signed issuer URL (maximum 2048 characters)")
	cmd.Flags().String("preset-type", "", "Identity provider type: github_actions, aws, google_cloud, microsoft_entra, kubernetes, or custom_oidc")
	cmd.Flags().Bool("enabled", false, "Whether this issuer may issue credentials")
	cmd.Flags().String("jwks-type", "", "Signing-key source: discovery, explicit_url, or inline")
	cmd.Flags().String("jwks-url", "", "Explicit JWKS URL (maximum 2048 characters)")
	cmd.Flags().String("jwks-discovery-base-url", "", "Base URL used for OpenID Connect discovery (maximum 2048 characters)")
	cmd.Flags().String("jwks-ca-cert-pem", "", "Optional PEM public CA certificates trusted instead of system roots (maximum 65536 characters)")
	cmd.Flags().String("jwks-keys", "", "Inline JWK set as a JSON array of 1-16 public JWK objects")
	cmd.Flags().Int64("token-max-lifetime-seconds", 0, "Maximum incoming exp minus iat, from 1 to 172800 seconds (48 hours; covers Azure managed identity tokens that live slightly over 24 hours)")
	cmd.Flags().String("token-replay-protection", "", "Incoming token reuse policy: single_use or allow_reuse")
	if update {
		cmd.Flags().Int64("expected-version", 0, "Current security_version used for optimistic concurrency (required)")
		_ = cmd.MarkFlagRequired("expected-version")
	} else {
		_ = cmd.MarkFlagRequired("name")
	}
}

func init() {
	addJSONOutputFlag(workloadIdentityIssuersListCmd)
	workloadIdentityIssuersListCmd.Flags().Int("page", 1, "Page number (1-based)")
	workloadIdentityIssuersListCmd.Flags().Int("per-page", 20, "Items per page (1-99; default 20)")
	workloadIdentityIssuersListCmd.Flags().String("query", "", "Case-insensitive search by name or description; an exact issuer ID also matches (maximum 256 characters)")
	workloadIdentityIssuersListCmd.Flags().String("status", "", "Filter by lifecycle status: active, archived, or all")
	workloadIdentityIssuersListCmd.Flags().Bool("enabled", false, "Filter by whether the issuer is enabled; pass --enabled=false explicitly to select disabled issuers")

	addJSONOutputFlag(workloadIdentityIssuersGetCmd)
	addWorkloadIdentityIssuerMutationFlags(workloadIdentityIssuersCreateCmd, false)
	addWorkloadIdentityIssuerMutationFlags(workloadIdentityIssuersUpdateCmd, true)
	workloadIdentityIssuersDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	addJSONOutputFlag(workloadIdentityIssuersVerifyCmd)

	workloadIdentityIssuersCmd.AddCommand(
		workloadIdentityIssuersListCmd,
		workloadIdentityIssuersGetCmd,
		workloadIdentityIssuersCreateCmd,
		workloadIdentityIssuersUpdateCmd,
		workloadIdentityIssuersDeleteCmd,
		workloadIdentityIssuersVerifyCmd,
	)
	workloadIdentityCmd.AddCommand(workloadIdentityIssuersCmd)
}

func getWorkloadIdentityIssuersHandler(cmd *cobra.Command) WorkloadIdentityIssuersCmd {
	client := getKernelClient(cmd)
	return WorkloadIdentityIssuersCmd{issuers: &client.WorkloadIdentityIssuers, prompter: interactive.NewPrompter()}
}

func workloadIdentityKeySourceInputFromFlags(cmd *cobra.Command) WorkloadIdentityKeySourceInput {
	keyType, _ := cmd.Flags().GetString("jwks-type")
	url, _ := cmd.Flags().GetString("jwks-url")
	discovery, _ := cmd.Flags().GetString("jwks-discovery-base-url")
	caCert, _ := cmd.Flags().GetString("jwks-ca-cert-pem")
	keys, _ := cmd.Flags().GetString("jwks-keys")
	result := WorkloadIdentityKeySourceInput{Type: keyType}
	if cmd.Flags().Changed("jwks-url") {
		result.URL = &url
	}
	if cmd.Flags().Changed("jwks-discovery-base-url") {
		result.DiscoveryBaseURL = &discovery
	}
	if cmd.Flags().Changed("jwks-ca-cert-pem") {
		result.CACertPEM = &caCert
	}
	if cmd.Flags().Changed("jwks-keys") {
		result.KeysJSON = &keys
	}
	result.Set = cmd.Flags().Changed("jwks-type") || result.URL != nil || result.DiscoveryBaseURL != nil || result.CACertPEM != nil || result.KeysJSON != nil
	return result
}

func workloadIdentityTokenValidationInputFromFlags(cmd *cobra.Command) WorkloadIdentityTokenValidationInput {
	maxLifetime, _ := cmd.Flags().GetInt64("token-max-lifetime-seconds")
	replay, _ := cmd.Flags().GetString("token-replay-protection")
	result := WorkloadIdentityTokenValidationInput{
		MaxLifetimeSeconds: Int64Flag{Set: cmd.Flags().Changed("token-max-lifetime-seconds"), Value: maxLifetime},
	}
	if cmd.Flags().Changed("token-replay-protection") {
		result.ReplayProtection = &replay
	}
	result.Set = result.MaxLifetimeSeconds.Set || result.ReplayProtection != nil
	return result
}

func runWorkloadIdentityIssuersList(cmd *cobra.Command, args []string) error {
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")
	query, _ := cmd.Flags().GetString("query")
	status, _ := cmd.Flags().GetString("status")
	output, _ := cmd.Flags().GetString("output")
	return getWorkloadIdentityIssuersHandler(cmd).List(cmd.Context(), WorkloadIdentityIssuersListInput{
		Page: page, PerPage: perPage, Query: query, Status: status, Enabled: readBoolFlag(cmd.Flags(), "enabled"), Output: output,
	})
}

func runWorkloadIdentityIssuersGet(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	return getWorkloadIdentityIssuersHandler(cmd).Get(cmd.Context(), WorkloadIdentityIssuerGetInput{ID: args[0], Output: output})
}

func runWorkloadIdentityIssuersCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	issuerURL, _ := cmd.Flags().GetString("issuer-url")
	presetType, _ := cmd.Flags().GetString("preset-type")
	output, _ := cmd.Flags().GetString("output")
	var issuerURLPtr, presetTypePtr *string
	if cmd.Flags().Changed("issuer-url") {
		issuerURLPtr = &issuerURL
	}
	if cmd.Flags().Changed("preset-type") {
		presetTypePtr = &presetType
	}
	return getWorkloadIdentityIssuersHandler(cmd).Create(cmd.Context(), WorkloadIdentityIssuerCreateInput{
		Name: name, Description: description, IssuerURL: issuerURLPtr, PresetType: presetTypePtr,
		Enabled: readBoolFlag(cmd.Flags(), "enabled"), JWKS: workloadIdentityKeySourceInputFromFlags(cmd),
		TokenValidation: workloadIdentityTokenValidationInputFromFlags(cmd), Output: output,
	})
}

func runWorkloadIdentityIssuersUpdate(cmd *cobra.Command, args []string) error {
	expectedVersion, _ := cmd.Flags().GetInt64("expected-version")
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	issuerURL, _ := cmd.Flags().GetString("issuer-url")
	presetType, _ := cmd.Flags().GetString("preset-type")
	output, _ := cmd.Flags().GetString("output")
	var namePtr, descriptionPtr, issuerURLPtr, presetTypePtr *string
	if cmd.Flags().Changed("name") {
		namePtr = &name
	}
	if cmd.Flags().Changed("description") {
		descriptionPtr = &description
	}
	if cmd.Flags().Changed("issuer-url") {
		issuerURLPtr = &issuerURL
	}
	if cmd.Flags().Changed("preset-type") {
		presetTypePtr = &presetType
	}
	return getWorkloadIdentityIssuersHandler(cmd).Update(cmd.Context(), WorkloadIdentityIssuerUpdateInput{
		ID: args[0], ExpectedVersion: expectedVersion, Name: namePtr, Description: descriptionPtr, IssuerURL: issuerURLPtr,
		PresetType: presetTypePtr, Enabled: readBoolFlag(cmd.Flags(), "enabled"), JWKS: workloadIdentityKeySourceInputFromFlags(cmd),
		TokenValidation: workloadIdentityTokenValidationInputFromFlags(cmd), Output: output,
	})
}

func runWorkloadIdentityIssuersDelete(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	return getWorkloadIdentityIssuersHandler(cmd).Delete(cmd.Context(), WorkloadIdentityIssuerDeleteInput{ID: args[0], SkipConfirm: yes})
}

func runWorkloadIdentityIssuersVerify(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	return getWorkloadIdentityIssuersHandler(cmd).Verify(cmd.Context(), WorkloadIdentityIssuerVerifyInput{ID: args[0], Output: output})
}
