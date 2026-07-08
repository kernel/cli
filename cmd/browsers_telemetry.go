package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// BrowserTelemetryService defines the subset we use for browser telemetry streaming.
type BrowserTelemetryService interface {
	StreamStreaming(ctx context.Context, id string, query kernel.BrowserTelemetryStreamParams, opts ...option.RequestOption) (stream *ssestream.Stream[kernel.BrowserTelemetryStreamResponse])
	Events(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], err error)
	EventsAutoPaging(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) *pagination.OffsetPaginationAutoPager[kernel.BrowserTelemetryEventsResponse]
}

type BrowsersTelemetryStreamInput struct {
	Identifier string
	Categories []string
	Types      []string
	Seq        int64
	Replay     string
	Output     string
}

type BrowsersTelemetryEventsInput struct {
	Identifier string
	Limit      int64
	Offset     int64
	Since      string
	Until      string
	Categories []string
	Types      []string
	All        bool
	Output     string
}

// parseTelemetryCategories parses a comma-separated list of category names to
// enable into a BrowserTelemetryCategoriesConfigParam. Selection is opt-in:
// only the listed categories are captured; everything else is off.
func parseTelemetryCategories(s string) (kernel.BrowserTelemetryCategoriesConfigParam, error) {
	p := kernel.BrowserTelemetryCategoriesConfigParam{}
	on := func() kernel.BrowserTelemetryCategoryConfigParam {
		return kernel.BrowserTelemetryCategoryConfigParam{Enabled: kernel.Opt(true)}
	}
	for _, part := range strings.Split(s, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		switch name {
		case "console":
			p.Console = on()
		case "network":
			p.Network = on()
		case "page":
			p.Page = on()
		case "interaction":
			p.Interaction = on()
		case "control":
			p.Control = on()
		case "connection":
			p.Connection = on()
		case "system":
			p.System = on()
		case "screenshot":
			p.Screenshot = on()
		case "captcha":
			p.Captcha = on()
		default:
			return p, fmt.Errorf("unknown category %q: must be one of %s", name, strings.Join(settableCategories, ", "))
		}
	}
	return p, nil
}

// buildNewTelemetryParam converts a --telemetry flag value to the create API param.
func buildNewTelemetryParam(s string) (kernel.BrowserNewParamsTelemetry, error) {
	switch s {
	case "all":
		return kernel.BrowserNewParamsTelemetry{Enabled: kernel.Opt(true)}, nil
	case "off":
		return kernel.BrowserNewParamsTelemetry{Enabled: kernel.Opt(false)}, nil
	default:
		p, err := parseTelemetryCategories(s)
		if err != nil {
			return kernel.BrowserNewParamsTelemetry{}, err
		}
		return kernel.BrowserNewParamsTelemetry{Browser: p}, nil
	}
}

// buildUpdateTelemetryParam converts a --telemetry flag value to the update API param.
func buildUpdateTelemetryParam(s string) (kernel.BrowserUpdateParamsTelemetry, error) {
	switch s {
	case "all":
		return kernel.BrowserUpdateParamsTelemetry{Enabled: kernel.Opt(true)}, nil
	case "off":
		return kernel.BrowserUpdateParamsTelemetry{Enabled: kernel.Opt(false)}, nil
	default:
		p, err := parseTelemetryCategories(s)
		if err != nil {
			return kernel.BrowserUpdateParamsTelemetry{}, err
		}
		return kernel.BrowserUpdateParamsTelemetry{Browser: p}, nil
	}
}

// settableCategories are the categories accepted by --telemetry=<categories>.
// The monitor category is not settable: it is collector-health metadata that
// flows automatically whenever a CDP category is captured.
var settableCategories = []string{
	"console", "network", "page", "interaction",
	"control", "connection", "system", "screenshot", "captcha",
}

// streamFilterCategories are the categories accepted by `telemetry stream --categories`.
// This is the full set of categories an event may carry, including the auto-managed monitor.
var streamFilterCategories = append(append([]string{}, settableCategories...), "monitor")

// telemetryEnabledCategories returns the categories captured by a session's
// telemetry config, in display order.
func telemetryEnabledCategories(cfg kernel.BrowserTelemetryConfig) []string {
	b := cfg.Browser
	ordered := []struct {
		name string
		on   bool
	}{
		{"console", b.Console.Enabled},
		{"network", b.Network.Enabled},
		{"page", b.Page.Enabled},
		{"interaction", b.Interaction.Enabled},
		{"control", b.Control.Enabled},
		{"connection", b.Connection.Enabled},
		{"system", b.System.Enabled},
		{"screenshot", b.Screenshot.Enabled},
		{"captcha", b.Captcha.Enabled},
	}
	on := make([]string, 0, len(ordered))
	for _, c := range ordered {
		if c.on {
			on = append(on, c.name)
		}
	}
	return on
}

