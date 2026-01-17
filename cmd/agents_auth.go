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

// AgentsAuthService defines the subset of the Kernel SDK agents auth client that we use.
type AgentsAuthService interface {
	New(ctx context.Context, body kernel.AgentAuthNewParams, opts ...option.RequestOption) (res *kernel.AuthAgent, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.AuthAgent, err error)
	List(ctx context.Context, query kernel.AgentAuthListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.AuthAgent], err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
}

// AgentsAuthInvocationsService defines the subset of the Kernel SDK agents auth invocations client.
type AgentsAuthInvocationsService interface {
	New(ctx context.Context, body kernel.AgentAuthInvocationNewParams, opts ...option.RequestOption) (res *kernel.AuthAgentInvocationCreateResponse, err error)
	Get(ctx context.Context, invocationID string, opts ...option.RequestOption) (res *kernel.AgentAuthInvocationResponse, err error)
	Exchange(ctx context.Context, invocationID string, body kernel.AgentAuthInvocationExchangeParams, opts ...option.RequestOption) (res *kernel.AgentAuthInvocationExchangeResponse, err error)
	Submit(ctx context.Context, invocationID string, body kernel.AgentAuthInvocationSubmitParams, opts ...option.RequestOption) (res *kernel.AgentAuthSubmitResponse, err error)
}

// AgentsAuthCmd handles agents auth operations independent of cobra.
type AgentsAuthCmd struct {
	auth        AgentsAuthService
	invocations AgentsAuthInvocationsService
}

type AgentsAuthListInput struct {
	Limit  int
	Offset int
	Output string
}

type AgentsAuthGetInput struct {
	ID     string
	Output string
}

type AgentsAuthCreateInput struct {
	ProfileName    string
	Domain         string
	CredentialName string
	LoginURL       string
	Output         string
}

type AgentsAuthDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type AgentsAuthInvocationCreateInput struct {
	AuthAgentID      string
	SaveCredentialAs string
	Output           string
}

type AgentsAuthInvocationGetInput struct {
	InvocationID string
	Output       string
}

type AgentsAuthInvocationExchangeInput struct {
	InvocationID string
	HandoffCode  string
	Output       string
}

type AgentsAuthInvocationSubmitInput struct {
	InvocationID string
	Values       map[string]string
	Output       string
}

