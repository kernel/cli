package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// BrowserTelemetryService defines the subset we use for browser telemetry streaming.
type BrowserTelemetryService interface {
	StreamStreaming(ctx context.Context, id string, query kernel.BrowserTelemetryStreamParams, opts ...option.RequestOption) (stream *ssestream.Stream[kernel.BrowserTelemetryStreamResponse])
}

type BrowsersTelemetryStartInput struct {
	Identifier string
	Output     string
}

type BrowsersTelemetryStopInput struct {
	Identifier string
	Output     string
}

type BrowsersTelemetrySetInput struct {
	Identifier string
	Categories string // e.g. "network=on,page=off"
	Output     string
}

type BrowsersTelemetryStatusInput struct {
	Identifier string
	Output     string
}

type BrowsersTelemetryStreamInput struct {
	Identifier string
	Categories []string
	Types      []string
	Seq        int64
	Output     string
}

func validateJSONOutput(out string) error {
	if out != "" && out != "json" {
		return errors.New("unsupported --output value: use 'json'")
	}
	return nil
}

func (b BrowsersCmd) TelemetryStart(ctx context.Context, in BrowsersTelemetryStartInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	res, err := b.browsers.Update(ctx, in.Identifier, kernel.BrowserUpdateParams{
		Telemetry: kernel.BrowserTelemetryRequestConfigParam{Enabled: kernel.Opt(true)},
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(res)
	}
	pterm.Success.Printf("Started telemetry for browser %s\n", in.Identifier)
	return nil
}

func (b BrowsersCmd) TelemetryStop(ctx context.Context, in BrowsersTelemetryStopInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	res, err := b.browsers.Update(ctx, in.Identifier, kernel.BrowserUpdateParams{
		Telemetry: kernel.BrowserTelemetryRequestConfigParam{Enabled: kernel.Opt(false)},
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(res)
	}
	pterm.Success.Printf("Stopped telemetry for browser %s\n", in.Identifier)
	return nil
}

// parseTelemetryCategories parses a comma-separated "name=on|off" string into
// a BrowserTelemetryCategoriesConfigParam. Unmentioned categories are omitted.
func parseTelemetryCategories(s string) (kernel.BrowserTelemetryCategoriesConfigParam, error) {
	p := kernel.BrowserTelemetryCategoriesConfigParam{}
	for _, part := range strings.Split(s, ",") {
		name, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return p, fmt.Errorf("invalid category assignment %q: expected name=on or name=off", part)
		}
		name, val = strings.TrimSpace(name), strings.TrimSpace(val)
		var enabled bool
		switch val {
		case "on":
			enabled = true
		case "off":
			enabled = false
		default:
			return p, fmt.Errorf("invalid value %q for category %q: must be 'on' or 'off'", val, name)
		}
		switch name {
		case "console":
			p.Console = kernel.BrowserTelemetryCategoryConfigParam{Enabled: kernel.Opt(enabled)}
		case "interaction":
			p.Interaction = kernel.BrowserTelemetryCategoryConfigParam{Enabled: kernel.Opt(enabled)}
		case "network":
			p.Network = kernel.BrowserTelemetryCategoryConfigParam{Enabled: kernel.Opt(enabled)}
		case "page":
			p.Page = kernel.BrowserTelemetryCategoryConfigParam{Enabled: kernel.Opt(enabled)}
		default:
			return p, fmt.Errorf("unknown category %q: must be one of %s", name, strings.Join(settableCategories, ", "))
		}
	}
	return p, nil
}

func (b BrowsersCmd) TelemetrySet(ctx context.Context, in BrowsersTelemetrySetInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	p, err := parseTelemetryCategories(in.Categories)
	if err != nil {
		return err
	}
	res, err := b.browsers.Update(ctx, in.Identifier, kernel.BrowserUpdateParams{
		Telemetry: kernel.BrowserTelemetryRequestConfigParam{Browser: p},
	})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(res)
	}
	pterm.Success.Printf("Updated telemetry categories for browser %s\n", in.Identifier)
	return nil
}

