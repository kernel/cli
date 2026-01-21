package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pkg/browser"
	"github.com/pquerna/otp/totp"
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

// AgentAuthRunInput contains all parameters for the automated auth run flow.
type AgentAuthRunInput struct {
	Domain           string
	ProfileName      string
	Values           map[string]string
	CredentialName   string
	SaveCredentialAs string
	TotpSecret       string
	ProxyID          string
	LoginURL         string
	AllowedDomains   []string
	Timeout          time.Duration
	OpenLiveView     bool
	Output           string
}

// AgentAuthRunResult is the result of a successful auth run.
type AgentAuthRunResult struct {
	ProfileName string `json:"profile_name"`
	ProfileID   string `json:"profile_id"`
	Domain      string `json:"domain"`
	AuthAgentID string `json:"auth_agent_id"`
}

// AgentAuthRunEvent represents a status update during the auth run (for JSON output).
type AgentAuthRunEvent struct {
	Type        string `json:"type"` // status, error, success, waiting
	Step        string `json:"step,omitempty"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
	LiveViewURL string `json:"live_view_url,omitempty"`
}

// AgentAuthRunCmd handles the automated auth run flow.
type AgentAuthRunCmd struct {
	auth        AgentAuthService
	invocations AgentAuthInvocationsService
	profiles    ProfilesService
	credentials CredentialsService
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

const (
	totpPeriod            = 30 // TOTP codes are valid for 30-second windows
	minSecondsRemaining   = 5  // Minimum seconds remaining before we wait for next window
)

// generateTOTPCode generates a TOTP code from a base32 secret.
// Waits for a fresh window if needed to ensure enough time to submit the code.
// If quiet is true, suppresses human-readable console output (for JSON mode).
func generateTOTPCode(secret string, quiet bool) (string, error) {
	// Check if we have enough time in the current window
	now := time.Now().Unix()
	secondsIntoWindow := now % totpPeriod
	remaining := totpPeriod - secondsIntoWindow

	if remaining < minSecondsRemaining {
		waitTime := remaining + 1 // Wait until just after the new window starts
		if !quiet {
			pterm.Info.Printf("TOTP window has only %ds remaining, waiting %ds for fresh window...\n", remaining, waitTime)
		}
		time.Sleep(time.Duration(waitTime) * time.Second)
	}

	// Clean the secret (remove spaces that may be added for readability)
	cleanSecret := strings.ReplaceAll(strings.ToUpper(secret), " ", "")

	code, err := totp.GenerateCode(cleanSecret, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP code: %w", err)
	}
	return code, nil
}

// Run executes the full automated auth flow: create profile, credential, auth agent, and run invocation to completion.
func (c AgentAuthRunCmd) Run(ctx context.Context, in AgentAuthRunInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Domain == "" {
		return fmt.Errorf("--domain is required")
	}
	if in.ProfileName == "" {
		return fmt.Errorf("--profile is required")
	}

	// Validate that we have credentials to work with
	if in.CredentialName == "" && len(in.Values) == 0 {
		return fmt.Errorf("must provide either --credential or --value flags with credentials")
	}

	jsonOutput := in.Output == "json"
	emitEvent := func(event AgentAuthRunEvent) {
		if jsonOutput {
			data, _ := json.Marshal(event)
			fmt.Println(string(data))
		}
	}

	// Step 1: Find or create the profile
	if !jsonOutput {
		pterm.Info.Printf("Looking for profile '%s'...\n", in.ProfileName)
	}
	emitEvent(AgentAuthRunEvent{Type: "status", Message: "Looking for profile"})

	var profileID string
	profile, err := c.profiles.Get(ctx, in.ProfileName)
	if err != nil {
		if !util.IsNotFound(err) {
			return util.CleanedUpSdkError{Err: err}
		}
		// Profile not found, create it
		if !jsonOutput {
			pterm.Info.Printf("Creating profile '%s'...\n", in.ProfileName)
		}
		emitEvent(AgentAuthRunEvent{Type: "status", Message: "Creating profile"})

		newProfile, err := c.profiles.New(ctx, kernel.ProfileNewParams{
			Name: kernel.Opt(in.ProfileName),
		})
		if err != nil {
			return util.CleanedUpSdkError{Err: err}
		}
		profileID = newProfile.ID
		if !jsonOutput {
			pterm.Success.Printf("Created profile: %s\n", newProfile.ID)
		}
	} else {
		profileID = profile.ID
		if !jsonOutput {
			pterm.Success.Printf("Found existing profile: %s\n", profile.ID)
		}
	}

	// Step 2: Handle credentials
	var credentialName string
	if in.CredentialName != "" {
		// Using existing credential
		credentialName = in.CredentialName
		if !jsonOutput {
			pterm.Info.Printf("Using existing credential '%s'\n", credentialName)
		}
		emitEvent(AgentAuthRunEvent{Type: "status", Message: "Using existing credential"})
	} else if in.SaveCredentialAs != "" {
		// Create new credential with provided values
		credentialName = in.SaveCredentialAs
		if !jsonOutput {
			pterm.Info.Printf("Creating credential '%s'...\n", credentialName)
		}
		emitEvent(AgentAuthRunEvent{Type: "status", Message: "Creating credential"})

		params := kernel.CredentialNewParams{
			CreateCredentialRequest: kernel.CreateCredentialRequestParam{
				Name:   credentialName,
				Domain: in.Domain,
				Values: in.Values,
			},
		}
		if in.TotpSecret != "" {
			params.CreateCredentialRequest.TotpSecret = kernel.Opt(in.TotpSecret)
		}

		_, err := c.credentials.New(ctx, params)
		if err != nil {
			return util.CleanedUpSdkError{Err: err}
		}
		if !jsonOutput {
			pterm.Success.Printf("Created credential: %s\n", credentialName)
		}
	}

	// Step 3: Create auth agent
	if !jsonOutput {
		pterm.Info.Printf("Creating auth agent for %s...\n", in.Domain)
	}
	emitEvent(AgentAuthRunEvent{Type: "status", Message: "Creating auth agent"})

	agentParams := kernel.AgentAuthNewParams{
		AuthAgentCreateRequest: kernel.AuthAgentCreateRequestParam{
			Domain:      in.Domain,
			ProfileName: in.ProfileName,
		},
	}
	if credentialName != "" {
		agentParams.AuthAgentCreateRequest.CredentialName = kernel.Opt(credentialName)
	}
	if in.LoginURL != "" {
		agentParams.AuthAgentCreateRequest.LoginURL = kernel.Opt(in.LoginURL)
	}
	if len(in.AllowedDomains) > 0 {
		agentParams.AuthAgentCreateRequest.AllowedDomains = in.AllowedDomains
	}
	if in.ProxyID != "" {
		agentParams.AuthAgentCreateRequest.Proxy = kernel.AuthAgentCreateRequestProxyParam{
			ProxyID: kernel.Opt(in.ProxyID),
		}
	}

	agent, err := c.auth.New(ctx, agentParams)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if !jsonOutput {
		pterm.Success.Printf("Created auth agent: %s\n", agent.ID)
	}

	// Step 4: Create invocation
	if !jsonOutput {
		pterm.Info.Println("Starting authentication flow...")
	}
	emitEvent(AgentAuthRunEvent{Type: "status", Message: "Starting authentication"})

	invocationParams := kernel.AgentAuthInvocationNewParams{
		AuthAgentInvocationCreateRequest: kernel.AuthAgentInvocationCreateRequestParam{
			AuthAgentID: agent.ID,
		},
	}
	if in.SaveCredentialAs != "" && credentialName == "" {
		// Save credential during invocation if we have values but didn't create upfront
		invocationParams.AuthAgentInvocationCreateRequest.SaveCredentialAs = kernel.Opt(in.SaveCredentialAs)
	}

	invocation, err := c.invocations.New(ctx, invocationParams)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	// Step 5: Polling loop
	deadline := time.Now().Add(in.Timeout)
	pollInterval := 2 * time.Second
	var lastStep string
	liveViewShown := false
	fieldsSubmitted := make(map[string]bool)

	if !jsonOutput {
		pterm.Info.Println("Waiting for authentication to complete...")
	}

	for {
		if time.Now().After(deadline) {
			emitEvent(AgentAuthRunEvent{Type: "error", Message: "Timeout waiting for authentication"})
			return fmt.Errorf("timeout waiting for authentication to complete")
		}

		resp, err := c.invocations.Get(ctx, invocation.InvocationID)
		if err != nil {
			return util.CleanedUpSdkError{Err: err}
		}

		// Emit status update if step changed
		if string(resp.Step) != lastStep {
			lastStep = string(resp.Step)
			emitEvent(AgentAuthRunEvent{
				Type:        "status",
				Step:        lastStep,
				Status:      string(resp.Status),
				LiveViewURL: resp.LiveViewURL,
			})
			if !jsonOutput {
				pterm.Info.Printf("Step: %s (Status: %s)\n", resp.Step, resp.Status)
			}
		}

		// Check terminal states
		switch resp.Status {
		case kernel.AgentAuthInvocationResponseStatusSuccess:
			if !jsonOutput {
				pterm.Success.Println("Authentication successful!")
				pterm.Success.Printf("Profile '%s' is now authenticated for %s\n", in.ProfileName, in.Domain)
			}
			result := AgentAuthRunResult{
				ProfileName: in.ProfileName,
				ProfileID:   profileID,
				Domain:      in.Domain,
				AuthAgentID: agent.ID,
			}
			if jsonOutput {
				emitEvent(AgentAuthRunEvent{Type: "success", Message: "Authentication successful"})
				data, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(data))
				return nil
			}
			return nil

		case kernel.AgentAuthInvocationResponseStatusFailed:
			errMsg := "Authentication failed"
			if resp.ErrorMessage != "" {
				errMsg = resp.ErrorMessage
			}
			emitEvent(AgentAuthRunEvent{Type: "error", Message: errMsg})
			return fmt.Errorf("authentication failed: %s", errMsg)

		case kernel.AgentAuthInvocationResponseStatusExpired:
			emitEvent(AgentAuthRunEvent{Type: "error", Message: "Authentication session expired"})
			return fmt.Errorf("authentication session expired")

		case kernel.AgentAuthInvocationResponseStatusCanceled:
			emitEvent(AgentAuthRunEvent{Type: "error", Message: "Authentication was canceled"})
			return fmt.Errorf("authentication was canceled")
		}

		// Handle awaiting_input step
		if resp.Step == kernel.AgentAuthInvocationResponseStepAwaitingInput {
			// Check for pending fields
			if len(resp.PendingFields) > 0 {
				// Build field values to submit
				submitValues := make(map[string]string)
				missingFields := []string{}

				for _, field := range resp.PendingFields {
					fieldName := field.Name
					// Check if we already submitted this field
					if fieldsSubmitted[fieldName] {
						continue
					}

					// Try to find a matching value
					if val, ok := in.Values[fieldName]; ok {
						submitValues[fieldName] = val
					} else {
						// Check common field name aliases
						matched := false
						aliases := map[string][]string{
							"identifier": {"username", "email", "login"},
							"username":   {"identifier", "email", "login"},
							"email":      {"identifier", "username", "login"},
							"password":   {"pass", "passwd"},
						}
						if alts, ok := aliases[fieldName]; ok {
							for _, alt := range alts {
								if val, ok := in.Values[alt]; ok {
									submitValues[fieldName] = val
									matched = true
									break
								}
							}
						}

						// Check if this looks like a TOTP/verification code field
						if !matched && in.TotpSecret != "" {
							fieldLower := strings.ToLower(fieldName)
							totpPatterns := []string{"totp", "code", "verification", "otp", "2fa", "mfa", "authenticator", "token"}
							for _, pattern := range totpPatterns {
								if strings.Contains(fieldLower, pattern) {
									code, err := generateTOTPCode(in.TotpSecret, jsonOutput)
									if err == nil {
										submitValues[fieldName] = code
										matched = true
										if !jsonOutput {
											pterm.Info.Printf("Generated TOTP code for field: %s\n", fieldName)
										}
									}
									break
								}
							}
						}

						if !matched {
							missingFields = append(missingFields, fieldName)
						}
					}
				}

				// Submit if we have values
				if len(submitValues) > 0 {
					if !jsonOutput {
						var fieldNames []string
						for k := range submitValues {
							fieldNames = append(fieldNames, k)
						}
						pterm.Info.Printf("Submitting fields: %s\n", strings.Join(fieldNames, ", "))
					}

					submitParams := kernel.AgentAuthInvocationSubmitParams{
						OfFieldValues: &kernel.AgentAuthInvocationSubmitParamsBodyFieldValues{
							FieldValues: submitValues,
						},
					}
					_, err := c.invocations.Submit(ctx, invocation.InvocationID, submitParams)
					if err != nil {
						return util.CleanedUpSdkError{Err: err}
					}

					// Mark fields as submitted
					for k := range submitValues {
						fieldsSubmitted[k] = true
					}
				}

				// Show live view if we have missing fields
				if len(missingFields) > 0 && !liveViewShown && resp.LiveViewURL != "" {
					liveViewShown = true
					emitEvent(AgentAuthRunEvent{
						Type:        "waiting",
						Message:     fmt.Sprintf("Need human input for: %s", strings.Join(missingFields, ", ")),
						LiveViewURL: resp.LiveViewURL,
					})
					if !jsonOutput {
						pterm.Warning.Printf("Missing values for fields: %s\n", strings.Join(missingFields, ", "))
						pterm.Info.Printf("Live view: %s\n", resp.LiveViewURL)
					}
					if in.OpenLiveView {
						_ = browser.OpenURL(resp.LiveViewURL)
					}
				}
			}

			// Check for MFA options
			if len(resp.MfaOptions) > 0 {
				// Check if TOTP is available and we have a secret
				hasTOTP := false
				for _, opt := range resp.MfaOptions {
					if opt.Type == "totp" {
						hasTOTP = true
						break
					}
				}

				if hasTOTP && in.TotpSecret != "" {
					// Generate and submit TOTP code
					code, err := generateTOTPCode(in.TotpSecret, jsonOutput)
					if err != nil {
						return err
					}

					if !jsonOutput {
						pterm.Info.Println("Submitting TOTP code...")
					}

					submitParams := kernel.AgentAuthInvocationSubmitParams{
						OfFieldValues: &kernel.AgentAuthInvocationSubmitParamsBodyFieldValues{
							FieldValues: map[string]string{"totp": code},
						},
					}
					_, err = c.invocations.Submit(ctx, invocation.InvocationID, submitParams)
					if err != nil {
						return util.CleanedUpSdkError{Err: err}
					}
				} else if !liveViewShown && resp.LiveViewURL != "" {
					// Need human for MFA
					liveViewShown = true
					var optTypes []string
					for _, opt := range resp.MfaOptions {
						optTypes = append(optTypes, opt.Type)
					}
					emitEvent(AgentAuthRunEvent{
						Type:        "waiting",
						Message:     fmt.Sprintf("MFA required: %s", strings.Join(optTypes, ", ")),
						LiveViewURL: resp.LiveViewURL,
					})
					if !jsonOutput {
						pterm.Warning.Printf("MFA required. Options: %s\n", strings.Join(optTypes, ", "))
						pterm.Info.Printf("Complete MFA at: %s\n", resp.LiveViewURL)
					}
					if in.OpenLiveView {
						_ = browser.OpenURL(resp.LiveViewURL)
					}
				}
			}
		}

		// Handle awaiting_external_action step
		if resp.Step == kernel.AgentAuthInvocationResponseStepAwaitingExternalAction && !liveViewShown {
			liveViewShown = true
			msg := "External action required"
			if resp.ExternalActionMessage != "" {
				msg = resp.ExternalActionMessage
			}
			emitEvent(AgentAuthRunEvent{
				Type:        "waiting",
				Message:     msg,
				LiveViewURL: resp.LiveViewURL,
			})
			if !jsonOutput {
				pterm.Warning.Printf("%s\n", msg)
				if resp.LiveViewURL != "" {
					pterm.Info.Printf("Live view: %s\n", resp.LiveViewURL)
				}
			}
			if in.OpenLiveView && resp.LiveViewURL != "" {
				_ = browser.OpenURL(resp.LiveViewURL)
			}
		}

		// Wait before next poll
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
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

var agentsAuthRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a complete auth flow",
	Long: `Run a complete authentication flow for a domain, automatically handling credential submission and polling.

This command orchestrates the entire agent auth process:
1. Creates or finds a profile with the given name
2. Creates a credential if --save-credential-as is specified
3. Creates an auth agent linking domain, profile, and credential
4. Starts an invocation and polls until completion
5. Auto-submits credentials when prompted
6. Auto-submits TOTP codes if --totp-secret is provided
7. Shows live view URL when human intervention is needed

Examples:
  # Basic auth with inline credentials
  kernel agents auth run --domain github.com --profile my-github \
    --value username=myuser --value password=mypass

  # With TOTP for automatic 2FA
  kernel agents auth run --domain github.com --profile my-github \
    --value username=myuser --value password=mypass \
    --totp-secret JBSWY3DPEHPK3PXP

  # Save credentials for future re-auth
  kernel agents auth run --domain github.com --profile my-github \
    --value username=myuser --value password=mypass \
    --save-credential-as github-creds

  # Re-use existing saved credential
  kernel agents auth run --domain github.com --profile my-github \
    --credential github-creds

  # Auto-open browser for human intervention
  kernel agents auth run --domain github.com --profile my-github \
    --credential github-creds --open`,
	Args: cobra.NoArgs,
	RunE: runAgentsAuthRun,
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

	// Auth run flags
	agentsAuthRunCmd.Flags().StringP("output", "o", "", "Output format: json for JSONL events")
	agentsAuthRunCmd.Flags().String("domain", "", "Target domain for authentication (required)")
	agentsAuthRunCmd.Flags().String("profile", "", "Profile name to use/create (required)")
	agentsAuthRunCmd.Flags().StringArray("value", []string{}, "Field name=value pair (e.g., --value username=foo --value password=bar)")
	agentsAuthRunCmd.Flags().String("credential", "", "Existing credential name to use")
	agentsAuthRunCmd.Flags().String("save-credential-as", "", "Save provided credentials under this name")
	agentsAuthRunCmd.Flags().String("totp-secret", "", "Base32 TOTP secret for automatic 2FA")
	agentsAuthRunCmd.Flags().String("proxy-id", "", "Proxy ID to use")
	agentsAuthRunCmd.Flags().String("login-url", "", "Custom login page URL")
	agentsAuthRunCmd.Flags().StringSlice("allowed-domain", []string{}, "Additional allowed domains")
	agentsAuthRunCmd.Flags().Duration("timeout", 5*time.Minute, "Maximum time to wait for auth completion")
	agentsAuthRunCmd.Flags().Bool("open", false, "Open live view URL in browser when human intervention needed")
	_ = agentsAuthRunCmd.MarkFlagRequired("domain")
	_ = agentsAuthRunCmd.MarkFlagRequired("profile")

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
	agentsAuthCmd.AddCommand(agentsAuthRunCmd)

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

func runAgentsAuthRun(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile")
	valuePairs, _ := cmd.Flags().GetStringArray("value")
	credentialName, _ := cmd.Flags().GetString("credential")
	saveCredentialAs, _ := cmd.Flags().GetString("save-credential-as")
	totpSecret, _ := cmd.Flags().GetString("totp-secret")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	loginURL, _ := cmd.Flags().GetString("login-url")
	allowedDomains, _ := cmd.Flags().GetStringSlice("allowed-domain")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	openLiveView, _ := cmd.Flags().GetBool("open")

	// Parse value pairs into map
	values := make(map[string]string)
	for _, pair := range valuePairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid value format: %s (expected key=value)", pair)
		}
		values[parts[0]] = parts[1]
	}

	authSvc := client.Agents.Auth
	profilesSvc := client.Profiles
	credentialsSvc := client.Credentials

	c := AgentAuthRunCmd{
		auth:        &authSvc,
		invocations: &authSvc.Invocations,
		profiles:    &profilesSvc,
		credentials: &credentialsSvc,
	}

	return c.Run(cmd.Context(), AgentAuthRunInput{
		Domain:           domain,
		ProfileName:      profileName,
		Values:           values,
		CredentialName:   credentialName,
		SaveCredentialAs: saveCredentialAs,
		TotpSecret:       totpSecret,
		ProxyID:          proxyID,
		LoginURL:         loginURL,
		AllowedDomains:   allowedDomains,
		Timeout:          timeout,
		OpenLiveView:     openLiveView,
		Output:           output,
	})
}
