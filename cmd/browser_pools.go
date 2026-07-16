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
	Limit  int
	Offset int
	Output string
}

func (c BrowserPoolsCmd) List(ctx context.Context, in BrowserPoolsListInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}

	params := kernel.BrowserPoolListParams{}
	if in.Limit > 0 {
		params.Limit = kernel.Int(int64(in.Limit))
	}
	if in.Offset > 0 {
		params.Offset = kernel.Int(int64(in.Offset))
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
		{"ID", "Name", "Available", "Acquired", "Created At", "Size"},
	}

	for _, p := range pools {
		tableData = append(tableData, []string{
			p.ID,
			util.OrDash(p.Name),
			fmt.Sprintf("%d", p.AvailableCount),
			fmt.Sprintf("%d", p.AcquiredCount),
			util.FormatLocal(p.CreatedAt),
			fmt.Sprintf("%d", p.BrowserPoolConfig.Size),
		})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

// buildPoolNewTelemetryParam converts a --telemetry flag value to the pool create param.
func buildPoolNewTelemetryParam(s string) (kernel.BrowserPoolNewParamsTelemetry, error) {
	enabled, browser, err := resolveTelemetryFlag(s)
	return kernel.BrowserPoolNewParamsTelemetry{Enabled: enabled, Browser: browser}, err
}

// buildPoolUpdateTelemetryParam converts a --telemetry flag value to the pool update param.
func buildPoolUpdateTelemetryParam(s string) (kernel.BrowserPoolUpdateParamsTelemetry, error) {
	enabled, browser, err := resolveTelemetryFlag(s)
	return kernel.BrowserPoolUpdateParamsTelemetry{Enabled: enabled, Browser: browser}, err
}

// buildPoolAcquireTelemetryParam converts a --telemetry flag value to the acquire override param.
func buildPoolAcquireTelemetryParam(s string) (kernel.BrowserPoolAcquireParamsTelemetry, error) {
	enabled, browser, err := resolveTelemetryFlag(s)
	return kernel.BrowserPoolAcquireParamsTelemetry{Enabled: enabled, Browser: browser}, err
}

// formatPoolTelemetry renders a pool's active telemetry config for the details table.
func formatPoolTelemetry(cfg kernel.BrowserTelemetryConfig) string {
	on := telemetryEnabledCategories(cfg)
	if len(on) == 0 {
		return "disabled"
	}
	return strings.Join(on, ", ")
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
	ProfileName      string
	ProxyID          string
	StartURL         string
	Extensions       []string
	Viewport         string
	ChromePolicy     string
	ChromePolicyFile string
	Telemetry        string
	Output           string
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

	if in.Telemetry != "" {
		t, err := buildPoolNewTelemetryParam(in.Telemetry)
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
	if in.Telemetry != "" {
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
		{"Telemetry", formatPoolTelemetry(cfg.Telemetry)},
	}

	PrintTableNoPad(rows, true)
	return nil
}

type BrowserPoolsUpdateInput struct {
	IDOrName               string
	Name                   string
	Size                   int64
	FillRate               int64
	TimeoutSeconds         int64
	Stealth                BoolFlag
	Headless               BoolFlag
	Kiosk                  BoolFlag
	RefreshOnProfileUpdate BoolFlag
	ProfileID              string
	ProfileName      string
	ProxyID          string
	StartURL         string
	ClearStartURL    bool
	Extensions       []string
	Viewport         string
	ChromePolicy     string
	ChromePolicyFile string
	Telemetry        string
	DiscardAllIdle   BoolFlag
	Output           string
}

func (c BrowserPoolsCmd) Update(ctx context.Context, in BrowserPoolsUpdateInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if err := validateStartURLFlag(in.StartURL); err != nil {
		return err
	}
	if in.StartURL != "" && in.ClearStartURL {
		return fmt.Errorf("cannot specify both --start-url and --clear-start-url")
	}

	params := kernel.BrowserPoolUpdateParams{}

	if in.Name != "" {
		params.Name = kernel.String(in.Name)
	}
	if in.Size > 0 {
		params.Size = kernel.Int(in.Size)
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
	} else if (in.ChromePolicy != "" || in.ChromePolicyFile != "") && in.Output != "json" {
		// An empty policy ({}) cannot clear an existing one: omitzero drops it before it
		// reaches the server. Warn instead of silently doing nothing, but stay quiet on the
		// json path so stdout remains valid JSON.
		pterm.Warning.Println("An empty chrome policy is ignored and does not clear the pool's existing policy; recreate the pool to remove a policy.")
	}

	if in.Telemetry != "" {
		t, err := buildPoolUpdateTelemetryParam(in.Telemetry)
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
	if in.Telemetry != "" {
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
	IDOrName       string
	TimeoutSeconds int64
	Name           string
	Tags           map[string]string
	Telemetry      string
	Output         string
}

// buildAcquireParams builds the SDK params for acquiring a browser from a pool.
// Shared by `browser-pools acquire` and the `browsers create --pool-id/--pool-name`
// path so the per-lease name/tags/telemetry forwarding cannot silently diverge
// between them. The telemetry override merges onto the pool's config for this lease.
func buildAcquireParams(name string, tags map[string]string, timeoutSeconds int64, telemetry string) (kernel.BrowserPoolAcquireParams, error) {
	params := kernel.BrowserPoolAcquireParams{}
	if timeoutSeconds > 0 {
		params.AcquireTimeoutSeconds = kernel.Int(timeoutSeconds)
	}
	if name != "" {
		params.Name = kernel.Opt(name)
	}
	if len(tags) > 0 {
		params.Tags = kernel.Tags(tags)
	}
	if telemetry != "" {
		t, err := buildPoolAcquireTelemetryParam(telemetry)
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

	params, err := buildAcquireParams(in.Name, in.Tags, in.TimeoutSeconds, in.Telemetry)
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
	browserPoolsListCmd.Flags().Int("limit", 0, "Maximum number of pools to return")
	browserPoolsListCmd.Flags().Int("offset", 0, "Number of pools to skip (for pagination)")

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
	browserPoolsCreateCmd.Flags().String("start-url", "", "Initial page to open for new browsers")
	browserPoolsCreateCmd.Flags().StringSlice("extension", []string{}, "Extension IDs or names")
	browserPoolsCreateCmd.Flags().String("viewport", "", "Viewport size (e.g. 1280x800)")
	browserPoolsCreateCmd.Flags().String("chrome-policy", "", "Custom Chrome enterprise policy as a JSON object")
	browserPoolsCreateCmd.Flags().String("chrome-policy-file", "", "Read Chrome enterprise policy (JSON object) from a file (use '-' for stdin)")
	browserPoolsCreateCmd.Flags().String("telemetry", "", "Configure telemetry for browsers warmed into the pool (opt-in): --telemetry=all (default set), --telemetry=off (disable), or --telemetry=console,network (capture exactly those categories)")
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
	browserPoolsUpdateCmd.Flags().String("proxy-id", "", "Proxy ID")
	browserPoolsUpdateCmd.Flags().String("start-url", "", "Initial page to open for new browsers")
	browserPoolsUpdateCmd.Flags().Bool("clear-start-url", false, "Clear the pool start URL")
	browserPoolsUpdateCmd.Flags().StringSlice("extension", []string{}, "Extension IDs or names")
	browserPoolsUpdateCmd.Flags().String("viewport", "", "Viewport size (e.g. 1280x800)")
	browserPoolsUpdateCmd.Flags().String("chrome-policy", "", "Custom Chrome enterprise policy as a JSON object")
	browserPoolsUpdateCmd.Flags().String("chrome-policy-file", "", "Read Chrome enterprise policy (JSON object) from a file (use '-' for stdin)")
	browserPoolsUpdateCmd.MarkFlagsMutuallyExclusive("chrome-policy", "chrome-policy-file")
	browserPoolsUpdateCmd.Flags().String("telemetry", "", "Update pool telemetry: --telemetry=all (reset to default set), --telemetry=off (disable), or --telemetry=console,network (merge those categories into the current selection). Applies only to browsers warmed after the update.")
	browserPoolsUpdateCmd.Flags().Bool("discard-all-idle", false, "Discard all idle browsers")
	addJSONOutputFlag(browserPoolsUpdateCmd)

	browserPoolsDeleteCmd.Flags().Bool("force", false, "Force delete even if browsers are leased")

	browserPoolsAcquireCmd.Flags().Int64("timeout", 0, "Acquire timeout in seconds")
	browserPoolsAcquireCmd.Flags().String("name", "", "Optional name for the acquired session (applies to this lease; cleared on release)")
	browserPoolsAcquireCmd.Flags().StringArray("tag", nil, "Set a tag KEY=VALUE on the acquired session (repeatable; applies to this lease)")
	browserPoolsAcquireCmd.Flags().String("telemetry", "", "Telemetry override for this lease only, merged onto the pool's config: --telemetry=all, --telemetry=off, or --telemetry=console,network")
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
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.List(cmd.Context(), BrowserPoolsListInput{Limit: limit, Offset: offset, Output: out})
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
	startURL, _ := cmd.Flags().GetString("start-url")
	extensions, _ := cmd.Flags().GetStringSlice("extension")
	viewport, _ := cmd.Flags().GetString("viewport")
	chromePolicy, _ := cmd.Flags().GetString("chrome-policy")
	chromePolicyFile, _ := cmd.Flags().GetString("chrome-policy-file")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	output, _ := cmd.Flags().GetString("output")

	in := BrowserPoolsCreateInput{
		Name:             name,
		Size:             size,
		FillRate:         fillRate,
		TimeoutSeconds:   timeout,
		Stealth:          BoolFlag{Set: cmd.Flags().Changed("stealth"), Value: stealth},
		Headless:         BoolFlag{Set: cmd.Flags().Changed("headless"), Value: headless},
		Kiosk:                  BoolFlag{Set: cmd.Flags().Changed("kiosk"), Value: kiosk},
		RefreshOnProfileUpdate: BoolFlag{Set: cmd.Flags().Changed("refresh-on-profile-update"), Value: refreshOnProfileUpdate},
		ProfileID:              profileID,
		ProfileName:      profileName,
		ProxyID:          proxyID,
		StartURL:         startURL,
		Extensions:       extensions,
		Viewport:         viewport,
		ChromePolicy:     chromePolicy,
		ChromePolicyFile: chromePolicyFile,
		Telemetry:        telemetry,
		Output:           output,
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
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	startURL, _ := cmd.Flags().GetString("start-url")
	clearStartURL, _ := cmd.Flags().GetBool("clear-start-url")
	extensions, _ := cmd.Flags().GetStringSlice("extension")
	viewport, _ := cmd.Flags().GetString("viewport")
	chromePolicy, _ := cmd.Flags().GetString("chrome-policy")
	chromePolicyFile, _ := cmd.Flags().GetString("chrome-policy-file")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	discardIdle, _ := cmd.Flags().GetBool("discard-all-idle")
	output, _ := cmd.Flags().GetString("output")

	in := BrowserPoolsUpdateInput{
		IDOrName:         args[0],
		Name:             name,
		Size:             size,
		FillRate:         fillRate,
		TimeoutSeconds:   timeout,
		Stealth:          BoolFlag{Set: cmd.Flags().Changed("stealth"), Value: stealth},
		Headless:         BoolFlag{Set: cmd.Flags().Changed("headless"), Value: headless},
		Kiosk:                  BoolFlag{Set: cmd.Flags().Changed("kiosk"), Value: kiosk},
		RefreshOnProfileUpdate: BoolFlag{Set: cmd.Flags().Changed("refresh-on-profile-update"), Value: refreshOnProfileUpdate},
		ProfileID:              profileID,
		ProfileName:      profileName,
		ProxyID:          proxyID,
		StartURL:         startURL,
		ClearStartURL:    clearStartURL,
		Extensions:       extensions,
		Viewport:         viewport,
		ChromePolicy:     chromePolicy,
		ChromePolicyFile: chromePolicyFile,
		Telemetry:        telemetry,
		DiscardAllIdle:   BoolFlag{Set: cmd.Flags().Changed("discard-all-idle"), Value: discardIdle},
		Output:           output,
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
	tags, _ := tagsFromFlag(cmd, "tag")
	telemetry, _ := cmd.Flags().GetString("telemetry")
	output, _ := cmd.Flags().GetString("output")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Acquire(cmd.Context(), BrowserPoolsAcquireInput{
		IDOrName:       args[0],
		TimeoutSeconds: timeout,
		Name:           name,
		Tags:           tags,
		Telemetry:      telemetry,
		Output:         output,
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