func (b BrowsersCmd) TelemetryStatus(ctx context.Context, in BrowsersTelemetryStatusInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	browser, err := b.browsers.Get(ctx, in.Identifier, kernel.BrowserGetParams{})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	if in.Output == "json" {
		return util.PrintPrettyJSON(browser.Telemetry)
	}
	if !browser.Telemetry.JSON.Browser.Valid() {
		pterm.Println("enabled:     off")
		return nil
	}
	pterm.Println("enabled:     on")
	cfg := browser.Telemetry.Browser
	pterm.Printf("console:     %s\n", categoryOnOff(cfg.Console))
	pterm.Printf("interaction: %s\n", categoryOnOff(cfg.Interaction))
	pterm.Printf("network:     %s\n", categoryOnOff(cfg.Network))
	pterm.Printf("page:        %s\n", categoryOnOff(cfg.Page))
	return nil
}

// categoryOnOff returns "on" or "off" for a category config, respecting the SDK
// default: if the enabled field is absent from the response, it defaults to true.
func categoryOnOff(c kernel.BrowserTelemetryCategoryConfig) string {
	if !c.JSON.Enabled.Valid() {
		return "on"
	}
	if c.Enabled {
		return "on"
	}
	return "off"
}

// eventCategoryFromRaw reads the category field directly from the raw event JSON.
// Returns "" if the field is absent — callers that need a category must handle the empty case.
func eventCategoryFromRaw(ev kernel.BrowserTelemetryEventUnion) string {
	var obj struct {
		Category string `json:"category"`
	}
	if raw := ev.RawJSON(); raw != "" {
		if err := json.Unmarshal([]byte(raw), &obj); err == nil {
			return obj.Category
		}
	}
	return ""
}

// shouldEmit applies client-side category/type filters to a telemetry event.
func shouldEmit(ev kernel.BrowserTelemetryEventUnion, categories, types []string) bool {
	if len(categories) > 0 && !slices.Contains(categories, eventCategoryFromRaw(ev)) {
		return false
	}
	if len(types) > 0 && !slices.Contains(types, ev.Type) {
		return false
	}
	return true
}

// settableCategories are the categories accepted by the set subcommand.
var settableCategories = []string{"console", "interaction", "network", "page"}

// knownTelemetryCategories are the real API event categories observable on stream.
var knownTelemetryCategories = []string{"console", "network", "page", "interaction", "system"}

var knownTelemetryTypes = []string{
	"console_log", "console_error",
	"network_request", "network_response", "network_loading_failed", "network_idle",
	"page_navigation", "page_dom_content_loaded", "page_load", "page_tab_opened",
	"page_layout_shift", "page_lcp", "page_layout_settled", "page_navigation_settled",
	"interaction_click", "interaction_key", "interaction_scroll_settled",
	"monitor_screenshot", "monitor_disconnected", "monitor_reconnected",
	"monitor_reconnect_failed", "monitor_init_failed",
}

func (b BrowsersCmd) TelemetryStream(ctx context.Context, in BrowsersTelemetryStreamInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	for _, c := range in.Categories {
		if !slices.Contains(knownTelemetryCategories, c) {
			return fmt.Errorf("unknown category %q: must be one of %s", c, strings.Join(knownTelemetryCategories, ", "))
		}
	}
	for _, t := range in.Types {
		if !slices.Contains(knownTelemetryTypes, t) {
			pterm.Warning.Printf("unrecognized event type %q — no events will match\n", t)
		}
	}
	br, err := b.browsers.Get(ctx, in.Identifier, kernel.BrowserGetParams{})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserTelemetryStreamParams{}
	if in.Seq > 0 {
		params.LastEventID = kernel.Opt(strconv.FormatInt(in.Seq, 10))
	}
	stream := b.telemetry.StreamStreaming(ctx, br.SessionID, params)
	defer stream.Close()
	for stream.Next() {
		ev := stream.Current()
		if !shouldEmit(ev.Event, in.Categories, in.Types) {
			continue
		}
		if in.Output == "json" {
			_ = util.PrintCompactJSONLine(ev)
			continue
		}
		ts := time.UnixMicro(ev.Event.Ts).Local().Format("15:04:05")
		pterm.Printf("%s  [%s]  %s\n", ts, eventCategoryFromRaw(ev.Event), ev.Event.Type)
	}
	if err := stream.Err(); err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	return nil
}

