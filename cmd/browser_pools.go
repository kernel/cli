package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// BrowserPoolsService defines the subset of the Kernel SDK browser pools client that we use.
type BrowserPoolsService interface {
	List(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.BrowserPool], err error)
	New(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (res *kernel.BrowserPool, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.BrowserPool, err error)
	Update(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (res *kernel.BrowserPool, err error)
	Delete(ctx context.Context, id string, body kernel.BrowserPoolDeleteParams, opts ...option.RequestOption) (err error)
	Acquire(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (res *kernel.BrowserPoolAcquireResponse, err error)
	Release(ctx context.Context, id string, body kernel.BrowserPoolReleaseParams, opts ...option.RequestOption) (err error)
	Flush(ctx context.Context, id string, opts ...option.RequestOption) (err error)
}

type BrowserPoolsCmd struct {
	client BrowserPoolsService
}

type BrowserPoolsListInput struct {
	Name   string
	Query  string
	Limit  int
	Offset int
	Region string
	Output string
}

func (c BrowserPoolsCmd) List(ctx context.Context, in BrowserPoolsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.BrowserPoolListParams{}
	if in.Name != "" {
		params.Name = kernel.String(in.Name)
	}
	if in.Query != "" {
		params.Query = kernel.String(in.Query)
	}
	if in.Limit > 0 {
		params.Limit = kernel.Int(int64(in.Limit))
	}
	if in.Offset > 0 {
		params.Offset = kernel.Int(int64(in.Offset))
	}
	region, err := parseRegionFlag(in.Region)
	if err != nil {
		return err
	}
	if region != "" {
		params.Region = kernel.BrowserPoolListParamsRegion(region)
	}

	page, err := c.client.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var pools []kernel.BrowserPool
	if page != nil {
		pools = page.Items
	}

	if in.Output == "json" {
		if len(pools) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(pools)
	}

	if len(pools) == 0 {
		pterm.Info.Println("No browser pools found")
		return nil
	}

	tableData := pterm.TableData{
		{"ID", "Name", "Available", "Acquired", "Created At", "Region", "Size"},
	}

	for _, p := range pools {
		tableData = append(tableData, []string{
			p.ID,
			util.OrDash(p.Name),
			fmt.Sprintf("%d", p.AvailableCount),
			fmt.Sprintf("%d", p.AcquiredCount),
			util.FormatLocal(p.CreatedAt),
			util.OrDash(string(p.Region)),
			fmt.Sprintf("%d", p.BrowserPoolConfig.Size),
		})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

// buildPoolNewTelemetryParam converts --telemetry and --telemetry-cdp-exclude flag
// values to the pool create param.
func buildPoolNewTelemetryParam(s, cdpExclude string) (kernel.BrowserPoolNewParamsTelemetry, error) {
	enabled, browser, err := resolveTelemetryFlag(s, cdpExclude)
	return kernel.BrowserPoolNewParamsTelemetry{Enabled: enabled, Browser: browser}, err
}

// buildPoolUpdateTelemetryParam converts --telemetry and --telemetry-cdp-exclude flag
// values to the pool update param.
func buildPoolUpdateTelemetryParam(s, cdpExclude string) (kernel.BrowserPoolUpdateParamsTelemetry, error) {
	enabled, browser, err := resolveTelemetryFlag(s, cdpExclude)
	return kernel.BrowserPoolUpdateParamsTelemetry{Enabled: enabled, Browser: browser}, err
}

// buildPoolAcquireTelemetryParam converts --telemetry and --telemetry-cdp-exclude flag
// values to the acquire override param.
func buildPoolAcquireTelemetryParam(s, cdpExclude string) (kernel.BrowserPoolAcquireParamsTelemetry, error) {
	enabled, browser, err := resolveTelemetryFlag(s, cdpExclude)
	return kernel.BrowserPoolAcquireParamsTelemetry{Enabled: enabled, Browser: browser}, err
}

// formatPoolTelemetry renders a pool's active telemetry config for the details table.
func formatPoolTelemetry(cfg kernel.BrowserTelemetryConfig) string {
	on := telemetryEnabledCategories(cfg)
	if len(on) == 0 {
		return "disabled"
	}
	base := strings.Join(on, ", ")
	if ex := formatCdpExcludedMethods(cfg.Browser.Control.Cdp.ExcludedMethods); ex != "" {
		return base + " (excluding CDP methods: " + ex + ")"
	}
	return base
}

type BrowserPoolsCreateInput struct {
	Name                   string
	Size                   int64
	FillRate               int64
	TimeoutSeconds         int64
	Stealth                BoolFlag
	Headless               BoolFlag
	Kiosk                  BoolFlag
	RefreshOnProfileUpdate BoolFlag
	ProfileID              string
	ProfileName            string
	ProxyID                string
	Region                 string
	PrivateHosts           []string
	StartURL               string
	Extensions             []string
	Viewport               string
	ChromePolicy           string
	ChromePolicyFile       string
	Telemetry              string
	TelemetryCdpExclude    string
	Output                 string
}

func (c BrowserPoolsCmd) Create(ctx context.Context, in BrowserPoolsCreateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if err := validateStartURLFlag(in.StartURL); err != nil {
		return err
	}

	params := kernel.BrowserPoolNewParams{
		Size: in.Size,
	}

	if in.Name != "" {
		params.Name = kernel.String(in.Name)
	}
	if in.FillRate > 0 {
		params.FillRatePerMinute = kernel.Int(in.FillRate)
	}
	if in.TimeoutSeconds > 0 {
		params.TimeoutSeconds = kernel.Int(in.TimeoutSeconds)
	}
	if in.Stealth.Set {
		params.Stealth = kernel.Bool(in.Stealth.Value)
	}
	if in.Headless.Set {
		params.Headless = kernel.Bool(in.Headless.Value)
	}
	if in.Kiosk.Set {
		params.KioskMode = kernel.Bool(in.Kiosk.Value)
	}
	if in.RefreshOnProfileUpdate.Set {
		params.RefreshOnProfileUpdate = kernel.Bool(in.RefreshOnProfileUpdate.Value)
	}

	profileID, profileName, profileSet, err := resolvePoolProfile(in.ProfileID, in.ProfileName)
	if err != nil {
		pterm.Error.Println(err.Error())
		return nil
	}
	if profileSet {
		if profileID != "" {
			params.Profile.ID = kernel.String(profileID)
		} else {
			params.Profile.Name = kernel.String(profileName)
		}
	}

	if in.ProxyID != "" {
		params.ProxyID = kernel.String(in.ProxyID)
	}
	if in.StartURL != "" {
		params.StartURL = kernel.String(in.StartURL)
	}

	region, err := parseRegionFlag(in.Region)
	if err != nil {
		return err
	}
	if region != "" {
		params.Region = kernel.BrowserPoolNewParamsRegion(region)
	}

	network, err := buildNetworkParam(in.PrivateHosts)
	if err != nil {
		return err
	}
	if len(network.PrivateHosts) > 0 {
		params.Network = network
	}

	params.Extensions = buildExtensionsParam(in.Extensions)

	viewport, err := buildViewportParam(in.Viewport)
	if err != nil {
		pterm.Error.Println(err.Error())
		return nil
	}
	if viewport != nil {
		params.Viewport = *viewport
	}

	chromePolicy, err := parseChromePolicy(in.ChromePolicy, in.ChromePolicyFile)
	if err != nil {
		return err
	}
	if len(chromePolicy) > 0 {
		params.ChromePolicy = chromePolicy
	}

	if in.Telemetry != "" || in.TelemetryCdpExclude != "" {
		t, err := buildPoolNewTelemetryParam(in.Telemetry, in.TelemetryCdpExclude)
		if err != nil {
			return err
		}
		params.Telemetry = t
	}

	pool, err := c.client.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(pool)
	}

	if pool.Name != "" {
		pterm.Success.Printf("Created browser pool %s (%s)\n", pool.Name, pool.ID)
	} else {
		pterm.Success.Printf("Created browser pool %s\n", pool.ID)
	}
	if in.Telemetry != "" || in.TelemetryCdpExclude != "" {
		printTelemetrySummary(pool.BrowserPoolConfig.Telemetry)
	}
	return nil
}

type BrowserPoolsGetInput struct {
	IDOrName string
	Output   string
}

func (c BrowserPoolsCmd) Get(ctx context.Context, in BrowserPoolsGetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	pool, err := c.client.Get(ctx, in.IDOrName)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(pool)
	}

	cfg := pool.BrowserPoolConfig

	rows := pterm.TableData{
		{"Property", "Value"},
		{"ID", pool.ID},
		{"Name", util.OrDash(pool.Name)},
		{"Created At", util.FormatLocal(pool.CreatedAt)},
		{"Region", util.OrDash(string(pool.Region))},
		{"Size", fmt.Sprintf("%d", cfg.Size)},
		{"Available", fmt.Sprintf("%d", pool.AvailableCount)},
		{"Acquired", fmt.Sprintf("%d", pool.AcquiredCount)},
		{"Fill Rate", formatFillRate(cfg.FillRatePerMinute)},
		{"Timeout", fmt.Sprintf("%d seconds", cfg.TimeoutSeconds)},
		{"Headless", fmt.Sprintf("%t", cfg.Headless)},
		{"Stealth", fmt.Sprintf("%t", cfg.Stealth)},
		{"Kiosk Mode", fmt.Sprintf("%t", cfg.KioskMode)},
		{"Refresh On Profile Update", fmt.Sprintf("%t", cfg.RefreshOnProfileUpdate)},
		{"Profile", formatProfile(cfg.Profile)},
		{"Proxy ID", util.OrDash(cfg.ProxyID)},
		{"Start URL", util.OrDash(cfg.StartURL)},
		{"Extensions", formatExtensions(cfg.Extensions)},
		{"Viewport", formatViewport(cfg.Viewport)},
		{"Private Hosts", formatPrivateHosts(cfg.Network)},
		{"Telemetry", formatPoolTelemetry(cfg.Telemetry)},
	}

	PrintTableNoPad(rows, true)
	return nil
}

type BrowserPoolsUpdateInput struct {
	IDOrName               string
	Name                   string
	Size                   int64
	FillRate               Int64Flag
	TimeoutSeconds         int64
	Stealth                BoolFlag
	Headless               BoolFlag
	Kiosk                  BoolFlag
	RefreshOnProfileUpdate BoolFlag
	ProfileID              string
	ProfileName            string
	ClearProfile           bool
	ProxyID                string
	ClearProxy             bool
	StartURL               string
	ClearStartURL          bool
	Extensions             []string
	ClearExtensions        bool
	PrivateHosts           []string
	ClearPrivateHosts      bool
	Viewport               string
	ChromePolicy           string
	ChromePolicyFile       string
	ClearChromePolicy      bool
	Telemetry              string
	TelemetryCdpExclude    string
	DiscardAllIdle         BoolFlag
	Output                 string
}

func validateBrowserPoolUpdateInput(in BrowserPoolsUpdateInput) error {
	if in.StartURL != "" && in.ClearStartURL {
		return fmt.Errorf("cannot specify both --start-url and --clear-start-url")
	}
	if in.FillRate.Set && in.FillRate.Value < 0 {
		return fmt.Errorf("--fill-rate must be zero or greater")
	}
	if in.ProxyID != "" && in.ClearProxy {
		return fmt.Errorf("cannot specify both --proxy-id and --clear-proxy")
	}
	if (in.ProfileID != "" || in.ProfileName != "") && in.ClearProfile {
		return fmt.Errorf("cannot specify --clear-profile with --profile-id or --profile-name")
	}
	if len(in.Extensions) > 0 && in.ClearExtensions {
		return fmt.Errorf("cannot specify both --extension and --clear-extensions")
	}
	if (in.ChromePolicy != "" || in.ChromePolicyFile != "") && in.ClearChromePolicy {
		return fmt.Errorf("cannot specify --clear-chrome-policy with --chrome-policy or --chrome-policy-file")
	}
	if len(normalizePrivateHosts(in.PrivateHosts)) > 0 && in.ClearPrivateHosts {
		return fmt.Errorf("cannot specify both --private-host and --clear-private-hosts")
	}
	return nil
}

func (c BrowserPoolsCmd) Update(ctx context.Context, in BrowserPoolsUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if err := validateStartURLFlag(in.StartURL); err != nil {
		return err
	}
	if err := validateBrowserPoolUpdateInput(in); err != nil {
		return err
	}

	params := kernel.BrowserPoolUpdateParams{}

	if in.Name != "" {
		params.Name = kernel.String(in.Name)
	}
	if in.Size > 0 {
		params.Size = kernel.Int(in.Size)
	}
	if in.FillRate.Set {
		params.FillRatePerMinute = kernel.Int(in.FillRate.Value)
	}
	if in.TimeoutSeconds > 0 {
		params.TimeoutSeconds = kernel.Int(in.TimeoutSeconds)
	}
	if in.Stealth.Set {
		params.Stealth = kernel.Bool(in.Stealth.Value)
	}
	if in.Headless.Set {
		params.Headless = kernel.Bool(in.Headless.Value)
	}
	if in.Kiosk.Set {
		params.KioskMode = kernel.Bool(in.Kiosk.Value)
	}
	if in.DiscardAllIdle.Set {
		params.DiscardAllIdle = kernel.Bool(in.DiscardAllIdle.Value)
	}
	if in.RefreshOnProfileUpdate.Set {
		params.RefreshOnProfileUpdate = kernel.Bool(in.RefreshOnProfileUpdate.Value)
	}

	profileID, profileName, profileSet, err := resolvePoolProfile(in.ProfileID, in.ProfileName)
	if err != nil {
		pterm.Error.Println(err.Error())
		return nil
	}
	if in.ClearProfile {
		params.Profile.ID = kernel.String("")
	} else if profileSet {
		if profileID != "" {
			params.Profile.ID = kernel.String(profileID)
		} else {
			params.Profile.Name = kernel.String(profileName)
		}
	}

	if in.ClearProxy {
		params.ProxyID = kernel.String("")
	} else if in.ProxyID != "" {
		params.ProxyID = kernel.String(in.ProxyID)
	}
	if in.ClearStartURL {
		params.StartURL = kernel.String("")
	} else if in.StartURL != "" {
		params.StartURL = kernel.String(in.StartURL)
	}

	params.Extensions = buildExtensionsParam(in.Extensions)

	viewport, err := buildViewportParam(in.Viewport)
	if err != nil {
		pterm.Error.Println(err.Error())
		return nil
	}
	if viewport != nil {
		params.Viewport = *viewport
	}

	chromePolicy, err := parseChromePolicy(in.ChromePolicy, in.ChromePolicyFile)
	if err != nil {
		return err
	}
	if len(chromePolicy) > 0 {
		params.ChromePolicy = chromePolicy
	}
	network, err := buildNetworkParam(in.PrivateHosts)
	if err != nil {
		return err
	}
	if len(network.PrivateHosts) > 0 {
		params.Network = network
	}

	extraFields := map[string]any{}
	// The SDK's omitzero encoder drops empty collections, so explicit clears use
	// its extra-fields escape hatch to preserve {} and [] on the wire.
	if in.ClearExtensions {
		extraFields["extensions"] = []kernel.BrowserExtensionParam{}
	}
	if in.ClearChromePolicy || (chromePolicy != nil && len(chromePolicy) == 0) {
		extraFields["chrome_policy"] = map[string]any{}
	}
	if in.ClearPrivateHosts {
		extraFields["network"] = map[string]any{}
	}
	if len(extraFields) > 0 {
		params.SetExtraFields(extraFields)
	}

	if in.Telemetry != "" || in.TelemetryCdpExclude != "" {
		t, err := buildPoolUpdateTelemetryParam(in.Telemetry, in.TelemetryCdpExclude)
		if err != nil {
			return err
		}
		params.Telemetry = t
	}

	pool, err := c.client.Update(ctx, in.IDOrName, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(pool)
	}

	if pool.Name != "" {
		pterm.Success.Printf("Updated browser pool %s (%s)\n", pool.Name, pool.ID)
	} else {
		pterm.Success.Printf("Updated browser pool %s\n", pool.ID)
	}
	if in.Telemetry != "" || in.TelemetryCdpExclude != "" {
		printTelemetrySummary(pool.BrowserPoolConfig.Telemetry)
	}
	return nil
}

type BrowserPoolsDeleteInput struct {
	IDOrName string
	Force    bool
}

func (c BrowserPoolsCmd) Delete(ctx context.Context, in BrowserPoolsDeleteInput) error {
	params := kernel.BrowserPoolDeleteParams{}
	if in.Force {
		params.Force = kernel.Bool(true)
	}
	err := c.client.Delete(ctx, in.IDOrName, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted browser pool %s\n", in.IDOrName)
	return nil
}

type BrowserPoolsAcquireInput struct {
	IDOrName            string
	TimeoutSeconds      int64
	Name                string
	StartURL            string
	Tags                map[string]string
	Telemetry           string
	TelemetryCdpExclude string
	Output              string
}

// buildAcquireParams builds the SDK params for acquiring a browser from a pool.
// Shared by `browser-pools acquire` and the `browsers create --pool-id/--pool-name`
// path so the per-lease name/tags/start-url/telemetry forwarding cannot silently
// diverge between them. The telemetry override merges onto the pool's config for
// this lease.
func buildAcquireParams(name string, tags map[string]string, timeoutSeconds int64, telemetry, telemetryCdpExclude, startURL string) (kernel.BrowserPoolAcquireParams, error) {
	params := kernel.BrowserPoolAcquireParams{}
	if timeoutSeconds > 0 {
		params.AcquireTimeoutSeconds = kernel.Int(timeoutSeconds)
	}
	if name != "" {
		params.Name = kernel.Opt(name)
	}
	if startURL != "" {
		params.StartURL = kernel.Opt(startURL)
	}
	if len(tags) > 0 {
		params.Tags = kernel.Tags(tags)
	}
	if telemetry != "" || telemetryCdpExclude != "" {
		t, err := buildPoolAcquireTelemetryParam(telemetry, telemetryCdpExclude)
		if err != nil {
			return kernel.BrowserPoolAcquireParams{}, err
		}
		params.Telemetry = t
	}
	return params, nil
}

func (c BrowserPoolsCmd) Acquire(ctx context.Context, in BrowserPoolsAcquireInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params, err := buildAcquireParams(in.Name, in.Tags, in.TimeoutSeconds, in.Telemetry, in.TelemetryCdpExclude, in.StartURL)
	if err != nil {
		return err
	}
	resp, err := c.client.Acquire(ctx, in.IDOrName, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if resp == nil {
		if in.Output == "json" {
			fmt.Println("null")
			return nil
		}
		pterm.Warning.Println("Acquire request timed out (no browser available). Retry to continue waiting.")
		return nil
	}

	if in.Output == "json" {
		return util.PrintPrettyJSON(resp)
	}

	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Session ID", resp.SessionID},
	}
	if resp.Name != "" {
		tableData = append(tableData, []string{"Name", resp.Name})
	}
	tableData = append(tableData,
		[]string{"CDP WebSocket URL", resp.CdpWsURL},
		[]string{"Live View URL", resp.BrowserLiveViewURL},
	)
	if resp.StartURL != "" {
		tableData = append(tableData, []string{"Start URL", resp.StartURL})
	}
	if len(resp.Tags) > 0 {
		tableData = append(tableData, []string{"Tags", formatTags(resp.Tags)})
	}
	PrintTableNoPad(tableData, true)
	return nil
}

type BrowserPoolsReleaseInput struct {
	IDOrName  string
	SessionID string
	Reuse     BoolFlag
}

func (c BrowserPoolsCmd) Release(ctx context.Context, in BrowserPoolsReleaseInput) error {
	params := kernel.BrowserPoolReleaseParams{
		SessionID: in.SessionID,
	}
	if in.Reuse.Set {
		params.Reuse = kernel.Bool(in.Reuse.Value)
	}
	err := c.client.Release(ctx, in.IDOrName, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Reuse.Set && !in.Reuse.Value {
		pterm.Success.Printf("Deleted browser %s from pool %s\n", in.SessionID, in.IDOrName)
	} else {
		pterm.Success.Printf("Released browser %s back to pool %s\n", in.SessionID, in.IDOrName)
	}
	return nil
}

type BrowserPoolsFlushInput struct {
	IDOrName string
}

func (c BrowserPoolsCmd) Flush(ctx context.Context, in BrowserPoolsFlushInput) error {
	err := c.client.Flush(ctx, in.IDOrName)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Flushed idle browsers from pool %s\n", in.IDOrName)
	return nil
}

var browserPoolsCmd = &cobra.Command{
	Use:     "browser-pools",
	Aliases: []string{"browser-pool", "pool", "pools"},
	Short:   "Manage browser pools",
	Long:    "Commands for managing Kernel browser pools",
}

var browserPoolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List browser pools",
	RunE:  runBrowserPoolsList,
}

var browserPoolsCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new browser pool",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runBrowserPoolsCreate,
}

var browserPoolsGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get details of a browser pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowserPoolsGet,
}

var browserPoolsUpdateCmd = &cobra.Command{
	Use:   "update <id-or-name>",
	Short: "Update a browser pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowserPoolsUpdate,
}

var browserPoolsDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a browser pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowserPoolsDelete,
}