// printTelemetrySummary echoes the categories telemetry will capture, so the
// effect of an opt-in selection is obvious after create/update.
func printTelemetrySummary(cfg kernel.BrowserTelemetryConfig) {
	on := telemetryEnabledCategories(cfg)
	if len(on) == 0 {
		pterm.Info.Println("Telemetry: disabled")
		return
	}
	pterm.Info.Printf("Telemetry capturing: %s\n", strings.Join(on, ", "))
}

// shouldEmit applies client-side category/type filters to a telemetry event.
func shouldEmit(category, eventType string, categories, types []string) bool {
	if len(categories) > 0 && !slices.Contains(categories, category) {
		return false
	}
	if len(types) > 0 && !slices.Contains(types, eventType) {
		return false
	}
	return true
}

func (b BrowsersCmd) TelemetryStream(ctx context.Context, in BrowsersTelemetryStreamInput) error {
	if b.telemetry == nil {
		return fmt.Errorf("telemetry service not available")
	}
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Seq != -1 && in.Seq < 1 {
		return fmt.Errorf("invalid --seq value %d: must be >= 1 (resumes after sequence N; omit --seq to stream from now)", in.Seq)
	}
	for _, c := range in.Categories {
		if !slices.Contains(streamFilterCategories, c) {
			return fmt.Errorf("invalid --categories value %q: must be one of %s", c, strings.Join(streamFilterCategories, ", "))
		}
	}
	if in.Replay != "" && in.Replay != "all" {
		return fmt.Errorf("invalid --replay value %q: only \"all\" is supported (omit --replay to stream from now)", in.Replay)
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	br, err := b.browsers.Get(ctx, in.Identifier, kernel.BrowserGetParams{})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserTelemetryStreamParams{}
	if in.Seq >= 0 {
		params.LastEventID = kernel.Opt(strconv.FormatInt(in.Seq, 10))
	}
	if in.Replay != "" {
		params.Replay = kernel.Opt(in.Replay)
	}
	stream := b.telemetry.StreamStreaming(ctx, br.SessionID, params)
	defer stream.Close()
	for stream.Next() {
		ev := stream.Current()
		cat := ev.Event.Category
		if !shouldEmit(cat, ev.Event.Type, in.Categories, in.Types) {
			continue
		}
		if in.Output == "json" {
			if err := util.PrintCompactJSONLine(ev); err != nil {
				return err
			}
			continue
		}
		ts := time.UnixMicro(ev.Event.Ts).Local().Format("2006-01-02 15:04:05")
		pterm.Printf("%s\t[%s]\t%s\n", ts, cat, ev.Event.Type)
	}
	if err := stream.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return util.CleanedUpSdkError{Err: err}
	}
	return nil
}

func runBrowsersTelemetryStream(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output")
	categories, _ := cmd.Flags().GetStringSlice("categories")
	types, _ := cmd.Flags().GetStringSlice("types")
	seq, _ := cmd.Flags().GetInt64("seq")
	replay, _ := cmd.Flags().GetString("replay")
	b := BrowsersCmd{browsers: &svc, telemetry: &svc.Telemetry}
	return b.TelemetryStream(cmd.Context(), BrowsersTelemetryStreamInput{
		Identifier: args[0],
		Categories: categories,
		Types:      types,
		Seq:        seq,
		Replay:     replay,
		Output:     out,
	})
}

