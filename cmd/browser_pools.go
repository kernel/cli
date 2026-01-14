package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// BrowserPoolsService defines the subset of the Kernel SDK browser pools client that we use.
type BrowserPoolsService interface {
	List(ctx context.Context, opts ...option.RequestOption) (res *[]kernel.BrowserPool, err error)
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
	Output string
}

func (c BrowserPoolsCmd) List(ctx context.Context, in BrowserPoolsListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	pools, err := c.client.List(ctx)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if pools == nil || len(*pools) == 0 {
			fmt.Println("[]")
			return nil
		}
		return util.PrintPrettyJSONSlice(*pools)
	}

	if pools == nil || len(*pools) == 0 {
		pterm.Info.Println("No browser pools found")
		return nil
	}

	tableData := pterm.TableData{
		{"ID", "Name", "Available", "Acquired", "Created At", "Size"},
	}

	for _, p := range *pools {
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

type BrowserPoolsCreateInput struct {
	Name               string
	Size               int64
	FillRate           int64
	TimeoutSeconds     int64
	Stealth            BoolFlag
	Headless           BoolFlag
	Kiosk              BoolFlag
	ProfileID          string
	ProfileName        string
	ProfileSaveChanges BoolFlag
	ProxyID            string
	Extensions         []string
	Viewport           string
	Output             string
}

func (c BrowserPoolsCmd) Create(ctx context.Context, in BrowserPoolsCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
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

	profile, err := buildProfileParam(in.ProfileID, in.ProfileName, in.ProfileSaveChanges)
	if err != nil {
		pterm.Error.Println(err.Error())
		return nil
	}
	if profile != nil {
		params.Profile = *profile
	}

	if in.ProxyID != "" {
		params.ProxyID = kernel.String(in.ProxyID)
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
	return nil
}

type BrowserPoolsGetInput struct {
	IDOrName string
	Output   string
}

func (c BrowserPoolsCmd) Get(ctx context.Context, in BrowserPoolsGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
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
		{"Profile", formatProfile(cfg.Profile)},
		{"Proxy ID", util.OrDash(cfg.ProxyID)},
		{"Extensions", formatExtensions(cfg.Extensions)},
		{"Viewport", formatViewport(cfg.Viewport)},
	}

	PrintTableNoPad(rows, true)
	return nil
}

type BrowserPoolsUpdateInput struct {
	IDOrName           string
	Name               string
	Size               int64
	FillRate           int64
	TimeoutSeconds     int64
	Stealth            BoolFlag
	Headless           BoolFlag
	Kiosk              BoolFlag
	ProfileID          string
	ProfileName        string
	ProfileSaveChanges BoolFlag
	ProxyID            string
	Extensions         []string
	Viewport           string
	DiscardAllIdle     BoolFlag
	Output             string
}

func (c BrowserPoolsCmd) Update(ctx context.Context, in BrowserPoolsUpdateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.BrowserPoolUpdateParams{}

	if in.Name != "" {
		params.Name = kernel.String(in.Name)
	}
	if in.Size > 0 {
		params.Size = in.Size
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

	profile, err := buildProfileParam(in.ProfileID, in.ProfileName, in.ProfileSaveChanges)
	if err != nil {
		pterm.Error.Println(err.Error())
		return nil
	}
	if profile != nil {
		params.Profile = *profile
	}

	if in.ProxyID != "" {
		params.ProxyID = kernel.String(in.ProxyID)
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
	Output         string
}

func (c BrowserPoolsCmd) Acquire(ctx context.Context, in BrowserPoolsAcquireInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.BrowserPoolAcquireParams{}
	if in.TimeoutSeconds > 0 {
		params.AcquireTimeoutSeconds = kernel.Int(in.TimeoutSeconds)
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
		{"CDP WebSocket URL", resp.CdpWsURL},
		{"Live View URL", resp.BrowserLiveViewURL},
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
	pterm.Success.Printf("Released browser %s back to pool %s\n", in.SessionID, in.IDOrName)
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
	Use:   "create",
	Short: "Create a new browser pool",
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
	browserPoolsListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	browserPoolsCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	browserPoolsCreateCmd.Flags().String("name", "", "Optional unique name for the pool")
	browserPoolsCreateCmd.Flags().Int64("size", 0, "Number of browsers in the pool")
	_ = browserPoolsCreateCmd.MarkFlagRequired("size")
	browserPoolsCreateCmd.Flags().Int64("fill-rate", 0, "Fill rate per minute")
	browserPoolsCreateCmd.Flags().Int64("timeout", 0, "Idle timeout in seconds")
	browserPoolsCreateCmd.Flags().Bool("stealth", false, "Enable stealth mode")
	browserPoolsCreateCmd.Flags().Bool("headless", false, "Enable headless mode")
	browserPoolsCreateCmd.Flags().Bool("kiosk", false, "Enable kiosk mode")
	browserPoolsCreateCmd.Flags().String("profile-id", "", "Profile ID")
	browserPoolsCreateCmd.Flags().String("profile-name", "", "Profile name")
	browserPoolsCreateCmd.Flags().Bool("save-changes", false, "Save changes to profile")
	browserPoolsCreateCmd.Flags().String("proxy-id", "", "Proxy ID")
	browserPoolsCreateCmd.Flags().StringSlice("extension", []string{}, "Extension IDs or names")
	browserPoolsCreateCmd.Flags().String("viewport", "", "Viewport size (e.g. 1280x800)")

	browserPoolsGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	browserPoolsUpdateCmd.Flags().String("name", "", "Update the pool name")
	browserPoolsUpdateCmd.Flags().Int64("size", 0, "Number of browsers in the pool")
	browserPoolsUpdateCmd.Flags().Int64("fill-rate", 0, "Fill rate per minute")
	browserPoolsUpdateCmd.Flags().Int64("timeout", 0, "Idle timeout in seconds")
	browserPoolsUpdateCmd.Flags().Bool("stealth", false, "Enable stealth mode")
	browserPoolsUpdateCmd.Flags().Bool("headless", false, "Enable headless mode")
	browserPoolsUpdateCmd.Flags().Bool("kiosk", false, "Enable kiosk mode")
	browserPoolsUpdateCmd.Flags().String("profile-id", "", "Profile ID")
	browserPoolsUpdateCmd.Flags().String("profile-name", "", "Profile name")
	browserPoolsUpdateCmd.Flags().Bool("save-changes", false, "Save changes to profile")
	browserPoolsUpdateCmd.Flags().String("proxy-id", "", "Proxy ID")
	browserPoolsUpdateCmd.Flags().StringSlice("extension", []string{}, "Extension IDs or names")
	browserPoolsUpdateCmd.Flags().String("viewport", "", "Viewport size (e.g. 1280x800)")
	browserPoolsUpdateCmd.Flags().Bool("discard-all-idle", false, "Discard all idle browsers")
	browserPoolsUpdateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	browserPoolsDeleteCmd.Flags().Bool("force", false, "Force delete even if browsers are leased")

	browserPoolsAcquireCmd.Flags().Int64("timeout", 0, "Acquire timeout in seconds")
	browserPoolsAcquireCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

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
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.List(cmd.Context(), BrowserPoolsListInput{Output: out})
}

func runBrowserPoolsCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	name, _ := cmd.Flags().GetString("name")
	size, _ := cmd.Flags().GetInt64("size")
	fillRate, _ := cmd.Flags().GetInt64("fill-rate")
	timeout, _ := cmd.Flags().GetInt64("timeout")
	stealth, _ := cmd.Flags().GetBool("stealth")
	headless, _ := cmd.Flags().GetBool("headless")
	kiosk, _ := cmd.Flags().GetBool("kiosk")
	profileID, _ := cmd.Flags().GetString("profile-id")
	profileName, _ := cmd.Flags().GetString("profile-name")
	saveChanges, _ := cmd.Flags().GetBool("save-changes")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	extensions, _ := cmd.Flags().GetStringSlice("extension")
	viewport, _ := cmd.Flags().GetString("viewport")
	output, _ := cmd.Flags().GetString("output")

	in := BrowserPoolsCreateInput{
		Name:               name,
		Size:               size,
		FillRate:           fillRate,
		TimeoutSeconds:     timeout,
		Stealth:            BoolFlag{Set: cmd.Flags().Changed("stealth"), Value: stealth},
		Headless:           BoolFlag{Set: cmd.Flags().Changed("headless"), Value: headless},
		Kiosk:              BoolFlag{Set: cmd.Flags().Changed("kiosk"), Value: kiosk},
		ProfileID:          profileID,
		ProfileName:        profileName,
		ProfileSaveChanges: BoolFlag{Set: cmd.Flags().Changed("save-changes"), Value: saveChanges},
		ProxyID:            proxyID,
		Extensions:         extensions,
		Viewport:           viewport,
		Output:             output,
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
	profileID, _ := cmd.Flags().GetString("profile-id")
	profileName, _ := cmd.Flags().GetString("profile-name")
	saveChanges, _ := cmd.Flags().GetBool("save-changes")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	extensions, _ := cmd.Flags().GetStringSlice("extension")
	viewport, _ := cmd.Flags().GetString("viewport")
	discardIdle, _ := cmd.Flags().GetBool("discard-all-idle")
	output, _ := cmd.Flags().GetString("output")

	in := BrowserPoolsUpdateInput{
		IDOrName:           args[0],
		Name:               name,
		Size:               size,
		FillRate:           fillRate,
		TimeoutSeconds:     timeout,
		Stealth:            BoolFlag{Set: cmd.Flags().Changed("stealth"), Value: stealth},
		Headless:           BoolFlag{Set: cmd.Flags().Changed("headless"), Value: headless},
		Kiosk:              BoolFlag{Set: cmd.Flags().Changed("kiosk"), Value: kiosk},
		ProfileID:          profileID,
		ProfileName:        profileName,
		ProfileSaveChanges: BoolFlag{Set: cmd.Flags().Changed("save-changes"), Value: saveChanges},
		ProxyID:            proxyID,
		Extensions:         extensions,
		Viewport:           viewport,
		DiscardAllIdle:     BoolFlag{Set: cmd.Flags().Changed("discard-all-idle"), Value: discardIdle},
		Output:             output,
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
	output, _ := cmd.Flags().GetString("output")
	c := BrowserPoolsCmd{client: &client.BrowserPools}
	return c.Acquire(cmd.Context(), BrowserPoolsAcquireInput{IDOrName: args[0], TimeoutSeconds: timeout, Output: output})
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

func buildProfileParam(profileID, profileName string, saveChanges BoolFlag) (*kernel.BrowserProfileParam, error) {
	if profileID != "" && profileName != "" {
		return nil, fmt.Errorf("must specify at most one of --profile-id or --profile-name")
	}
	if profileID == "" && profileName == "" {
		return nil, nil
	}

	profile := kernel.BrowserProfileParam{
		SaveChanges: kernel.Bool(saveChanges.Value),
	}
	if profileID != "" {
		profile.ID = kernel.String(profileID)
	} else if profileName != "" {
		profile.Name = kernel.String(profileName)
	}
	return &profile, nil
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

func formatProfile(profile kernel.BrowserProfile) string {
	name := util.FirstOrDash(profile.Name, profile.ID)
	if name == "-" {
		return "-"
	}
	if profile.SaveChanges {
		return fmt.Sprintf("%s (save changes: true)", name)
	}
	return fmt.Sprintf("%s (save changes: false)", name)
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
