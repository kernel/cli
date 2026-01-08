package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/onkernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pkg/browser"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// AgentAuthService defines the subset of the Kernel SDK agent auth client that we use.
type AgentAuthService interface {
	New(ctx context.Context, body kernel.AgentAuthNewParams, opts ...option.RequestOption) (*kernel.AuthAgent, error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.AuthAgent, error)
	List(ctx context.Context, query kernel.AgentAuthListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.AuthAgent], error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) error
}

// AgentAuthInvocationService defines the subset we use for agent auth invocations.
type AgentAuthInvocationService interface {
	New(ctx context.Context, body kernel.AgentAuthInvocationNewParams, opts ...option.RequestOption) (*kernel.AuthAgentInvocationCreateResponse, error)
	Get(ctx context.Context, invocationID string, opts ...option.RequestOption) (*kernel.AgentAuthInvocationResponse, error)
	Submit(ctx context.Context, invocationID string, body kernel.AgentAuthInvocationSubmitParams, opts ...option.RequestOption) (*kernel.AgentAuthSubmitResponse, error)
}

// AgentAuthCmd handles agent auth operations.
type AgentAuthCmd struct {
	auth        AgentAuthService
	invocations AgentAuthInvocationService
	browsers    BrowsersService
}

// CreateInput holds input for creating an auth agent.
type CreateInput struct {
	Domain         string
	ProfileName    string
	CredentialName string
	LoginURL       string
	AllowedDomains []string
}

// Create creates a new auth agent.
func (a AgentAuthCmd) Create(ctx context.Context, in CreateInput) error {
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

	agent, err := a.auth.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", agent.ID})
	rows = append(rows, []string{"Domain", agent.Domain})
	rows = append(rows, []string{"Profile Name", agent.ProfileName})
	rows = append(rows, []string{"Status", string(agent.Status)})
	if agent.CredentialName != "" {
		rows = append(rows, []string{"Credential", agent.CredentialName})
	}
	if len(agent.AllowedDomains) > 0 {
		rows = append(rows, []string{"Allowed Domains", fmt.Sprintf("%v", agent.AllowedDomains)})
	}

	PrintTableNoPad(rows, true)

	pterm.Println()
	pterm.Info.Println("Next step: Start the authentication flow with:")
	pterm.Printf("  kernel agents auth invoke %s\n", agent.ID)

	return nil
}

// InvokeInput holds input for starting an invocation.
type InvokeInput struct {
	AuthAgentID      string
	SaveCredentialAs string
	NoBrowser        bool
}