var browserPoolsAcquireCmd = &cobra.Command{
	Use:   "acquire <id-or-name>",
	Short: "Acquire a browser from the pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowserPoolsAcquire,
}

var browserPoolsReleaseCmd = &cobra.Command{
	Use:   "release <id-or-name>",
	Short: "Release a browser back to the pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowserPoolsRelease,
}

var browserPoolsFlushCmd = &cobra.Command{
	Use:   "flush <id-or-name>",
	Short: "Flush idle browsers from the pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowserPoolsFlush,
}

func init() {
	addJSONOutputFlag(browserPoolsListCmd)
	browserPoolsListCmd.Flags().String("name", "", "Filter by exact browser pool name")
	browserPoolsListCmd.Flags().String("query", "", "Search browser pools by name (IDs match by exact value)")
	browserPoolsListCmd.Flags().Int("limit", 0, "Maximum number of pools to return")
	browserPoolsListCmd.Flags().Int("offset", 0, "Number of pools to skip (for pagination)")
	browserPoolsListCmd.Flags().String("region", "", "Filter by geographic region: 'us-east' or 'eu-west' (omit to list pools in all regions)")

	addJSONOutputFlag(browserPoolsCreateCmd)
	browserPoolsCreateCmd.Flags().String("name", "", "Optional unique name for the pool")
	browserPoolsCreateCmd.Flags().Int64("size", 0, "Number of browsers in the pool")
	_ = browserPoolsCreateCmd.MarkFlagRequired("size")
	browserPoolsCreateCmd.Flags().Int64("fill-rate", 0, "Fill rate per minute")
	browserPoolsCreateCmd.Flags().Int64("timeout", 0, "Idle timeout in seconds")
	browserPoolsCreateCmd.Flags().Bool("stealth", false, "Enable stealth mode")
	browserPoolsCreateCmd.Flags().Bool("headless", false, "Enable headless mode")
	browserPoolsCreateCmd.Flags().Bool("kiosk", false, "Enable kiosk mode")
	browserPoolsCreateCmd.Flags().Bool("refresh-on-profile-update", false, "Flush idle browsers when the pool's profile is updated")
	browserPoolsCreateCmd.Flags().String("profile-id", "", "Profile ID")
	browserPoolsCreateCmd.Flags().String("profile-name", "", "Profile name")
	browserPoolsCreateCmd.Flags().String("proxy-id", "", "Proxy ID")
	browserPoolsCreateCmd.Flags().String("region", "", "Geographic region for the pool: 'us-east' or 'eu-west'. Fixed once the pool is created; requires a Start-Up or Enterprise plan and defaults to us-east")
	browserPoolsCreateCmd.Flags().StringSlice("private-host", nil, "Destinations browsers in the pool reach directly through their own network instead of Kernel-managed egress, for private hosts on a VPN or tunnel they join (repeat or comma-separated, max 32). Accepts hostname patterns ('*.example.ts.net'), IPs ('10.1.30.63', '[fd00::1]'), and private CIDRs ('100.64.0.0/10'). Replaces the default private ranges (RFC1918, 100.64.0.0/10, fc00::/7); omit to keep them")
	browserPoolsCreateCmd.Flags().String("start-url", "", "Initial page to open for new browsers")
	browserPoolsCreateCmd.Flags().StringSlice("extension", []string{}, "Extension IDs or names")
	browserPoolsCreateCmd.Flags().String("viewport", "", "Viewport size (e.g. 1280x800)")
	browserPoolsCreateCmd.Flags().String("chrome-policy", "", "Custom Chrome enterprise policy as a JSON object")
	browserPoolsCreateCmd.Flags().String("chrome-policy-file", "", "Read Chrome enterprise policy (JSON object) from a file (use '-' for stdin)")
	browserPoolsCreateCmd.Flags().String("telemetry", "", "Configure telemetry for browsers warmed into the pool (opt-in): --telemetry=all (default set), --telemetry=off (disable), or --telemetry=console,network (capture exactly those categories)")
	browserPoolsCreateCmd.Flags().String("telemetry-cdp-exclude", "", "Leave the named CDP methods out of control telemetry's cdp_command events, comma-separated (e.g. Input.dispatchMouseEvent,Page.captureScreenshot); --telemetry-cdp-exclude=none clears the list. Excluded commands are still relayed to the browser, they just produce no event")
	browserPoolsCreateCmd.MarkFlagsMutuallyExclusive("chrome-policy", "chrome-policy-file")

	addJSONOutputFlag(browserPoolsGetCmd)

	browserPoolsUpdateCmd.Flags().String("name", "", "Update the pool name")
	browserPoolsUpdateCmd.Flags().Int64("size", 0, "Number of browsers in the pool")
	browserPoolsUpdateCmd.Flags().Int64("fill-rate", 0, "Fill rate per minute")
	browserPoolsUpdateCmd.Flags().Int64("timeout", 0, "Idle timeout in seconds")
	browserPoolsUpdateCmd.Flags().Bool("stealth", false, "Enable stealth mode")
	browserPoolsUpdateCmd.Flags().Bool("headless", false, "Enable headless mode")
	browserPoolsUpdateCmd.Flags().Bool("kiosk", false, "Enable kiosk mode")
	browserPoolsUpdateCmd.Flags().Bool("refresh-on-profile-update", false, "Flush idle browsers when the pool's profile is updated")
	browserPoolsUpdateCmd.Flags().String("profile-id", "", "Profile ID")
	browserPoolsUpdateCmd.Flags().String("profile-name", "", "Profile name")
	browserPoolsUpdateCmd.Flags().Bool("clear-profile", false, "Remove the pool profile")
	browserPoolsUpdateCmd.Flags().String("proxy-id", "", "Proxy ID")
	browserPoolsUpdateCmd.Flags().Bool("clear-proxy", false, "Remove the pool proxy")
	browserPoolsUpdateCmd.Flags().String("start-url", "", "Initial page to open for new browsers")
	browserPoolsUpdateCmd.Flags().Bool("clear-start-url", false, "Clear the pool start URL")
	browserPoolsUpdateCmd.Flags().StringSlice("extension", []string{}, "Extension IDs or names")
	browserPoolsUpdateCmd.Flags().Bool("clear-extensions", false, "Remove all pool extensions")
	browserPoolsUpdateCmd.Flags().StringSlice("private-host", nil, "Replace the destinations browsers in the pool reach directly through their own network instead of Kernel-managed egress (repeat or comma-separated, max 32). Accepts hostname patterns ('*.example.ts.net'), IPs ('10.1.30.63', '[fd00::1]'), and private CIDRs ('100.64.0.0/10'). Only applies to browsers created after the update")
	browserPoolsUpdateCmd.Flags().Bool("clear-private-hosts", false, "Remove the private-host override and restore the default private ranges")
	browserPoolsUpdateCmd.Flags().String("viewport", "", "Viewport size (e.g. 1280x800)")
	browserPoolsUpdateCmd.Flags().String("chrome-policy", "", "Custom Chrome enterprise policy as a JSON object")
	browserPoolsUpdateCmd.Flags().String("chrome-policy-file", "", "Read Chrome enterprise policy (JSON object) from a file (use '-' for stdin)")
	browserPoolsUpdateCmd.Flags().Bool("clear-chrome-policy", false, "Remove the pool's custom Chrome enterprise policy")
	browserPoolsUpdateCmd.MarkFlagsMutuallyExclusive("chrome-policy", "chrome-policy-file")
	browserPoolsUpdateCmd.MarkFlagsMutuallyExclusive("private-host", "clear-private-hosts")
	browserPoolsUpdateCmd.Flags().String("telemetry", "", "Update pool telemetry: --telemetry=all (reset to default set), --telemetry=off (disable), or --telemetry=console,network (merge those categories into the current selection). Applies only to browsers warmed after the update.")
	browserPoolsUpdateCmd.Flags().String("telemetry-cdp-exclude", "", "Leave the named CDP methods out of control telemetry's cdp_command events, comma-separated (e.g. Input.dispatchMouseEvent,Page.captureScreenshot); --telemetry-cdp-exclude=none clears the list. Excluded commands are still relayed to the browser, they just produce no event")
	browserPoolsUpdateCmd.Flags().Bool("discard-all-idle", false, "Discard all idle browsers")
	addJSONOutputFlag(browserPoolsUpdateCmd)

	browserPoolsDeleteCmd.Flags().Bool("force", false, "Force delete even if browsers are leased")

	browserPoolsAcquireCmd.Flags().Int64("timeout", 0, "Acquire timeout in seconds")
	browserPoolsAcquireCmd.Flags().String("name", "", "Optional name for the acquired session (applies to this lease; cleared on release)")
	browserPoolsAcquireCmd.Flags().String("start-url", "", "URL to navigate the acquired browser to, overriding the pool's start URL for this acquire only (best-effort)")
	browserPoolsAcquireCmd.Flags().StringArray("tag", nil, "Set a tag KEY=VALUE on the acquired session (repeatable; applies to this lease)")
	browserPoolsAcquireCmd.Flags().String("telemetry", "", "Telemetry override for this lease only, merged onto the pool's config: --telemetry=all, --telemetry=off, or --telemetry=console,network")
	browserPoolsAcquireCmd.Flags().String("telemetry-cdp-exclude", "", "Leave the named CDP methods out of control telemetry's cdp_command events, comma-separated (e.g. Input.dispatchMouseEvent,Page.captureScreenshot); --telemetry-cdp-exclude=none clears the list. Excluded commands are still relayed to the browser, they just produce no event")
	addJSONOutputFlag(browserPoolsAcquireCmd)

	browserPoolsReleaseCmd.Flags().String("session-id", "", "Browser session ID to release")
	_ = browserPoolsReleaseCmd.MarkFlagRequired("session-id")
	browserPoolsReleaseCmd.Flags().Bool("reuse", true, "Reuse the browser instance")

	browserPoolsCmd.AddCommand(browserPoolsListCmd)
	browserPoolsCmd.AddCommand(browserPoolsCreateCmd)
	browserPoolsCmd.AddCommand(browserPoolsGetCmd)
	browserPoolsCmd.AddCommand(browserPoolsUpdateCmd)
	browserPoolsCmd.AddCommand(browserPoolsDeleteCmd)
	browserPoolsCmd.AddCommand(browserPoolsAcquireCmd)
	browserPoolsCmd.AddCommand(browserPoolsReleaseCmd)
	browserPoolsCmd.AddCommand(browserPoolsFlushCmd)
}

