package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// AuthConnectionService defines the subset of the Kernel SDK auth connection client that we use.
type AuthConnectionService interface {
	New(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (res *kernel.ManagedAuth, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.ManagedAuth, err error)
	List(ctx context.Context, query kernel.AuthConnectionListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.ManagedAuth], err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
	Login(ctx context.Context, id string, body kernel.AuthConnectionLoginParams, opts ...option.RequestOption) (res *kernel.LoginResponse, err error)
	Submit(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (res *kernel.SubmitFieldsResponse, err error)
	FollowStreaming(ctx context.Context, id string, opts ...option.RequestOption) (stream *ssestream.Stream[kernel.AuthConnectionFollowResponseUnion])
}

// AuthConnectionCmd handles auth connection operations independent of cobra.
type AuthConnectionCmd struct {
	svc AuthConnectionService
}

type AuthConnectionCreateInput struct {
	Domain              string
	ProfileName         string
	LoginURL            string
	AllowedDomains      []string
	CredentialName      string
	CredentialProvider  string
	CredentialPath      string
	CredentialAuto      bool
	ProxyID             string
	ProxyName           string
	SaveCredentials     bool
	NoSaveCredentials   bool
	HealthCheckInterval int
	Output              string
}

type AuthConnectionGetInput struct {
	ID     string
	Output string
}

type AuthConnectionListInput struct {
	Domain      string
	ProfileName string
	Limit       int
	Offset      int
	Output      string
}

type AuthConnectionDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type AuthConnectionLoginInput struct {
	ID        string
	ProxyID   string
	ProxyName string
	Output    string
}

type AuthConnectionSubmitInput struct {
	ID                string
	FieldValues       map[string]string
	MfaOptionID       string
	SSOButtonSelector string
	Output            string
}

type AuthConnectionFollowInput struct {
	ID     string
	Output string
}

func (c AuthConnectionCmd) Create(ctx context.Context, in AuthConnectionCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Domain == "" {
		return fmt.Errorf("--domain is required")
	}
	if in.ProfileName == "" {
		return fmt.Errorf("--profile-name is required")
	}

	params := kernel.AuthConnectionNewParams{
		ManagedAuthCreateRequest: kernel.ManagedAuthCreateRequestParam{
			Domain:      in.Domain,
			ProfileName: in.ProfileName,
		},
	}
	if in.LoginURL != "" {
		params.ManagedAuthCreateRequest.LoginURL = kernel.Opt(in.LoginURL)
	}
	if len(in.AllowedDomains) > 0 {
		params.ManagedAuthCreateRequest.AllowedDomains = in.AllowedDomains
	}
	if in.HealthCheckInterval > 0 {
		params.ManagedAuthCreateRequest.HealthCheckInterval = kernel.Opt(int64(in.HealthCheckInterval))
	}

	// Handle credential reference
	if in.CredentialName != "" {
		params.ManagedAuthCreateRequest.Credential = kernel.ManagedAuthCreateRequestCredentialParam{
			Name: kernel.Opt(in.CredentialName),
		}
	} else if in.CredentialProvider != "" {
		params.ManagedAuthCreateRequest.Credential = kernel.ManagedAuthCreateRequestCredentialParam{
			Provider: kernel.Opt(in.CredentialProvider),
		}
		if in.CredentialPath != "" {
			params.ManagedAuthCreateRequest.Credential.Path = kernel.Opt(in.CredentialPath)
		}
		if in.CredentialAuto {
			params.ManagedAuthCreateRequest.Credential.Auto = kernel.Opt(true)
		}
	}

	if in.ProxyID != "" || in.ProxyName != "" {
		params.ManagedAuthCreateRequest.Proxy = kernel.ManagedAuthCreateRequestProxyParam{}
		if in.ProxyID != "" {
			params.ManagedAuthCreateRequest.Proxy.ID = kernel.Opt(in.ProxyID)
		}
		if in.ProxyName != "" {
			params.ManagedAuthCreateRequest.Proxy.Name = kernel.Opt(in.ProxyName)
		}
	}

	if in.NoSaveCredentials {
		params.ManagedAuthCreateRequest.SaveCredentials = kernel.Opt(false)
	}

	if in.Output != "json" {
		pterm.Info.Printf("Creating managed auth for %s...\n", in.Domain)
	}

	auth, err := c.svc.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(auth)
	}

	pterm.Success.Printf("Created managed auth: %s\n", auth.ID)

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", auth.ID},
		{"Domain", auth.Domain},
		{"Profile Name", auth.ProfileName},
		{"Status", string(auth.Status)},
		{"Can Reauth", fmt.Sprintf("%t", auth.CanReauth)},
	}
	if auth.CanReauthReason != "" {
		tableData = append(tableData, []string{"Can Reauth Reason", auth.CanReauthReason})
	}
	if auth.Credential.Name != "" {
		tableData = append(tableData, []string{"Credential Name", auth.Credential.Name})
	}
	if auth.Credential.Provider != "" {
		tableData = append(tableData, []string{"Credential Provider", auth.Credential.Provider})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AuthConnectionCmd) Get(ctx context.Context, in AuthConnectionGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	auth, err := c.svc.Get(ctx, in.ID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(auth)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", auth.ID},
		{"Domain", auth.Domain},
		{"Profile Name", auth.ProfileName},
		{"Status", string(auth.Status)},
		{"Can Reauth", fmt.Sprintf("%t", auth.CanReauth)},
	}
	if auth.CanReauthReason != "" {
		tableData = append(tableData, []string{"Can Reauth Reason", auth.CanReauthReason})
	}
	if auth.Credential.Name != "" {
		tableData = append(tableData, []string{"Credential Name", auth.Credential.Name})
	}
	if auth.Credential.Provider != "" {
		tableData = append(tableData, []string{"Credential Provider", auth.Credential.Provider})
	}
	if auth.FlowStatus != "" {
		tableData = append(tableData, []string{"Flow Status", string(auth.FlowStatus)})
	}
	if auth.FlowStep != "" {
		tableData = append(tableData, []string{"Flow Step", string(auth.FlowStep)})
	}
	if auth.HostedURL != "" {
		tableData = append(tableData, []string{"Hosted URL", auth.HostedURL})
	}
	if auth.LiveViewURL != "" {
		tableData = append(tableData, []string{"Live View URL", auth.LiveViewURL})
	}
	if auth.ErrorMessage != "" {
		tableData = append(tableData, []string{"Error Message", auth.ErrorMessage})
	}
	if !auth.LastAuthAt.IsZero() {
		tableData = append(tableData, []string{"Last Auth At", util.FormatLocal(auth.LastAuthAt)})
	}
	if len(auth.AllowedDomains) > 0 {
		tableData = append(tableData, []string{"Allowed Domains", strings.Join(auth.AllowedDomains, ", ")})
	}
	if auth.HealthCheckInterval > 0 {
		tableData = append(tableData, []string{"Health Check Interval", fmt.Sprintf("%d seconds", auth.HealthCheckInterval)})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AuthConnectionCmd) List(ctx context.Context, in AuthConnectionListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.AuthConnectionListParams{}
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

	page, err := c.svc.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var auths []kernel.ManagedAuth
	if page != nil {
		auths = page.Items
	}

	if in.Output == "json" {
		if len(auths) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(auths)
	}

	if len(auths) == 0 {
		pterm.Info.Println("No managed auths found")
		return nil
	}

	tableData := pterm.TableData{{"ID", "Domain", "Profile Name", "Status", "Can Reauth"}}
	for _, auth := range auths {
		tableData = append(tableData, []string{
			auth.ID,
			auth.Domain,
			auth.ProfileName,
			string(auth.Status),
			fmt.Sprintf("%t", auth.CanReauth),
		})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AuthConnectionCmd) Delete(ctx context.Context, in AuthConnectionDeleteInput) error {
	if !in.SkipConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete managed auth '%s'?", in.ID)
		pterm.DefaultInteractiveConfirm.DefaultText = msg
		ok, _ := pterm.DefaultInteractiveConfirm.Show()
		if !ok {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}
	}

	if err := c.svc.Delete(ctx, in.ID); err != nil {
		if util.IsNotFound(err) {
			pterm.Info.Printf("Managed auth '%s' not found\n", in.ID)
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted managed auth: %s\n", in.ID)
	return nil
}

func (c AuthConnectionCmd) Login(ctx context.Context, in AuthConnectionLoginInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.AuthConnectionLoginParams{}
	if in.ProxyID != "" || in.ProxyName != "" {
		params.Proxy = kernel.AuthConnectionLoginParamsProxy{}
		if in.ProxyID != "" {
			params.Proxy.ID = kernel.Opt(in.ProxyID)
		}
		if in.ProxyName != "" {
			params.Proxy.Name = kernel.Opt(in.ProxyName)
		}
	}

	if in.Output != "json" {
		pterm.Info.Println("Starting login flow...")
	}

	resp, err := c.svc.Login(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	pterm.Success.Printf("Login flow started: %s\n", resp.FlowType)

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", resp.ID},
		{"Flow Type", string(resp.FlowType)},
		{"Hosted URL", resp.HostedURL},
		{"Flow Expires At", util.FormatLocal(resp.FlowExpiresAt)},
	}
	if resp.LiveViewURL != "" {
		tableData = append(tableData, []string{"Live View URL", resp.LiveViewURL})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AuthConnectionCmd) Submit(ctx context.Context, in AuthConnectionSubmitInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	// Validate that we have some input to submit
	hasFields := len(in.FieldValues) > 0
	hasMfaOption := in.MfaOptionID != ""
	hasSSOButton := in.SSOButtonSelector != ""

	if !hasFields && !hasMfaOption && !hasSSOButton {
		return fmt.Errorf("must provide at least one of: --field, --mfa-option-id, or --sso-button-selector")
	}

	params := kernel.AuthConnectionSubmitParams{
		SubmitFieldsRequest: kernel.SubmitFieldsRequestParam{
			Fields: in.FieldValues,
		},
	}
	if hasMfaOption {
		params.SubmitFieldsRequest.MfaOptionID = kernel.Opt(in.MfaOptionID)
	}
	if hasSSOButton {
		params.SubmitFieldsRequest.SSOButtonSelector = kernel.Opt(in.SSOButtonSelector)
	}

	if in.Output != "json" {
		pterm.Info.Println("Submitting to managed auth...")
	}

	resp, err := c.svc.Submit(ctx, in.ID, params)
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

func (c AuthConnectionCmd) Follow(ctx context.Context, in AuthConnectionFollowInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	stream := c.svc.FollowStreaming(ctx, in.ID)
	if stream == nil {
		return fmt.Errorf("failed to establish SSE stream")
	}
	defer stream.Close()

	if in.Output != "json" {
		pterm.Info.Println("Following managed auth events (Ctrl+C to stop)...")
	}

	for stream.Next() {
		event := stream.Current()

		if in.Output == "json" {
			if err := util.PrintPrettyJSON(event); err != nil {
				return err
			}
			continue
		}

		// Human-readable output
		switch event.Event {
		case "managed_auth_state":
			state := event.AsManagedAuthState()
			pterm.Info.Printf("[%s] Status: %s, Step: %s\n",
				state.Timestamp.Local().Format(time.RFC3339),
				state.FlowStatus,
				state.FlowStep)
			if len(state.DiscoveredFields) > 0 {
				var fieldNames []string
				for _, f := range state.DiscoveredFields {
					fieldNames = append(fieldNames, f.Name)
				}
				pterm.Info.Printf("  Discovered fields: %s\n", strings.Join(fieldNames, ", "))
			}
			if state.ErrorMessage != "" {
				pterm.Error.Printf("  Error: %s\n", state.ErrorMessage)
			}
			if state.WebsiteError != "" {
				pterm.Warning.Printf("  Website error: %s\n", state.WebsiteError)
			}
		case "error":
			errEvent := event.AsError()
			pterm.Error.Printf("Error: %s\n", errEvent.Error.Message)
		case "sse_heartbeat":
			// Silently ignore heartbeats for human-readable output
		}
	}

	if err := stream.Err(); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output != "json" {
		pterm.Success.Println("Stream ended")
	}
	return nil
}

// --- Cobra wiring ---

var authConnectionsCmd = &cobra.Command{
	Use:   "connections",
	Short: "Manage auth connections (managed auth)",
	Long:  "Commands for managing authentication connections that keep profiles logged into domains",
}

var authConnectionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a managed auth connection",
	Long:  "Create managed authentication for a profile and domain combination",
	Args:  cobra.NoArgs,
	RunE:  runAuthConnectionsCreate,
}

var authConnectionsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a managed auth by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthConnectionsGet,
}

var authConnectionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed auths",
	Args:  cobra.NoArgs,
	RunE:  runAuthConnectionsList,
}

var authConnectionsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a managed auth",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthConnectionsDelete,
}

var authConnectionsLoginCmd = &cobra.Command{
	Use:   "login <id>",
	Short: "Start a login flow",
	Long:  "Start a login flow for the managed auth, returns a hosted URL for authentication",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthConnectionsLogin,
}

var authConnectionsSubmitCmd = &cobra.Command{
	Use:   "submit <id>",
	Short: "Submit field values to a login flow",
	Long: `Submit field values for the login form. Poll the managed auth to track progress.

Examples:
  # Submit field values
  kernel auth connections submit <id> --field username=myuser --field password=mypass

  # Select an MFA option
  kernel auth connections submit <id> --mfa-option-id <id>

  # Click an SSO button
  kernel auth connections submit <id> --sso-button-selector "//button[@id='google-sso']"`,
	Args: cobra.ExactArgs(1),
	RunE: runAuthConnectionsSubmit,
}

var authConnectionsFollowCmd = &cobra.Command{
	Use:   "follow <id>",
	Short: "Follow login flow events",
	Long:  "Establish an SSE stream to receive real-time login flow state updates",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthConnectionsFollow,
}

func init() {
	// Create flags
	authConnectionsCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	authConnectionsCreateCmd.Flags().String("domain", "", "Target domain for authentication (required)")
	authConnectionsCreateCmd.Flags().String("profile-name", "", "Name of the profile to manage (required)")
	authConnectionsCreateCmd.Flags().String("login-url", "", "Optional login page URL to skip discovery")
	authConnectionsCreateCmd.Flags().StringSlice("allowed-domain", []string{}, "Additional allowed domains (repeatable)")
	authConnectionsCreateCmd.Flags().String("credential-name", "", "Kernel credential name to use")
	authConnectionsCreateCmd.Flags().String("credential-provider", "", "External credential provider name")
	authConnectionsCreateCmd.Flags().String("credential-path", "", "Provider-specific path (e.g., VaultName/ItemName)")
	authConnectionsCreateCmd.Flags().Bool("credential-auto", false, "Lookup by domain from the specified provider")
	authConnectionsCreateCmd.Flags().String("proxy-id", "", "Proxy ID to use")
	authConnectionsCreateCmd.Flags().String("proxy-name", "", "Proxy name to use")
	authConnectionsCreateCmd.Flags().Bool("no-save-credentials", false, "Disable saving credentials after successful login")
	authConnectionsCreateCmd.Flags().Int("health-check-interval", 0, "Interval in seconds between health checks (300-86400)")
	_ = authConnectionsCreateCmd.MarkFlagRequired("domain")
	_ = authConnectionsCreateCmd.MarkFlagRequired("profile-name")

	// Get flags
	authConnectionsGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// List flags
	authConnectionsListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	authConnectionsListCmd.Flags().String("domain", "", "Filter by domain")
	authConnectionsListCmd.Flags().String("profile-name", "", "Filter by profile name")
	authConnectionsListCmd.Flags().Int("limit", 0, "Maximum number of results to return")
	authConnectionsListCmd.Flags().Int("offset", 0, "Number of results to skip")

	// Delete flags
	authConnectionsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Login flags
	authConnectionsLoginCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	authConnectionsLoginCmd.Flags().String("proxy-id", "", "Proxy ID to use for this login")
	authConnectionsLoginCmd.Flags().String("proxy-name", "", "Proxy name to use for this login")

	// Submit flags
	authConnectionsSubmitCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	authConnectionsSubmitCmd.Flags().StringArray("field", []string{}, "Field name=value pair (repeatable)")
	authConnectionsSubmitCmd.Flags().String("mfa-option-id", "", "MFA option ID if user selected an MFA method")
	authConnectionsSubmitCmd.Flags().String("sso-button-selector", "", "XPath selector if user chose an SSO button")

	// Follow flags
	authConnectionsFollowCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// Wire up commands
	authConnectionsCmd.AddCommand(authConnectionsCreateCmd)
	authConnectionsCmd.AddCommand(authConnectionsGetCmd)
	authConnectionsCmd.AddCommand(authConnectionsListCmd)
	authConnectionsCmd.AddCommand(authConnectionsDeleteCmd)
	authConnectionsCmd.AddCommand(authConnectionsLoginCmd)
	authConnectionsCmd.AddCommand(authConnectionsSubmitCmd)
	authConnectionsCmd.AddCommand(authConnectionsFollowCmd)

	authCmd.AddCommand(authConnectionsCmd)
}

func runAuthConnectionsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile-name")
	loginURL, _ := cmd.Flags().GetString("login-url")
	allowedDomains, _ := cmd.Flags().GetStringSlice("allowed-domain")
	credentialName, _ := cmd.Flags().GetString("credential-name")
	credentialProvider, _ := cmd.Flags().GetString("credential-provider")
	credentialPath, _ := cmd.Flags().GetString("credential-path")
	credentialAuto, _ := cmd.Flags().GetBool("credential-auto")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	proxyName, _ := cmd.Flags().GetString("proxy-name")
	noSaveCredentials, _ := cmd.Flags().GetBool("no-save-credentials")
	healthCheckInterval, _ := cmd.Flags().GetInt("health-check-interval")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Create(cmd.Context(), AuthConnectionCreateInput{
		Domain:              domain,
		ProfileName:         profileName,
		LoginURL:            loginURL,
		AllowedDomains:      allowedDomains,
		CredentialName:      credentialName,
		CredentialProvider:  credentialProvider,
		CredentialPath:      credentialPath,
		CredentialAuto:      credentialAuto,
		ProxyID:             proxyID,
		ProxyName:           proxyName,
		NoSaveCredentials:   noSaveCredentials,
		HealthCheckInterval: healthCheckInterval,
		Output:              output,
	})
}

func runAuthConnectionsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Get(cmd.Context(), AuthConnectionGetInput{
		ID:     args[0],
		Output: output,
	})
}