func (b BrowsersCmd) TelemetryEvents(ctx context.Context, in BrowsersTelemetryEventsInput) error {
	if b.telemetry == nil {
		return fmt.Errorf("telemetry service not available")
	}
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	if in.Limit != 0 && (in.Limit < 1 || in.Limit > 100) {
		return fmt.Errorf("invalid --limit value %d: must be between 1 and 100", in.Limit)
	}
	for _, c := range in.Categories {
		if !slices.Contains(streamFilterCategories, c) {
			return fmt.Errorf("invalid --categories value %q: must be one of %s", c, strings.Join(streamFilterCategories, ", "))
		}
	}

	// Resolve a name to a session ID. The events archive outlives the session, so
	// a 404 (ended or unknown session) is not fatal: fall back to the identifier
	// as-is, since its archive may still be readable. Surface any other error.
	sessionID := in.Identifier
	if br, gerr := b.browsers.Get(ctx, in.Identifier, kernel.BrowserGetParams{}); gerr == nil {
		sessionID = br.SessionID
	} else if !util.IsNotFound(gerr) {
		return util.CleanedUpSdkError{Err: gerr}
	}

	// A --types filter is client-side (the archive endpoint filters only by
	// category), so it must see every page to be complete. Walk the whole window
	// whenever --all or a --types filter is set; otherwise read a single page and
	// surface the X-Next-Offset cursor for manual --offset paging.
	fullScan := in.All || len(in.Types) > 0

	params := kernel.BrowserTelemetryEventsParams{}
	if in.Limit > 0 {
		params.Limit = kernel.Opt(in.Limit)
	}
	if in.Offset > 0 && !fullScan {
		params.Offset = kernel.Opt(in.Offset)
	} else if in.Since != "" {
		// Offset is an opaque cursor that encodes the window start, so --since is
		// ignored once paging by offset; only send it for the first page. A full
		// scan ignores --offset entirely and walks the window from --since.
		params.Since = kernel.Opt(in.Since)
	}
	// --until still bounds the page even when paging by offset.
	if in.Until != "" {
		params.Until = kernel.Opt(in.Until)
	}
	// Send each category as a repeated query param. The SDK serializes a []string
	// field as a single comma-joined value, but the endpoint expects the parameter
	// repeated, so a comma-joined value matches no category.
	opts := make([]option.RequestOption, 0, len(in.Categories)+1)
	for _, c := range in.Categories {
		opts = append(opts, option.WithQueryAdd("category", c))
	}

	var items []kernel.BrowserTelemetryEventsResponse
	nextOffset := ""

	if fullScan {
		pager := b.telemetry.EventsAutoPaging(ctx, sessionID, params, opts...)
		for pager.Next() {
			it := pager.Current()
			if shouldEmit(it.Event.Category, it.Event.Type, nil, in.Types) {
				items = append(items, it)
			}
		}
		if err := pager.Err(); err != nil {
			return util.CleanedUpSdkError{Err: err}
		}
	} else {
		var raw *http.Response
		page, err := b.telemetry.Events(ctx, sessionID, params, append(opts, option.WithResponseInto(&raw))...)
		if err != nil {
			return util.CleanedUpSdkError{Err: err}
		}
		if page != nil {
			items = page.Items
		}
		// The API sets X-Has-More=true while more pages remain; X-Next-Offset is
		// the cursor to pass as --offset for the next page. Surface it (in JSON and
		// as the table hint) only when there is actually a next page.
		if raw != nil && strings.EqualFold(raw.Header.Get("X-Has-More"), "true") {
			nextOffset = raw.Header.Get("X-Next-Offset")
		}
	}

	if in.Output == "json" {
		events := make([]json.RawMessage, 0, len(items))
		for _, it := range items {
			r := it.RawJSON()
			if r == "" {
				r = "{}"
			}
			events = append(events, json.RawMessage(r))
		}
		payload := struct {
			Events     []json.RawMessage `json:"events"`
			NextOffset string            `json:"next_offset,omitempty"`
		}{Events: events, NextOffset: nextOffset}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(items) == 0 {
		pterm.Info.Println("No telemetry events found")
		return nil
	}

	rows := pterm.TableData{{"Seq", "Time", "Category", "Type"}}
	for _, it := range items {
		ts := time.UnixMicro(it.Event.Ts).Local().Format("2006-01-02 15:04:05")
		rows = append(rows, []string{
			strconv.FormatInt(it.Seq, 10),
			ts,
			it.Event.Category,
			it.Event.Type,
		})
	}
	PrintTableNoPad(rows, true)
	if nextOffset != "" {
		pterm.Info.Printf("More events available — re-run with --offset %s\n", nextOffset)
	}
	return nil
}

func runBrowsersTelemetryEvents(cmd *cobra.Command, args []string) error {
	client := getKernelClient(cmd)
	svc := client.Browsers
	out, _ := cmd.Flags().GetString("output")
	limit, _ := cmd.Flags().GetInt64("limit")
	offset, _ := cmd.Flags().GetInt64("offset")
	since, _ := cmd.Flags().GetString("since")
	until, _ := cmd.Flags().GetString("until")
	categories, _ := cmd.Flags().GetStringSlice("categories")
	types, _ := cmd.Flags().GetStringSlice("types")
	all, _ := cmd.Flags().GetBool("all")
	b := BrowsersCmd{browsers: &svc, telemetry: &svc.Telemetry}
	return b.TelemetryEvents(cmd.Context(), BrowsersTelemetryEventsInput{
		Identifier: args[0],
		Limit:      limit,
		Offset:     offset,
		Since:      since,
		Until:      until,
		Categories: categories,
		Types:      types,
		All:        all,
		Output:     out,
	})
}