func runBrowserPoolsList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	out, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")
	query, _ := cmd.Flags().GetString("query")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	region, _ := cmd.Flags().GetString("region")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.List(cmd.Context(), BrowserPoolsListInput{Name: name, Query: query, Limit: limit, Offset: offset, Region: region, Output: out})
}

func runBrowserPoolsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	name, _ := cmd.Flags().GetString("name")
	if len(args) > 0 && args[0] != "" {
		if cmd.Flags().Changed("name") {
			return fmt.Errorf("cannot specify pool name as both a positional argument and --name flag")
		}
		name = args[0]
	}
	size, _ := cmd.Flags().GetInt64("size")
	fillRate, _ := cmd.Flags().GetInt64("fill-rate")
	timeout, _ := cmd.Flags().GetInt64("timeout")
	stealth, _ := cmd.Flags().GetBool("stealth")
	headless, _ := cmd.Flags().GetBool("headless")
	kiosk, _ := cmd.Flags().GetBool("kiosk")
	refreshOnProfileUpdate, _ := cmd.Flags().GetBool("refresh-on-profile-update")
	profileID, _ := cmd.Flags().GetString("profile-id")
	profileName, _ := cmd.Flags().GetString("profile-name")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	region, _ := cmd.Flags().GetString("region")
	privateHosts, _ := cmd.Flags().GetStringSlice("private-host")
	startURL, _ := cmd.Flags().GetString("start-url")
	extensions, _ := cmd.Flags().GetStringSlice("extension")
	viewport, _ := cmd.Flags().GetString("viewport")
	chromePolicy, _ := cmd.Flags().GetString("chrome-policy")
	chromePolicyFile, _ := cmd.Flags().GetString("chrome-policy-file")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	telemetryCdpExclude, _ := cmd.Flags().GetString("telemetry-cdp-exclude")
	output, _ := cmd.Flags().GetString("output")

	in := BrowserPoolsCreateInput{
		Name:                   name,
		Size:                   size,
		FillRate:               fillRate,
		TimeoutSeconds:         timeout,
		Stealth:                BoolFlag{Set: cmd.Flags().Changed("stealth"), Value: stealth},
		Headless:               BoolFlag{Set: cmd.Flags().Changed("headless"), Value: headless},
		Kiosk:                  BoolFlag{Set: cmd.Flags().Changed("kiosk"), Value: kiosk},
		RefreshOnProfileUpdate: BoolFlag{Set: cmd.Flags().Changed("refresh-on-profile-update"), Value: refreshOnProfileUpdate},
		ProfileID:              profileID,
		ProfileName:            profileName,
		ProxyID:                proxyID,
		Region:                 region,
		PrivateHosts:           privateHosts,
		StartURL:               startURL,
		Extensions:             extensions,
		Viewport:               viewport,
		ChromePolicy:           chromePolicy,
		ChromePolicyFile:       chromePolicyFile,
		Telemetry:              telemetry,
		TelemetryCdpExclude:    telemetryCdpExclude,
		Output:                 output,
	}

	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Create(cmd.Context(), in)
}

func runBrowserPoolsGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	out, _ := cmd.Flags().GetString("output")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Get(cmd.Context(), BrowserPoolsGetInput{IDOrName: args[0], Output: out})
}

func runBrowserPoolsUpdate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	name, _ := cmd.Flags().GetString("name")
	size, _ := cmd.Flags().GetInt64("size")
	fillRate, _ := cmd.Flags().GetInt64("fill-rate")
	timeout, _ := cmd.Flags().GetInt64("timeout")
	stealth, _ := cmd.Flags().GetBool("stealth")
	headless, _ := cmd.Flags().GetBool("headless")
	kiosk, _ := cmd.Flags().GetBool("kiosk")
	refreshOnProfileUpdate, _ := cmd.Flags().GetBool("refresh-on-profile-update")
	profileID, _ := cmd.Flags().GetString("profile-id")
	profileName, _ := cmd.Flags().GetString("profile-name")
	clearProfile, _ := cmd.Flags().GetBool("clear-profile")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	clearProxy, _ := cmd.Flags().GetBool("clear-proxy")
	startURL, _ := cmd.Flags().GetString("start-url")
	clearStartURL, _ := cmd.Flags().GetBool("clear-start-url")
	extensions, _ := cmd.Flags().GetStringSlice("extension")
	clearExtensions, _ := cmd.Flags().GetBool("clear-extensions")
	privateHosts, _ := cmd.Flags().GetStringSlice("private-host")
	clearPrivateHosts, _ := cmd.Flags().GetBool("clear-private-hosts")
	viewport, _ := cmd.Flags().GetString("viewport")
	chromePolicy, _ := cmd.Flags().GetString("chrome-policy")
	chromePolicyFile, _ := cmd.Flags().GetString("chrome-policy-file")
	clearChromePolicy, _ := cmd.Flags().GetBool("clear-chrome-policy")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	telemetryCdpExclude, _ := cmd.Flags().GetString("telemetry-cdp-exclude")
	discardIdle, _ := cmd.Flags().GetBool("discard-all-idle")
	output, _ := cmd.Flags().GetString("output")

	in := BrowserPoolsUpdateInput{
		IDOrName:               args[0],
		Name:                   name,
		Size:                   size,
		FillRate:               Int64Flag{Set: cmd.Flags().Changed("fill-rate"), Value: fillRate},
		TimeoutSeconds:         timeout,
		Stealth:                BoolFlag{Set: cmd.Flags().Changed("stealth"), Value: stealth},
		Headless:               BoolFlag{Set: cmd.Flags().Changed("headless"), Value: headless},
		Kiosk:                  BoolFlag{Set: cmd.Flags().Changed("kiosk"), Value: kiosk},
		RefreshOnProfileUpdate: BoolFlag{Set: cmd.Flags().Changed("refresh-on-profile-update"), Value: refreshOnProfileUpdate},
		ProfileID:              profileID,
		ProfileName:            profileName,
		ClearProfile:           clearProfile,
		ProxyID:                proxyID,
		ClearProxy:             clearProxy,
		StartURL:               startURL,
		ClearStartURL:          clearStartURL,
		Extensions:             extensions,
		ClearExtensions:        clearExtensions,
		PrivateHosts:           privateHosts,
		ClearPrivateHosts:      clearPrivateHosts,
		Viewport:               viewport,
		ChromePolicy:           chromePolicy,
		ChromePolicyFile:       chromePolicyFile,
		ClearChromePolicy:      clearChromePolicy,
		Telemetry:              telemetry,
		TelemetryCdpExclude:    telemetryCdpExclude,
		DiscardAllIdle:         BoolFlag{Set: cmd.Flags().Changed("discard-all-idle"), Value: discardIdle},
		Output:                 output,
	}

	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Update(cmd.Context(), in)
}

func runBrowserPoolsDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	force, _ := cmd.Flags().GetBool("force")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Delete(cmd.Context(), BrowserPoolsDeleteInput{IDOrName: args[0], Force: force})
}

func runBrowserPoolsAcquire(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	timeout, _ := cmd.Flags().GetInt64("timeout")
	name, _ := cmd.Flags().GetString("name")
	startURL, _ := cmd.Flags().GetString("start-url")
	tags, _ := tagsFromFlag(cmd, "tag")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	telemetryCdpExclude, _ := cmd.Flags().GetString("telemetry-cdp-exclude")
	output, _ := cmd.Flags().GetString("output")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Acquire(cmd.Context(), BrowserPoolsAcquireInput{
		IDOrName:            args[0],
		TimeoutSeconds:      timeout,
		Name:                name,
		StartURL:            startURL,
		Tags:                tags,
		Telemetry:           telemetry,
		TelemetryCdpExclude: telemetryCdpExclude,
		Output:              output,
	})
}

func runBrowserPoolsRelease(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	sessionID, _ := cmd.Flags().GetString("session-id")
	reuse, _ := cmd.Flags().GetBool("reuse")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Release(cmd.Context(), BrowserPoolsReleaseInput{
		IDOrName:  args[0],
		SessionID: sessionID,
		Reuse:     BoolFlag{Set: cmd.Flags().Changed("reuse"), Value: reuse},
	})
}

