package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/kernel/kernel-go-sdk/shared"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// BrowsersService defines the subset of the Kernel SDK browser client that we use.
// See https://github.com/kernel/kernel-go-sdk/blob/main/browser.go
type BrowsersService interface {
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.BrowserGetResponse, err error)
	List(ctx context.Context, query kernel.BrowserListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.BrowserListResponse], err error)
	New(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (res *kernel.BrowserNewResponse, err error)
	Delete(ctx context.Context, body kernel.BrowserDeleteParams, opts ...option.RequestOption) (err error)
	DeleteByID(ctx context.Context, id string, opts ...option.RequestOption) (err error)
	LoadExtensions(ctx context.Context, id string, body kernel.BrowserLoadExtensionsParams, opts ...option.RequestOption) (err error)
}

// BrowserReplaysService defines the subset we use for browser replays.
type BrowserReplaysService interface {
	List(ctx context.Context, id string, opts ...option.RequestOption) (res *[]kernel.BrowserReplayListResponse, err error)
	Download(ctx context.Context, replayID string, query kernel.BrowserReplayDownloadParams, opts ...option.RequestOption) (res *http.Response, err error)
	Start(ctx context.Context, id string, body kernel.BrowserReplayStartParams, opts ...option.RequestOption) (res *kernel.BrowserReplayStartResponse, err error)
	Stop(ctx context.Context, replayID string, body kernel.BrowserReplayStopParams, opts ...option.RequestOption) (err error)
}

// BrowserFSService defines the subset we use for browser filesystem APIs.
type BrowserFSService interface {
	NewDirectory(ctx context.Context, id string, body kernel.BrowserFNewDirectoryParams, opts ...option.RequestOption) (err error)
	DeleteDirectory(ctx context.Context, id string, body kernel.BrowserFDeleteDirectoryParams, opts ...option.RequestOption) (err error)
	DeleteFile(ctx context.Context, id string, body kernel.BrowserFDeleteFileParams, opts ...option.RequestOption) (err error)
	DownloadDirZip(ctx context.Context, id string, query kernel.BrowserFDownloadDirZipParams, opts ...option.RequestOption) (res *http.Response, err error)
	FileInfo(ctx context.Context, id string, query kernel.BrowserFFileInfoParams, opts ...option.RequestOption) (res *kernel.BrowserFFileInfoResponse, err error)
	ListFiles(ctx context.Context, id string, query kernel.BrowserFListFilesParams, opts ...option.RequestOption) (res *[]kernel.BrowserFListFilesResponse, err error)
	Move(ctx context.Context, id string, body kernel.BrowserFMoveParams, opts ...option.RequestOption) (err error)
	ReadFile(ctx context.Context, id string, query kernel.BrowserFReadFileParams, opts ...option.RequestOption) (res *http.Response, err error)
	SetFilePermissions(ctx context.Context, id string, body kernel.BrowserFSetFilePermissionsParams, opts ...option.RequestOption) (err error)
	Upload(ctx context.Context, id string, body kernel.BrowserFUploadParams, opts ...option.RequestOption) (err error)
	UploadZip(ctx context.Context, id string, body kernel.BrowserFUploadZipParams, opts ...option.RequestOption) (err error)
	WriteFile(ctx context.Context, id string, contents io.Reader, body kernel.BrowserFWriteFileParams, opts ...option.RequestOption) (err error)
}

// BrowserProcessService defines the subset we use for browser process APIs.
type BrowserProcessService interface {
	Exec(ctx context.Context, id string, body kernel.BrowserProcessExecParams, opts ...option.RequestOption) (res *kernel.BrowserProcessExecResponse, err error)
	Kill(ctx context.Context, processID string, params kernel.BrowserProcessKillParams, opts ...option.RequestOption) (res *kernel.BrowserProcessKillResponse, err error)
	Spawn(ctx context.Context, id string, body kernel.BrowserProcessSpawnParams, opts ...option.RequestOption) (res *kernel.BrowserProcessSpawnResponse, err error)
	Status(ctx context.Context, processID string, query kernel.BrowserProcessStatusParams, opts ...option.RequestOption) (res *kernel.BrowserProcessStatusResponse, err error)
	Stdin(ctx context.Context, processID string, params kernel.BrowserProcessStdinParams, opts ...option.RequestOption) (res *kernel.BrowserProcessStdinResponse, err error)
	StdoutStreamStreaming(ctx context.Context, processID string, query kernel.BrowserProcessStdoutStreamParams, opts ...option.RequestOption) (stream *ssestream.Stream[kernel.BrowserProcessStdoutStreamResponse])
}

// BrowserLogService defines the subset we use for browser log APIs.
type BrowserLogService interface {
	StreamStreaming(ctx context.Context, id string, query kernel.BrowserLogStreamParams, opts ...option.RequestOption) (stream *ssestream.Stream[shared.LogEvent])
}

// BrowserPlaywrightService defines the subset we use for Playwright execution.
type BrowserPlaywrightService interface {
	Execute(ctx context.Context, id string, body kernel.BrowserPlaywrightExecuteParams, opts ...option.RequestOption) (res *kernel.BrowserPlaywrightExecuteResponse, err error)
}

// BrowserComputerService defines the subset we use for OS-level mouse & screen.
type BrowserComputerService interface {
	CaptureScreenshot(ctx context.Context, id string, body kernel.BrowserComputerCaptureScreenshotParams, opts ...option.RequestOption) (res *http.Response, err error)
	ClickMouse(ctx context.Context, id string, body kernel.BrowserComputerClickMouseParams, opts ...option.RequestOption) (err error)
	DragMouse(ctx context.Context, id string, body kernel.BrowserComputerDragMouseParams, opts ...option.RequestOption) (err error)
	MoveMouse(ctx context.Context, id string, body kernel.BrowserComputerMoveMouseParams, opts ...option.RequestOption) (err error)
	PressKey(ctx context.Context, id string, body kernel.BrowserComputerPressKeyParams, opts ...option.RequestOption) (err error)
	Scroll(ctx context.Context, id string, body kernel.BrowserComputerScrollParams, opts ...option.RequestOption) (err error)
	SetCursorVisibility(ctx context.Context, id string, body kernel.BrowserComputerSetCursorVisibilityParams, opts ...option.RequestOption) (res *kernel.BrowserComputerSetCursorVisibilityResponse, err error)
	TypeText(ctx context.Context, id string, body kernel.BrowserComputerTypeTextParams, opts ...option.RequestOption) (err error)
}

// BoolFlag captures whether a boolean flag was set explicitly and its value.
type BoolFlag struct {
	Set   bool
	Value bool
}

// Regular expression to validate CUID2 identifiers (24 lowercase alphanumeric characters).
var cuidRegex = regexp.MustCompile(`^[a-z0-9]{24}$`)

// getAvailableViewports returns the list of supported viewport configurations.
func getAvailableViewports() []string {
	return []string{
		"2560x1440@10",
		"1920x1080@25",
		"1920x1200@25",
		"1440x900@25",
		"1024x768@60",
		"1200x800@60",
	}
}

// parseViewport parses a viewport string (e.g., "1920x1080@25") and returns width, height, and refresh rate.
// Returns error if the format is invalid.
func parseViewport(viewport string) (width, height, refreshRate int64, err error) {
	parts := strings.Split(viewport, "@")
	var dimStr string
	if len(parts) == 1 {
		dimStr = parts[0]
		refreshRate = 0
	} else if len(parts) == 2 {
		dimStr = parts[0]
		rr, parseErr := strconv.ParseInt(parts[1], 10, 64)
		if parseErr != nil {
			return 0, 0, 0, fmt.Errorf("invalid refresh rate: %v", parseErr)
		}
		refreshRate = rr
	} else {
		return 0, 0, 0, fmt.Errorf("invalid viewport format")
	}

	dims := strings.Split(dimStr, "x")
	if len(dims) != 2 {
		return 0, 0, 0, fmt.Errorf("invalid viewport format, expected WIDTHxHEIGHT[@RATE]")
	}

	w, err := strconv.ParseInt(dims[0], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid width: %v", err)
	}
	h, err := strconv.ParseInt(dims[1], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid height: %v", err)
	}

	return w, h, refreshRate, nil
}

