package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/param"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// OrgLimitsService defines the subset of the Kernel SDK organization limits client that we use.
type OrgLimitsService interface {
	Get(ctx context.Context, opts ...option.RequestOption) (res *kernel.OrgLimits, err error)
	Update(ctx context.Context, body kernel.OrganizationLimitUpdateParams, opts ...option.RequestOption) (res *kernel.OrgLimits, err error)
}

// OrgEntitlementsService defines the organization entitlements operation used by the CLI.
type OrgEntitlementsService interface {
	Get(ctx context.Context, opts ...option.RequestOption) (res *kernel.OrgEntitlements, err error)
}

type OrgCmd struct {
	limits       OrgLimitsService
	entitlements OrgEntitlementsService
}

type OrgLimitsGetInput struct {
	Output string
}

type OrgLimitsSetInput struct {
	DefaultProjectMaxConcurrentSessions Int64Flag
	Output                              string
}

type OrgEntitlementsInput struct {
	Output string
}

func (c OrgCmd) Entitlements(ctx context.Context, in OrgEntitlementsInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	entitlements, err := c.entitlements.Get(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if entitlements == nil {
			fmt.Println("null")
			return nil
		}
		return util.PrintPrettyJSON(entitlements)
	}

	renderOrgEntitlements(entitlements)
	return nil
}

func (c OrgCmd) LimitsGet(ctx context.Context, in OrgLimitsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	limits, err := c.limits.Get(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if limits == nil {
			fmt.Println("null")
			return nil
		}
		return util.PrintPrettyJSON(limits)
	}

	renderOrgLimits(limits)
	return nil
}