func runBrowserPoolsFlush(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Flush(cmd.Context(), BrowserPoolsFlushInput{IDOrName: args[0]})
}

// resolvePoolProfile validates and resolves a pool profile selection. Browser
// pools have their own profile type with no save_changes; this helper works for
// both create and update param types by returning the resolved id/name plus
// whether a profile was selected at all.
func resolvePoolProfile(profileID, profileName string) (id, name string, set bool, err error) {
	if profileID != "" && profileName != "" {
		return "", "", false, fmt.Errorf("must specify at most one of --profile-id or --profile-name")
	}
	if profileID == "" && profileName == "" {
		return "", "", false, nil
	}
	return profileID, profileName, true, nil
}

func validateStartURLFlag(startURL string) error {
	if strings.HasPrefix(startURL, "-") {
		return fmt.Errorf("--start-url requires a URL value")
	}
	return nil
}

func buildExtensionsParam(extensions []string) []kernel.BrowserExtensionParam {
	if len(extensions) == 0 {
		return nil
	}

	var result []kernel.BrowserExtensionParam
	for _, ext := range extensions {
		val := strings.TrimSpace(ext)
		if val == "" {
			continue
		}
		item := kernel.BrowserExtensionParam{}
		if cuidRegex.MatchString(val) {
			item.ID = kernel.String(val)
		} else {
			item.Name = kernel.String(val)
		}
		result = append(result, item)
	}
	return result
}

