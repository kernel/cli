package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

// AuthConnectionService defines the subset of the Kernel SDK auth connection client that we use.
type AuthConnectionService interface {
	New(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (res *kernel.ManagedAuth, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.ManagedAuth, err error)
	Update(ctx context.Context, id string, body kernel.AuthConnectionUpdateParams, opts ...option.RequestOption) (res *kernel.ManagedAuth, err error)
	List(ctx context.Context, query kernel.AuthConnectionListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.ManagedAuth], err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
	Login(ctx context.Context, id string, body kernel.AuthConnectionLoginParams, opts ...option.RequestOption) (res *kernel.LoginResponse, err error)
	Submit(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (res *kernel.SubmitFieldsResponse, err error)
	Timeline(ctx context.Context, id string, query kernel.AuthConnectionTimelineParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent], err error)
	FollowStreaming(ctx context.Context, id string, opts ...option.RequestOption) (stream *ssestream.Stream[kernel.AuthConnectionFollowResponseUnion])
}

// AuthConnectionCmd handles auth connection operations independent of cobra.
type AuthConnectionCmd struct {
	svc      AuthConnectionService
	prompter interactive.Prompter
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
	ProxyMode           string
	Stealth             BoolFlag
	SaveCredentials     bool
	NoSaveCredentials   bool
	HealthCheckInterval int
	NoHealthChecks      bool
	NoAutoReauth        bool
	RecordSession       BoolFlag
	Telemetry           string
	TelemetryExport     string
	Output              string
}

type AuthConnectionGetInput struct {
	ID     string
	Output string
}

type AuthConnectionUpdateInput struct {
	ID                     string
	LoginURL               string
	LoginURLSet            bool
	AllowedDomains         []string
	AllowedDomainsSet      bool
	CredentialName         string
	CredentialNameSet      bool
	CredentialProvider     string
	CredentialProviderSet  bool
	CredentialPath         string
	CredentialPathSet      bool
	CredentialAuto         BoolFlag
	ProxyID                string
	ProxyIDSet             bool
	ProxyName              string
	ProxyNameSet           bool
	ProxyMode              string
	Stealth                BoolFlag
	SaveCredentials        BoolFlag
	HealthCheckInterval    int
	HealthCheckIntervalSet bool
	HealthChecks           BoolFlag
	AutoReauth             BoolFlag
	RecordSession          BoolFlag
	Telemetry              string
	TelemetryExport        string
	Output                 string
}

type AuthConnectionListInput struct {
	Domain      string
	ProfileName string
	Query       string
	Limit       int
	Offset      int
	Output      string
}

type AuthConnectionDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type AuthConnectionLoginInput struct {
	ID              string
	ProxyID         string
	ProxyName       string
	ProxyMode       string
	Stealth         BoolFlag
	RecordSession   BoolFlag
	Telemetry       string
	TelemetryExport string
	Output          string
}

type AuthConnectionSubmitInput struct {
	ID string
	// FieldValues holds legacy --field name=value pairs, submitted as `fields`.
	FieldValues map[string]string
	// CanonicalFieldValues holds --field-value id=value pairs, submitted as the
	// canonical `field_values` keyed by the field IDs the API returned.
	CanonicalFieldValues map[string]string
	// SelectedChoiceID is the canonical choice ID from the API's `choices` list.
	SelectedChoiceID  string
	MfaOptionID       string
	SignInOptionID    string
	SSOButtonSelector string
	SSOProvider       string
	Output            string
}

type AuthConnectionTimelineInput struct {
	ID      string
	Type    string
	Page    int
	PerPage int
	Output  string
}

type AuthConnectionFollowInput struct {
	ID     string
	Output string
}

func (c AuthConnectionCmd) Create(ctx context.Context, in AuthConnectionCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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
		} else {
			// Default to domain auto-lookup when no explicit --credential-path is
			// given. This matches the dashboard's UX, where picking a provider
			// without a specific item always means "look up by domain". Without
			// this default, the server receives { provider } with no path or
			// auto flag, which is a valid-but-inert credential reference that
			// causes the managed auth session to never fetch credentials.
			params.ManagedAuthCreateRequest.Credential.Auto = kernel.Opt(true)
		}
		if in.CredentialAuto {
			params.ManagedAuthCreateRequest.Credential.Auto = kernel.Opt(true)
		}
	}

	sel := proxySelection{ID: in.ProxyID, Name: in.ProxyName, Mode: in.ProxyMode}
	if sel.set() {
		proxy, err := buildProxyConfigParam(sel)
		if err != nil {
			return err
		}
		params.ManagedAuthCreateRequest.Browser.Proxy = proxy
	}

	if in.Stealth.Set {
		params.ManagedAuthCreateRequest.Browser.Stealth = kernel.Opt(in.Stealth.Value)
	}

	if in.NoSaveCredentials {
		params.ManagedAuthCreateRequest.SaveCredentials = kernel.Opt(false)
	}

	if in.NoHealthChecks {
		params.ManagedAuthCreateRequest.HealthChecks = kernel.Opt(false)
	}

	if in.NoAutoReauth {
		params.ManagedAuthCreateRequest.AutoReauth = kernel.Opt(false)
	}

	if in.RecordSession.Set {
		params.ManagedAuthCreateRequest.RecordSession = kernel.Opt(in.RecordSession.Value)
	}

	if in.Telemetry != "" || in.TelemetryExport != "" {
		t, err := buildManagedAuthTelemetryParam(in.Telemetry, in.TelemetryExport, true)
		if err != nil {
			return err
		}
		params.ManagedAuthCreateRequest.Browser.Telemetry = t
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
	printManagedAuthSummary(auth)
	return nil
}