// Invoke starts an auth invocation and handles the hosted UI flow.
func (a AgentAuthCmd) Invoke(ctx context.Context, in InvokeInput) error {
	params := kernel.AgentAuthInvocationNewParams{
		AuthAgentInvocationCreateRequest: kernel.AuthAgentInvocationCreateRequestParam{
			AuthAgentID: in.AuthAgentID,
		},
	}

	if in.SaveCredentialAs != "" {
		params.AuthAgentInvocationCreateRequest.SaveCredentialAs = kernel.Opt(in.SaveCredentialAs)
	}

	invocation, err := a.invocations.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Info.Println("Invocation created")
	pterm.Println(fmt.Sprintf("  Invocation ID: %s", invocation.InvocationID))
	pterm.Println(fmt.Sprintf("  Type: %s", invocation.Type))
	pterm.Println(fmt.Sprintf("  Expires: %s", invocation.ExpiresAt.Format(time.RFC3339)))
	pterm.Println()

	pterm.Info.Println("Open this URL in your browser to log in:")
	pterm.Println()
	pterm.Println(fmt.Sprintf("  %s", invocation.HostedURL))
	pterm.Println()

	if !in.NoBrowser {
		if err := browser.OpenURL(invocation.HostedURL); err != nil {
			pterm.Warning.Printf("Could not open browser automatically: %v\n", err)
		} else {
			pterm.Info.Println("(Opened in browser)")
		}
	}

	pterm.Println()
	pterm.Info.Println("Polling for completion...")

	startTime := time.Now()
	maxWaitTime := 5 * time.Minute
	pollInterval := 2 * time.Second

	for time.Since(startTime) < maxWaitTime {
		state, err := a.invocations.Get(ctx, invocation.InvocationID)
		if err != nil {
			return util.CleanedUpSdkError{Err: err}
		}

		elapsed := int(time.Since(startTime).Seconds())
		pterm.Println(fmt.Sprintf("  [%ds] status=%s, step=%s", elapsed, state.Status, state.Step))

		// Show live view URL on first poll
		if state.LiveViewURL != "" && elapsed < 5 {
			pterm.Println(fmt.Sprintf("    Live view: %s", state.LiveViewURL))
		}

		// Show pending fields if any
		if len(state.PendingFields) > 0 {
			var fieldNames []string
			for _, f := range state.PendingFields {
				fieldNames = append(fieldNames, f.Name)
			}
			pterm.Println(fmt.Sprintf("    Fields: %v", fieldNames))
		}

		// Show SSO buttons if any
		if len(state.PendingSSOButtons) > 0 {
			var providers []string
			for _, b := range state.PendingSSOButtons {
				providers = append(providers, b.Provider)
			}
			pterm.Println(fmt.Sprintf("    SSO buttons: %v", providers))
		}

		// Show external action message
		if state.Step == kernel.AgentAuthInvocationResponseStepAwaitingExternalAction && state.ExternalActionMessage != "" {
			pterm.Warning.Printf("    External action required: %s\n", state.ExternalActionMessage)
		}

		switch state.Status {
		case kernel.AgentAuthInvocationResponseStatusSuccess:
			pterm.Println()
			pterm.Success.Println("Login completed successfully!")

			// Fetch and display the auth agent
			agent, err := a.auth.Get(ctx, in.AuthAgentID)
			if err != nil {
				pterm.Warning.Printf("Could not fetch auth agent: %v\n", err)
				return nil
			}

			pterm.Println()
			pterm.Println(fmt.Sprintf("  Auth Agent: %s", agent.ID))
			pterm.Println(fmt.Sprintf("  Profile: %s", agent.ProfileName))
			pterm.Println(fmt.Sprintf("  Domain: %s", agent.Domain))
			pterm.Println(fmt.Sprintf("  Status: %s", agent.Status))
			if agent.CredentialName != "" {
				pterm.Println(fmt.Sprintf("  Credential: %s", agent.CredentialName))
			}

			pterm.Println()
			pterm.Info.Printf("You can now create browsers with profile: %s\n", agent.ProfileName)
			return nil

		case kernel.AgentAuthInvocationResponseStatusExpired:
			pterm.Println()
			pterm.Error.Println("Invocation expired")
			return nil

		case kernel.AgentAuthInvocationResponseStatusCanceled:
			pterm.Println()
			pterm.Error.Println("Invocation was canceled")
			return nil

		case kernel.AgentAuthInvocationResponseStatusFailed:
			pterm.Println()
			pterm.Error.Println("Invocation failed")
			if state.ErrorMessage != "" {
				pterm.Error.Printf("  Error: %s\n", state.ErrorMessage)
			}
			return nil
		}

		time.Sleep(pollInterval)
	}

	pterm.Error.Println("Polling timed out after 5 minutes")
	return nil
}

// GetInput holds input for getting an auth agent.
type GetInput struct {
	ID string
}

// Get retrieves an auth agent by ID.
func (a AgentAuthCmd) Get(ctx context.Context, in GetInput) error {
	agent, err := a.auth.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"ID", agent.ID})
	rows = append(rows, []string{"Domain", agent.Domain})
	rows = append(rows, []string{"Profile Name", agent.ProfileName})
	rows = append(rows, []string{"Status", string(agent.Status)})
	if agent.CredentialName != "" {
		rows = append(rows, []string{"Credential", agent.CredentialName})
	}
	if len(agent.AllowedDomains) > 0 {
		rows = append(rows, []string{"Allowed Domains", fmt.Sprintf("%v", agent.AllowedDomains)})
	}
	rows = append(rows, []string{"Can Reauth", fmt.Sprintf("%t", agent.CanReauth)})
	rows = append(rows, []string{"Has Selectors", fmt.Sprintf("%t", agent.HasSelectors)})
	if !agent.LastAuthCheckAt.IsZero() {
		rows = append(rows, []string{"Last Auth Check", agent.LastAuthCheckAt.Format(time.RFC3339)})
	}

	PrintTableNoPad(rows, true)
	return nil
}

// DeleteInput holds input for deleting an auth agent.
type DeleteInput struct {
	ID string
}

// Delete removes an auth agent.
func (a AgentAuthCmd) Delete(ctx context.Context, in DeleteInput) error {
	if err := a.auth.Delete(ctx, in.ID); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	pterm.Success.Printf("Auth agent %s deleted\n", in.ID)
	return nil
}

// ListInput holds input for listing auth agents.
type ListInput struct {
	Domain      string
	ProfileName string
	Limit       int64
	Offset      int64
}

