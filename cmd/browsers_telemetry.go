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

// settableCategories are the categories accepted by --telemetry=<categories>.
// "system" is always-on and cannot be toggled, but is valid as a --categories stream filter.
var settableCategories = []string{"console", "interaction", "network", "page"}

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

func (b BrowsersCmd) TelemetryStream(ctx context.Context, in BrowsersTelemetryStreamInput) error {
	if err := validateJSONOutput(in.Output); err != nil {
		return err
	}
	for _, c := range in.Categories {
		if c != "system" && !slices.Contains(settableCategories, c) {
			return fmt.Errorf("unknown category %q: must be one of %s", c, strings.Join(append(settableCategories, "system"), ", "))
		}
	}
	if b.telemetry == nil {
		pterm.Error.Println("telemetry service not available")
		return nil
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