func printManagedAuthSummary(auth *kernel.ManagedAuth) {
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"ID", auth.ID},
		{"Domain", auth.Domain},
		{"Profile Name", auth.ProfileName},
		{"Status", string(auth.Status)},
		{"Can Reauth", fmt.Sprintf("%t", auth.CanReauth)},
	}
	if auth.CanReauthReason != "" {
		tableData = append(tableData, []string{"Can Reauth Reason", string(auth.CanReauthReason)})
	}
	if auth.Credential.Name != "" {
		tableData = append(tableData, []string{"Credential Name", auth.Credential.Name})
	}
	if auth.Credential.Provider != "" {
		tableData = append(tableData, []string{"Credential Provider", auth.Credential.Provider})
	}
	tableData = append(tableData, managedAuthBrowserRows(auth.Browser)...)
	PrintTableNoPad(tableData, true)
}

// managedAuthBrowserRows renders the browser configuration a connection applies to
// its login, reauthentication, and health-check sessions.
func managedAuthBrowserRows(cfg kernel.ManagedAuthBrowserConfig) pterm.TableData {
	rows := pterm.TableData{}
	if proxy := formatBrowserProxyConfig(cfg.Proxy); proxy != "" {
		rows = append(rows, []string{"Browser Proxy", proxy})
	}
	// Stealth defaults to true when omitted, so only report what the API sent.
	if cfg.JSON.Stealth.Valid() {
		rows = append(rows, []string{"Browser Stealth", fmt.Sprintf("%t", cfg.Stealth)})
	}
	if cfg.Telemetry.Enabled || len(telemetryEnabledCategories(kernel.BrowserTelemetryConfig{Browser: cfg.Telemetry.Browser})) > 0 {
		rows = append(rows, []string{"Browser Telemetry", formatManagedAuthTelemetry(cfg.Telemetry)})
	}
	return rows
}

func (c AuthConnectionCmd) Update(ctx context.Context, in AuthConnectionUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.AuthConnectionUpdateParams{
		ManagedAuthUpdateRequest: kernel.ManagedAuthUpdateRequestParam{},
	}
	hasChanges := false

	if in.HealthCheckIntervalSet {
		params.ManagedAuthUpdateRequest.HealthCheckInterval = kernel.Opt(int64(in.HealthCheckInterval))
		hasChanges = true
	}
	if in.LoginURLSet {
		params.ManagedAuthUpdateRequest.LoginURL = kernel.Opt(in.LoginURL)
		hasChanges = true
	}
	if in.SaveCredentials.Set {
		params.ManagedAuthUpdateRequest.SaveCredentials = kernel.Opt(in.SaveCredentials.Value)
		hasChanges = true
	}
	if in.HealthChecks.Set {
		params.ManagedAuthUpdateRequest.HealthChecks = kernel.Opt(in.HealthChecks.Value)
		hasChanges = true
	}
	if in.AutoReauth.Set {
		params.ManagedAuthUpdateRequest.AutoReauth = kernel.Opt(in.AutoReauth.Value)
		hasChanges = true
	}
	if in.RecordSession.Set {
		params.ManagedAuthUpdateRequest.RecordSession = kernel.Opt(in.RecordSession.Value)
		hasChanges = true
	}
	if in.AllowedDomainsSet {
		params.ManagedAuthUpdateRequest.AllowedDomains = in.AllowedDomains
		hasChanges = true
	}

	credentialChanged := in.CredentialNameSet || in.CredentialProviderSet || in.CredentialPathSet || in.CredentialAuto.Set
	if credentialChanged {
		if strings.TrimSpace(in.CredentialName) != "" && strings.TrimSpace(in.CredentialProvider) != "" {
			return fmt.Errorf("credential reference must use either --credential-name or --credential-provider")
		}
		params.ManagedAuthUpdateRequest.Credential = kernel.ManagedAuthUpdateRequestCredentialParam{}
		if in.CredentialNameSet {
			params.ManagedAuthUpdateRequest.Credential.Name = kernel.Opt(in.CredentialName)
		}
		if in.CredentialProviderSet {
			params.ManagedAuthUpdateRequest.Credential.Provider = kernel.Opt(in.CredentialProvider)
		}
		if in.CredentialPathSet {
			params.ManagedAuthUpdateRequest.Credential.Path = kernel.Opt(in.CredentialPath)
		}
		if in.CredentialAuto.Set {
			params.ManagedAuthUpdateRequest.Credential.Auto = kernel.Opt(in.CredentialAuto.Value)
		}
		hasChanges = true
	}

	// A proxy is selected by ID or name, so an empty value is not a way to clear it:
	// dropping back to the connection's stealth-derived egress is a mode change.
	if (in.ProxyIDSet && in.ProxyID == "") || (in.ProxyNameSet && in.ProxyName == "") {
		return fmt.Errorf("proxy selection requires a non-empty value; use --proxy-mode=default to drop the selected proxy, or --proxy-mode=direct for direct egress")
	}

	sel := proxySelection{ID: in.ProxyID, Name: in.ProxyName, Mode: in.ProxyMode}
	if sel.set() {
		proxy, err := buildProxyConfigParam(sel)
		if err != nil {
			return err
		}
		params.ManagedAuthUpdateRequest.Browser.Proxy = proxy
		hasChanges = true
	}

	if in.Stealth.Set {
		params.ManagedAuthUpdateRequest.Browser.Stealth = kernel.Opt(in.Stealth.Value)
		hasChanges = true
	}

	if in.Telemetry != "" || in.TelemetryExport != "" {
		t, err := buildManagedAuthTelemetryParam(in.Telemetry, in.TelemetryExport, false)
		if err != nil {
			return err
		}
		params.ManagedAuthUpdateRequest.Browser.Telemetry = t
		hasChanges = true
	}

	if !hasChanges {
		return fmt.Errorf("must provide at least one field to update")
	}

	if in.Output != "json" {
		pterm.Info.Printf("Updating managed auth %s...\n", in.ID)
	}

	auth, err := c.svc.Update(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(auth)
	}

	pterm.Success.Printf("Updated managed auth: %s\n", auth.ID)
	printManagedAuthSummary(auth)
	return nil
}

// managedAuthInputField is the shared shape of a canonical input field. The SDK
// models the one on `get` and the one on the `follow` event stream as two
// identical but distinct types, so both are converted to this before rendering.
type managedAuthInputField struct {
	ID              string
	Label           string
	Type            string
	Ref             string
	Hint            string
	Required        bool
	ReplaceExisting bool
}