// List lists auth agents.
func (a AgentAuthCmd) List(ctx context.Context, in ListInput) error {
	params := kernel.AgentAuthListParams{}
	if in.Domain != "" {
		params.Domain = kernel.Opt(in.Domain)
	}
	if in.ProfileName != "" {
		params.ProfileName = kernel.Opt(in.ProfileName)
	}
	if in.Limit > 0 {
		params.Limit = kernel.Opt(in.Limit)
	}
	if in.Offset > 0 {
		params.Offset = kernel.Opt(in.Offset)
	}

	page, err := a.auth.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	agents := page.Items
	if len(agents) == 0 {
		pterm.Info.Println("No auth agents found")
		return nil
	}

	rows := pterm.TableData{{"ID", "Domain", "Profile", "Status", "Can Reauth"}}
	for _, agent := range agents {
		rows = append(rows, []string{
			agent.ID,
			agent.Domain,
			agent.ProfileName,
			string(agent.Status),
			fmt.Sprintf("%t", agent.CanReauth),
		})
	}

	PrintTableNoPad(rows, true)
	return nil
}

// GetInvocationInput holds input for getting an invocation.
type GetInvocationInput struct {
	InvocationID string
}

// GetInvocation retrieves an invocation by ID.
func (a AgentAuthCmd) GetInvocation(ctx context.Context, in GetInvocationInput) error {
	state, err := a.invocations.Get(ctx, in.InvocationID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	rows := pterm.TableData{{"Property", "Value"}}
	rows = append(rows, []string{"Status", string(state.Status)})
	rows = append(rows, []string{"Step", string(state.Step)})
	rows = append(rows, []string{"Type", string(state.Type)})
	rows = append(rows, []string{"Domain", state.Domain})
	rows = append(rows, []string{"App Name", state.AppName})
	rows = append(rows, []string{"Expires", state.ExpiresAt.Format(time.RFC3339)})
	if state.LiveViewURL != "" {
		rows = append(rows, []string{"Live View", state.LiveViewURL})
	}
	if state.ErrorMessage != "" {
		rows = append(rows, []string{"Error", state.ErrorMessage})
	}
	if state.ExternalActionMessage != "" {
		rows = append(rows, []string{"External Action", state.ExternalActionMessage})
	}

	PrintTableNoPad(rows, true)

	// Show pending fields if any
	if len(state.PendingFields) > 0 {
		pterm.Println()
		pterm.Info.Println("Pending Fields:")
		fieldRows := pterm.TableData{{"Name", "Type", "Label", "Required"}}
		for _, f := range state.PendingFields {
			fieldRows = append(fieldRows, []string{f.Name, string(f.Type), f.Label, fmt.Sprintf("%t", f.Required)})
		}
		PrintTableNoPad(fieldRows, true)
	}

	// Show SSO buttons if any
	if len(state.PendingSSOButtons) > 0 {
		pterm.Println()
		pterm.Info.Println("SSO Buttons:")
		buttonRows := pterm.TableData{{"Provider", "Label", "Selector"}}
		for _, b := range state.PendingSSOButtons {
			buttonRows = append(buttonRows, []string{b.Provider, b.Label, b.Selector})
		}
		PrintTableNoPad(buttonRows, true)
	}

	return nil
}

// SubmitInvocationInput holds input for submitting to an invocation.
type SubmitInvocationInput struct {
	InvocationID string
	FieldValues  map[string]string
	SSOButton    string
}

// SubmitInvocation submits field values or clicks an SSO button.
func (a AgentAuthCmd) SubmitInvocation(ctx context.Context, in SubmitInvocationInput) error {
	var params kernel.AgentAuthInvocationSubmitParams

	if in.SSOButton != "" {
		params.OfSSOButton = &kernel.AgentAuthInvocationSubmitParamsBodySSOButton{
			SSOButton: in.SSOButton,
		}
	} else if len(in.FieldValues) > 0 {
		params.OfFieldValues = &kernel.AgentAuthInvocationSubmitParamsBodyFieldValues{
			FieldValues: in.FieldValues,
		}
	} else {
		return fmt.Errorf("must provide either --field or --sso-button")
	}

	resp, err := a.invocations.Submit(ctx, in.InvocationID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
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
	Long:  "Commands for managing Kernel agents",
}

var agentsAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage auth agents",
	Long:  "Commands for managing agent authentication",
}

var agentsAuthCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an auth agent",
	Long:  "Create a new auth agent for a domain and profile",
	Args:  cobra.NoArgs,
	RunE:  runAgentsAuthCreate,
}

var agentsAuthInvokeCmd = &cobra.Command{
	Use:   "invoke <auth-agent-id>",
	Short: "Start an auth invocation",
	Long:  "Start an authentication invocation using the hosted UI flow",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvoke,
}