// Inputs for each command
type BrowsersCreateInput struct {
	PersistenceID      string
	TimeoutSeconds     int
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

type BrowsersDeleteInput struct {
	Identifier  string
	SkipConfirm bool
}

type BrowsersViewInput struct {
	Identifier string
	Output     string
}

type BrowsersGetInput struct {
	Identifier string
	Output     string
}

// BrowsersCmd is a cobra-independent command handler for browsers operations.
type BrowsersCmd struct {
	browsers   BrowsersService
	replays    BrowserReplaysService
	fs         BrowserFSService
	process    BrowserProcessService
	logs       BrowserLogService
	computer   BrowserComputerService
	playwright BrowserPlaywrightService
}

type BrowsersListInput struct {
	Output         string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

func (b BrowsersCmd) List(ctx context.Context, in BrowsersListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	params := kernel.BrowserListParams{}
	if in.IncludeDeleted {
		params.IncludeDeleted = kernel.Opt(true)
	}
	if in.Limit > 0 {
		params.Limit = kernel.Opt(int64(in.Limit))
	}
	if in.Offset > 0 {
		params.Offset = kernel.Opt(int64(in.Offset))
	}

	page, err := b.browsers.List(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	var browsers []kernel.BrowserListResponse
	if page != nil {
		browsers = page.Items
	}

	if in.Output == "json" {
		if len(browsers) == 0 {
			fmt.Println("[]")
			return nil
		}
		bs, err := json.MarshalIndent(browsers, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	if len(browsers) == 0 {
		pterm.Info.Println("No running browsers found")
		return nil
	}

	// Prepare table data
	headers := []string{"Browser ID", "Created At", "Persistent ID", "Profile", "CDP WS URL", "Live View URL"}
	if in.IncludeDeleted {
		headers = append(headers, "Deleted At")
	}
	tableData := pterm.TableData{headers}

	for _, browser := range browsers {
		persistentID := "-"
		if browser.Persistence.ID != "" {
			persistentID = browser.Persistence.ID
		}

		profile := "-"
		if browser.Profile.Name != "" {
			profile = browser.Profile.Name
		} else if browser.Profile.ID != "" {
			profile = browser.Profile.ID
		}

		row := []string{
			browser.SessionID,
			util.FormatLocal(browser.CreatedAt),
			persistentID,
			profile,
			truncateURL(browser.CdpWsURL, 50),
			truncateURL(browser.BrowserLiveViewURL, 50),
		}

		if in.IncludeDeleted {
			deletedAt := "-"
			if !browser.DeletedAt.IsZero() {
				deletedAt = util.FormatLocal(browser.DeletedAt)
			}
			row = append(row, deletedAt)
		}

		tableData = append(tableData, row)
	}

	PrintTableNoPad(tableData, true)
	return nil
}

func (b BrowsersCmd) Create(ctx context.Context, in BrowsersCreateInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if in.Output != "json" {
		pterm.Info.Println("Creating browser session...")
	}
	params := kernel.BrowserNewParams{}
	if in.PersistenceID != "" {
		params.Persistence = kernel.BrowserPersistenceParam{ID: in.PersistenceID}
	}
	if in.TimeoutSeconds > 0 {
		params.TimeoutSeconds = kernel.Opt(int64(in.TimeoutSeconds))
	}
	if in.Stealth.Set {
		params.Stealth = kernel.Opt(in.Stealth.Value)
	}
	if in.Headless.Set {
		params.Headless = kernel.Opt(in.Headless.Value)
	}
	if in.Kiosk.Set {
		params.KioskMode = kernel.Opt(in.Kiosk.Value)
	}

	// Validate profile selection: at most one of profile-id or profile-name must be provided
	if in.ProfileID != "" && in.ProfileName != "" {
		pterm.Error.Println("must specify at most one of --profile-id or --profile-name")
		return nil
	} else if in.ProfileID != "" || in.ProfileName != "" {
		params.Profile = kernel.BrowserProfileParam{
			SaveChanges: kernel.Opt(in.ProfileSaveChanges.Value),
		}
		if in.ProfileID != "" {
			params.Profile.ID = kernel.Opt(in.ProfileID)
		} else if in.ProfileName != "" {
			params.Profile.Name = kernel.Opt(in.ProfileName)
		}
	}

	// Add proxy if specified
	if in.ProxyID != "" {
		params.ProxyID = kernel.Opt(in.ProxyID)
	}

	// Map extensions (IDs or names) into params.Extensions
	if len(in.Extensions) > 0 {
		for _, ext := range in.Extensions {
			val := strings.TrimSpace(ext)
			if val == "" {
				continue
			}
			item := kernel.BrowserExtensionParam{}
			if cuidRegex.MatchString(val) {
				item.ID = kernel.Opt(val)
			} else {
				item.Name = kernel.Opt(val)
			}
			params.Extensions = append(params.Extensions, item)
		}
	}

	// Add viewport if specified
	if in.Viewport != "" {
		width, height, refreshRate, err := parseViewport(in.Viewport)
		if err != nil {
			pterm.Error.Printf("Invalid viewport format: %v\n", err)
			return nil
		}
		params.Viewport = kernel.BrowserViewportParam{
			Width:  width,
			Height: height,
		}
		if refreshRate > 0 {
			params.Viewport.RefreshRate = kernel.Opt(refreshRate)
		}
	}

	browser, err := b.browsers.New(ctx, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		bs, err := json.MarshalIndent(browser, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	printBrowserSessionResult(browser.SessionID, browser.CdpWsURL, browser.BrowserLiveViewURL, browser.Persistence, browser.Profile)
	return nil
}

func printBrowserSessionResult(sessionID, cdpURL, liveViewURL string, persistence kernel.BrowserPersistence, profile kernel.Profile) {
	tableData := buildBrowserTableData(sessionID, cdpURL, liveViewURL, persistence, profile)
	PrintTableNoPad(tableData, true)
}

// buildBrowserTableData creates a base table with common browser session fields.
func buildBrowserTableData(sessionID, cdpURL, liveViewURL string, persistence kernel.BrowserPersistence, profile kernel.Profile) pterm.TableData {
	tableData := pterm.TableData{
		{"Property", "Value"},
		{"Session ID", sessionID},
		{"CDP WebSocket URL", cdpURL},
	}
	if liveViewURL != "" {
		tableData = append(tableData, []string{"Live View URL", liveViewURL})
	}
	if persistence.ID != "" {
		tableData = append(tableData, []string{"Persistent ID", persistence.ID})
	}
	if profile.ID != "" || profile.Name != "" {
		profVal := profile.Name
		if profVal == "" {
			profVal = profile.ID
		}
		tableData = append(tableData, []string{"Profile", profVal})
	}
	return tableData
}

func (b BrowsersCmd) Delete(ctx context.Context, in BrowsersDeleteInput) error {
	if !in.SkipConfirm {
		found, err := b.browsers.Get(ctx, in.Identifier)
		if err != nil {
			return util.CleanedUpSdkError{Err: err}
		}

		confirmMsg := fmt.Sprintf("Are you sure you want to delete browser \"%s\"?", in.Identifier)
		pterm.DefaultInteractiveConfirm.DefaultText = confirmMsg
		result, _ := pterm.DefaultInteractiveConfirm.Show()
		if !result {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}

		if found.Persistence.ID == in.Identifier {
			err = b.browsers.Delete(ctx, kernel.BrowserDeleteParams{PersistentID: in.Identifier})
			if err != nil && !util.IsNotFound(err) {
				return util.CleanedUpSdkError{Err: err}
			}
			pterm.Success.Printf("Successfully deleted browser: %s\n", in.Identifier)
			return nil
		}

		pterm.Info.Printf("Deleting browser: %s\n", in.Identifier)
		err = b.browsers.DeleteByID(ctx, in.Identifier)
		if err != nil && !util.IsNotFound(err) {
			return util.CleanedUpSdkError{Err: err}
		}
		pterm.Success.Printf("Successfully deleted browser: %s\n", in.Identifier)
		return nil
	}

	// Skip confirmation: try both deletion modes without listing first
	// Treat not found as a success (idempotent delete)
	var nonNotFoundErrors []error

	// Attempt by session ID
	if err := b.browsers.DeleteByID(ctx, in.Identifier); err != nil {
		if !util.IsNotFound(err) {
			nonNotFoundErrors = append(nonNotFoundErrors, err)
		}
	}

	// Attempt by persistent ID (backward compatibility)
	if err := b.browsers.Delete(ctx, kernel.BrowserDeleteParams{PersistentID: in.Identifier}); err != nil {
		if !util.IsNotFound(err) {
			nonNotFoundErrors = append(nonNotFoundErrors, err)
		}
	}

	if len(nonNotFoundErrors) >= 2 {
		// Both failed with meaningful errors; report one
		return util.CleanedUpSdkError{Err: nonNotFoundErrors[0]}
	}

	pterm.Success.Printf("Successfully deleted (or already absent) browser: %s\n", in.Identifier)
	return nil
}

func (b BrowsersCmd) View(ctx context.Context, in BrowsersViewInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	browser, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		result := map[string]string{"liveViewUrl": browser.BrowserLiveViewURL}
		bs, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(bs))
		return nil
	}

	if browser.BrowserLiveViewURL == "" {
		if browser.Headless {
			pterm.Warning.Println("This browser is running in headless mode and does not have a live view URL")
		} else {
			pterm.Warning.Println("No live view URL available for this browser")
		}
		return nil
	}

	fmt.Println(browser.BrowserLiveViewURL)
	return nil
}

func (b BrowsersCmd) Get(ctx context.Context, in BrowsersGetInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	browser, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		bs, err := json.MarshalIndent(browser, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	// Build table starting with common browser fields
	tableData := buildBrowserTableData(
		browser.SessionID,
		browser.CdpWsURL,
		browser.BrowserLiveViewURL,
		browser.Persistence,
		browser.Profile,
	)

	// Append additional detailed fields
	tableData = append(tableData, []string{"Created At", util.FormatLocal(browser.CreatedAt)})
	tableData = append(tableData, []string{"Timeout (seconds)", fmt.Sprintf("%d", browser.TimeoutSeconds)})
	tableData = append(tableData, []string{"Headless", fmt.Sprintf("%t", browser.Headless)})
	tableData = append(tableData, []string{"Stealth", fmt.Sprintf("%t", browser.Stealth)})
	tableData = append(tableData, []string{"Kiosk Mode", fmt.Sprintf("%t", browser.KioskMode)})
	if browser.Viewport.Width > 0 && browser.Viewport.Height > 0 {
		viewportStr := fmt.Sprintf("%dx%d", browser.Viewport.Width, browser.Viewport.Height)
		if browser.Viewport.RefreshRate > 0 {
			viewportStr = fmt.Sprintf("%s@%d", viewportStr, browser.Viewport.RefreshRate)
		}
		tableData = append(tableData, []string{"Viewport", viewportStr})
	}
	if browser.ProxyID != "" {
		tableData = append(tableData, []string{"Proxy ID", browser.ProxyID})
	}
	if !browser.DeletedAt.IsZero() {
		tableData = append(tableData, []string{"Deleted At", util.FormatLocal(browser.DeletedAt)})
	}

	PrintTableNoPad(tableData, true)
	return nil
}

// Logs
type BrowsersLogsStreamInput struct {
	Identifier        string
	Source            string
	Follow            BoolFlag
	Path              string
	SupervisorProcess string
}

func (b BrowsersCmd) LogsStream(ctx context.Context, in BrowsersLogsStreamInput) error {
	if b.logs == nil {
		pterm.Error.Println("logs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserLogStreamParams{Source: kernel.BrowserLogStreamParamsSource(in.Source)}
	if in.Follow.Set {
		params.Follow = kernel.Opt(in.Follow.Value)
	}
	if in.Path != "" {
		params.Path = kernel.Opt(in.Path)
	}
	if in.SupervisorProcess != "" {
		params.SupervisorProcess = kernel.Opt(in.SupervisorProcess)
	}
	stream := b.logs.StreamStreaming(ctx, br.SessionID, params)
	if stream == nil {
		pterm.Error.Println("failed to open log stream")
		return nil
	}
	defer stream.Close()
	for stream.Next() {
		ev := stream.Current()
		pterm.Println(fmt.Sprintf("[%s] %s", util.FormatLocal(ev.Timestamp), ev.Message))
	}
	if err := stream.Err(); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return nil
}

// Computer (mouse/screen)
type BrowsersComputerClickMouseInput struct {
	Identifier string
	X          int64
	Y          int64
	NumClicks  int64
	Button     string
	ClickType  string
	HoldKeys   []string
}

type BrowsersComputerMoveMouseInput struct {
	Identifier string
	X          int64
	Y          int64
	HoldKeys   []string
}

type BrowsersComputerScreenshotInput struct {
	Identifier string
	X          int64
	Y          int64
	Width      int64
	Height     int64
	To         string
	HasRegion  bool
}

type BrowsersComputerTypeTextInput struct {
	Identifier string
	Text       string
	Delay      int64
}

type BrowsersComputerPressKeyInput struct {
	Identifier string
	Keys       []string
	Duration   int64
	HoldKeys   []string
}

type BrowsersComputerScrollInput struct {
	Identifier string
	X          int64
	Y          int64
	DeltaX     int64
	DeltaXSet  bool
	DeltaY     int64
	DeltaYSet  bool
	HoldKeys   []string
}

type BrowsersComputerDragMouseInput struct {
	Identifier      string
	Path            [][]int64
	Delay           int64
	StepDelayMs     int64
	StepsPerSegment int64
	Button          string
	HoldKeys        []string
}

type BrowsersComputerSetCursorInput struct {
	Identifier string
	Hidden     bool
}

func (b BrowsersCmd) ComputerClickMouse(ctx context.Context, in BrowsersComputerClickMouseInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	body := kernel.BrowserComputerClickMouseParams{X: in.X, Y: in.Y}
	if in.NumClicks > 0 {
		body.NumClicks = kernel.Opt(in.NumClicks)
	}
	if in.Button != "" {
		body.Button = kernel.BrowserComputerClickMouseParamsButton(in.Button)
	}
	if in.ClickType != "" {
		body.ClickType = kernel.BrowserComputerClickMouseParamsClickType(in.ClickType)
	}
	if len(in.HoldKeys) > 0 {
		body.HoldKeys = in.HoldKeys
	}
	if err := b.computer.ClickMouse(ctx, br.SessionID, body); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Clicked mouse at (%d,%d)\n", in.X, in.Y)
	return nil
}

func (b BrowsersCmd) ComputerMoveMouse(ctx context.Context, in BrowsersComputerMoveMouseInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	body := kernel.BrowserComputerMoveMouseParams{X: in.X, Y: in.Y}
	if len(in.HoldKeys) > 0 {
		body.HoldKeys = in.HoldKeys
	}
	if err := b.computer.MoveMouse(ctx, br.SessionID, body); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Moved mouse to (%d,%d)\n", in.X, in.Y)
	return nil
}

func (b BrowsersCmd) ComputerScreenshot(ctx context.Context, in BrowsersComputerScreenshotInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	var body kernel.BrowserComputerCaptureScreenshotParams
	if in.HasRegion {
		body.Region = kernel.BrowserComputerCaptureScreenshotParamsRegion{X: in.X, Y: in.Y, Width: in.Width, Height: in.Height}
	}
	res, err := b.computer.CaptureScreenshot(ctx, br.SessionID, body)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()
	if in.To == "" {
		pterm.Error.Println("--to is required to save the screenshot")
		return nil
	}
	f, err := os.Create(in.To)
	if err != nil {
		pterm.Error.Printf("Failed to create file: %v\n", err)
		return nil
	}
	defer f.Close()
	if _, err := io.Copy(f, res.Body); err != nil {
		pterm.Error.Printf("Failed to write file: %v\n", err)
		return nil
	}
	pterm.Success.Printf("Saved screenshot to %s\n", in.To)
	return nil
}

func (b BrowsersCmd) ComputerTypeText(ctx context.Context, in BrowsersComputerTypeTextInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	body := kernel.BrowserComputerTypeTextParams{Text: in.Text}
	if in.Delay > 0 {
		body.Delay = kernel.Opt(in.Delay)
	}
	if err := b.computer.TypeText(ctx, br.SessionID, body); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Typed text: %s\n", in.Text)
	return nil
}

func (b BrowsersCmd) ComputerPressKey(ctx context.Context, in BrowsersComputerPressKeyInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if len(in.Keys) == 0 {
		pterm.Error.Println("no keys specified")
		return nil
	}
	body := kernel.BrowserComputerPressKeyParams{Keys: in.Keys}
	if in.Duration > 0 {
		body.Duration = kernel.Opt(in.Duration)
	}
	if len(in.HoldKeys) > 0 {
		body.HoldKeys = in.HoldKeys
	}
	if err := b.computer.PressKey(ctx, br.SessionID, body); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Pressed keys: %s\n", strings.Join(in.Keys, ","))
	return nil
}

func (b BrowsersCmd) ComputerScroll(ctx context.Context, in BrowsersComputerScrollInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	body := kernel.BrowserComputerScrollParams{X: in.X, Y: in.Y}
	if in.DeltaXSet {
		body.DeltaX = kernel.Opt(in.DeltaX)
	}
	if in.DeltaYSet {
		body.DeltaY = kernel.Opt(in.DeltaY)
	}
	if len(in.HoldKeys) > 0 {
		body.HoldKeys = in.HoldKeys
	}
	if err := b.computer.Scroll(ctx, br.SessionID, body); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Scrolled at (%d,%d)\n", in.X, in.Y)
	return nil
}

func (b BrowsersCmd) ComputerDragMouse(ctx context.Context, in BrowsersComputerDragMouseInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if len(in.Path) < 2 {
		pterm.Error.Println("path must include at least two points")
		return nil
	}
	body := kernel.BrowserComputerDragMouseParams{Path: in.Path}
	if in.Delay > 0 {
		body.Delay = kernel.Opt(in.Delay)
	}
	if in.StepDelayMs > 0 {
		body.StepDelayMs = kernel.Opt(in.StepDelayMs)
	}
	if in.StepsPerSegment > 0 {
		body.StepsPerSegment = kernel.Opt(in.StepsPerSegment)
	}
	if in.Button != "" {
		body.Button = kernel.BrowserComputerDragMouseParamsButton(in.Button)
	}
	if len(in.HoldKeys) > 0 {
		body.HoldKeys = in.HoldKeys
	}
	if err := b.computer.DragMouse(ctx, br.SessionID, body); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Dragged mouse over %d points\n", len(in.Path))
	return nil
}

func (b BrowsersCmd) ComputerSetCursor(ctx context.Context, in BrowsersComputerSetCursorInput) error {
	if b.computer == nil {
		pterm.Error.Println("computer service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	body := kernel.BrowserComputerSetCursorVisibilityParams{Hidden: in.Hidden}
	_, err = b.computer.SetCursorVisibility(ctx, br.SessionID, body)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Hidden {
		pterm.Success.Println("Cursor hidden")
	} else {
		pterm.Success.Println("Cursor shown")
	}
	return nil
}

// Replays
type BrowsersReplaysListInput struct {
	Identifier string
	Output     string
}

type BrowsersReplaysStartInput struct {
	Identifier         string
	Framerate          int
	MaxDurationSeconds int
	Output             string
}

type BrowsersReplaysStopInput struct {
	Identifier string
	ReplayID   string
}

type BrowsersReplaysDownloadInput struct {
	Identifier string
	ReplayID   string
	Output     string
}

func (b BrowsersCmd) ReplaysList(ctx context.Context, in BrowsersReplaysListInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	items, err := b.replays.List(ctx, br.SessionID)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if items == nil || len(*items) == 0 {
			fmt.Println("[]")
			return nil
		}
		bs, err := json.MarshalIndent(*items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	if items == nil || len(*items) == 0 {
		pterm.Info.Println("No replays found")
		return nil
	}
	rows := pterm.TableData{{"Replay ID", "Started At", "Finished At", "View URL"}}
	for _, r := range *items {
		rows = append(rows, []string{r.ReplayID, util.FormatLocal(r.StartedAt), util.FormatLocal(r.FinishedAt), truncateURL(r.ReplayViewURL, 60)})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) ReplaysStart(ctx context.Context, in BrowsersReplaysStartInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	body := kernel.BrowserReplayStartParams{}
	if in.Framerate > 0 {
		body.Framerate = kernel.Opt(int64(in.Framerate))
	}
	if in.MaxDurationSeconds > 0 {
		body.MaxDurationInSeconds = kernel.Opt(int64(in.MaxDurationSeconds))
	}
	res, err := b.replays.Start(ctx, br.SessionID, body)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		bs, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	rows := pterm.TableData{{"Property", "Value"}, {"Replay ID", res.ReplayID}, {"View URL", res.ReplayViewURL}, {"Started At", util.FormatLocal(res.StartedAt)}}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) ReplaysStop(ctx context.Context, in BrowsersReplaysStopInput) error {
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	err = b.replays.Stop(ctx, in.ReplayID, kernel.BrowserReplayStopParams{ID: br.SessionID})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Stopped replay %s for browser %s\n", in.ReplayID, br.SessionID)
	return nil
}

func (b BrowsersCmd) ReplaysDownload(ctx context.Context, in BrowsersReplaysDownloadInput) error {
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	res, err := b.replays.Download(ctx, in.ReplayID, kernel.BrowserReplayDownloadParams{ID: br.SessionID})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()
	if in.Output == "" {
		pterm.Info.Printf("Downloaded replay %s (%s)\n", in.ReplayID, res.Header.Get("content-type"))
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	f, err := os.Create(in.Output)
	if err != nil {
		pterm.Error.Printf("Failed to create file: %v\n", err)
		return nil
	}
	defer f.Close()
	if _, err := io.Copy(f, res.Body); err != nil {
		pterm.Error.Printf("Failed to write file: %v\n", err)
		return nil
	}
	pterm.Success.Printf("Saved replay to %s\n", in.Output)
	return nil
}

// Process
type BrowsersProcessExecInput struct {
	Identifier string
	Command    string
	Args       []string
	Cwd        string
	Timeout    int
	AsUser     string
	AsRoot     BoolFlag
	Output     string
}

type BrowsersProcessSpawnInput struct {
	Identifier string
	Command    string
	Args       []string
	Cwd        string
	Timeout    int
	AsUser     string
	AsRoot     BoolFlag
	Output     string
}

type BrowsersProcessKillInput struct {
	Identifier string
	ProcessID  string
	Signal     string
}

type BrowsersProcessStatusInput struct {
	Identifier string
	ProcessID  string
}

type BrowsersProcessStdinInput struct {
	Identifier string
	ProcessID  string
	DataB64    string
}

type BrowsersProcessStdoutStreamInput struct {
	Identifier string
	ProcessID  string
}

// Playwright
type BrowsersPlaywrightExecuteInput struct {
	Identifier string
	Code       string
	Timeout    int64
}

func (b BrowsersCmd) PlaywrightExecute(ctx context.Context, in BrowsersPlaywrightExecuteInput) error {
	if b.playwright == nil {
		pterm.Error.Println("playwright service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserPlaywrightExecuteParams{Code: in.Code}
	if in.Timeout > 0 {
		params.TimeoutSec = kernel.Opt(in.Timeout)
	}
	res, err := b.playwright.Execute(ctx, br.SessionID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	rows := pterm.TableData{{"Property", "Value"}, {"Success", fmt.Sprintf("%t", res.Success)}}
	PrintTableNoPad(rows, true)

	if res.Stdout != "" {
		pterm.Info.Println("stdout:")
		fmt.Println(res.Stdout)
	}
	if res.Stderr != "" {
		pterm.Info.Println("stderr:")
		fmt.Fprintln(os.Stderr, res.Stderr)
	}
	if res.Result != nil {
		bs, err := json.MarshalIndent(res.Result, "", "  ")
		if err == nil {
			pterm.Info.Println("result:")
			fmt.Println(string(bs))
		}
	}
	if !res.Success && res.Error != "" {
		pterm.Error.Printf("error: %s\n", res.Error)
	}
	return nil
}

func (b BrowsersCmd) ProcessExec(ctx context.Context, in BrowsersProcessExecInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if b.process == nil {
		pterm.Error.Println("process service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserProcessExecParams{Command: in.Command}
	if len(in.Args) > 0 {
		params.Args = in.Args
	}
	if in.Cwd != "" {
		params.Cwd = kernel.Opt(in.Cwd)
	}
	if in.Timeout > 0 {
		params.TimeoutSec = kernel.Opt(int64(in.Timeout))
	}
	if in.AsUser != "" {
		params.AsUser = kernel.Opt(in.AsUser)
	}
	if in.AsRoot.Set {
		params.AsRoot = kernel.Opt(in.AsRoot.Value)
	}
	res, err := b.process.Exec(ctx, br.SessionID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		bs, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	rows := pterm.TableData{{"Property", "Value"}, {"Exit Code", fmt.Sprintf("%d", res.ExitCode)}, {"Duration (ms)", fmt.Sprintf("%d", res.DurationMs)}}
	PrintTableNoPad(rows, true)
	if res.StdoutB64 != "" {
		data, err := base64.StdEncoding.DecodeString(res.StdoutB64)
		if err != nil {
			pterm.Error.Printf("stdout decode error: %v\n", err)
		} else if len(data) > 0 {
			pterm.Info.Println("stdout:")
			os.Stdout.Write(data)
			if data[len(data)-1] != '\n' {
				fmt.Println()
			}
		}
	}
	if res.StderrB64 != "" {
		data, err := base64.StdEncoding.DecodeString(res.StderrB64)
		if err != nil {
			pterm.Error.Printf("stderr decode error: %v\n", err)
		} else if len(data) > 0 {
			pterm.Info.Println("stderr:")
			os.Stderr.Write(data)
			if data[len(data)-1] != '\n' {
				fmt.Fprintln(os.Stderr)
			}
		}
	}
	return nil
}

func (b BrowsersCmd) ProcessSpawn(ctx context.Context, in BrowsersProcessSpawnInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if b.process == nil {
		pterm.Error.Println("process service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserProcessSpawnParams{Command: in.Command}
	if len(in.Args) > 0 {
		params.Args = in.Args
	}
	if in.Cwd != "" {
		params.Cwd = kernel.Opt(in.Cwd)
	}
	if in.Timeout > 0 {
		params.TimeoutSec = kernel.Opt(int64(in.Timeout))
	}
	if in.AsUser != "" {
		params.AsUser = kernel.Opt(in.AsUser)
	}
	if in.AsRoot.Set {
		params.AsRoot = kernel.Opt(in.AsRoot.Value)
	}
	res, err := b.process.Spawn(ctx, br.SessionID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		bs, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	rows := pterm.TableData{{"Property", "Value"}, {"Process ID", res.ProcessID}, {"PID", fmt.Sprintf("%d", res.Pid)}, {"Started At", util.FormatLocal(res.StartedAt)}}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) ProcessKill(ctx context.Context, in BrowsersProcessKillInput) error {
	if b.process == nil {
		pterm.Error.Println("process service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserProcessKillParams{ID: br.SessionID, Signal: kernel.BrowserProcessKillParamsSignal(in.Signal)}
	_, err = b.process.Kill(ctx, in.ProcessID, params)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Sent %s to process %s on %s\n", in.Signal, in.ProcessID, br.SessionID)
	return nil
}

func (b BrowsersCmd) ProcessStatus(ctx context.Context, in BrowsersProcessStatusInput) error {
	if b.process == nil {
		pterm.Error.Println("process service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	res, err := b.process.Status(ctx, in.ProcessID, kernel.BrowserProcessStatusParams{ID: br.SessionID})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	rows := pterm.TableData{{"Property", "Value"}, {"State", string(res.State)}, {"CPU %", fmt.Sprintf("%.2f", res.CPUPct)}, {"Mem Bytes", fmt.Sprintf("%d", res.MemBytes)}, {"Exit Code", fmt.Sprintf("%d", res.ExitCode)}}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) ProcessStdin(ctx context.Context, in BrowsersProcessStdinInput) error {
	if b.process == nil {
		pterm.Error.Println("process service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	_, err = b.process.Stdin(ctx, in.ProcessID, kernel.BrowserProcessStdinParams{ID: br.SessionID, DataB64: in.DataB64})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Println("Wrote to stdin")
	return nil
}

func (b BrowsersCmd) ProcessStdoutStream(ctx context.Context, in BrowsersProcessStdoutStreamInput) error {
	if b.process == nil {
		pterm.Error.Println("process service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	stream := b.process.StdoutStreamStreaming(ctx, in.ProcessID, kernel.BrowserProcessStdoutStreamParams{ID: br.SessionID})
	if stream == nil {
		pterm.Error.Println("failed to open stdout stream")
		return nil
	}
	defer stream.Close()
	for stream.Next() {
		ev := stream.Current()
		if ev.Event == "exit" {
			pterm.Info.Printf("process exited with code %d\n", ev.ExitCode)
			continue
		}
		data, err := base64.StdEncoding.DecodeString(ev.DataB64)
		if err != nil {
			pterm.Error.Printf("decode error: %v\n", err)
			continue
		}
		os.Stdout.Write(data)
	}
	if err := stream.Err(); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return nil
}

// FS (minimal scaffolding)
type BrowsersFSNewDirInput struct {
	Identifier string
	Path       string
	Mode       string
}

type BrowsersFSDeleteDirInput struct {
	Identifier string
	Path       string
}

type BrowsersFSDeleteFileInput struct {
	Identifier string
	Path       string
}

type BrowsersFSDownloadDirZipInput struct {
	Identifier string
	Path       string
	Output     string
}

type BrowsersFSFileInfoInput struct {
	Identifier string
	Path       string
	Output     string
}

type BrowsersFSListFilesInput struct {
	Identifier string
	Path       string
	Output     string
}

type BrowsersFSMoveInput struct {
	Identifier string
	SrcPath    string
	DestPath   string
}

type BrowsersFSReadFileInput struct {
	Identifier string
	Path       string
	Output     string
}

type BrowsersFSSetPermsInput struct {
	Identifier string
	Path       string
	Mode       string
	Owner      string
	Group      string
}

// Upload inputs
type BrowsersFSUploadInput struct {
	Identifier string
	Mappings   []struct {
		Local string
		Dest  string
	}
	DestDir string
	Paths   []string
}

type BrowsersFSUploadZipInput struct {
	Identifier string
	ZipPath    string
	DestDir    string
}

type BrowsersFSWriteFileInput struct {
	Identifier string
	DestPath   string
	Mode       string
	SourcePath string
}

type BrowsersExtensionsUploadInput struct {
	Identifier     string
	ExtensionPaths []string
}

func (b BrowsersCmd) FSNewDirectory(ctx context.Context, in BrowsersFSNewDirInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserFNewDirectoryParams{Path: in.Path}
	if in.Mode != "" {
		params.Mode = kernel.Opt(in.Mode)
	}
	if err := b.fs.NewDirectory(ctx, br.SessionID, params); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Created directory %s\n", in.Path)
	return nil
}

func (b BrowsersCmd) FSDeleteDirectory(ctx context.Context, in BrowsersFSDeleteDirInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if err := b.fs.DeleteDirectory(ctx, br.SessionID, kernel.BrowserFDeleteDirectoryParams{Path: in.Path}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted directory %s\n", in.Path)
	return nil
}

func (b BrowsersCmd) FSDeleteFile(ctx context.Context, in BrowsersFSDeleteFileInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if err := b.fs.DeleteFile(ctx, br.SessionID, kernel.BrowserFDeleteFileParams{Path: in.Path}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Deleted file %s\n", in.Path)
	return nil
}

func (b BrowsersCmd) FSDownloadDirZip(ctx context.Context, in BrowsersFSDownloadDirZipInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	res, err := b.fs.DownloadDirZip(ctx, br.SessionID, kernel.BrowserFDownloadDirZipParams{Path: in.Path})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()
	if in.Output == "" {
		_, _ = io.Copy(io.Discard, res.Body)
		pterm.Info.Println("Downloaded zip (discarded; specify --output to save)")
		return nil
	}
	f, err := os.Create(in.Output)
	if err != nil {
		pterm.Error.Printf("Failed to create file: %v\n", err)
		return nil
	}
	defer f.Close()
	if _, err := io.Copy(f, res.Body); err != nil {
		pterm.Error.Printf("Failed to write file: %v\n", err)
		return nil
	}
	pterm.Success.Printf("Saved zip to %s\n", in.Output)
	return nil
}

func (b BrowsersCmd) FSFileInfo(ctx context.Context, in BrowsersFSFileInfoInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	res, err := b.fs.FileInfo(ctx, br.SessionID, kernel.BrowserFFileInfoParams{Path: in.Path})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		bs, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	rows := pterm.TableData{{"Property", "Value"}, {"Path", res.Path}, {"Name", res.Name}, {"Mode", res.Mode}, {"IsDir", fmt.Sprintf("%t", res.IsDir)}, {"SizeBytes", fmt.Sprintf("%d", res.SizeBytes)}, {"ModTime", util.FormatLocal(res.ModTime)}}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) FSListFiles(ctx context.Context, in BrowsersFSListFilesInput) error {
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}

	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	res, err := b.fs.ListFiles(ctx, br.SessionID, kernel.BrowserFListFilesParams{Path: in.Path})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if in.Output == "json" {
		if res == nil || len(*res) == 0 {
			fmt.Println("[]")
			return nil
		}
		bs, err := json.MarshalIndent(*res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(bs))
		return nil
	}

	if res == nil || len(*res) == 0 {
		pterm.Info.Println("No files found")
		return nil
	}
	rows := pterm.TableData{{"Mode", "Size", "ModTime", "Name", "Path"}}
	for _, f := range *res {
		rows = append(rows, []string{f.Mode, fmt.Sprintf("%d", f.SizeBytes), util.FormatLocal(f.ModTime), f.Name, f.Path})
	}
	PrintTableNoPad(rows, true)
	return nil
}

func (b BrowsersCmd) FSMove(ctx context.Context, in BrowsersFSMoveInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if err := b.fs.Move(ctx, br.SessionID, kernel.BrowserFMoveParams{SrcPath: in.SrcPath, DestPath: in.DestPath}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Moved %s -> %s\n", in.SrcPath, in.DestPath)
	return nil
}

func (b BrowsersCmd) FSReadFile(ctx context.Context, in BrowsersFSReadFileInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	res, err := b.fs.ReadFile(ctx, br.SessionID, kernel.BrowserFReadFileParams{Path: in.Path})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	defer res.Body.Close()
	if in.Output == "" {
		_, _ = io.Copy(os.Stdout, res.Body)
		return nil
	}
	f, err := os.Create(in.Output)
	if err != nil {
		pterm.Error.Printf("Failed to create file: %v\n", err)
		return nil
	}
	defer f.Close()
	if _, err := io.Copy(f, res.Body); err != nil {
		pterm.Error.Printf("Failed to write file: %v\n", err)
		return nil
	}
	pterm.Success.Printf("Saved file to %s\n", in.Output)
	return nil
}

func (b BrowsersCmd) FSSetPermissions(ctx context.Context, in BrowsersFSSetPermsInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserFSetFilePermissionsParams{Path: in.Path, Mode: in.Mode}
	if in.Owner != "" {
		params.Owner = kernel.Opt(in.Owner)
	}
	if in.Group != "" {
		params.Group = kernel.Opt(in.Group)
	}
	if err := b.fs.SetFilePermissions(ctx, br.SessionID, params); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Updated permissions for %s\n", in.Path)
	return nil
}

func (b BrowsersCmd) FSUpload(ctx context.Context, in BrowsersFSUploadInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	var files []kernel.BrowserFUploadParamsFile
	var toClose []io.Closer
	for _, m := range in.Mappings {
		f, err := os.Open(m.Local)
		if err != nil {
			pterm.Error.Printf("Failed to open %s: %v\n", m.Local, err)
			for _, c := range toClose {
				_ = c.Close()
			}
			return nil
		}
		toClose = append(toClose, f)
		files = append(files, kernel.BrowserFUploadParamsFile{DestPath: m.Dest, File: f})
	}
	if in.DestDir != "" && len(in.Paths) > 0 {
		for _, lp := range in.Paths {
			f, err := os.Open(lp)
			if err != nil {
				pterm.Error.Printf("Failed to open %s: %v\n", lp, err)
				for _, c := range toClose {
					_ = c.Close()
				}
				return nil
			}
			toClose = append(toClose, f)
			dest := filepath.Join(in.DestDir, filepath.Base(lp))
			files = append(files, kernel.BrowserFUploadParamsFile{DestPath: dest, File: f})
		}
	}
	if len(files) == 0 {
		pterm.Error.Println("no files specified for upload")
		return nil
	}
	defer func() {
		for _, c := range toClose {
			_ = c.Close()
		}
	}()
	if err := b.fs.Upload(ctx, br.SessionID, kernel.BrowserFUploadParams{Files: files}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if len(files) == 1 {
		pterm.Success.Println("Uploaded 1 file")
	} else {
		pterm.Success.Printf("Uploaded %d files\n", len(files))
	}
	return nil
}

func (b BrowsersCmd) FSUploadZip(ctx context.Context, in BrowsersFSUploadZipInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	f, err := os.Open(in.ZipPath)
	if err != nil {
		pterm.Error.Printf("Failed to open zip: %v\n", err)
		return nil
	}
	defer f.Close()
	if err := b.fs.UploadZip(ctx, br.SessionID, kernel.BrowserFUploadZipParams{DestPath: in.DestDir, ZipFile: f}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Uploaded zip to %s\n", in.DestDir)
	return nil
}

func (b BrowsersCmd) FSWriteFile(ctx context.Context, in BrowsersFSWriteFileInput) error {
	if b.fs == nil {
		pterm.Error.Println("fs service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	var reader io.Reader
	if in.SourcePath != "" {
		f, err := os.Open(in.SourcePath)
		if err != nil {
			pterm.Error.Printf("Failed to open input: %v\n", err)
			return nil
		}
		defer f.Close()
		reader = f
	} else {
		pterm.Error.Println("--source is required")
		return nil
	}
	params := kernel.BrowserFWriteFileParams{Path: in.DestPath}
	if in.Mode != "" {
		params.Mode = kernel.Opt(in.Mode)
	}
	if err := b.fs.WriteFile(ctx, br.SessionID, reader, params); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	pterm.Success.Printf("Wrote file to %s\n", in.DestPath)
	return nil
}

func (b BrowsersCmd) ExtensionsUpload(ctx context.Context, in BrowsersExtensionsUploadInput) error {
	if b.browsers == nil {
		pterm.Error.Println("browsers service not available")
		return nil
	}
	br, err := b.browsers.Get(ctx, in.Identifier)
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if len(in.ExtensionPaths) == 0 {
		pterm.Error.Println("no extension paths provided")
		return nil
	}

	var extensions []kernel.BrowserLoadExtensionsParamsExtension
	var tempZipFiles []string
	var openFiles []*os.File

	defer func() {
		for _, f := range openFiles {
			_ = f.Close()
		}
		for _, zipPath := range tempZipFiles {
			_ = os.Remove(zipPath)
		}
	}()

	for _, extPath := range in.ExtensionPaths {
		info, err := os.Stat(extPath)
		if err != nil {
			pterm.Error.Printf("Failed to stat %s: %v\n", extPath, err)
			return nil
		}
		if !info.IsDir() {
			pterm.Error.Printf("Path %s is not a directory\n", extPath)
			return nil
		}

		extName := generateRandomExtensionName()
		tempZipPath := filepath.Join(os.TempDir(), fmt.Sprintf("kernel-ext-%s.zip", extName))

		pterm.Info.Printf("Zipping %s as %s...\n", extPath, extName)
		if err := util.ZipDirectory(extPath, tempZipPath); err != nil {
			pterm.Error.Printf("Failed to zip %s: %v\n", extPath, err)
			return nil
		}
		tempZipFiles = append(tempZipFiles, tempZipPath)

		zipFile, err := os.Open(tempZipPath)
		if err != nil {
			pterm.Error.Printf("Failed to open zip %s: %v\n", tempZipPath, err)
			return nil
		}
		openFiles = append(openFiles, zipFile)

		extensions = append(extensions, kernel.BrowserLoadExtensionsParamsExtension{
			Name:    extName,
			ZipFile: zipFile,
		})
	}

	pterm.Info.Printf("Uploading %d extension(s) to browser %s...\n", len(extensions), br.SessionID)
	if err := b.browsers.LoadExtensions(ctx, br.SessionID, kernel.BrowserLoadExtensionsParams{
		Extensions: extensions,
	}); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}

	if len(extensions) == 1 {
		pterm.Success.Println("Successfully uploaded 1 extension and restarted Chromium")
	} else {
		pterm.Success.Printf("Successfully uploaded %d extensions and restarted Chromium\n", len(extensions))
	}
	return nil
}

// generateRandomExtensionName generates a random name matching pattern ^[A-Za-z0-9._-]{1,64}$
func generateRandomExtensionName() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
	const nameLen = 16
	result := make([]byte, nameLen)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		result[i] = chars[n.Int64()]
	}
	return string(result)
}

var browsersCmd = &cobra.Command{
	Use:     "browsers",
	Aliases: []string{"browser"},
	Short:   "Manage browsers",
	Long:    "Commands for managing Kernel browsers",
}

var browsersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List running browsers",
	RunE:  runBrowsersList,
}

var browsersCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new browser session",
	RunE:  runBrowsersCreate,
}

var browsersDeleteCmd = &cobra.Command{
	Use:   "delete <id> [ids...]",
	Short: "Delete a browser",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runBrowsersDelete,
}

var browsersViewCmd = &cobra.Command{
	Use:   "view <id>",
	Short: "Get the live view URL for a browser",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowsersView,
}

var browsersGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get detailed information about a browser session",
	Long:  "Retrieve and display detailed information about a specific browser session including configuration, URLs, and status.",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowsersGet,
}

func init() {
	// list flags
	browsersListCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	browsersListCmd.Flags().Bool("include-deleted", false, "Include soft-deleted browser sessions in the results")
	browsersListCmd.Flags().Int("limit", 0, "Maximum number of results to return (default 20, max 100)")
	browsersListCmd.Flags().Int("offset", 0, "Number of results to skip (for pagination)")

	// get flags
	browsersGetCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	// view flags
	browsersViewCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")

	browsersCmd.AddCommand(browsersListCmd)
	browsersCmd.AddCommand(browsersCreateCmd)
	browsersCmd.AddCommand(browsersDeleteCmd)
	browsersCmd.AddCommand(browsersViewCmd)
	browsersCmd.AddCommand(browsersGetCmd)

	// logs
	logsRoot := &cobra.Command{Use: "logs", Short: "Browser logs operations"}
	logsStream := &cobra.Command{Use: "stream <id>", Short: "Stream browser logs", Args: cobra.ExactArgs(1), RunE: runBrowsersLogsStream}
	logsStream.Flags().String("source", "", "Log source: path or supervisor")
	logsStream.Flags().Bool("follow", true, "Follow the log stream")
	logsStream.Flags().String("path", "", "File path when source=path")
	logsStream.Flags().String("supervisor-process", "", "Supervisor process name when source=supervisor. Useful values to use: chromium, kernel-images-api, neko")
	_ = logsStream.MarkFlagRequired("source")
	logsRoot.AddCommand(logsStream)
	browsersCmd.AddCommand(logsRoot)

	// replays
	replaysRoot := &cobra.Command{Use: "replays", Short: "Manage browser replays"}
	replaysList := &cobra.Command{Use: "list <id>", Short: "List replays for a browser", Args: cobra.ExactArgs(1), RunE: runBrowsersReplaysList}
	replaysList.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	replaysStart := &cobra.Command{Use: "start <id>", Short: "Start a replay recording", Args: cobra.ExactArgs(1), RunE: runBrowsersReplaysStart}
	replaysStart.Flags().Int("framerate", 0, "Recording framerate (fps)")
	replaysStart.Flags().Int("max-duration", 0, "Maximum duration in seconds")
	replaysStart.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	replaysStop := &cobra.Command{Use: "stop <id> <replay-id>", Short: "Stop a replay recording", Args: cobra.ExactArgs(2), RunE: runBrowsersReplaysStop}
	replaysDownload := &cobra.Command{Use: "download <id> <replay-id>", Short: "Download a replay video", Args: cobra.ExactArgs(2), RunE: runBrowsersReplaysDownload}
	replaysDownload.Flags().StringP("output-file", "f", "", "Output file path for the replay video")
	replaysRoot.AddCommand(replaysList, replaysStart, replaysStop, replaysDownload)
	browsersCmd.AddCommand(replaysRoot)

	// process
	procRoot := &cobra.Command{Use: "process", Short: "Manage processes inside the browser VM"}
	procExec := &cobra.Command{Use: "exec <id> [--] [command...]", Short: "Execute a command synchronously", Args: cobra.MinimumNArgs(1), RunE: runBrowsersProcessExec}
	procExec.Flags().String("command", "", "Command to execute (optional; if omitted, trailing args are executed via /bin/bash -c)")
	procExec.Flags().StringSlice("args", []string{}, "Command arguments")
	procExec.Flags().String("cwd", "", "Working directory")
	procExec.Flags().Int("timeout", 0, "Timeout in seconds")
	procExec.Flags().String("as-user", "", "Run as user")
	procExec.Flags().Bool("as-root", false, "Run as root")
	procExec.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	procSpawn := &cobra.Command{Use: "spawn <id> [--] [command...]", Short: "Execute a command asynchronously", Args: cobra.MinimumNArgs(1), RunE: runBrowsersProcessSpawn}
	procSpawn.Flags().String("command", "", "Command to execute (optional; if omitted, trailing args are executed via /bin/bash -c)")
	procSpawn.Flags().StringSlice("args", []string{}, "Command arguments")
	procSpawn.Flags().String("cwd", "", "Working directory")
	procSpawn.Flags().Int("timeout", 0, "Timeout in seconds")
	procSpawn.Flags().String("as-user", "", "Run as user")
	procSpawn.Flags().Bool("as-root", false, "Run as root")
	procSpawn.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	procKill := &cobra.Command{Use: "kill <id> <process-id>", Short: "Send a signal to a process", Args: cobra.ExactArgs(2), RunE: runBrowsersProcessKill}
	procKill.Flags().String("signal", "TERM", "Signal to send (TERM, KILL, INT, HUP)")
	procStatus := &cobra.Command{Use: "status <id> <process-id>", Short: "Get process status", Args: cobra.ExactArgs(2), RunE: runBrowsersProcessStatus}
	procStdin := &cobra.Command{Use: "stdin <id> <process-id>", Short: "Write to process stdin (base64)", Args: cobra.ExactArgs(2), RunE: runBrowsersProcessStdin}
	procStdin.Flags().String("data-b64", "", "Base64-encoded data to write to stdin")
	_ = procStdin.MarkFlagRequired("data-b64")
	procStdoutStream := &cobra.Command{Use: "stdout-stream <id> <process-id>", Short: "Stream process stdout/stderr", Args: cobra.ExactArgs(2), RunE: runBrowsersProcessStdoutStream}
	procRoot.AddCommand(procExec, procSpawn, procKill, procStatus, procStdin, procStdoutStream)
	browsersCmd.AddCommand(procRoot)

	// fs
	fsRoot := &cobra.Command{Use: "fs", Short: "Browser filesystem operations"}
	fsNewDir := &cobra.Command{Use: "new-directory <id>", Short: "Create a new directory", Args: cobra.ExactArgs(1), RunE: runBrowsersFSNewDirectory}
	fsNewDir.Flags().String("path", "", "Absolute directory path to create")
	_ = fsNewDir.MarkFlagRequired("path")
	fsNewDir.Flags().String("mode", "", "Directory mode (octal string)")
	fsDelDir := &cobra.Command{Use: "delete-directory <id>", Short: "Delete a directory", Args: cobra.ExactArgs(1), RunE: runBrowsersFSDeleteDirectory}
	fsDelDir.Flags().String("path", "", "Absolute directory path to delete")
	_ = fsDelDir.MarkFlagRequired("path")
	fsDelFile := &cobra.Command{Use: "delete-file <id>", Short: "Delete a file", Args: cobra.ExactArgs(1), RunE: runBrowsersFSDeleteFile}
	fsDelFile.Flags().String("path", "", "Absolute file path to delete")
	_ = fsDelFile.MarkFlagRequired("path")
	fsDownloadZip := &cobra.Command{Use: "download-dir-zip <id>", Short: "Download a directory as zip", Args: cobra.ExactArgs(1), RunE: runBrowsersFSDownloadDirZip}
	fsDownloadZip.Flags().String("path", "", "Absolute directory path to download")
	_ = fsDownloadZip.MarkFlagRequired("path")
	fsDownloadZip.Flags().StringP("output", "o", "", "Output zip file path")
	fsFileInfo := &cobra.Command{Use: "file-info <id>", Short: "Get file or directory info", Args: cobra.ExactArgs(1), RunE: runBrowsersFSFileInfo}
	fsFileInfo.Flags().String("path", "", "Absolute file or directory path")
	_ = fsFileInfo.MarkFlagRequired("path")
	fsFileInfo.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	fsListFiles := &cobra.Command{Use: "list-files <id>", Short: "List files in a directory", Args: cobra.ExactArgs(1), RunE: runBrowsersFSListFiles}
	fsListFiles.Flags().String("path", "", "Absolute directory path")
	_ = fsListFiles.MarkFlagRequired("path")
	fsListFiles.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	fsMove := &cobra.Command{Use: "move <id>", Short: "Move or rename a file or directory", Args: cobra.ExactArgs(1), RunE: runBrowsersFSMove}
	fsMove.Flags().String("src", "", "Absolute source path")
	fsMove.Flags().String("dest", "", "Absolute destination path")
	_ = fsMove.MarkFlagRequired("src")
	_ = fsMove.MarkFlagRequired("dest")
	fsReadFile := &cobra.Command{Use: "read-file <id>", Short: "Read a file", Args: cobra.ExactArgs(1), RunE: runBrowsersFSReadFile}
	fsReadFile.Flags().String("path", "", "Absolute file path")
	_ = fsReadFile.MarkFlagRequired("path")
	fsReadFile.Flags().StringP("output", "o", "", "Output file path (optional)")
	fsSetPerms := &cobra.Command{Use: "set-permissions <id>", Short: "Set file permissions or ownership", Args: cobra.ExactArgs(1), RunE: runBrowsersFSSetPermissions}
	fsSetPerms.Flags().String("path", "", "Absolute path")
	fsSetPerms.Flags().String("mode", "", "File mode bits (octal string)")
	_ = fsSetPerms.MarkFlagRequired("path")
	_ = fsSetPerms.MarkFlagRequired("mode")
	fsSetPerms.Flags().String("owner", "", "New owner username or UID")
	fsSetPerms.Flags().String("group", "", "New group name or GID")

	// fs upload
	fsUpload := &cobra.Command{Use: "upload <id>", Short: "Upload one or more files", Args: cobra.ExactArgs(1), RunE: runBrowsersFSUpload}
	fsUpload.Flags().StringSlice("file", []string{}, "Mapping local:remote (repeatable)")
	fsUpload.Flags().String("dest-dir", "", "Destination directory for uploads")
	fsUpload.Flags().StringSlice("paths", []string{}, "Local file paths to upload")

	// fs upload-zip
	fsUploadZip := &cobra.Command{Use: "upload-zip <id>", Short: "Upload a zip and extract it", Args: cobra.ExactArgs(1), RunE: runBrowsersFSUploadZip}
	fsUploadZip.Flags().String("zip", "", "Local zip file path")
	_ = fsUploadZip.MarkFlagRequired("zip")
	fsUploadZip.Flags().String("dest-dir", "", "Destination directory to extract to")
	_ = fsUploadZip.MarkFlagRequired("dest-dir")

	// fs write-file
	fsWriteFile := &cobra.Command{Use: "write-file <id>", Short: "Write a file from local data", Args: cobra.ExactArgs(1), RunE: runBrowsersFSWriteFile}
	fsWriteFile.Flags().String("path", "", "Destination absolute file path")
	_ = fsWriteFile.MarkFlagRequired("path")
	fsWriteFile.Flags().String("mode", "", "File mode (octal string)")
	fsWriteFile.Flags().String("source", "", "Local source file path")
	_ = fsWriteFile.MarkFlagRequired("source")

	fsRoot.AddCommand(fsNewDir, fsDelDir, fsDelFile, fsDownloadZip, fsFileInfo, fsListFiles, fsMove, fsReadFile, fsSetPerms, fsUpload, fsUploadZip, fsWriteFile)
	browsersCmd.AddCommand(fsRoot)

	// extensions
	extensionsRoot := &cobra.Command{Use: "extensions", Short: "Add browser extensions to a running instance"}
	extensionsUpload := &cobra.Command{Use: "upload <id> <extension-path>...", Short: "Upload one or more unpacked extensions and restart Chromium", Args: cobra.MinimumNArgs(2), RunE: runBrowsersExtensionsUpload}
	extensionsRoot.AddCommand(extensionsUpload)
	browsersCmd.AddCommand(extensionsRoot)

	// computer
	computerRoot := &cobra.Command{Use: "computer", Short: "OS-level mouse & screen controls"}
	computerClick := &cobra.Command{Use: "click-mouse <id>", Short: "Click mouse at coordinates", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerClickMouse}
	computerClick.Flags().Int64("x", 0, "X coordinate")
	computerClick.Flags().Int64("y", 0, "Y coordinate")
	_ = computerClick.MarkFlagRequired("x")
	_ = computerClick.MarkFlagRequired("y")
	computerClick.Flags().Int64("num-clicks", 1, "Number of clicks")
	computerClick.Flags().String("button", "left", "Mouse button: left,right,middle,back,forward")
	computerClick.Flags().String("click-type", "click", "Click type: down,up,click")
	computerClick.Flags().StringSlice("hold-key", []string{}, "Modifier keys to hold (repeatable)")

	computerMove := &cobra.Command{Use: "move-mouse <id>", Short: "Move mouse to coordinates", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerMoveMouse}
	computerMove.Flags().Int64("x", 0, "X coordinate")
	computerMove.Flags().Int64("y", 0, "Y coordinate")
	_ = computerMove.MarkFlagRequired("x")
	_ = computerMove.MarkFlagRequired("y")
	computerMove.Flags().StringSlice("hold-key", []string{}, "Modifier keys to hold (repeatable)")

	computerScreenshot := &cobra.Command{Use: "screenshot <id>", Short: "Capture a screenshot (optionally of a region)", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerScreenshot}
	computerScreenshot.Flags().Int64("x", 0, "Top-left X")
	computerScreenshot.Flags().Int64("y", 0, "Top-left Y")
	computerScreenshot.Flags().Int64("width", 0, "Region width")
	computerScreenshot.Flags().Int64("height", 0, "Region height")
	computerScreenshot.Flags().String("to", "", "Output file path for the PNG image")
	_ = computerScreenshot.MarkFlagRequired("to")

	computerType := &cobra.Command{Use: "type <id>", Short: "Type text on the browser instance", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerTypeText}
	computerType.Flags().String("text", "", "Text to type")
	_ = computerType.MarkFlagRequired("text")
	computerType.Flags().Int64("delay", 0, "Delay in milliseconds between keystrokes")

	// computer press-key
	computerPressKey := &cobra.Command{Use: "press-key <id>", Short: "Press one or more keys", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerPressKey}
	computerPressKey.Flags().StringSlice("key", []string{}, "Key symbols to press (repeatable)")
	_ = computerPressKey.MarkFlagRequired("key")
	computerPressKey.Flags().Int64("duration", 0, "Duration to hold keys down in ms (0=tap)")
	computerPressKey.Flags().StringSlice("hold-key", []string{}, "Modifier keys to hold (repeatable)")

	// computer scroll
	computerScroll := &cobra.Command{Use: "scroll <id>", Short: "Scroll the mouse wheel", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerScroll}
	computerScroll.Flags().Int64("x", 0, "X coordinate")
	computerScroll.Flags().Int64("y", 0, "Y coordinate")
	_ = computerScroll.MarkFlagRequired("x")
	_ = computerScroll.MarkFlagRequired("y")
	computerScroll.Flags().Int64("delta-x", 0, "Horizontal scroll amount (+right, -left)")
	computerScroll.Flags().Int64("delta-y", 0, "Vertical scroll amount (+down, -up)")
	computerScroll.Flags().StringSlice("hold-key", []string{}, "Modifier keys to hold (repeatable)")

	// computer drag-mouse
	computerDrag := &cobra.Command{Use: "drag-mouse <id>", Short: "Drag the mouse along a path", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerDragMouse}
	computerDrag.Flags().StringArray("point", []string{}, "Add a point as x,y (repeatable)")
	computerDrag.Flags().Int64("delay", 0, "Delay before dragging starts in ms")
	computerDrag.Flags().Int64("step-delay-ms", 0, "Delay between steps while dragging (ms)")
	computerDrag.Flags().Int64("steps-per-segment", 0, "Number of move steps per path segment")
	computerDrag.Flags().String("button", "left", "Mouse button: left,middle,right")
	computerDrag.Flags().StringSlice("hold-key", []string{}, "Modifier keys to hold (repeatable)")

	// computer set-cursor
	computerSetCursor := &cobra.Command{Use: "set-cursor <id>", Short: "Hide or show the cursor", Args: cobra.ExactArgs(1), RunE: runBrowsersComputerSetCursor}
	computerSetCursor.Flags().String("hidden", "", "Whether to hide the cursor: true or false")
	_ = computerSetCursor.MarkFlagRequired("hidden")

	computerRoot.AddCommand(computerClick, computerMove, computerScreenshot, computerType, computerPressKey, computerScroll, computerDrag, computerSetCursor)
	browsersCmd.AddCommand(computerRoot)

	// playwright
	playwrightRoot := &cobra.Command{Use: "playwright", Short: "Playwright operations"}
	playwrightExecute := &cobra.Command{Use: "execute <id> [code]", Short: "Execute Playwright/TypeScript code against the browser", Args: cobra.MinimumNArgs(1), RunE: runBrowsersPlaywrightExecute}
	playwrightExecute.Flags().Int64("timeout", 0, "Maximum execution time in seconds (default per server)")
	playwrightRoot.AddCommand(playwrightExecute)
	browsersCmd.AddCommand(playwrightRoot)

	// Add flags for create command
	browsersCreateCmd.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	browsersCreateCmd.Flags().StringP("persistent-id", "p", "", "[DEPRECATED] Use --timeout and profiles instead. Unique identifier for browser session persistence")
	_ = browsersCreateCmd.Flags().MarkDeprecated("persistent-id", "use --timeout (up to 72 hours) and profiles instead")
	browsersCreateCmd.Flags().BoolP("stealth", "s", false, "Launch browser in stealth mode to avoid detection")
	browsersCreateCmd.Flags().BoolP("headless", "H", false, "Launch browser without GUI access")
	browsersCreateCmd.Flags().Bool("kiosk", false, "Launch browser in kiosk mode")
	browsersCreateCmd.Flags().IntP("timeout", "t", 60, "Timeout in seconds for the browser session")
	browsersCreateCmd.Flags().String("profile-id", "", "Profile ID to load into the browser session (mutually exclusive with --profile-name)")
	browsersCreateCmd.Flags().String("profile-name", "", "Profile name to load into the browser session (mutually exclusive with --profile-id)")
	browsersCreateCmd.Flags().Bool("save-changes", false, "If set, save changes back to the profile when the session ends")
	browsersCreateCmd.Flags().String("proxy-id", "", "Proxy ID to use for the browser session")
	browsersCreateCmd.Flags().StringSlice("extension", []string{}, "Extension IDs or names to load (repeatable; may be passed multiple times or comma-separated)")
	browsersCreateCmd.Flags().String("viewport", "", "Browser viewport size (e.g., 1920x1080@25). Supported: 2560x1440@10, 1920x1080@25, 1920x1200@25, 1440x900@25, 1024x768@60, 1200x800@60")
	browsersCreateCmd.Flags().Bool("viewport-interactive", false, "Interactively select viewport size from list")
	browsersCreateCmd.Flags().String("pool-id", "", "Browser pool ID to acquire from (mutually exclusive with --pool-name)")
	browsersCreateCmd.Flags().String("pool-name", "", "Browser pool name to acquire from (mutually exclusive with --pool-id)")

	// Add flags for delete command
	browsersDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// no flags for view; it takes a single positional argument
}

func runBrowsersList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc}
	out, _ := cmd.Flags().GetString("output")
	includeDeleted, _ := cmd.Flags().GetBool("include-deleted")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	return b.List(cmd.Context(), BrowsersListInput{
		Output:         out,
		IncludeDeleted: includeDeleted,
		Limit:          limit,
		Offset:         offset,
	})
}

func runBrowsersCreate(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)

	// Get flag values
	persistenceID, _ := cmd.Flags().GetString("persistent-id")
	if persistenceID != "" {
		pterm.Warning.Println("--persistent-id is deprecated. Use --timeout (up to 72 hours) and profiles instead.")
	}
	stealthVal, _ := cmd.Flags().GetBool("stealth")
	headlessVal, _ := cmd.Flags().GetBool("headless")
	kioskVal, _ := cmd.Flags().GetBool("kiosk")
	timeout, _ := cmd.Flags().GetInt("timeout")
	profileID, _ := cmd.Flags().GetString("profile-id")
	profileName, _ := cmd.Flags().GetString("profile-name")
	saveChanges, _ := cmd.Flags().GetBool("save-changes")
	proxyID, _ := cmd.Flags().GetString("proxy-id")
	extensions, _ := cmd.Flags().GetStringSlice("extension")
	viewport, _ := cmd.Flags().GetString("viewport")
	viewportInteractive, _ := cmd.Flags().GetBool("viewport-interactive")
	poolID, _ := cmd.Flags().GetString("pool-id")
	poolName, _ := cmd.Flags().GetString("pool-name")
	output, _ := cmd.Flags().GetString("output")

	if poolID != "" && poolName != "" {
		pterm.Error.Println("must specify at most one of --pool-id or --pool-name")
		return nil
	}

	if poolID != "" || poolName != "" {
		// When using a pool, configuration comes from the pool itself.
		allowedFlags := map[string]bool{
			"pool-id":   true,
			"pool-name": true,
			"timeout":   true,
			"output":    true,
			// Global persistent flags that don't configure browsers
			"no-color":  true,
			"log-level": true,
		}

		// Check if any browser configuration flags were set (which would conflict).
		var conflicts []string
		cmd.Flags().Visit(func(f *pflag.Flag) {
			if !allowedFlags[f.Name] {
				conflicts = append(conflicts, "--"+f.Name)
			}
		})

		if len(conflicts) > 0 {
			flagLabel := "--pool-id"
			if poolName != "" {
				flagLabel = "--pool-name"
			}
			pterm.Warning.Printf("You specified %s, but also provided browser configuration flags: %s\n", flagLabel, strings.Join(conflicts, ", "))
			pterm.Info.Println("When using a pool, all browser configuration comes from the pool itself.")
			pterm.Info.Println("The conflicting flags will be ignored.")

			result, _ := pterm.DefaultInteractiveConfirm.Show("Continue with pool configuration?")
			if !result {
				pterm.Info.Println("Cancelled. Remove conflicting flags or omit the pool flag.")
				return nil
			}
			pterm.Success.Println("Proceeding with pool configuration...")
		}

		pool := poolID
		if pool == "" {
			pool = poolName
		}

		if output != "json" {
			pterm.Info.Printf("Acquiring browser from pool %s...\n", pool)
		}
		poolSvc := client.BrowserPools

		acquireParams := kernel.BrowserPoolAcquireParams{}
		if cmd.Flags().Changed("timeout") && timeout > 0 {
			acquireParams.AcquireTimeoutSeconds = kernel.Int(int64(timeout))
		}

		resp, err := (&poolSvc).Acquire(cmd.Context(), pool, acquireParams)
		if err != nil {
			return util.CleanedUpSdkError{Err: err}
		}
		if resp == nil {
			if output == "json" {
				fmt.Println("null")
				return nil
			}
			pterm.Error.Println("Acquire request timed out (no browser available). Retry to continue waiting.")
			return nil
		}
		if output == "json" {
			bs, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(bs))
			return nil
		}
		printBrowserSessionResult(resp.SessionID, resp.CdpWsURL, resp.BrowserLiveViewURL, resp.Persistence, resp.Profile)
		return nil
	}

	// Handle interactive viewport selection
	if viewportInteractive {
		if viewport != "" {
			pterm.Warning.Println("Both --viewport and --viewport-interactive specified; using interactive mode")
		}
		options := getAvailableViewports()
		selectedViewport, err := pterm.DefaultInteractiveSelect.
			WithOptions(options).
			WithDefaultText("Select a viewport size:").
			Show()
		if err != nil {
			pterm.Error.Printf("Failed to select viewport: %v\n", err)
			return nil
		}
		viewport = selectedViewport
	}

	in := BrowsersCreateInput{
		PersistenceID:      persistenceID,
		TimeoutSeconds:     timeout,
		Stealth:            BoolFlag{Set: cmd.Flags().Changed("stealth"), Value: stealthVal},
		Headless:           BoolFlag{Set: cmd.Flags().Changed("headless"), Value: headlessVal},
		Kiosk:              BoolFlag{Set: cmd.Flags().Changed("kiosk"), Value: kioskVal},
		ProfileID:          profileID,
		ProfileName:        profileName,
		ProfileSaveChanges: BoolFlag{Set: cmd.Flags().Changed("save-changes"), Value: saveChanges},
		ProxyID:            proxyID,
		Extensions:         extensions,
		Viewport:           viewport,
		Output:             output,
	}

	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc}
	return b.Create(cmd.Context(), in)
}

func runBrowsersDelete(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc}
	// Iterate all provided identifiers
	for _, identifier := range args {
		if err := b.Delete(cmd.Context(), BrowsersDeleteInput{Identifier: identifier, SkipConfirm: skipConfirm}); err != nil {
			return err
		}
	}
	return nil
}

func runBrowsersView(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	output, _ := cmd.Flags().GetString("output")

	identifier := args[0]

	in := BrowsersViewInput{Identifier: identifier, Output: output}
	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc}
	return b.View(cmd.Context(), in)
}

func runBrowsersGet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	out, _ := cmd.Flags().GetString("output")

	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc}
	return b.Get(cmd.Context(), BrowsersGetInput{
		Identifier: args[0],
		Output:     out,
	})
}

func runBrowsersLogsStream(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	followVal, _ := cmd.Flags().GetBool("follow")
	source, _ := cmd.Flags().GetString("source")
	path, _ := cmd.Flags().GetString("path")
	supervisor, _ := cmd.Flags().GetString("supervisor-process")
	b := BrowsersCmd{browsers: &svc, logs: &svc.Logs}
	return b.LogsStream(cmd.Context(), BrowsersLogsStreamInput{
		Identifier:        args[0],
		Source:            source,
		Follow:            BoolFlag{Set: cmd.Flags().Changed("follow"), Value: followVal},
		Path:              path,
		SupervisorProcess: supervisor,
	})
}

func runBrowsersReplaysList(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	output, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, replays: &svc.Replays}
	return b.ReplaysList(cmd.Context(), BrowsersReplaysListInput{Identifier: args[0], Output: output})
}

func runBrowsersReplaysStart(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	fr, _ := cmd.Flags().GetInt("framerate")
	md, _ := cmd.Flags().GetInt("max-duration")
	output, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, replays: &svc.Replays}
	return b.ReplaysStart(cmd.Context(), BrowsersReplaysStartInput{Identifier: args[0], Framerate: fr, MaxDurationSeconds: md, Output: output})
}

func runBrowsersReplaysStop(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc, replays: &svc.Replays}
	return b.ReplaysStop(cmd.Context(), BrowsersReplaysStopInput{Identifier: args[0], ReplayID: args[1]})
}

func runBrowsersReplaysDownload(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output-file")
	b := BrowsersCmd{browsers: &svc, replays: &svc.Replays}
	return b.ReplaysDownload(cmd.Context(), BrowsersReplaysDownloadInput{Identifier: args[0], ReplayID: args[1], Output: out})
}

func runBrowsersProcessExec(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	command, _ := cmd.Flags().GetString("command")
	argv, _ := cmd.Flags().GetStringSlice("args")
	cwd, _ := cmd.Flags().GetString("cwd")
	timeout, _ := cmd.Flags().GetInt("timeout")
	asUser, _ := cmd.Flags().GetString("as-user")
	asRoot, _ := cmd.Flags().GetBool("as-root")
	if command == "" && len(args) > 1 {
		// Treat trailing args after identifier as a shell command
		shellCmd := strings.Join(args[1:], " ")
		command = "/bin/bash"
		argv = []string{"-c", shellCmd}
	}
	output, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, process: &svc.Process}
	return b.ProcessExec(cmd.Context(), BrowsersProcessExecInput{Identifier: args[0], Command: command, Args: argv, Cwd: cwd, Timeout: timeout, AsUser: asUser, AsRoot: BoolFlag{Set: cmd.Flags().Changed("as-root"), Value: asRoot}, Output: output})
}

func runBrowsersProcessSpawn(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	command, _ := cmd.Flags().GetString("command")
	argv, _ := cmd.Flags().GetStringSlice("args")
	cwd, _ := cmd.Flags().GetString("cwd")
	timeout, _ := cmd.Flags().GetInt("timeout")
	asUser, _ := cmd.Flags().GetString("as-user")
	asRoot, _ := cmd.Flags().GetBool("as-root")
	if command == "" && len(args) > 1 {
		shellCmd := strings.Join(args[1:], " ")
		command = "/bin/bash"
		argv = []string{"-c", shellCmd}
	}
	output, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, process: &svc.Process}
	return b.ProcessSpawn(cmd.Context(), BrowsersProcessSpawnInput{Identifier: args[0], Command: command, Args: argv, Cwd: cwd, Timeout: timeout, AsUser: asUser, AsRoot: BoolFlag{Set: cmd.Flags().Changed("as-root"), Value: asRoot}, Output: output})
}

func runBrowsersProcessKill(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	signal, _ := cmd.Flags().GetString("signal")
	b := BrowsersCmd{browsers: &svc, process: &svc.Process}
	return b.ProcessKill(cmd.Context(), BrowsersProcessKillInput{Identifier: args[0], ProcessID: args[1], Signal: signal})
}

func runBrowsersProcessStatus(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc, process: &svc.Process}
	return b.ProcessStatus(cmd.Context(), BrowsersProcessStatusInput{Identifier: args[0], ProcessID: args[1]})
}

func runBrowsersProcessStdin(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	data, _ := cmd.Flags().GetString("data-b64")
	b := BrowsersCmd{browsers: &svc, process: &svc.Process}
	return b.ProcessStdin(cmd.Context(), BrowsersProcessStdinInput{Identifier: args[0], ProcessID: args[1], DataB64: data})
}

func runBrowsersProcessStdoutStream(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc, process: &svc.Process}
	return b.ProcessStdoutStream(cmd.Context(), BrowsersProcessStdoutStreamInput{Identifier: args[0], ProcessID: args[1]})
}

func runBrowsersPlaywrightExecute(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers

	var code string
	if len(args) >= 2 {
		code = strings.Join(args[1:], " ")
	} else {
		// Read code from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			pterm.Error.Println("no code provided. Provide code as an argument or pipe via stdin")
			return nil
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			pterm.Error.Printf("failed to read stdin: %v\n", err)
			return nil
		}
		code = string(data)
	}
	timeout, _ := cmd.Flags().GetInt64("timeout")
	b := BrowsersCmd{browsers: &svc, playwright: &svc.Playwright}
	return b.PlaywrightExecute(cmd.Context(), BrowsersPlaywrightExecuteInput{Identifier: args[0], Code: strings.TrimSpace(code), Timeout: timeout})
}

func runBrowsersFSNewDirectory(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	mode, _ := cmd.Flags().GetString("mode")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSNewDirectory(cmd.Context(), BrowsersFSNewDirInput{Identifier: args[0], Path: path, Mode: mode})
}

func runBrowsersFSDeleteDirectory(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSDeleteDirectory(cmd.Context(), BrowsersFSDeleteDirInput{Identifier: args[0], Path: path})
}

func runBrowsersFSDeleteFile(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSDeleteFile(cmd.Context(), BrowsersFSDeleteFileInput{Identifier: args[0], Path: path})
}

func runBrowsersFSDownloadDirZip(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	out, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSDownloadDirZip(cmd.Context(), BrowsersFSDownloadDirZipInput{Identifier: args[0], Path: path, Output: out})
}

func runBrowsersFSFileInfo(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	output, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSFileInfo(cmd.Context(), BrowsersFSFileInfoInput{Identifier: args[0], Path: path, Output: output})
}

func runBrowsersFSListFiles(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	output, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSListFiles(cmd.Context(), BrowsersFSListFilesInput{Identifier: args[0], Path: path, Output: output})
}

func runBrowsersFSMove(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	src, _ := cmd.Flags().GetString("src")
	dest, _ := cmd.Flags().GetString("dest")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSMove(cmd.Context(), BrowsersFSMoveInput{Identifier: args[0], SrcPath: src, DestPath: dest})
}

func runBrowsersFSReadFile(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	out, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSReadFile(cmd.Context(), BrowsersFSReadFileInput{Identifier: args[0], Path: path, Output: out})
}

func runBrowsersFSSetPermissions(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	mode, _ := cmd.Flags().GetString("mode")
	owner, _ := cmd.Flags().GetString("owner")
	group, _ := cmd.Flags().GetString("group")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSSetPermissions(cmd.Context(), BrowsersFSSetPermsInput{Identifier: args[0], Path: path, Mode: mode, Owner: owner, Group: group})
}

func runBrowsersFSUpload(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	fileMaps, _ := cmd.Flags().GetStringSlice("file")
	destDir, _ := cmd.Flags().GetString("dest-dir")
	paths, _ := cmd.Flags().GetStringSlice("paths")
	var mappings []struct {
		Local string
		Dest  string
	}
	for _, m := range fileMaps {
		// format: local:remote
		parts := strings.SplitN(m, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			pterm.Error.Printf("invalid --file mapping: %s\n", m)
			return nil
		}
		mappings = append(mappings, struct {
			Local string
			Dest  string
		}{Local: parts[0], Dest: parts[1]})
	}
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSUpload(cmd.Context(), BrowsersFSUploadInput{Identifier: args[0], Mappings: mappings, DestDir: destDir, Paths: paths})
}

func runBrowsersFSUploadZip(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	zipPath, _ := cmd.Flags().GetString("zip")
	destDir, _ := cmd.Flags().GetString("dest-dir")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSUploadZip(cmd.Context(), BrowsersFSUploadZipInput{Identifier: args[0], ZipPath: zipPath, DestDir: destDir})
}

func runBrowsersFSWriteFile(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	path, _ := cmd.Flags().GetString("path")
	mode, _ := cmd.Flags().GetString("mode")
	input, _ := cmd.Flags().GetString("source")
	b := BrowsersCmd{browsers: &svc, fs: &svc.Fs}
	return b.FSWriteFile(cmd.Context(), BrowsersFSWriteFileInput{Identifier: args[0], DestPath: path, Mode: mode, SourcePath: input})
}

func runBrowsersExtensionsUpload(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	b := BrowsersCmd{browsers: &svc}
	return b.ExtensionsUpload(cmd.Context(), BrowsersExtensionsUploadInput{Identifier: args[0], ExtensionPaths: args[1:]})
}

func runBrowsersComputerClickMouse(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	x, _ := cmd.Flags().GetInt64("x")
	y, _ := cmd.Flags().GetInt64("y")
	numClicks, _ := cmd.Flags().GetInt64("num-clicks")
	button, _ := cmd.Flags().GetString("button")
	clickType, _ := cmd.Flags().GetString("click-type")
	holdKeys, _ := cmd.Flags().GetStringSlice("hold-key")
	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerClickMouse(cmd.Context(), BrowsersComputerClickMouseInput{Identifier: args[0], X: x, Y: y, NumClicks: numClicks, Button: button, ClickType: clickType, HoldKeys: holdKeys})
}

func runBrowsersComputerMoveMouse(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	x, _ := cmd.Flags().GetInt64("x")
	y, _ := cmd.Flags().GetInt64("y")
	holdKeys, _ := cmd.Flags().GetStringSlice("hold-key")
	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerMoveMouse(cmd.Context(), BrowsersComputerMoveMouseInput{Identifier: args[0], X: x, Y: y, HoldKeys: holdKeys})
}

func runBrowsersComputerScreenshot(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	x, _ := cmd.Flags().GetInt64("x")
	y, _ := cmd.Flags().GetInt64("y")
	w, _ := cmd.Flags().GetInt64("width")
	h, _ := cmd.Flags().GetInt64("height")
	to, _ := cmd.Flags().GetString("to")
	bx := cmd.Flags().Changed("x")
	by := cmd.Flags().Changed("y")
	bw := cmd.Flags().Changed("width")
	bh := cmd.Flags().Changed("height")
	useRegion := bx || by || bw || bh
	if useRegion {
		if !(bx && by && bw && bh) {
			pterm.Error.Println("if specifying region, you must provide --x, --y, --width, and --height")
			return nil
		}
		if w <= 0 || h <= 0 {
			pterm.Error.Println("--width and --height must be greater than zero")
			return nil
		}
	}
	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerScreenshot(cmd.Context(), BrowsersComputerScreenshotInput{Identifier: args[0], X: x, Y: y, Width: w, Height: h, To: to, HasRegion: useRegion})
}

func runBrowsersComputerTypeText(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	text, _ := cmd.Flags().GetString("text")
	delay, _ := cmd.Flags().GetInt64("delay")
	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerTypeText(cmd.Context(), BrowsersComputerTypeTextInput{Identifier: args[0], Text: text, Delay: delay})
}

func runBrowsersComputerPressKey(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	keys, _ := cmd.Flags().GetStringSlice("key")
	duration, _ := cmd.Flags().GetInt64("duration")
	holdKeys, _ := cmd.Flags().GetStringSlice("hold-key")
	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerPressKey(cmd.Context(), BrowsersComputerPressKeyInput{Identifier: args[0], Keys: keys, Duration: duration, HoldKeys: holdKeys})
}

func runBrowsersComputerScroll(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	x, _ := cmd.Flags().GetInt64("x")
	y, _ := cmd.Flags().GetInt64("y")
	dx, _ := cmd.Flags().GetInt64("delta-x")
	dy, _ := cmd.Flags().GetInt64("delta-y")
	dxSet := cmd.Flags().Changed("delta-x")
	dySet := cmd.Flags().Changed("delta-y")
	holdKeys, _ := cmd.Flags().GetStringSlice("hold-key")
	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerScroll(cmd.Context(), BrowsersComputerScrollInput{Identifier: args[0], X: x, Y: y, DeltaX: dx, DeltaXSet: dxSet, DeltaY: dy, DeltaYSet: dySet, HoldKeys: holdKeys})
}

func runBrowsersComputerDragMouse(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	points, _ := cmd.Flags().GetStringArray("point")
	delay, _ := cmd.Flags().GetInt64("delay")
	stepDelayMs, _ := cmd.Flags().GetInt64("step-delay-ms")
	stepsPerSegment, _ := cmd.Flags().GetInt64("steps-per-segment")
	button, _ := cmd.Flags().GetString("button")
	holdKeys, _ := cmd.Flags().GetStringSlice("hold-key")

	// Parse points of form x,y into [][]int64
	var path [][]int64
	for _, p := range points {
		parts := strings.SplitN(p, ",", 2)
		if len(parts) != 2 {
			pterm.Error.Printf("invalid --point value: %s (expected x,y)\n", p)
			return nil
		}
		x, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			pterm.Error.Printf("invalid x in --point %s: %v\n", p, err)
			return nil
		}
		y, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			pterm.Error.Printf("invalid y in --point %s: %v\n", p, err)
			return nil
		}
		path = append(path, []int64{x, y})
	}

	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerDragMouse(cmd.Context(), BrowsersComputerDragMouseInput{Identifier: args[0], Path: path, Delay: delay, StepDelayMs: stepDelayMs, StepsPerSegment: stepsPerSegment, Button: button, HoldKeys: holdKeys})
}

func runBrowsersComputerSetCursor(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	hiddenStr, _ := cmd.Flags().GetString("hidden")

	var hidden bool
	switch strings.ToLower(hiddenStr) {
	case "true", "1", "yes":
		hidden = true
	case "false", "0", "no":
		hidden = false
	default:
		pterm.Error.Printf("Invalid value for --hidden: %s (expected true or false)\n", hiddenStr)
		return nil
	}

	b := BrowsersCmd{browsers: &svc, computer: &svc.Computer}
	return b.ComputerSetCursor(cmd.Context(), BrowsersComputerSetCursorInput{Identifier: args[0], Hidden: hidden})
}

func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	return url[:maxLen-3] + "..."
}