// managedAuthInputChoice is the choice counterpart of managedAuthInputField.
type managedAuthInputChoice struct {
	ID                string
	Label             string
	DisplayText       string
	Type              string
	MfaType           string
	MaskedDestination string
}

// formatManagedAuthField renders one canonical input field as
// `id (Label) [type, ref=…, required, hint="…"]`. The hint carries the API's
// context for the field, such as the masked destination a one-time code was
// sent to, so it is often what tells the user which value to supply.
func formatManagedAuthField(f managedAuthInputField) string {
	meta := make([]string, 0, 5)
	if f.Type != "" {
		meta = append(meta, f.Type)
	}
	if f.Ref != "" {
		meta = append(meta, "ref="+f.Ref)
	}
	if f.Required {
		meta = append(meta, "required")
	}
	if f.ReplaceExisting {
		meta = append(meta, "replace-existing")
	}
	if f.Hint != "" {
		meta = append(meta, fmt.Sprintf("hint=%q", f.Hint))
	}

	entry := f.ID
	if f.Label != "" {
		entry = fmt.Sprintf("%s (%s)", f.ID, f.Label)
	}
	if len(meta) > 0 {
		entry = fmt.Sprintf("%s [%s]", entry, strings.Join(meta, ", "))
	}
	return entry
}

// formatManagedAuthChoice renders one canonical choice as
// `id (Label) [type, sms, to=+1 ••• 1234]`. The MFA type and masked destination
// are what distinguish otherwise identical-looking options, so both are shown
// when the API captured them.
func formatManagedAuthChoice(c managedAuthInputChoice) string {
	meta := make([]string, 0, 3)
	if c.Type != "" {
		meta = append(meta, c.Type)
	}
	if c.MfaType != "" {
		meta = append(meta, c.MfaType)
	}
	if c.MaskedDestination != "" {
		meta = append(meta, "to="+c.MaskedDestination)
	}

	// display_text is the text as it appeared on the page; it stands in when the
	// API did not derive a separate label.
	label := c.Label
	if label == "" {
		label = c.DisplayText
	}

	entry := c.ID
	if label != "" {
		entry = fmt.Sprintf("%s (%s)", c.ID, label)
	}
	if len(meta) > 0 {
		entry = fmt.Sprintf("%s [%s]", entry, strings.Join(meta, ", "))
	}
	return entry
}

