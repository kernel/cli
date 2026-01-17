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
	"github.com/spf13/cobra"
)

// AgentAuthService defines the subset of the Kernel SDK agent auth client that we use.
type AgentAuthService interface {
	New(ctx context.Context, body kernel.AgentAuthNewParams, opts ...option.RequestOption) (res *kernel.AuthAgent, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.AuthAgent, err error)
	List(ctx context.Context, query kernel.AgentAuthListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.AuthAgent], err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
}

// AgentAuthInvocationsService defines the subset of the Kernel SDK agent auth invocations client that we use.
type AgentAuthInvocationsService interface {
	New(ctx context.Context, body kernel.AgentAuthInvocationNewParams, opts ...option.RequestOption) (res *kernel.AuthAgentInvocationCreateResponse, err error)
	Get(ctx context.Context, invocationID string, opts ...option.RequestOption) (res *kernel.AgentAuthInvocationResponse, err error)
	Exchange(ctx context.Context, invocationID string, body kernel.AgentAuthInvocationExchangeParams, opts ...option.RequestOption) (res *kernel.AgentAuthInvocationExchangeResponse, err error)
	Submit(ctx context.Context, invocationID string, body kernel.AgentAuthInvocationSubmitParams, opts ...option.RequestOption) (res *kernel.AgentAuthSubmitResponse, err error)
}

// AgentAuthCmd handles agent auth operations independent of cobra.
type AgentAuthCmd struct {
	auth        AgentAuthService
	invocations AgentAuthInvocationsService
}

type AgentAuthCreateInput struct {
	Domain         string
	ProfileName    string
	CredentialName string
	LoginURL       string
	AllowedDomains []string
	ProxyID        string
	Output         string
}

type AgentAuthGetInput struct {
	ID     string
	Output string
}

type AgentAuthListInput struct {
	Domain      string
	ProfileName string
	Limit       int
	Offset      int
	Output      string
}

type AgentAuthDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type AgentAuthInvocationCreateInput struct {
	AuthAgentID      string
	SaveCredentialAs string
	Output           string
}

type AgentAuthInvocationGetInput struct {
	InvocationID string
	Output       string
}

type AgentAuthInvocationExchangeInput struct {
	InvocationID string
	Code         string
	Output       string
}

type AgentAuthInvocationSubmitInput struct {
	InvocationID    string
	FieldValues     map[string]string
	SSOButton       string
	SelectedMfaType string
	Output          string
}