var agentsAuthGetCmd = &cobra.Command{
	Use:   "get <auth-agent-id>",
	Short: "Get auth agent details",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthGet,
}

var agentsAuthDeleteCmd = &cobra.Command{
	Use:   "delete <auth-agent-id>",
	Short: "Delete an auth agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthDelete,
}

var agentsAuthListCmd = &cobra.Command{
	Use:   "list",
	Short: "List auth agents",
	Long:  "List auth agents with optional filters",
	Args:  cobra.NoArgs,
	RunE:  runAgentsAuthList,
}

var agentsAuthInvocationCmd = &cobra.Command{
	Use:   "invocation",
	Short: "Manage auth invocations",
	Long:  "Commands for managing individual auth invocations",
}

var agentsAuthInvocationGetCmd = &cobra.Command{
	Use:   "get <invocation-id>",
	Short: "Get invocation details",
	Long:  "Get the current status and details of an invocation",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvocationGet,
}

var agentsAuthInvocationSubmitCmd = &cobra.Command{
	Use:   "submit <invocation-id>",
	Short: "Submit to an invocation",
	Long:  "Submit field values or click an SSO button for an invocation",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsAuthInvocationSubmit,
}

func init() {
	agentsAuthCmd.AddCommand(agentsAuthCreateCmd)
	agentsAuthCmd.AddCommand(agentsAuthInvokeCmd)
	agentsAuthCmd.AddCommand(agentsAuthGetCmd)
	agentsAuthCmd.AddCommand(agentsAuthDeleteCmd)
	agentsAuthCmd.AddCommand(agentsAuthListCmd)
	agentsAuthCmd.AddCommand(agentsAuthInvocationCmd)
	agentsCmd.AddCommand(agentsAuthCmd)

	// invocation subcommands
	agentsAuthInvocationCmd.AddCommand(agentsAuthInvocationGetCmd)
	agentsAuthInvocationCmd.AddCommand(agentsAuthInvocationSubmitCmd)

	// create flags
	agentsAuthCreateCmd.Flags().String("domain", "", "Target domain to authenticate with")
	agentsAuthCreateCmd.Flags().String("profile-name", "", "Profile name to use or create")
	agentsAuthCreateCmd.Flags().String("credential-name", "", "Optional credential name to link")
	agentsAuthCreateCmd.Flags().String("login-url", "", "Optional login URL to skip discovery")
	agentsAuthCreateCmd.Flags().StringSlice("allowed-domains", nil, "Additional allowed domains for OAuth redirects")
	agentsAuthCreateCmd.Flags().BoolP("interactive", "i", false, "Interactive mode - select profile and credential from lists")

	// invoke flags
	agentsAuthInvokeCmd.Flags().String("save-credential-as", "", "Save credentials under this name after successful login")
	agentsAuthInvokeCmd.Flags().Bool("no-browser", false, "Don't automatically open browser")

	// list flags
	agentsAuthListCmd.Flags().String("domain", "", "Filter by domain")
	agentsAuthListCmd.Flags().String("profile-name", "", "Filter by profile name")
	agentsAuthListCmd.Flags().Int64("limit", 0, "Maximum number of results")
	agentsAuthListCmd.Flags().Int64("offset", 0, "Number of results to skip")

	// invocation submit flags
	agentsAuthInvocationSubmitCmd.Flags().StringToString("field", nil, "Field values to submit (key=value)")
	agentsAuthInvocationSubmitCmd.Flags().String("sso-button", "", "SSO button selector to click")
}

func runAgentsAuthCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	ctx := cmd.Context()

	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile-name")
	credentialName, _ := cmd.Flags().GetString("credential-name")
	loginURL, _ := cmd.Flags().GetString("login-url")
	allowedDomains, _ := cmd.Flags().GetStringSlice("allowed-domains")
	interactive, _ := cmd.Flags().GetBool("interactive")

	if interactive {
		var err error

		// Prompt for domain if not provided
		if domain == "" {
			domain, err = pterm.DefaultInteractiveTextInput.
				WithDefaultText("Enter target domain (e.g., github.com)").
				Show()
			if err != nil {
				return fmt.Errorf("failed to get domain: %w", err)
			}
		}

		// Let user select profile or create new
		if profileName == "" {
			profiles, err := client.Profiles.List(ctx)
			if err != nil {
				return util.CleanedUpSdkError{Err: err}
			}

			options := []string{"[Create new profile]"}
			profileMap := make(map[string]string) // display -> name
			if profiles != nil {
				for _, p := range *profiles {
					display := p.Name
					if display == "" {
						display = p.ID
					}
					options = append(options, display)
					profileMap[display] = p.Name
					if p.Name == "" {
						profileMap[display] = p.ID
					}
				}
			}

			selected, err := pterm.DefaultInteractiveSelect.
				WithDefaultText("Select a profile").
				WithOptions(options).
				WithFilter(true).
				Show()
			if err != nil {
				return fmt.Errorf("failed to select profile: %w", err)
			}

			if selected == "[Create new profile]" {
				profileName, err = pterm.DefaultInteractiveTextInput.
					WithDefaultText("Enter new profile name").
					Show()
				if err != nil {
					return fmt.Errorf("failed to get profile name: %w", err)
				}
			} else {
				profileName = profileMap[selected]
			}
		}

		// Let user select credential (optional)
		if credentialName == "" {
			credsPage, err := client.Credentials.List(ctx, kernel.CredentialListParams{})
			if err != nil {
				pterm.Warning.Printf("Could not fetch credentials: %v\n", err)
			} else if len(credsPage.Items) > 0 {
				options := []string{"[Skip - no credential]"}
				credMap := make(map[string]string) // display -> name
				for _, c := range credsPage.Items {
					display := fmt.Sprintf("%s (%s)", c.Name, c.Domain)
					options = append(options, display)
					credMap[display] = c.Name
				}

				selected, err := pterm.DefaultInteractiveSelect.
					WithDefaultText("Select a credential (optional)").
					WithOptions(options).
					WithFilter(true).
					Show()
				if err != nil {
					return fmt.Errorf("failed to select credential: %w", err)
				}

				if selected != "[Skip - no credential]" {
					credentialName = credMap[selected]
				}
			}
		}

		// Validate we have required fields
		if domain == "" {
			return fmt.Errorf("domain is required")
		}
		if profileName == "" {
			return fmt.Errorf("profile name is required")
		}
	} else {
		// Non-interactive mode - validate required fields
		if domain == "" {
			return fmt.Errorf("--domain is required (or use -i for interactive mode)")
		}
		if profileName == "" {
			return fmt.Errorf("--profile-name is required (or use -i for interactive mode)")
		}
	}

	svc := client.Agents.Auth
	a := AgentAuthCmd{auth: &svc}

	return a.Create(ctx, CreateInput{
		Domain:         domain,
		ProfileName:    profileName,
		CredentialName: credentialName,
		LoginURL:       loginURL,
		AllowedDomains: allowedDomains,
	})
}

func runAgentsAuthInvoke(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	saveCredentialAs, _ := cmd.Flags().GetString("save-credential-as")
	noBrowser, _ := cmd.Flags().GetBool("no-browser")

	svc := client.Agents.Auth
	invocationsSvc := client.Agents.Auth.Invocations
	a := AgentAuthCmd{auth: &svc, invocations: &invocationsSvc}

	return a.Invoke(cmd.Context(), InvokeInput{
		AuthAgentID:      args[0],
		SaveCredentialAs: saveCredentialAs,
		NoBrowser:        noBrowser,
	})
}

func runAgentsAuthGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	svc := client.Agents.Auth
	a := AgentAuthCmd{auth: &svc}

	return a.Get(cmd.Context(), GetInput{ID: args[0]})
}

func runAgentsAuthDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	svc := client.Agents.Auth
	a := AgentAuthCmd{auth: &svc}

	return a.Delete(cmd.Context(), DeleteInput{ID: args[0]})
}

func runAgentsAuthList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile-name")
	limit, _ := cmd.Flags().GetInt64("limit")
	offset, _ := cmd.Flags().GetInt64("offset")

	svc := client.Agents.Auth
	a := AgentAuthCmd{auth: &svc}

	return a.List(cmd.Context(), ListInput{
		Domain:      domain,
		ProfileName: profileName,
		Limit:       limit,
		Offset:      offset,
	})
}

func runAgentsAuthInvocationGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	invocationsSvc := client.Agents.Auth.Invocations
	a := AgentAuthCmd{invocations: &invocationsSvc}

	return a.GetInvocation(cmd.Context(), GetInvocationInput{InvocationID: args[0]})
}

func runAgentsAuthInvocationSubmit(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	fieldValues, _ := cmd.Flags().GetStringToString("field")
	ssoButton, _ := cmd.Flags().GetString("sso-button")

	invocationsSvc := client.Agents.Auth.Invocations
	a := AgentAuthCmd{invocations: &invocationsSvc}

	return a.SubmitInvocation(cmd.Context(), SubmitInvocationInput{
		InvocationID: args[0],
		FieldValues:  fieldValues,
		SSOButton:    ssoButton,
	})
}