func init() {
	// browsersCmd is a package-level var (browsers.go), initialized before init() runs.
	telemetryRoot := &cobra.Command{Use: "telemetry", Short: "Browser telemetry operations"}
	telemetryStream := &cobra.Command{Use: "stream <id>", Short: "Stream live telemetry events", Args: cobra.ExactArgs(1), RunE: runBrowsersTelemetryStream}
	telemetryStream.Flags().StringSlice("categories", []string{}, "Filter by API event category (console,network,page,interaction,system); system covers all monitor_* events")
	telemetryStream.Flags().StringSlice("types", []string{}, "Filter by event type (e.g. network_response,console_error)")
	telemetryStream.Flags().Int64("seq", 0, "Resume stream from sequence number (Last-Event-ID)")
	telemetryStream.Flags().StringP("output", "o", "", "Output format: json for newline-delimited JSON envelopes")
	telemetryStart := &cobra.Command{Use: "start <id>", Short: "Start telemetry capture (enabled: true)", Args: cobra.ExactArgs(1), RunE: runBrowsersTelemetryStart}
	telemetryStart.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	telemetryStop := &cobra.Command{Use: "stop <id>", Short: "Stop telemetry capture (enabled: false)", Args: cobra.ExactArgs(1), RunE: runBrowsersTelemetryStop}
	telemetryStop.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	telemetrySet := &cobra.Command{Use: "set <id> <name=on|off>...", Short: "Set per-category telemetry config", Args: cobra.MinimumNArgs(2), RunE: runBrowsersTelemetrySet}
	telemetrySet.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	telemetryStatus := &cobra.Command{Use: "status <id>", Short: "Show current telemetry configuration", Args: cobra.ExactArgs(1), RunE: runBrowsersTelemetryStatus}
	telemetryStatus.Flags().StringP("output", "o", "", "Output format: json for raw API response")
	telemetryRoot.AddCommand(telemetryStream, telemetryStart, telemetryStop, telemetrySet, telemetryStatus)
	browsersCmd.AddCommand(telemetryRoot)
}

func runBrowsersTelemetryStart(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc}
	return b.TelemetryStart(cmd.Context(), BrowsersTelemetryStartInput{Identifier: args[0], Output: out})
}

func runBrowsersTelemetryStop(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc}
	return b.TelemetryStop(cmd.Context(), BrowsersTelemetryStopInput{Identifier: args[0], Output: out})
}

func runBrowsersTelemetrySet(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc}
	return b.TelemetrySet(cmd.Context(), BrowsersTelemetrySetInput{Identifier: args[0], Categories: strings.Join(args[1:], ","), Output: out})
}

func runBrowsersTelemetryStatus(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output")
	b := BrowsersCmd{browsers: &svc}
	return b.TelemetryStatus(cmd.Context(), BrowsersTelemetryStatusInput{Identifier: args[0], Output: out})
}

func runBrowsersTelemetryStream(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output")
	categories, _ := cmd.Flags().GetStringSlice("categories")
	types, _ := cmd.Flags().GetStringSlice("types")
	seq, _ := cmd.Flags().GetInt64("seq")
	b := BrowsersCmd{browsers: &svc, telemetry: &svc.Telemetry}
	return b.TelemetryStream(cmd.Context(), BrowsersTelemetryStreamInput{
		Identifier: args[0],
		Categories: categories,
		Types:      types,
		Seq:        seq,
		Output:     out,
	})
}