func (c AgentAuthCmd) Create(ctx context.Context, in AgentAuthCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Domain == "" {
		return fmt.Errorf("--domain is required")
	}
	if in.ProfileName == "" {
		return fmt.Errorf("--profile-name is required")
	}

	params := kernel.AgentAuthNewParams{
		AuthAgentCreateRequest: kernel.AuthAgentCreateRequestParam{
			Domain:      in.Domain,
			ProfileName: in.ProfileName,
		},
	}
	if in.CredentialName != "" {
		params.AuthAgentCreateRequest.CredentialName = kernel.Opt(in.CredentialName)
	}
	if in.LoginURL != "" {
		params.AuthAgentCreateRequest.LoginURL = kernel.Opt(in.LoginURL)
	}
	if len(in.AllowedDomains) > 0 {
		params.AuthAgentCreateRequest.AllowedDomains = in.AllowedDomains
	}
	if in.ProxyID != "" {
		params.AuthAgentCreateRequest.Proxy = kernel.AuthAgentCreateRequestProxyParam{
			ProxyID: kernel.Opt(in.ProxyID),
		}
	}

	if in.Output != "json" {
		pterm.Info.Printf("Creating auth agent for %s...\n", in.Domain)
	}

	agent, err := c.auth.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(agent)
	}

	pterm.Success.Printf("Created auth agent: %s\n", agent.ID)

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", agent.ID},
		{"Domain", agent.Domain},
		{"Profile Name", agent.ProfileName},
		{"Status", string(agent.Status)},
		{"Can Reauth", fmt.Sprintf("%t", agent.CanReauth)},
	}
	if agent.CredentialName != "" {
		tableData = append(tableData, []string{"Credential Name", agent.CredentialName})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentAuthCmd) Get(ctx context.Context, in AgentAuthGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	agent, err := c.auth.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(agent)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", agent.ID},
		{"Domain", agent.Domain},
		{"Profile Name", agent.ProfileName},
		{"Status", string(agent.Status)},
		{"Can Reauth", fmt.Sprintf("%t", agent.CanReauth)},
		{"Has Selectors", fmt.Sprintf("%t", agent.HasSelectors)},
	}
	if agent.CredentialID != "" {
		tableData = append(tableData, []string{"Credential ID", agent.CredentialID})
	}
	if agent.CredentialName != "" {
		tableData = append(tableData, []string{"Credential Name", agent.CredentialName})
	}
	if agent.PostLoginURL != "" {
		tableData = append(tableData, []string{"Post-Login URL", agent.PostLoginURL})
	}
	if !agent.LastAuthCheckAt.IsZero() {
		tableData = append(tableData, []string{"Last Auth Check", util.FormatLocal(agent.LastAuthCheckAt)})
	}
	if len(agent.AllowedDomains) > 0 {
		tableData = append(tableData, []string{"Allowed Domains", strings.Join(agent.AllowedDomains, ", ")})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentAuthCmd) List(ctx context.Context, in AgentAuthListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.AgentAuthListParams{}
	if in.Domain != "" {
		params.Domain = kernel.Opt(in.Domain)
	}
	if in.ProfileName != "" {
		params.ProfileName = kernel.Opt(in.ProfileName)
	}
	if in.Limit > 0 {
		params.Limit = kernel.Opt(int64(in.Limit))
	}
	if in.Offset > 0 {
		params.Offset = kernel.Opt(int64(in.Offset))
	}

	page, err := c.auth.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var agents []kernel.AuthAgent
	if page != nil {
		agents = page.Items
	}

	if in.Output == "json" {
		if len(agents) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(agents)
	}

	if len(agents) == 0 {
		pterm.Info.Println("No auth agents found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Domain", "Profile Name", "Status", "Can Reauth"}}
	for _, agent := range agents {
		tableData = append(tableData, []string{
			agent.ID,
			agent.Domain,
			agent.ProfileName,
			string(agent.Status),
			fmt.Sprintf("%t", agent.CanReauth),
		})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentAuthCmd) Delete(ctx context.Context, in AgentAuthDeleteInput) error {
	if !in.SkipConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete auth agent '%s'?", in.ID)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := c.auth.Delete(ctx, in.ID); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Auth agent '%s' not found\n", in.ID)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted auth agent: %s\n", in.ID)
	return nil
}