func (c AuthConnectionCmd) Get(ctx context.Context, in AuthConnectionGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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
		tableData = append(tableData, []string{"Can Reauth Reason", string(auth.CanReauthReason)})
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
	// Canonical fields/choices supersede discovered_fields, mfa_options and
	// pending_sso_buttons. Show them first so the IDs needed by `submit
	// --field-value` and `submit --choice-id` are the first thing visible.
	if len(auth.Fields) > 0 {
		fields := make([]string, 0, len(auth.Fields))
		for _, f := range auth.Fields {
			fields = append(fields, formatManagedAuthField(managedAuthInputField{
				ID:              f.ID,
				Label:           f.Label,
				Type:            f.Type,
				Ref:             f.Ref,
				Hint:            f.Hint,
				Required:        f.Required,
				ReplaceExisting: f.ReplaceExisting,
			}))
		}
		tableData = append(tableData, []string{"Fields", strings.Join(fields, "; ")})
	}
	if len(auth.Choices) > 0 {
		choices := make([]string, 0, len(auth.Choices))
		for _, ch := range auth.Choices {
			choices = append(choices, formatManagedAuthChoice(managedAuthInputChoice{
				ID:                ch.ID,
				Label:             ch.Label,
				DisplayText:       ch.DisplayText,
				Type:              ch.Type,
				MfaType:           ch.MfaType,
				MaskedDestination: ch.MaskedDestination,
			}))
		}
		tableData = append(tableData, []string{"Choices", strings.Join(choices, "; ")})
	}
	if len(auth.DiscoveredFields) > 0 {
		discoveredFields := make([]string, 0, len(auth.DiscoveredFields))
		for _, field := range auth.DiscoveredFields {
			fieldName := field.Name
			if fieldName == "" {
				fieldName = field.Label
			} else if field.Label != "" && field.Label != field.Name {
				fieldName = fmt.Sprintf("%s (%s)", field.Name, field.Label)
			}

			fieldMeta := make([]string, 0, 2)
			if field.Type != "" {
				fieldMeta = append(fieldMeta, field.Type)
			}
			if field.Required {
				fieldMeta = append(fieldMeta, "required")
			}
			if len(fieldMeta) > 0 {
				fieldName = fmt.Sprintf("%s [%s]", fieldName, strings.Join(fieldMeta, ", "))
			}
			discoveredFields = append(discoveredFields, fieldName)
		}
		tableData = append(tableData, []string{"Discovered Fields", strings.Join(discoveredFields, "; ")})
	}
	if len(auth.MfaOptions) > 0 {
		mfaOptions := make([]string, 0, len(auth.MfaOptions))
		for _, option := range auth.MfaOptions {
			optionName := option.Label
			if optionName == "" {
				optionName = option.Type
			} else if option.Type != "" {
				optionName = fmt.Sprintf("%s (%s)", option.Label, option.Type)
			}
			mfaOptions = append(mfaOptions, optionName)
		}
		tableData = append(tableData, []string{"MFA Options", strings.Join(mfaOptions, "; ")})
	}
	if len(auth.PendingSSOButtons) > 0 {
		pendingSSOButtons := make([]string, 0, len(auth.PendingSSOButtons))
		for _, button := range auth.PendingSSOButtons {
			buttonLabel := button.Label
			if buttonLabel == "" {
				buttonLabel = button.Provider
			} else if button.Provider != "" {
				buttonLabel = fmt.Sprintf("%s (%s)", button.Label, button.Provider)
			}
			pendingSSOButtons = append(pendingSSOButtons, buttonLabel)
		}
		tableData = append(tableData, []string{"Pending SSO Buttons", strings.Join(pendingSSOButtons, "; ")})
	}
	if auth.ExternalActionMessage != "" {
		tableData = append(tableData, []string{"External Action", auth.ExternalActionMessage})
	}
	if auth.HostedURL != "" {
		tableData = append(tableData, []string{"Hosted URL", auth.HostedURL})
	}
	if auth.LiveViewURL != "" {
		tableData = append(tableData, []string{"Live View URL", auth.LiveViewURL})
	}
	if auth.WebsiteError != "" {
		tableData = append(tableData, []string{"Website Error", auth.WebsiteError})
	}
	if !auth.FlowExpiresAt.IsZero() {
		tableData = append(tableData, []string{"Flow Expires At", util.FormatLocal(auth.FlowExpiresAt)})
	}
	if auth.ErrorCode != "" {
		tableData = append(tableData, []string{"Error Code", auth.ErrorCode})
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
	if auth.BrowserSessionID != "" {
		tableData = append(tableData, []string{"Browser Session ID", auth.BrowserSessionID})
	}
	tableData = append(tableData, managedAuthBrowserRows(auth.Browser)...)

	PrintTableNoPad(tableData, true)
	return nil
}

func (c AuthConnectionCmd) List(ctx context.Context, in AuthConnectionListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.AuthConnectionListParams{}
	if in.Domain != "" {
		params.Domain = kernel.Opt(in.Domain)
	}
	if in.ProfileName != "" {
		params.ProfileName = kernel.Opt(in.ProfileName)
	}
	if in.Query != "" {
		params.Query = kernel.Opt(in.Query)
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
		if page == nil {
			fmt.Println("[]")
			return nil
		}
		if page.RawJSON() != "" {
			return util.PrintPrettyJSON(page)
		}
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
		ok, err := c.prompter.Confirm(
			fmt.Sprintf("delete managed auth '%s'", in.ID),
			fmt.Sprintf("Are you sure you want to delete managed auth '%s'?", in.ID),
		)
		if err != nil {
			return err
		}
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.AuthConnectionLoginParams{}
	sel := proxySelection{ID: in.ProxyID, Name: in.ProxyName, Mode: in.ProxyMode}
	if sel.set() {
		proxy, err := buildProxyConfigParam(sel)
		if err != nil {
			return err
		}
		params.Browser.Proxy = proxy
	}

	if in.Stealth.Set {
		params.Browser.Stealth = kernel.Opt(in.Stealth.Value)
	}

	if in.RecordSession.Set {
		params.RecordSession = kernel.Opt(in.RecordSession.Value)
	}

	if in.Telemetry != "" || in.TelemetryExport != "" {
		t, err := buildManagedAuthTelemetryParam(in.Telemetry, in.TelemetryExport, false)
		if err != nil {
			return err
		}
		params.Browser.Telemetry = t
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
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	// Validate that we have some input to submit
	hasFields := len(in.FieldValues) > 0
	hasCanonicalFields := len(in.CanonicalFieldValues) > 0
	hasChoice := in.SelectedChoiceID != ""
	hasMfaOption := in.MfaOptionID != ""
	hasSignInOption := in.SignInOptionID != ""
	hasSSOButton := in.SSOButtonSelector != ""
	hasSSOProvider := in.SSOProvider != ""
	submitModes := 0
	for _, active := range []bool{hasFields, hasCanonicalFields, hasChoice, hasMfaOption, hasSignInOption, hasSSOButton, hasSSOProvider} {
		if active {
			submitModes++
		}
	}

	const submitModeFlags = "--field-value, --choice-id, --field, --mfa-option-id, --sign-in-option-id, --sso-button-selector, or --sso-provider"
	if submitModes == 0 {
		return fmt.Errorf("must provide exactly one of: %s", submitModeFlags)
	}
	if submitModes > 1 {
		return fmt.Errorf("provide exactly one of: %s", submitModeFlags)
	}

	// Resolve MFA option: the user may pass the label (e.g. "Get a text"), the
	// type (e.g. "sms"), or the display string ("Get a text (sms)"). The API
	// expects the type, so look up the connection's available options and map
	// whatever the user provided to the correct type value.
	if hasMfaOption {
		conn, err := c.svc.Get(ctx, in.ID)
		if err != nil {
			return util.CleanedUpSdkError{Err: fmt.Errorf("failed to fetch connection for MFA option resolution: %w", err)}
		}
		if len(conn.MfaOptions) > 0 {
			resolved := false
			for _, opt := range conn.MfaOptions {
				displayName := fmt.Sprintf("%s (%s)", opt.Label, opt.Type)
				if strings.EqualFold(in.MfaOptionID, opt.Type) ||
					strings.EqualFold(in.MfaOptionID, opt.Label) ||
					strings.EqualFold(in.MfaOptionID, displayName) {
					in.MfaOptionID = opt.Type
					resolved = true
					break
				}
			}
			if !resolved {
				available := make([]string, 0, len(conn.MfaOptions))
				for _, opt := range conn.MfaOptions {
					available = append(available, fmt.Sprintf("%s (%s)", opt.Label, opt.Type))
				}
				return fmt.Errorf("unknown MFA option %q; available: %s", in.MfaOptionID, strings.Join(available, ", "))
			}
		}
	}

	params := kernel.AuthConnectionSubmitParams{
		SubmitFieldsRequest: kernel.SubmitFieldsRequestParam{},
	}
	// Only attach legacy `fields` when it carries values. An empty-but-non-nil map
	// still marshals as `"fields": {}`, which the API reads as a second submit mode
	// alongside whichever canonical or legacy selector the user actually chose.
	if hasFields {
		params.SubmitFieldsRequest.Fields = in.FieldValues
	}
	if hasCanonicalFields {
		params.SubmitFieldsRequest.FieldValues = in.CanonicalFieldValues
	}
	if hasChoice {
		params.SubmitFieldsRequest.SelectedChoiceID = kernel.Opt(in.SelectedChoiceID)
	}
	if hasMfaOption {
		params.SubmitFieldsRequest.MfaOptionID = kernel.Opt(in.MfaOptionID)
	}
	if hasSignInOption {
		params.SubmitFieldsRequest.SignInOptionID = kernel.Opt(in.SignInOptionID)
	}
	if hasSSOButton {
		params.SubmitFieldsRequest.SSOButtonSelector = kernel.Opt(in.SSOButtonSelector)
	}
	if hasSSOProvider {
		params.SubmitFieldsRequest.SSOProvider = kernel.Opt(in.SSOProvider)
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

func (c AuthConnectionCmd) Timeline(ctx context.Context, in AuthConnectionTimelineInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	page := in.Page
	perPage := in.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}

	params := kernel.AuthConnectionTimelineParams{}
	if in.Type != "" {
		switch in.Type {
		case string(kernel.AuthConnectionTimelineParamsTypeLogin),
			string(kernel.AuthConnectionTimelineParamsTypeReauth),
			string(kernel.AuthConnectionTimelineParamsTypeHealthCheck):
			params.Type = kernel.AuthConnectionTimelineParamsType(in.Type)
		default:
			return fmt.Errorf("invalid --type %q: must be one of login, reauth, health_check", in.Type)
		}
	}
	// Request one extra event so we can report whether another page exists
	// without spending a second round trip on the pagination headers.
	params.Limit = kernel.Opt(int64(perPage + 1))
	params.Offset = kernel.Opt(int64((page - 1) * perPage))

	result, err := c.svc.Timeline(ctx, in.ID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var events []kernel.ManagedAuthTimelineEvent
	if result != nil {
		events = result.Items
	}

	hasMore := len(events) > perPage
	if hasMore {
		events = events[:perPage]
	}

	if in.Output == "json" {
		rawEvents := make([]json.RawMessage, 0, len(events))
		for _, e := range events {
			r := e.RawJSON()
			if r == "" {
				r = "{}"
			}
			rawEvents = append(rawEvents, json.RawMessage(r))
		}
		payload := struct {
			Events  []json.RawMessage `json:"events"`
			Page    int               `json:"page"`
			PerPage int               `json:"per_page"`
			HasMore bool              `json:"has_more"`
		}{Events: rawEvents, Page: page, PerPage: perPage, HasMore: hasMore}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(events) == 0 {
		pterm.Info.Println("No timeline events found")
		return nil
	}

	tableData := pterm.TableData{{"Timestamp", "Type", "Status", "Step", "Browser Session", "Telemetry", "Details"}}
	for _, e := range events {
		details := e.ErrorMessage
		if details == "" {
			details = e.WebsiteError
		}
		if details == "" && e.PreviousStatus != "" {
			details = fmt.Sprintf("%s -> %s", e.PreviousStatus, e.Status)
		}
		// Telemetry only means something when the event has a browser session
		// whose events `kernel browsers telemetry` could fetch, and when the API
		// actually reported the field.
		telemetry := "-"
		if e.BrowserSessionID != "" && e.JSON.TelemetryCaptured.Valid() {
			telemetry = lo.Ternary(e.TelemetryCaptured, "yes", "no")
		}
		tableData = append(tableData, []string{
			util.FormatLocal(e.Timestamp),
			string(e.Type),
			string(e.Status),
			string(e.Step),
			util.OrDash(e.BrowserSessionID),
			telemetry,
			util.OrDash(details),
		})
	}
	PrintTableNoPad(tableData, true)

	pterm.Printf("\nPage: %d  Per-page: %d  Items this page: %d  Has more: %s\n", page, perPage, len(events), lo.Ternary(hasMore, "yes", "no"))
	if hasMore {
		next := fmt.Sprintf("kernel auth connections timeline %s --page %d --per-page %d", in.ID, page+1, perPage)
		if in.Type != "" {
			next += fmt.Sprintf(" --type %q", in.Type)
		}
		pterm.Printf("Next: %s\n", next)
	}
	return nil
}

func (c AuthConnectionCmd) Follow(ctx context.Context, in AuthConnectionFollowInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
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
			if len(state.Fields) > 0 {
				fields := make([]string, 0, len(state.Fields))
				for _, f := range state.Fields {
					fields = append(fields, formatManagedAuthField(managedAuthInputField{
						ID:              f.ID,
						Label:           f.Label,
						Type:            f.Type,
						Ref:             f.Ref,
						Hint:            f.Hint,
						Required:        f.Required,
						ReplaceExisting: f.ReplaceExisting,
					}))
				}
				pterm.Info.Printf("  Fields: %s\n", strings.Join(fields, ", "))
			}
			if len(state.Choices) > 0 {
				choices := make([]string, 0, len(state.Choices))
				for _, ch := range state.Choices {
					choices = append(choices, formatManagedAuthChoice(managedAuthInputChoice{
						ID:                ch.ID,
						Label:             ch.Label,
						DisplayText:       ch.DisplayText,
						Type:              ch.Type,
						MfaType:           ch.MfaType,
						MaskedDestination: ch.MaskedDestination,
					}))
				}
				pterm.Info.Printf("  Choices: %s\n", strings.Join(choices, ", "))
			}
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

var authConnectionsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a managed auth connection",
	Long:  "Update managed authentication settings like login URL, health checks, credential source, and proxy.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthConnectionsUpdate,
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

var authConnectionsTimelineCmd = &cobra.Command{
	Use:   "timeline <id>",
	Short: "List login, reauth, and health-check events",
	Long:  "List the managed auth connection's history of login, reauth, and health-check events, most recent first.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthConnectionsTimeline,
}

func init() {
	// Create flags
	addJSONOutputFlag(authConnectionsCreateCmd)
	authConnectionsCreateCmd.Flags().String("domain", "", "Target domain for authentication (required)")
	authConnectionsCreateCmd.Flags().String("profile-name", "", "Name of the profile to manage (required)")
	authConnectionsCreateCmd.Flags().String("login-url", "", "Optional login page URL to skip discovery")
	authConnectionsCreateCmd.Flags().StringSlice("allowed-domain", []string{}, "Additional allowed domains (repeatable)")
	authConnectionsCreateCmd.Flags().String("credential-name", "", "Kernel credential name to use")
	authConnectionsCreateCmd.Flags().String("credential-provider", "", "External credential provider name")
	authConnectionsCreateCmd.Flags().String("credential-path", "", "Provider-specific path (e.g., VaultName/ItemName)")
	authConnectionsCreateCmd.Flags().Bool("credential-auto", false, "Lookup by domain from the specified provider (defaults to true when --credential-provider is set without --credential-path)")
	authConnectionsCreateCmd.Flags().String("proxy-id", "", "Proxy ID to use for this connection's browser sessions (mutually exclusive with --proxy-name and --proxy-mode)")
	authConnectionsCreateCmd.Flags().String("proxy-name", "", "Proxy name to use for this connection's browser sessions (mutually exclusive with --proxy-id and --proxy-mode)")
	authConnectionsCreateCmd.Flags().String("proxy-mode", "", "Proxy egress mode instead of a selected proxy: 'direct' for no proxy regardless of stealth, or 'default' for the stealth-derived default")
	authConnectionsCreateCmd.Flags().Bool("stealth", true, "Run this connection's browser sessions in stealth mode; use --stealth=false to disable")
	authConnectionsCreateCmd.Flags().Bool("no-save-credentials", false, "Disable saving credentials after successful login")
	authConnectionsCreateCmd.Flags().Int("health-check-interval", 0, "Interval in seconds between health checks. Defaults to 3600 or your plan minimum, whichever is larger. The maximum is 86400; the minimum depends on your plan (Enterprise 300, Startup 1200, Hobbyist 3600, Free 21600)")
	authConnectionsCreateCmd.Flags().Bool("no-health-checks", false, "Disable periodic health checks (enabled by default)")
	authConnectionsCreateCmd.Flags().Bool("no-auto-reauth", false, "Mark expired sessions as NEEDS_AUTH instead of attempting automatic re-authentication (auto re-auth is enabled by default)")
	authConnectionsCreateCmd.Flags().Bool("record-session", false, "Record browser sessions for this connection by default (useful for debugging)")
	authConnectionsCreateCmd.Flags().String("telemetry", "", "Configure telemetry for this connection's browser sessions (opt-in): --telemetry=all (default set), --telemetry=off (disable), or --telemetry=console,network (capture exactly those categories)")
	authConnectionsCreateCmd.Flags().String("telemetry-export-otlp", "", "Export this connection's captured telemetry over OTLP to one of the org's configured destinations, by ID or name; --telemetry-export-otlp=off disables export. Implies --telemetry=all when --telemetry is not set, since export requires capture")
	_ = authConnectionsCreateCmd.MarkFlagRequired("domain")
	_ = authConnectionsCreateCmd.MarkFlagRequired("profile-name")
	authConnectionsCreateCmd.MarkFlagsMutuallyExclusive("credential-name", "credential-provider")

	// Get flags
	addJSONOutputFlag(authConnectionsGetCmd)

	// Update flags
	addJSONOutputFlag(authConnectionsUpdateCmd)
	authConnectionsUpdateCmd.Flags().String("login-url", "", "Login page URL (set to empty string to clear)")
	authConnectionsUpdateCmd.Flags().StringSlice("allowed-domain", []string{}, "Additional allowed domains (replaces existing list)")
	authConnectionsUpdateCmd.Flags().String("credential-name", "", "Kernel credential name to use")
	authConnectionsUpdateCmd.Flags().String("credential-provider", "", "External credential provider name")
	authConnectionsUpdateCmd.Flags().String("credential-path", "", "Provider-specific path (e.g., VaultName/ItemName)")
	authConnectionsUpdateCmd.Flags().Bool("credential-auto", false, "Lookup by domain from the specified provider")
	authConnectionsUpdateCmd.Flags().String("proxy-id", "", "Proxy ID to use for future browser sessions (mutually exclusive with --proxy-name and --proxy-mode)")
	authConnectionsUpdateCmd.Flags().String("proxy-name", "", "Proxy name to use for future browser sessions (mutually exclusive with --proxy-id and --proxy-mode)")
	authConnectionsUpdateCmd.Flags().String("proxy-mode", "", "Proxy egress mode instead of a selected proxy: 'direct' for no proxy regardless of stealth, or 'default' to drop a selected proxy and use the stealth-derived default")
	authConnectionsUpdateCmd.Flags().Bool("stealth", true, "Set whether future browser sessions run in stealth mode; use --stealth=false to disable")
	authConnectionsUpdateCmd.Flags().Bool("save-credentials", false, "Enable saving credentials after successful login")
	authConnectionsUpdateCmd.Flags().Bool("no-save-credentials", false, "Disable saving credentials after successful login")
	authConnectionsUpdateCmd.Flags().Int("health-check-interval", 0, "Interval in seconds between health checks. The maximum is 86400; the minimum depends on your plan (Enterprise 300, Startup 1200, Hobbyist 3600, Free 21600)")
	authConnectionsUpdateCmd.Flags().Bool("health-checks", false, "Enable periodic health checks")
	authConnectionsUpdateCmd.Flags().Bool("no-health-checks", false, "Disable periodic health checks")
	authConnectionsUpdateCmd.Flags().Bool("auto-reauth", false, "Permit automatic re-authentication when a health check detects an expired session")
	authConnectionsUpdateCmd.Flags().Bool("no-auto-reauth", false, "Mark expired sessions as NEEDS_AUTH instead of attempting automatic re-authentication")
	authConnectionsUpdateCmd.Flags().Bool("record-session", false, "Set whether browser sessions are recorded by default; use --record-session=false to disable")
	authConnectionsUpdateCmd.Flags().String("telemetry", "", "Update telemetry for future browser sessions: --telemetry=all (reset to default set), --telemetry=off (disable), or --telemetry=console,network (merge those categories into the current selection)")
	authConnectionsUpdateCmd.Flags().String("telemetry-export-otlp", "", "Update where future sessions export captured telemetry over OTLP, by destination ID or name; --telemetry-export-otlp=off disables export. Naming a destination requires passing --telemetry in the same command, since export and capture are validated together")
	authConnectionsUpdateCmd.MarkFlagsMutuallyExclusive("credential-name", "credential-provider")
	authConnectionsUpdateCmd.MarkFlagsMutuallyExclusive("save-credentials", "no-save-credentials")
	authConnectionsUpdateCmd.MarkFlagsMutuallyExclusive("health-checks", "no-health-checks")
	authConnectionsUpdateCmd.MarkFlagsMutuallyExclusive("auto-reauth", "no-auto-reauth")

	// List flags
	addJSONOutputFlag(authConnectionsListCmd)
	authConnectionsListCmd.Flags().String("domain", "", "Filter by domain")
	authConnectionsListCmd.Flags().String("profile-name", "", "Filter by profile name")
	authConnectionsListCmd.Flags().String("query", "", "Search auth connections by ID, domain, or profile name")
	authConnectionsListCmd.Flags().Int("limit", 0, "Maximum number of results to return")
	authConnectionsListCmd.Flags().Int("offset", 0, "Number of results to skip")

	// Delete flags
	authConnectionsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Login flags
	addJSONOutputFlag(authConnectionsLoginCmd)
	authConnectionsLoginCmd.Flags().String("proxy-id", "", "Proxy ID to use for this login (mutually exclusive with --proxy-name and --proxy-mode)")
	authConnectionsLoginCmd.Flags().String("proxy-name", "", "Proxy name to use for this login (mutually exclusive with --proxy-id and --proxy-mode)")
	authConnectionsLoginCmd.Flags().String("proxy-mode", "", "Proxy egress mode for this login instead of a selected proxy: 'direct' for no proxy regardless of stealth, or 'default' for the stealth-derived default")
	authConnectionsLoginCmd.Flags().Bool("stealth", true, "Override stealth mode for this login's browser session; use --stealth=false to disable")
	authConnectionsLoginCmd.Flags().Bool("record-session", false, "Override whether this login's browser session is recorded; use --record-session=false to disable")
	authConnectionsLoginCmd.Flags().String("telemetry", "", "Telemetry override for this login only, merged onto the connection's config: --telemetry=all, --telemetry=off, or --telemetry=console,network")
	authConnectionsLoginCmd.Flags().String("telemetry-export-otlp", "", "Export override for this login only: an OTLP destination ID or name; --telemetry-export-otlp=off disables export for this login. Naming a destination requires passing --telemetry in the same command, since export and capture are validated together")

	// Submit flags
	addJSONOutputFlag(authConnectionsSubmitCmd)
	authConnectionsSubmitCmd.Flags().StringArray("field-value", []string{}, "Canonical field-id=value pair from the connection's `fields` list (repeatable)")
	authConnectionsSubmitCmd.Flags().String("choice-id", "", "Canonical choice ID from the connection's `choices` list")
	authConnectionsSubmitCmd.Flags().StringArray("field", []string{}, "Legacy field name=value pair (repeatable); prefer --field-value")
	authConnectionsSubmitCmd.Flags().String("mfa-option-id", "", "MFA option ID if user selected an MFA method")
	authConnectionsSubmitCmd.Flags().String("sign-in-option-id", "", "Sign-in option ID if the flow returned non-MFA choices")
	authConnectionsSubmitCmd.Flags().String("sso-button-selector", "", "XPath selector if user chose an SSO button")
	authConnectionsSubmitCmd.Flags().String("sso-provider", "", "SSO provider if user chose an SSO button by provider (e.g. google, github)")

	// Follow flags
	addJSONOutputFlag(authConnectionsFollowCmd)

	// Timeline flags
	addJSONOutputFlag(authConnectionsTimelineCmd)
	authConnectionsTimelineCmd.Flags().String("type", "", "Filter to a single event type: login, reauth, or health_check")
	authConnectionsTimelineCmd.Flags().Int("page", 1, "Page number (1-based)")
	authConnectionsTimelineCmd.Flags().Int("per-page", 20, "Items per page (default 20)")

	// Wire up commands
	authConnectionsCmd.AddCommand(authConnectionsCreateCmd)
	authConnectionsCmd.AddCommand(authConnectionsUpdateCmd)
	authConnectionsCmd.AddCommand(authConnectionsGetCmd)
	authConnectionsCmd.AddCommand(authConnectionsListCmd)
	authConnectionsCmd.AddCommand(authConnectionsDeleteCmd)
	authConnectionsCmd.AddCommand(authConnectionsLoginCmd)
	authConnectionsCmd.AddCommand(authConnectionsSubmitCmd)
	authConnectionsCmd.AddCommand(authConnectionsFollowCmd)
	authConnectionsCmd.AddCommand(authConnectionsTimelineCmd)

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
	proxyMode, _ := cmd.Flags().GetString("proxy-mode")
	noSaveCredentials, _ := cmd.Flags().GetBool("no-save-credentials")
	healthCheckInterval, _ := cmd.Flags().GetInt("health-check-interval")
	noHealthChecks, _ := cmd.Flags().GetBool("no-health-checks")
	noAutoReauth, _ := cmd.Flags().GetBool("no-auto-reauth")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	telemetryExport, _ := cmd.Flags().GetString("telemetry-export-otlp")

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
		ProxyMode:           proxyMode,
		Stealth:             readBoolFlag(cmd.Flags(), "stealth"),
		NoSaveCredentials:   noSaveCredentials,
		HealthCheckInterval: healthCheckInterval,
		NoHealthChecks:      noHealthChecks,
		NoAutoReauth:        noAutoReauth,
		RecordSession:       readBoolFlag(cmd.Flags(), "record-session"),
		Telemetry:           telemetry,
		TelemetryExport:     telemetryExport,
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

func runAuthConnectionsUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	loginURL, _ := cmd.Flags().GetString("login-url")
	allowedDomains, _ := cmd.Flags().GetStringSlice("allowed-domain")
	credentialName, _ := cmd.Flags().GetString("credential-name")
	credentialProvider, _ := cmd.Flags().GetString("credential-provider")
	credentialPath, _ := cmd.Flags().GetString("credential-path")
	credentialAuto, _ := cmd.Flags().GetBool("credential-auto")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	proxyName, _ := cmd.Flags().GetString("proxy-name")
	proxyMode, _ := cmd.Flags().GetString("proxy-mode")
	saveCredentials, _ := cmd.Flags().GetBool("save-credentials")
	noSaveCredentials, _ := cmd.Flags().GetBool("no-save-credentials")
	healthCheckInterval, _ := cmd.Flags().GetInt("health-check-interval")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	telemetryExport, _ := cmd.Flags().GetString("telemetry-export-otlp")

	saveCredentialsFlag := BoolFlag{}

	if cmd.Flags().Changed("save-credentials") {
		saveCredentialsFlag = BoolFlag{Set: true, Value: saveCredentials}
	}
	if cmd.Flags().Changed("no-save-credentials") {
		saveCredentialsFlag = BoolFlag{Set: true, Value: !noSaveCredentials}
	}

	// Each of these is a tri-state override: --x sets true, --no-x sets false, and
	// omitting both leaves the connection's current value untouched.
	togglePair := func(onFlag, offFlag string) BoolFlag {
		if cmd.Flags().Changed(onFlag) {
			v, _ := cmd.Flags().GetBool(onFlag)
			return BoolFlag{Set: true, Value: v}
		}
		if cmd.Flags().Changed(offFlag) {
			v, _ := cmd.Flags().GetBool(offFlag)
			return BoolFlag{Set: true, Value: !v}
		}
		return BoolFlag{}
	}

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Update(cmd.Context(), AuthConnectionUpdateInput{
		ID:                     args[0],
		LoginURL:               loginURL,
		LoginURLSet:            cmd.Flags().Changed("login-url"),
		AllowedDomains:         allowedDomains,
		AllowedDomainsSet:      cmd.Flags().Changed("allowed-domain"),
		CredentialName:         credentialName,
		CredentialNameSet:      cmd.Flags().Changed("credential-name"),
		CredentialProvider:     credentialProvider,
		CredentialProviderSet:  cmd.Flags().Changed("credential-provider"),
		CredentialPath:         credentialPath,
		CredentialPathSet:      cmd.Flags().Changed("credential-path"),
		CredentialAuto:         BoolFlag{Set: cmd.Flags().Changed("credential-auto"), Value: credentialAuto},
		ProxyID:                proxyID,
		ProxyIDSet:             cmd.Flags().Changed("proxy-id"),
		ProxyName:              proxyName,
		ProxyNameSet:           cmd.Flags().Changed("proxy-name"),
		ProxyMode:              proxyMode,
		Stealth:                readBoolFlag(cmd.Flags(), "stealth"),
		SaveCredentials:        saveCredentialsFlag,
		HealthCheckInterval:    healthCheckInterval,
		HealthCheckIntervalSet: cmd.Flags().Changed("health-check-interval"),
		HealthChecks:           togglePair("health-checks", "no-health-checks"),
		AutoReauth:             togglePair("auto-reauth", "no-auto-reauth"),
		RecordSession:          readBoolFlag(cmd.Flags(), "record-session"),
		Telemetry:              telemetry,
		TelemetryExport:        telemetryExport,
		Output:                 output,
	})
}

func runAuthConnectionsList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	domain, _ := cmd.Flags().GetString("domain")
	profileName, _ := cmd.Flags().GetString("profile-name")
	query, _ := cmd.Flags().GetString("query")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.List(cmd.Context(), AuthConnectionListInput{
		Domain:      domain,
		ProfileName: profileName,
		Query:       query,
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
	proxyMode, _ := cmd.Flags().GetString("proxy-mode")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	telemetryExport, _ := cmd.Flags().GetString("telemetry-export-otlp")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Login(cmd.Context(), AuthConnectionLoginInput{
		ID:              args[0],
		ProxyID:         proxyID,
		ProxyName:       proxyName,
		ProxyMode:       proxyMode,
		Stealth:         readBoolFlag(cmd.Flags(), "stealth"),
		RecordSession:   readBoolFlag(cmd.Flags(), "record-session"),
		Telemetry:       telemetry,
		TelemetryExport: telemetryExport,
		Output:          output,
	})
}

func runAuthConnectionsSubmit(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	fieldPairs, _ := cmd.Flags().GetStringArray("field")
	canonicalFieldPairs, _ := cmd.Flags().GetStringArray("field-value")
	choiceID, _ := cmd.Flags().GetString("choice-id")
	mfaOptionID, _ := cmd.Flags().GetString("mfa-option-id")
	signInOptionID, _ := cmd.Flags().GetString("sign-in-option-id")
	ssoButtonSelector, _ := cmd.Flags().GetString("sso-button-selector")
	ssoProvider, _ := cmd.Flags().GetString("sso-provider")

	// Parse field pairs into map
	fieldValues := make(map[string]string)
	for _, pair := range fieldPairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid field format: %s (expected key=value)", pair)
		}
		fieldValues[parts[0]] = parts[1]
	}

	canonicalFieldValues, err := parseStringMapFlag(canonicalFieldPairs, "--field-value")
	if err != nil {
		return err
	}

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Submit(cmd.Context(), AuthConnectionSubmitInput{
		ID:                   args[0],
		FieldValues:          fieldValues,
		CanonicalFieldValues: canonicalFieldValues,
		SelectedChoiceID:     choiceID,
		MfaOptionID:          mfaOptionID,
		SignInOptionID:       signInOptionID,
		SSOButtonSelector:    ssoButtonSelector,
		SSOProvider:          ssoProvider,
		Output:               output,
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

func runAuthConnectionsTimeline(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")
	eventType, _ := cmd.Flags().GetString("type")
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")

	svc := client.Auth.Connections
	c := AuthConnectionCmd{svc: &svc}
	return c.Timeline(cmd.Context(), AuthConnectionTimelineInput{
		ID:      args[0],
		Type:    eventType,
		Page:    page,
		PerPage: perPage,
		Output:  output,
	})
}