func (c OrgCmd) LimitsSet(ctx context.Context, in OrgLimitsSetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	if !in.DefaultProjectMaxConcurrentSessions.Set {
		return fmt.Errorf("must provide --default-project-max-concurrent-sessions")
	}
	if in.DefaultProjectMaxConcurrentSessions.Value < 0 {
		return fmt.Errorf("--default-project-max-concurrent-sessions must be non-negative (got %d); use 0 to remove the default", in.DefaultProjectMaxConcurrentSessions.Value)
	}

	limits, err := c.limits.Update(ctx, kernel.OrganizationLimitUpdateParams{
		UpdateOrgLimitsRequest: kernel.UpdateOrgLimitsRequestParam{
			DefaultProjectMaxConcurrentSessions: param.NewOpt(in.DefaultProjectMaxConcurrentSessions.Value),
		},
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if limits == nil {
			fmt.Println("null")
			return nil
		}
		return util.PrintPrettyJSON(limits)
	}

	pterm.Success.Println("Organization limits updated:")
	renderOrgLimits(limits)
	return nil
}

func renderOrgLimits(limits *kernel.OrgLimits) {
	if limits == nil {
		pterm.Info.Println("No organization limits found")
		return
	}

	rows := pterm.TableData{
		{"Limit", "Value"},
		// max_concurrent_sessions is read-only and always present; only the
		// per-project default is nullable, so reuse the "unlimited" rendering.
		{"Max Concurrent Sessions", fmt.Sprintf("%d", limits.MaxConcurrentSessions)},
		{"Default Project Max Concurrent Sessions", formatProjectLimitValue(limits.DefaultProjectMaxConcurrentSessions, limits.JSON.DefaultProjectMaxConcurrentSessions)},
	}

	// Managed auth limits are plan-derived and only returned by newer API
	// versions, so render each row only when the field is present. A null
	// max_auth_connections means unlimited, so presence — not validity — is the
	// right check here.
	if orgLimitFieldPresent(limits.JSON.MaxAuthConnections) {
		rows = append(rows, []string{"Max Auth Connections", formatProjectLimitValue(limits.MaxAuthConnections, limits.JSON.MaxAuthConnections)})
	}
	if orgLimitFieldPresent(limits.JSON.AuthConnectionsUsed) {
		rows = append(rows, []string{"Auth Connections Used", fmt.Sprintf("%d", limits.AuthConnectionsUsed)})
	}
	if orgLimitFieldPresent(limits.JSON.MinHealthCheckIntervalSeconds) {
		rows = append(rows, []string{"Min Health Check Interval", fmt.Sprintf("%ds", limits.MinHealthCheckIntervalSeconds)})
	}

	PrintTableNoPad(rows, true)
}

// orgLimitFieldPresent reports whether the API returned the field at all,
// treating an explicit JSON null as present so nullable limits still render.
func orgLimitFieldPresent(field respjson.Field) bool {
	return field.Raw() != respjson.Omitted
}

func renderOrgEntitlements(entitlements *kernel.OrgEntitlements) {
	if entitlements == nil {
		pterm.Info.Println("No organization entitlements found")
		return
	}

	PrintTableNoPad(orgEntitlementRows(entitlements), true)
}

func orgEntitlementRows(entitlements *kernel.OrgEntitlements) pterm.TableData {
	status := formatNullableEntitlementString(entitlements.Plan.Status, entitlements.Plan.JSON.Status)
	trialEndsAt := "unknown"
	if entitlements.Plan.JSON.TrialEndsAt.Raw() == respjson.Null {
		trialEndsAt = "none"
	} else if entitlements.Plan.JSON.TrialEndsAt.Valid() {
		trialEndsAt = util.FormatLocal(entitlements.Plan.TrialEndsAt)
	}

	features := entitlements.Features
	limits := entitlements.Limits
	return pterm.TableData{
		{"Category", "Entitlement", "Value"},
		{"Plan", "Contractual plan", entitlements.Plan.ID},
		{"Plan", "Effective plan", entitlements.Plan.EffectiveID},
		{"Plan", "Status", status},
		{"Plan", "Trialing", fmt.Sprintf("%t", entitlements.Plan.IsTrialing)},
		{"Plan", "Trial ends at", trialEndsAt},
		{"Feature", "Profiles", fmt.Sprintf("%t", features.Profiles.Enabled)},
		{"Feature", "File I/O", fmt.Sprintf("%t", features.FileIo.Enabled)},
		{"Feature", "Browser replays", fmt.Sprintf("%t", features.BrowserReplays.Enabled)},
		{"Feature", "Browser replay retention (days)", fmt.Sprintf("%d", features.BrowserReplays.RetentionDays)},
		{"Feature", "Browser extensions", fmt.Sprintf("%t", features.BrowserExtensions.Enabled)},
		{"Feature", "Max stored extensions", formatEntitlementLimitValue(features.BrowserExtensions.MaxStoredPerOrg, features.BrowserExtensions.JSON.MaxStoredPerOrg)},
		{"Feature", "Browser pools", fmt.Sprintf("%t", features.BrowserPools.Enabled)},
		{"Feature", "Managed auth", fmt.Sprintf("%t", features.ManagedAuth.Enabled)},
		{"Feature", "Max managed auth connections", formatEntitlementLimitValue(features.ManagedAuth.MaxConnections, features.ManagedAuth.JSON.MaxConnections)},
		{"Feature", "Health check minimum (seconds)", fmt.Sprintf("%d", features.ManagedAuth.HealthCheckIntervalMinSeconds)},
		{"Feature", "Health check default (seconds)", fmt.Sprintf("%d", features.ManagedAuth.HealthCheckIntervalDefaultSeconds)},
		{"Feature", "Health check maximum (seconds)", fmt.Sprintf("%d", features.ManagedAuth.HealthCheckIntervalMaxSeconds)},
		{"Feature", "Credentials", fmt.Sprintf("%t", features.Credentials.Enabled)},
		{"Feature", "Credential providers", fmt.Sprintf("%t", features.CredentialProviders.Enabled)},
		{"Feature", "Managed proxies", fmt.Sprintf("%t", features.ManagedProxies.Enabled)},
		{"Feature", "Custom proxies", fmt.Sprintf("%t", features.CustomProxies.Enabled)},
		{"Feature", "Proxy bypass hosts", fmt.Sprintf("%t", features.ProxyBypassHosts.Enabled)},
		{"Feature", "GPU", fmt.Sprintf("%t", features.GPU.Enabled)},
		{"Limit", "Max concurrent browsers", fmt.Sprintf("%d", limits.MaxConcurrentBrowsers)},
		{"Limit", "Max concurrent invocations", fmt.Sprintf("%d", limits.MaxConcurrentInvocations)},
		{"Limit", "Default max concurrent invocations per app", fmt.Sprintf("%d", limits.DefaultMaxConcurrentInvocationsPerApp)},
	}
}

func formatNullableEntitlementString(value string, field respjson.Field) string {
	if field.Raw() == respjson.Null {
		return "none"
	}
	if !field.Valid() {
		return "unknown"
	}
	var decoded string
	if err := json.Unmarshal([]byte(field.Raw()), &decoded); err != nil {
		return "unknown"
	}
	return value
}

func formatEntitlementLimitValue(value int64, field respjson.Field) string {
	if field.Raw() == respjson.Null {
		return "unlimited"
	}
	if !field.Valid() {
		return "unknown"
	}
	return fmt.Sprintf("%d", value)
}

// --- Cobra wiring ---

var orgCmd = &cobra.Command{
	Use:     "org",
	Aliases: []string{"organization"},
	Short:   "Manage organization-wide settings",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var orgLimitsCmd = &cobra.Command{
	Use:   "limits",
	Short: "Manage organization limits",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var orgLimitsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get organization limits",
	Long:  "Show the organization's effective limits: the concurrency limit, the default per-project cap applied to projects without an explicit override, and the plan-derived managed auth limits along with current auth connection usage.",
	Args:  cobra.NoArgs,
	RunE:  runOrgLimitsGet,
}

var orgLimitsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set the default per-project concurrency cap",
	Long:  "Set the default per-project concurrency cap applied to projects without an explicit override. Use 0 to remove the default. The default cannot exceed the organization's concurrency limit.",
	Args:  cobra.NoArgs,
	RunE:  runOrgLimitsSet,
}

var orgEntitlementsCmd = &cobra.Command{
	Use:   "entitlements",
	Short: "Get effective organization entitlements",
	Long:  "Show the authenticated organization's effective feature access and limits after applying its plan, trial, status, and organization-specific overrides. Unlimited values are shown as unlimited.",
	Args:  cobra.NoArgs,
	RunE:  runOrgEntitlements,
}

func getOrgHandler(cmd *cobra.Command) OrgCmd {
	client := getKernelClient(cmd)
	return OrgCmd{
		limits:       &client.Organization.Limits,
		entitlements: &client.Organization.Entitlements,
	}
}

func runOrgLimitsGet(cmd *cobra.Command, args []string) error {
	c := getOrgHandler(cmd)
	output, _ := cmd.Flags().GetString("output")
	return c.LimitsGet(cmd.Context(), OrgLimitsGetInput{Output: output})
}

func runOrgLimitsSet(cmd *cobra.Command, args []string) error {
	c := getOrgHandler(cmd)
	defaultMax, _ := cmd.Flags().GetInt64("default-project-max-concurrent-sessions")
	output, _ := cmd.Flags().GetString("output")
	return c.LimitsSet(cmd.Context(), OrgLimitsSetInput{
		DefaultProjectMaxConcurrentSessions: Int64Flag{
			Set:   cmd.Flags().Changed("default-project-max-concurrent-sessions"),
			Value: defaultMax,
		},
		Output: output,
	})
}

func runOrgEntitlements(cmd *cobra.Command, args []string) error {
	c := getOrgHandler(cmd)
	output, _ := cmd.Flags().GetString("output")
	return c.Entitlements(cmd.Context(), OrgEntitlementsInput{Output: output})
}

func init() {
	addJSONOutputFlag(orgLimitsGetCmd)
	orgLimitsSetCmd.Flags().Int64("default-project-max-concurrent-sessions", 0, "Default maximum concurrent browsers for projects without an explicit override (0 to remove the default)")
	addJSONOutputFlag(orgLimitsSetCmd)
	addJSONOutputFlag(orgEntitlementsCmd)

	orgLimitsCmd.AddCommand(orgLimitsGetCmd)
	orgLimitsCmd.AddCommand(orgLimitsSetCmd)
	orgCmd.AddCommand(orgLimitsCmd)
	orgCmd.AddCommand(orgEntitlementsCmd)
}