func (c AgentsAuthCmd) List(ctx context.Context, in AgentsAuthListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.AgentAuthListParams{}
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

	tableData := pterm.TableData{{"ID", "Profile Name", "Domain", "Status", "Can Reauth"}}
	for _, agent := range agents {
		canReauth := "-"
		if agent.CanReauth {
			canReauth = "Yes"
		}
		tableData = append(tableData, []string{
			agent.ID,
			agent.ProfileName,
			agent.Domain,
			string(agent.Status),
			canReauth,
		})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentsAuthCmd) Get(ctx context.Context, in AgentsAuthGetInput) error {
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

	canReauth := "No"
	if agent.CanReauth {
		canReauth = "Yes"
	}
	credName := agent.CredentialName
	if credName == "" {
		credName = "-"
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", agent.ID},
		{"Profile Name", agent.ProfileName},
		{"Domain", agent.Domain},
		{"Status", string(agent.Status)},
		{"Can Reauth", canReauth},
		{"Credential Name", credName},
		{"Credential ID", util.OrDash(agent.CredentialID)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentsAuthCmd) Create(ctx context.Context, in AgentsAuthCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.ProfileName == "" {
		return fmt.Errorf("--profile-name is required")
	}
	if in.Domain == "" {
		return fmt.Errorf("--domain is required")
	}

	params := kernel.AgentAuthNewParams{
		AuthAgentCreateRequest: kernel.AuthAgentCreateRequestParam{
			ProfileName: in.ProfileName,
			Domain:      in.Domain,
		},
	}
	if in.CredentialName != "" {
		params.AuthAgentCreateRequest.CredentialName = kernel.Opt(in.CredentialName)
	}
	if in.LoginURL != "" {
		params.AuthAgentCreateRequest.LoginURL = kernel.Opt(in.LoginURL)
	}

	if in.Output != "json" {
		pterm.Info.Printf("Creating auth agent for profile '%s' on domain '%s'...\n", in.ProfileName, in.Domain)
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
		{"Profile Name", agent.ProfileName},
		{"Domain", agent.Domain},
		{"Status", string(agent.Status)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentsAuthCmd) Delete(ctx context.Context, in AgentsAuthDeleteInput) error {
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

// Invocation methods

func (c AgentsAuthCmd) InvocationCreate(ctx context.Context, in AgentsAuthInvocationCreateInput) error {
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
		pterm.Info.Printf("Creating auth invocation for agent '%s'...\n", in.AuthAgentID)
	}

	resp, err := c.invocations.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	pterm.Success.Printf("Created auth invocation: %s\n", resp.InvocationID)

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Invocation ID", resp.InvocationID},
		{"Type", string(resp.Type)},
		{"Hosted URL", resp.HostedURL},
		{"Handoff Code", resp.HandoffCode},
		{"Expires At", util.FormatLocal(resp.ExpiresAt)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentsAuthCmd) InvocationGet(ctx context.Context, in AgentsAuthInvocationGetInput) error {
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

	liveViewURL := resp.LiveViewURL
	if liveViewURL == "" {
		liveViewURL = "-"
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Domain", resp.Domain},
		{"Status", string(resp.Status)},
		{"Step", string(resp.Step)},
		{"Type", string(resp.Type)},
		{"Live View URL", liveViewURL},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentsAuthCmd) InvocationExchange(ctx context.Context, in AgentsAuthInvocationExchangeInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.HandoffCode == "" {
		return fmt.Errorf("--code is required")
	}

	if in.Output != "json" {
		pterm.Info.Printf("Exchanging handoff code for invocation '%s'...\n", in.InvocationID)
	}

	params := kernel.AgentAuthInvocationExchangeParams{
		Code: in.HandoffCode,
	}

	resp, err := c.invocations.Exchange(ctx, in.InvocationID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	pterm.Success.Printf("Exchange complete for invocation: %s\n", resp.InvocationID)

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Invocation ID", resp.InvocationID},
		{"JWT", truncateString(resp.Jwt, 50)},
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AgentsAuthCmd) InvocationSubmit(ctx context.Context, in AgentsAuthInvocationSubmitInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if len(in.Values) == 0 {
		return fmt.Errorf("at least one --value is required")
	}

	params := kernel.AgentAuthInvocationSubmitParams{
		OfFieldValues: &kernel.AgentAuthInvocationSubmitParamsBodyFieldValues{
			FieldValues: in.Values,
		},
	}

	if in.Output != "json" {
		pterm.Info.Printf("Submitting values for auth invocation '%s'...\n", in.InvocationID)
	}

	resp, err := c.invocations.Submit(ctx, in.InvocationID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	if resp.Accepted {
		pterm.Success.Printf("Values submitted successfully for invocation: %s\n", in.InvocationID)
	} else {
		pterm.Warning.Printf("Values submission not accepted for invocation: %s\n", in.InvocationID)
	}

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// --- Cobra wiring ---

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage AI agents",
	Long:  "Commands for managing Kernel AI agents",
}

var agentsAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage auth agents",
	Long:  "Commands for managing authentication agents for automatic login",
}

var agentsAuthListCmd = &cobra.Command{
	Use:   "list",
	Short: "List auth agents",
	Args:  cobra.NoArgs,
	RunE:  runAgentsAuthList,
}

var agentsAuthGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get an auth agent by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthGet,
}

var agentsAuthCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new auth agent",
	Long: `Create a new authentication agent for automatic login.

Examples:
  # Create an auth agent with a profile and domain
  kernel agents auth create --profile-name "my-profile" --domain "example.com"

  # Create an auth agent with an existing credential
  kernel agents auth create --profile-name "my-profile" --domain "example.com" --credential-name "my-cred"`,
	Args: cobra.NoArgs,
	RunE: runAgentsAuthCreate,
}

var agentsAuthDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an auth agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthDelete,
}

// Invocations subcommands
var agentsAuthInvocationsCmd = &cobra.Command{
	Use:   "invocations",
	Short: "Manage auth agent invocations",
	Long:  "Commands for managing authentication agent invocations",
}

var agentsAuthInvocationsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new auth invocation",
	Args:  cobra.NoArgs,
	RunE:  runAgentsAuthInvocationsCreate,
}

var agentsAuthInvocationsGetCmd = &cobra.Command{
	Use:   "get <invocation-id>",
	Short: "Get an auth invocation by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvocationsGet,
}

var agentsAuthInvocationsExchangeCmd = &cobra.Command{
	Use:   "exchange <invocation-id>",
	Short: "Exchange handoff code for JWT",
	Long:  `Exchange a handoff code for a JWT token to authenticate API calls for this invocation.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvocationsExchange,
}

var agentsAuthInvocationsSubmitCmd = &cobra.Command{
	Use:   "submit <invocation-id>",
	Short: "Submit values for an auth invocation",
	Long:  `Submit field values for an authentication invocation (e.g., username, password, 2FA codes).`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvocationsSubmit,
}

func init() {
	// agents auth subcommands
	agentsAuthCmd.AddCommand(agentsAuthListCmd)
	agentsAuthCmd.AddCommand(agentsAuthGetCmd)
	agentsAuthCmd.AddCommand(agentsAuthCreateCmd)
	agentsAuthCmd.AddCommand(agentsAuthDeleteCmd)
	agentsAuthCmd.AddCommand(agentsAuthInvocationsCmd)

	// agents auth invocations subcommands
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsCreateCmd)
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsGetCmd)
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsExchangeCmd)
	agentsAuthInvocationsCmd.AddCommand(agentsAuthInvocationsSubmitCmd)

	// agents subcommands
	agentsCmd.AddCommand(agentsAuthCmd)

	// List flags
	agentsAuthListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthListCmd.Flags().Int("limit", 0, "Maximum number of results to return")
	agentsAuthListCmd.Flags().Int("offset", 0, "Number of results to skip")

	// Get flags
	agentsAuthGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Create flags
	agentsAuthCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthCreateCmd.Flags().String("profile-name", "", "Name of the profile to use (required)")
	agentsAuthCreateCmd.Flags().String("domain", "", "Target domain for authentication (required)")
	agentsAuthCreateCmd.Flags().String("credential-name", "", "Name of existing credential to link (optional)")
	agentsAuthCreateCmd.Flags().String("login-url", "", "Login page URL to skip discovery (optional)")
	_ = agentsAuthCreateCmd.MarkFlagRequired("profile-name")
	_ = agentsAuthCreateCmd.MarkFlagRequired("domain")

	// Delete flags
	agentsAuthDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Invocations create flags
	agentsAuthInvocationsCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthInvocationsCreateCmd.Flags().String("auth-agent-id", "", "ID of the auth agent (required)")
	agentsAuthInvocationsCreateCmd.Flags().String("save-credential-as", "", "Save submitted credentials under this name (optional)")
	_ = agentsAuthInvocationsCreateCmd.MarkFlagRequired("auth-agent-id")

	// Invocations get flags
	agentsAuthInvocationsGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Invocations exchange flags
	agentsAuthInvocationsExchangeCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthInvocationsExchangeCmd.Flags().String("code", "", "Handoff code from create invocation (required)")
	_ = agentsAuthInvocationsExchangeCmd.MarkFlagRequired("code")

	// Invocations submit flags
	agentsAuthInvocationsSubmitCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	agentsAuthInvocationsSubmitCmd.Flags().StringArray("value", []string{}, "Field name=value pair (repeatable)")
}

func runAgentsAuthList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	svc := client.Agents.Auth
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.List(cmd.Context(), AgentsAuthListInput{
		Limit:  limit,
		Offset: offset,
		Output: output,
	})
}

func runAgentsAuthGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Agents.Auth
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.Get(cmd.Context(), AgentsAuthGetInput{
		ID:     args[0],
		Output: output,
	})
}

func runAgentsAuthCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	profileName, _ := cmd.Flags().GetString("profile-name")
	domain, _ := cmd.Flags().GetString("domain")
	credentialName, _ := cmd.Flags().GetString("credential-name")
	loginURL, _ := cmd.Flags().GetString("login-url")

	svc := client.Agents.Auth
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.Create(cmd.Context(), AgentsAuthCreateInput{
		ProfileName:    profileName,
		Domain:         domain,
		CredentialName: credentialName,
		LoginURL:       loginURL,
		Output:         output,
	})
}

func runAgentsAuthDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skip, _ := cmd.Flags().GetBool("yes")

	svc := client.Agents.Auth
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.Delete(cmd.Context(), AgentsAuthDeleteInput{
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
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.InvocationCreate(cmd.Context(), AgentsAuthInvocationCreateInput{
		AuthAgentID:      authAgentID,
		SaveCredentialAs: saveCredentialAs,
		Output:           output,
	})
}

func runAgentsAuthInvocationsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Agents.Auth
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.InvocationGet(cmd.Context(), AgentsAuthInvocationGetInput{
		InvocationID: args[0],
		Output:       output,
	})
}

func runAgentsAuthInvocationsExchange(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	code, _ := cmd.Flags().GetString("code")

	svc := client.Agents.Auth
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.InvocationExchange(cmd.Context(), AgentsAuthInvocationExchangeInput{
		InvocationID: args[0],
		HandoffCode:  code,
		Output:       output,
	})
}

func runAgentsAuthInvocationsSubmit(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	valuePairs, _ := cmd.Flags().GetStringArray("value")

	// Parse value pairs into map
	values := make(map[string]string)
	for _, pair := range valuePairs {
		parts := splitValue(pair)
		if len(parts) != 2 {
			return fmt.Errorf("invalid value format: %s (expected key=value)", pair)
		}
		values[parts[0]] = parts[1]
	}

	svc := client.Agents.Auth
	invSvc := client.Agents.Auth.Invocations
	c := AgentsAuthCmd{auth: &svc, invocations: &invSvc}
	return c.InvocationSubmit(cmd.Context(), AgentsAuthInvocationSubmitInput{
		InvocationID: args[0],
		Values:       values,
		Output:       output,
	})
}

// splitValue splits a key=value string into [key, value]
func splitValue(s string) []string {
	idx := -1
	for i, c := range s {
		if c == '=' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