func buildViewportParam(viewport string) (*kernel.BrowserViewportParam, error) {
	if viewport == "" {
		return nil, nil
	}

	width, height, refreshRate, err := parseViewport(viewport)
	if err != nil {
		return nil, fmt.Errorf("invalid viewport format: %v", err)
	}

	vp := kernel.BrowserViewportParam{
		Width:  width,
		Height: height,
	}
	if refreshRate > 0 {
		vp.RefreshRate = kernel.Int(refreshRate)
	}
	return &vp, nil
}

func formatFillRate(rate int64) string {
	if rate > 0 {
		return fmt.Sprintf("%d%%", rate)
	}
	return "-"
}

func formatProfile(profile kernel.BrowserPoolBrowserPoolConfigProfile) string {
	return util.FirstOrDash(profile.Name, profile.ID)
}

func formatExtensions(extensions []kernel.BrowserExtension) string {
	var names []string
	for _, ext := range extensions {
		if name := util.FirstOrDash(ext.Name, ext.ID); name != "-" {
			names = append(names, name)
		}
	}
	return util.JoinOrDash(names...)
}

func formatChromePolicy(policy map[string]any) string {
	if len(policy) == 0 {
		return "-"
	}

	data, err := json.Marshal(policy)
	if err != nil {
		return fmt.Sprintf("%v", policy)
	}

	return string(data)
}

func formatViewport(viewport kernel.BrowserViewport) string {
	if viewport.Width == 0 || viewport.Height == 0 {
		return "-"
	}
	s := fmt.Sprintf("%dx%d", viewport.Width, viewport.Height)
	if viewport.RefreshRate > 0 {
		s += fmt.Sprintf("@%d", viewport.RefreshRate)
	}
	return s
}
