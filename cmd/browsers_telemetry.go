package cmd

import (
	"context"
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

type BrowsersTelemetryStreamInput struct {
	Identifier string
	Categories []string
	Types      []string
	Seq        int64
	Output     string
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

// buildTelemetryParam converts a --telemetry flag value to the API param.
func buildTelemetryParam(s string) (kernel.BrowserTelemetryRequestConfigParam, error) {
	switch s {
	case "all":
		return kernel.BrowserTelemetryRequestConfigParam{Enabled: kernel.Opt(true)}, nil
	case "off":
		return kernel.BrowserTelemetryRequestConfigParam{Enabled: kernel.Opt(false)}, nil
	default:
		p, err := parseTelemetryCategories(s)
		if err != nil {
			return kernel.BrowserTelemetryRequestConfigParam{}, err
		}
		return kernel.BrowserTelemetryRequestConfigParam{Enabled: kernel.Opt(true), Browser: p}, nil
	}
}

// settableCategories are the categories accepted by --telemetry=<categories>.
// "system" is always-on and cannot be toggled, but is valid as a --categories stream filter.
var settableCategories = []string{"console", "interaction", "network", "page"}

// eventCategory derives the category from the event type prefix.
// "monitor_*" maps to "system"; all others use the prefix before the first "_".
// TODO(sdk): kernel-go-sdk should surface Category directly on BrowserTelemetryEventUnion.
func eventCategory(ev kernel.BrowserTelemetryEventUnion) string {
	prefix, _, ok := strings.Cut(ev.Type, "_")
	if !ok {
		return ev.Type
	}
	if prefix == "monitor" {
		return "system"
	}
	return prefix
}

// shouldEmit applies client-side category/type filters to a telemetry event.
func shouldEmit(ev kernel.BrowserTelemetryEventUnion, categories, types []string) bool {
	if len(categories) > 0 && !slices.Contains(categories, eventCategory(ev)) {
		return false
	}
	if len(types) > 0 && !slices.Contains(types, ev.Type) {
		return false
	}
	return true
}

func (b BrowsersCmd) TelemetryStream(ctx context.Context, in BrowsersTelemetryStreamInput) error {
	if b.telemetry == nil {
		return fmt.Errorf("telemetry service not available")
	}
	if in.Output != "" && in.Output != "json" {
		return fmt.Errorf("unsupported --output value: use 'json'")
	}
	if in.Seq < -1 {
		return fmt.Errorf("--seq must be >= 0 (use --seq=0 to resume from the beginning, or omit to stream from now)")
	}
	br, err := b.browsers.Get(ctx, in.Identifier, kernel.BrowserGetParams{})
	if err != nil {
		return util.CleanedUpSdkError{Err: err}
	}
	params := kernel.BrowserTelemetryStreamParams{}
	if in.Seq >= 0 {
		params.LastEventID = kernel.Opt(strconv.FormatInt(in.Seq, 10))
	}
	stream := b.telemetry.StreamStreaming(ctx, br.SessionID, params)
	defer stream.Close()
	for stream.Next() {
		ev := stream.Current()
		cat := eventCategory(ev.Event)
		if len(in.Categories) > 0 && !slices.Contains(in.Categories, cat) {
			continue
		}
		if len(in.Types) > 0 && !slices.Contains(in.Types, ev.Event.Type) {
			continue
		}
		if in.Output == "json" {
			if err := util.PrintCompactJSONLine(ev); err != nil {
				return err
			}
			continue
		}
		ts := time.UnixMicro(ev.Event.Ts).Local().Format("15:04:05")
		pterm.Printf("%s\t[%s]\t%s\n", ts, cat, ev.Event.Type)
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
	telemetryStream.Flags().Int64("seq", -1, "Resume stream from sequence number (Last-Event-ID); 0 means from the beginning")
	telemetryStream.Flags().StringP("output", "o", "", "Output format: json for newline-delimited JSON envelopes")
	telemetryRoot.AddCommand(telemetryStream)
	browsersCmd.AddCommand(telemetryRoot)
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