func runAuthConnectionsList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile-name")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.List(cmd.Context(), AuthConnectionListInput{
		Domain:      domain,
		ProfileName: profileName,
		Limit:       limit,
		Offset:      offset,
		Output:      output,
	})
}

func runAuthConnectionsDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skip, _ := cmd.Flags().GetBool("yes")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Delete(cmd.Context(), AuthConnectionDeleteInput{
		ID:          args[0],
		SkipConfirm: skip,
	})
}

func runAuthConnectionsLogin(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	proxyName, _ := cmd.Flags().GetString("proxy-name")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Login(cmd.Context(), AuthConnectionLoginInput{
		ID:        args[0],
		ProxyID:   proxyID,
		ProxyName: proxyName,
		Output:    output,
	})
}

func runAuthConnectionsSubmit(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	fieldPairs, _ := cmd.Flags().GetStringArray("field")
	mfaOptionID, _ := cmd.Flags().GetString("mfa-option-id")
	ssoButtonSelector, _ := cmd.Flags().GetString("sso-button-selector")

	// Parse field pairs into map
	fieldValues := make(map[string]string)
	for _, pair := range fieldPairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid field format: %s (expected key=value)", pair)
		}
		fieldValues[parts[0]] = parts[1]
	}

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Submit(cmd.Context(), AuthConnectionSubmitInput{
		ID:                args[0],
		FieldValues:       fieldValues,
		MfaOptionID:       mfaOptionID,
		SSOButtonSelector: ssoButtonSelector,
		Output:            output,
	})
}

func runAuthConnectionsFollow(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Follow(cmd.Context(), AuthConnectionFollowInput{
		ID:     args[0],
		Output: output,
	})
}