func (c AgentAuthCmd) InvocationCreate(ctx context.Context, in AgentAuthInvocationCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.AuthAgentID == "" {
		return fmt.Errorf("--auth-agent-id is required")
	}

	params := kernel.AgentAuthInvocationNewParams{
		AuthAgentInvocationCreateRequest: kernel.AuthAgentInvocationCreateRequestParam{
			AuthAgentID: in.AuthAgentID,
		},
	}
	if in.SaveCredentialAs != "" {
		params.AuthAgentInvocationCreateRequest.SaveCredentialAs = kernel.Opt(in.SaveCredentialAs)
	}

	if in.Output != "json" {
		pterm.Info.Println("Creating auth invocation...")
	}

	resp, err := c.invocations.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	pterm.Success.Printf("Created invocation: %s\n", resp.InvocationID)

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Invocation ID", resp.InvocationID},
		{"Type", string(resp.Type)},
		{"Handoff Code", resp.HandoffCode},
		{"Hosted URL", resp.HostedURL},
		{"Expires At", util.FormatLocal(resp.ExpiresAt)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentAuthCmd) InvocationGet(ctx context.Context, in AgentAuthInvocationGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	resp, err := c.invocations.Get(ctx, in.InvocationID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"App Name", resp.AppName},
		{"Domain", resp.Domain},
		{"Type", string(resp.Type)},
		{"Status", string(resp.Status)},
		{"Step", string(resp.Step)},
		{"Expires At", util.FormatLocal(resp.ExpiresAt)},
	}
	if resp.LiveViewURL != "" {
		tableData = append(tableData, []string{"Live View URL", resp.LiveViewURL})
	}
	if resp.ErrorMessage != "" {
		tableData = append(tableData, []string{"Error Message", resp.ErrorMessage})
	}
	if resp.ExternalActionMessage != "" {
		tableData = append(tableData, []string{"External Action", resp.ExternalActionMessage})
	}
	if len(resp.PendingFields) > 0 {
		var fields []string
		for _, f := range resp.PendingFields {
			fields = append(fields, f.Name)
		}
		tableData = append(tableData, []string{"Pending Fields", strings.Join(fields, ", ")})
	}
	if len(resp.SubmittedFields) > 0 {
		tableData = append(tableData, []string{"Submitted Fields", strings.Join(resp.SubmittedFields, ", ")})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentAuthCmd) InvocationExchange(ctx context.Context, in AgentAuthInvocationExchangeInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Code == "" {
		return fmt.Errorf("--code is required")
	}

	params := kernel.AgentAuthInvocationExchangeParams{
		Code: in.Code,
	}

	resp, err := c.invocations.Exchange(ctx, in.InvocationID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	pterm.Success.Printf("Exchanged code for JWT\n")

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Invocation ID", resp.InvocationID},
		{"JWT", resp.Jwt},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentAuthCmd) InvocationSubmit(ctx context.Context, in AgentAuthInvocationSubmitInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	// Validate that exactly one of the submit types is provided
	hasFields := len(in.FieldValues) > 0
	hasSSO := in.SSOButton != ""
	hasMFA := in.SelectedMfaType != ""

	count := 0
	if hasFields {
		count++
	}
	if hasSSO {
		count++
	}
	if hasMFA {
		count++
	}

	if count == 0 {
		return fmt.Errorf("must provide one of: --field (field values), --sso-button, or --mfa-type")
	}
	if count > 1 {
		return fmt.Errorf("can only provide one of: --field (field values), --sso-button, or --mfa-type")
	}

	var params kernel.AgentAuthInvocationSubmitParams
	if hasFields {
		params.OfFieldValues = &kernel.AgentAuthInvocationSubmitParamsBodyFieldValues{
			FieldValues: in.FieldValues,
		}
	} else if hasSSO {
		params.OfSSOButton = &kernel.AgentAuthInvocationSubmitParamsBodySSOButton{
			SSOButton: in.SSOButton,
		}
	} else if hasMFA {
		params.OfSelectedMfaType = &kernel.AgentAuthInvocationSubmitParamsBodySelectedMfaType{
			SelectedMfaType: in.SelectedMfaType,
		}
	}

	if in.Output != "json" {
		pterm.Info.Println("Submitting to invocation...")
	}

	resp, err := c.invocations.Submit(ctx, in.InvocationID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	if resp.Accepted {
		pterm.Success.Println("Submission accepted")
	} else {
		pterm.Warning.Println("Submission not accepted")
	}
	return nil
}

// --- Cobra wiring ---

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage agents",
	Long:  "Commands for managing Kernel agents (auth, etc.)",
}

var agentsAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage auth agents",
	Long:  "Commands for managing authentication agents that handle login flows",
}

var agentsAuthCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an auth agent",
	Long:  "Create or find an auth agent for a specific domain and profile combination",
	Args:  cobra.NoArgs,
	RunE:  runAgentsAuthCreate,
}

var agentsAuthGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get an auth agent by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthGet,
}

var agentsAuthListCmd = &cobra.Command{
	Use:   "list",
	Short: "List auth agents",
	Args:  cobra.NoArgs,
	RunE:  runAgentsAuthList,
}

var agentsAuthDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an auth agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthDelete,
}

var agentsAuthInvocationsCmd = &cobra.Command{
	Use:   "invocations",
	Short: "Manage auth invocations",
	Long:  "Commands for managing authentication invocations (login flows)",
}

var agentsAuthInvocationsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an auth invocation",
	Long:  "Start a new authentication flow for an auth agent",
	Args:  cobra.NoArgs,
	RunE:  runAgentsAuthInvocationsCreate,
}

var agentsAuthInvocationsGetCmd = &cobra.Command{
	Use:   "get <invocation-id>",
	Short: "Get an auth invocation",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvocationsGet,
}

var agentsAuthInvocationsExchangeCmd = &cobra.Command{
	Use:   "exchange <invocation-id>",
	Short: "Exchange a handoff code for a JWT",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvocationsExchange,
}

var agentsAuthInvocationsSubmitCmd = &cobra.Command{
	Use:   "submit <invocation-id>",
	Short: "Submit field values to an invocation",
	Long: `Submit field values, SSO button click, or MFA selection to an auth invocation.

Examples:
  # Submit field values
  kernel agents auth invocations submit <id> --field username=myuser --field password=mypass

  # Click an SSO button
  kernel agents auth invocations submit <id> --sso-button "//button[@id='google-sso']"

  # Select an MFA method
  kernel agents auth invocations submit <id> --mfa-type sms`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentsAuthInvocationsSubmit,
}

func init() {
	// Auth create flags
	agentsAuthCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthCreateCmd.Flags().String("domain", "", "Target domain for authentication (required)")
	agentsAuthCreateCmd.Flags().String("profile-name", "", "Name of the profile to use (required)")
	agentsAuthCreateCmd.Flags().String("credential-name", "", "Optional credential name to link for auto-fill")
	agentsAuthCreateCmd.Flags().String("login-url", "", "Optional login page URL")
	agentsAuthCreateCmd.Flags().StringSlice("allowed-domain", []string{}, "Additional allowed domains (repeatable)")
	agentsAuthCreateCmd.Flags().String("proxy-id", "", "Optional proxy ID to use")
	_ = agentsAuthCreateCmd.MarkFlagRequired("domain")
	_ = agentsAuthCreateCmd.MarkFlagRequired("profile-name")

	// Auth get flags
	agentsAuthGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Auth list flags
	agentsAuthListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthListCmd.Flags().String("domain", "", "Filter by domain")
	agentsAuthListCmd.Flags().String("profile-name", "", "Filter by profile name")
	agentsAuthListCmd.Flags().Int("limit", 0, "Maximum number of results to return")
	agentsAuthListCmd.Flags().Int("offset", 0, "Number of results to skip")

	// Auth delete flags
	agentsAuthDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Invocations create flags
	agentsAuthInvocationsCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthInvocationsCreateCmd.Flags().String("auth-agent-id", "", "ID of the auth agent (required)")
	agentsAuthInvocationsCreateCmd.Flags().String("save-credential-as", "", "Save credentials under this name on success")
	_ = agentsAuthInvocationsCreateCmd.MarkFlagRequired("auth-agent-id")

	// Invocations get flags
	agentsAuthInvocationsGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Invocations exchange flags
	agentsAuthInvocationsExchangeCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthInvocationsExchangeCmd.Flags().String("code", "", "Handoff code from the start endpoint (required)")
	_ = agentsAuthInvocationsExchangeCmd.MarkFlagRequired("code")

	// Invocations submit flags
	agentsAuthInvocationsSubmitCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthInvocationsSubmitCmd.Flags().StringArray("field", []string{}, "Field name=value pair (repeatable)")
	agentsAuthInvocationsSubmitCmd.Flags().String("sso-button", "", "Selector of SSO button to click")
	agentsAuthInvocationsSubmitCmd.Flags().String("mfa-type", "", "MFA type to select (sms, call, email, totp, push, security_key)")

	// Wire up commands
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsCreateCmd)
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsGetCmd)
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsExchangeCmd)
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsSubmitCmd)

	agentsAuthCmd.AddCommand(agentsAuthCreateCmd)
	agentsAuthCmd.AddCommand(agentsAuthGetCmd)
	agentsAuthCmd.AddCommand(agentsAuthListCmd)
	agentsAuthCmd.AddCommand(agentsAuthDeleteCmd)
	agentsAuthCmd.AddCommand(agentsAuthInvocationsCmd)

	agentsCmd.AddCommand(agentsAuthCmd)
}

func runAgentsAuthCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile-name")
	credentialName, _ := cmd.Flags().GetString("credential-name")
	loginURL, _ := cmd.Flags().GetString("login-url")
	allowedDomains, _ := cmd.Flags().GetStringSlice("allowed-domain")
	proxyID, _ := cmd.Flags().GetString("proxy-id")

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.Create(cmd.Context(), AgentAuthCreateInput{
		Domain:         domain,
		ProfileName:    profileName,
		CredentialName: credentialName,
		LoginURL:       loginURL,
		AllowedDomains: allowedDomains,
		ProxyID:        proxyID,
		Output:         output,
	})
}

func runAgentsAuthGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.Get(cmd.Context(), AgentAuthGetInput{
		ID:     args[0],
		Output: output,
	})
}

func runAgentsAuthList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile-name")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.List(cmd.Context(), AgentAuthListInput{
		Domain:      domain,
		ProfileName: profileName,
		Limit:       limit,
		Offset:      offset,
		Output:      output,
	})
}

func runAgentsAuthDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skip, _ := cmd.Flags().GetBool("yes")

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.Delete(cmd.Context(), AgentAuthDeleteInput{
		ID:          args[0],
		SkipConfirm: skip,
	})
}

func runAgentsAuthInvocationsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	authAgentID, _ := cmd.Flags().GetString("auth-agent-id")
	saveCredentialAs, _ := cmd.Flags().GetString("save-credential-as")

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.InvocationCreate(cmd.Context(), AgentAuthInvocationCreateInput{
		AuthAgentID:      authAgentID,
		SaveCredentialAs: saveCredentialAs,
		Output:           output,
	})
}

func runAgentsAuthInvocationsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.InvocationGet(cmd.Context(), AgentAuthInvocationGetInput{
		InvocationID: args[0],
		Output:       output,
	})
}

func runAgentsAuthInvocationsExchange(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	code, _ := cmd.Flags().GetString("code")

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.InvocationExchange(cmd.Context(), AgentAuthInvocationExchangeInput{
		InvocationID: args[0],
		Code:         code,
		Output:       output,
	})
}

func runAgentsAuthInvocationsSubmit(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	fieldPairs, _ := cmd.Flags().GetStringArray("field")
	ssoButton, _ := cmd.Flags().GetString("sso-button")
	mfaType, _ := cmd.Flags().GetString("mfa-type")

	// Parse field pairs into map
	fieldValues := make(map[string]string)
	for _, pair := range fieldPairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid field format: %s (expected key=value)", pair)
		}
		fieldValues[parts[0]] = parts[1]
	}

	svc := client.Agents.Auth
	c := AgentAuthCmd{auth: &svc, invocations: &svc.Invocations}
	return c.InvocationSubmit(cmd.Context(), AgentAuthInvocationSubmitInput{
		InvocationID:    args[0],
		FieldValues:     fieldValues,
		SSOButton:       ssoButton,
		SelectedMfaType: mfaType,
		Output:          output,
	})
}
