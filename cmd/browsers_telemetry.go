package cmd

import (
	"context"
	"errors"
	"fmt"
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
	b := BrowsersCmd{browsers: &svc, telemetry: &svc.Telemetry}
	return b.TelemetryStream(cmd.Context(), BrowsersTelemetryStreamInput{
		Identifier: args[0],
		Categories: categories,
		Types:      types,
		Seq:        seq,
		Output:     out,
	})
}
