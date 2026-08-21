package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/stretchr/testify/assert"
)

// captureStdout redirects os.Stdout for the duration of the test and returns
// the captured output. Needed for paths that use fmt.Println rather than pterm.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

type FakeBrowserTelemetryService struct {
	StreamFunc           func() *ssestream.Stream[kernel.BrowserTelemetryStreamResponse]
	EventsFunc           func(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error)
	EventsAutoPagingFunc func(id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) *pagination.OffsetPaginationAutoPager[kernel.BrowserTelemetryEventsResponse]
}

func (f *FakeBrowserTelemetryService) StreamStreaming(ctx context.Context, id string, query kernel.BrowserTelemetryStreamParams, opts ...option.RequestOption) *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
	if f.StreamFunc != nil {
		return f.StreamFunc()
	}
	return makeStream([]kernel.BrowserTelemetryStreamResponse{})
}

func (f *FakeBrowserTelemetryService) Events(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error) {
	if f.EventsFunc != nil {
		return f.EventsFunc(ctx, id, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{}, nil
}

func (f *FakeBrowserTelemetryService) EventsAutoPaging(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) *pagination.OffsetPaginationAutoPager[kernel.BrowserTelemetryEventsResponse] {
	if f.EventsAutoPagingFunc != nil {
		return f.EventsAutoPagingFunc(id, query, opts...)
	}
	return pagination.NewOffsetPaginationAutoPager(&pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{}, nil)
}

func TestTelemetryStream_NilTelemetryErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "telemetry service not available")
}

func TestTelemetryStream_ContextCanceledExitsCleanly(t *testing.T) {
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{StreamFunc: func() *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
		return ssestream.NewStream[kernel.BrowserTelemetryStreamResponse](&testDecoder{}, context.Canceled)
	}}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Seq:        -1,
	})

	assert.NoError(t, err)
}

func TestTelemetryStream_NegativeSeqErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}, telemetry: &FakeBrowserTelemetryService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Seq:        -2,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --seq value -2")
}

func TestTelemetryStream_UnsupportedOutputErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}, telemetry: &FakeBrowserTelemetryService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Output:     "yaml",
		Seq:        -1,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestTelemetryStream_UnknownCategoryErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}, telemetry: &FakeBrowserTelemetryService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Categories: []string{"netowrk"},
		Seq:        -1,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --categories value")
}

func TestTelemetryStream_SystemCategoryAccepted(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	b := BrowsersCmd{browsers: fake, telemetry: &FakeBrowserTelemetryService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Categories: []string{"system"},
		Seq:        -1,
	})

	assert.NoError(t, err)
}

func TestTelemetryStream_EventsFlow(t *testing.T) {
	setupStdoutCapture(t)
	event := kernel.BrowserTelemetryStreamResponse{}
	if err := json.Unmarshal([]byte(`{"event":{"type":"network_response","ts":1000000}}`), &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{StreamFunc: func() *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
		return makeStream([]kernel.BrowserTelemetryStreamResponse{event})
	}}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Seq:        -1,
	})

	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), "network_response")
}

func TestTelemetryStream_EventsFlow_JSON(t *testing.T) {
	event := kernel.BrowserTelemetryStreamResponse{}
	if err := json.Unmarshal([]byte(`{"event":{"type":"network_response","ts":1000000}}`), &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{StreamFunc: func() *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
		return makeStream([]kernel.BrowserTelemetryStreamResponse{event})
	}}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	var err error
	out := captureStdout(t, func() {
		err = b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
			Identifier: "session123",
			Output:     "json",
			Seq:        -1,
		})
	})

	assert.NoError(t, err)
	assert.Contains(t, out, "network_response")
}

func TestTelemetryStream_FiltersDropNonMatchingEvents(t *testing.T) {
	setupStdoutCapture(t)
	consoleEvent := kernel.BrowserTelemetryStreamResponse{}
	if err := json.Unmarshal([]byte(`{"event":{"type":"console_log","category":"console","ts":1000000}}`), &consoleEvent); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	networkEvent := kernel.BrowserTelemetryStreamResponse{}
	if err := json.Unmarshal([]byte(`{"event":{"type":"network_response","category":"network","ts":2000000}}`), &networkEvent); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{StreamFunc: func() *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
		return makeStream([]kernel.BrowserTelemetryStreamResponse{consoleEvent, networkEvent})
	}}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Categories: []string{"network"},
		Seq:        -1,
	})

	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), "network_response")
	assert.NotContains(t, outBuf.String(), "console_log")
}

func TestTelemetryStream_TypesFilterDropsNonMatching(t *testing.T) {
	setupStdoutCapture(t)
	req := kernel.BrowserTelemetryStreamResponse{}
	if err := json.Unmarshal([]byte(`{"event":{"type":"network_request","category":"network","ts":1000000}}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resp := kernel.BrowserTelemetryStreamResponse{}
	if err := json.Unmarshal([]byte(`{"event":{"type":"network_response","category":"network","ts":2000000}}`), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{StreamFunc: func() *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
		return makeStream([]kernel.BrowserTelemetryStreamResponse{req, resp})
	}}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Types:      []string{"network_response"},
		Seq:        -1,
	})

	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), "network_response")
	assert.NotContains(t, outBuf.String(), "network_request")
}

func TestTelemetryStream_SeqZeroErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}, telemetry: &FakeBrowserTelemetryService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Seq:        0,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --seq value 0")
	assert.Contains(t, err.Error(), "must be >= 1")
}

func makeEvent(t *testing.T, raw string) kernel.BrowserTelemetryEventUnion {
	t.Helper()
	var ev kernel.BrowserTelemetryEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("makeEvent: %v", err)
	}
	return ev
}

func TestShouldEmit(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		categories []string
		types      []string
		want       bool
	}{
		{"no filters passes", `{"type":"network_response","category":"network","ts":0}`, nil, nil, true},
		{"matching category passes", `{"type":"network_response","category":"network","ts":0}`, []string{"network"}, nil, true},
		{"non-matching category drops", `{"type":"console_log","category":"console","ts":0}`, []string{"network"}, nil, false},
		{"monitor category matches monitor_disconnected", `{"type":"monitor_disconnected","category":"monitor","ts":0}`, []string{"monitor"}, nil, true},
		{"connection category matches cdp_connect", `{"type":"cdp_connect","category":"connection","ts":0}`, []string{"connection"}, nil, true},
		{"matching type passes", `{"type":"console_log","category":"console","ts":0}`, nil, []string{"console_log"}, true},
		{"non-matching type drops", `{"type":"network_response","category":"network","ts":0}`, nil, []string{"console_log"}, false},
		{"both filters pass when both match", `{"type":"network_response","category":"network","ts":0}`, []string{"network"}, []string{"network_response"}, true},
		{"both filters drop when type misses", `{"type":"network_response","category":"network","ts":0}`, []string{"network"}, []string{"console_log"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := makeEvent(t, tc.raw)
			assert.Equal(t, tc.want, shouldEmit(ev.Category, ev.Type, tc.categories, tc.types))
		})
	}
}

func TestParseTelemetryCategories_OptInList(t *testing.T) {
	p, err := parseTelemetryCategories("network,control,captcha,platform")

	assert.NoError(t, err)
	// Listed categories are enabled.
	for _, c := range []kernel.BrowserTelemetryCategoryConfigParam{p.Network, p.Captcha, p.Platform} {
		assert.True(t, c.Enabled.Valid())
		assert.True(t, c.Enabled.Value)
	}
	// Control carries its own config type, so it is checked separately.
	assert.True(t, p.Control.Enabled.Valid())
	assert.True(t, p.Control.Enabled.Value)
	// Unlisted categories are omitted (opt-in: the instance treats them as off).
	assert.False(t, p.Console.Enabled.Valid())
	assert.False(t, p.Page.Enabled.Valid())
	assert.False(t, p.Screenshot.Enabled.Valid())
}

func TestParseTelemetryCategories_InvalidCategory(t *testing.T) {
	_, err := parseTelemetryCategories("foo")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
}

func TestParseTelemetryCategories_WhitespaceTolerance(t *testing.T) {
	p, err := parseTelemetryCategories(" network , page ")

	assert.NoError(t, err)
	assert.True(t, p.Network.Enabled.Valid())
	assert.True(t, p.Network.Enabled.Value)
	assert.True(t, p.Page.Enabled.Valid())
	assert.True(t, p.Page.Enabled.Value)
}

// TestBuildTelemetryParam_WireEncoding locks in the three wire shapes the API
// expects: "all" sets Enabled=true without Browser (default set), "off" sets
// Enabled=false without Browser, and an opt-in list sets only Browser with the
// listed categories enabled (Enabled unset).
func TestBuildTelemetryParam_WireEncoding(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		p, err := buildNewTelemetryParam("all", "", "")
		assert.NoError(t, err)
		assert.True(t, p.Enabled.Valid())
		assert.True(t, p.Enabled.Value)
		assert.False(t, p.Browser.Network.Enabled.Valid())
	})
	t.Run("off", func(t *testing.T) {
		p, err := buildNewTelemetryParam("off", "", "")
		assert.NoError(t, err)
		assert.True(t, p.Enabled.Valid())
		assert.False(t, p.Enabled.Value)
		assert.False(t, p.Browser.Network.Enabled.Valid())
	})
	t.Run("opt-in list sets only Browser", func(t *testing.T) {
		p, err := buildNewTelemetryParam("network,control", "", "")
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Valid(), "Enabled must be unset for an opt-in selection")
		assert.True(t, p.Browser.Network.Enabled.Valid())
		assert.True(t, p.Browser.Network.Enabled.Value)
		assert.True(t, p.Browser.Control.Enabled.Valid())
		assert.True(t, p.Browser.Control.Enabled.Value)
	})
}

// TestBuildTelemetryParam_ExportWireEncoding locks in the OTLP export wire shapes.
// A destination is sent as id or name (never both) and implies capture on create;
// "off" sends enabled=false with no destination, since the API rejects
// enabled=false combined with one.
func TestBuildTelemetryParam_ExportWireEncoding(t *testing.T) {
	t.Run("destination by CUID sets id", func(t *testing.T) {
		p, err := buildNewTelemetryParam("", "", "abcdefghijklmnopqrstuvwx")
		assert.NoError(t, err)
		otlp := p.Export.Otlp
		assert.True(t, otlp.Destination.ID.Valid())
		assert.Equal(t, "abcdefghijklmnopqrstuvwx", otlp.Destination.ID.Value)
		assert.False(t, otlp.Destination.Name.Valid(), "name must be unset when id is sent")
		assert.False(t, otlp.Enabled.Valid(), "a destination implies enabled server-side")
	})
	t.Run("destination by name sets name", func(t *testing.T) {
		p, err := buildNewTelemetryParam("", "", "my-collector")
		assert.NoError(t, err)
		otlp := p.Export.Otlp
		assert.True(t, otlp.Destination.Name.Valid())
		assert.Equal(t, "my-collector", otlp.Destination.Name.Value)
		assert.False(t, otlp.Destination.ID.Valid(), "id must be unset when name is sent")
	})
	t.Run("destination implies capture on create", func(t *testing.T) {
		p, err := buildNewTelemetryParam("", "", "my-collector")
		assert.NoError(t, err)
		assert.True(t, p.Enabled.Valid(), "export requires capture, so create implies it")
		assert.True(t, p.Enabled.Value)
	})
	t.Run("explicit --telemetry selection is preserved", func(t *testing.T) {
		p, err := buildNewTelemetryParam("network,control", "", "my-collector")
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Valid(), "an opt-in selection must not be overridden")
		assert.True(t, p.Browser.Network.Enabled.Value)
		assert.Equal(t, "my-collector", p.Export.Otlp.Destination.Name.Value)
	})
	t.Run("off disables export without a destination", func(t *testing.T) {
		p, err := buildNewTelemetryParam("all", "", "off")
		assert.NoError(t, err)
		otlp := p.Export.Otlp
		assert.True(t, otlp.Enabled.Valid())
		assert.False(t, otlp.Enabled.Value)
		assert.False(t, otlp.Destination.ID.Valid())
		assert.False(t, otlp.Destination.Name.Valid())
	})
	t.Run("off does not imply capture", func(t *testing.T) {
		p, err := buildNewTelemetryParam("", "", "off")
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Valid(), "disabling export must not turn capture on")
	})
	// The API validates the request payload on its own rather than consulting the
	// stored config, so a destination needs a capture-enabling --telemetry in the
	// same request. Update and login refuse to supply one: doing so would replace
	// the connection's current category selection.
	t.Run("update requires an explicit --telemetry alongside a destination", func(t *testing.T) {
		_, err := buildManagedAuthTelemetryParam("", "", "my-collector", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "also requires --telemetry")
	})
	t.Run("login requires an explicit --telemetry alongside a destination", func(t *testing.T) {
		_, err := buildManagedAuthTelemetryParam("", "", "my-collector", false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "also requires --telemetry")
	})
	t.Run("update never implies capture when --telemetry is given", func(t *testing.T) {
		p, err := buildManagedAuthTelemetryParam("console", "", "my-collector", false)
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Valid(), "an opt-in selection must not be widened to the default set")
		assert.True(t, p.Browser.Console.Enabled.Value)
		assert.Equal(t, "my-collector", p.Export.Otlp.Destination.Name.Value)
	})
	t.Run("login never implies capture when --telemetry is given", func(t *testing.T) {
		p, err := buildManagedAuthTelemetryParam("console", "", "my-collector", false)
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Valid())
		assert.Equal(t, "my-collector", p.Export.Otlp.Destination.Name.Value)
	})
	t.Run("update and login allow export=off without --telemetry", func(t *testing.T) {
		u, err := buildManagedAuthTelemetryParam("", "", "off", false)
		assert.NoError(t, err)
		assert.False(t, u.Export.Otlp.Enabled.Value)
		l, err := buildManagedAuthTelemetryParam("", "", "off", false)
		assert.NoError(t, err)
		assert.False(t, l.Export.Otlp.Enabled.Value)
	})
	t.Run("auth connection create implies capture", func(t *testing.T) {
		p, err := buildManagedAuthTelemetryParam("", "", "my-collector", true)
		assert.NoError(t, err)
		assert.True(t, p.Enabled.Valid())
		assert.True(t, p.Enabled.Value)
	})
	t.Run("invalid category still errors with export set", func(t *testing.T) {
		_, err := buildNewTelemetryParam("bogus", "", "my-collector")
		assert.Error(t, err)
	})
	t.Run("telemetry=off with a destination is rejected", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			fn   func() error
		}{
			{"create", func() error { _, e := buildNewTelemetryParam("off", "", "my-collector"); return e }},
			{"auth create", func() error { _, e := buildManagedAuthTelemetryParam("off", "", "my-collector", true); return e }},
			{"auth update/login", func() error { _, e := buildManagedAuthTelemetryParam("off", "", "my-collector", false); return e }},
		} {
			err := tc.fn()
			assert.Error(t, err, tc.name)
			assert.Contains(t, err.Error(), "requires telemetry capture", tc.name)
		}
	})
	t.Run("telemetry=off with export=off is allowed", func(t *testing.T) {
		p, err := buildNewTelemetryParam("off", "", "off")
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Value)
		assert.False(t, p.Export.Otlp.Enabled.Value)
	})
	t.Run("empty export value errors", func(t *testing.T) {
		_, err := buildNewTelemetryParam("all", "", "   ")
		assert.Error(t, err)
	})
}

// TestPrintTelemetrySummary_Export covers the browser-session shape, where the
// resolved config reports destination as the ID string (not the {id, name} object
// an auth connection echoes back).
func TestPrintTelemetrySummary_Export(t *testing.T) {
	parse := func(raw string) kernel.BrowserTelemetryConfig {
		var cfg kernel.BrowserTelemetryConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return cfg
	}
	t.Run("reports the destination", func(t *testing.T) {
		setupStdoutCapture(t)
		printTelemetrySummary(parse(`{"browser":{"control":{"enabled":true}},"export":{"otlp":{"enabled":true,"destination":"dest-abc"}}}`))
		assert.Contains(t, outBuf.String(), "OTLP to: dest-abc")
	})
	t.Run("stays quiet when export is off", func(t *testing.T) {
		setupStdoutCapture(t)
		printTelemetrySummary(parse(`{"browser":{"control":{"enabled":true}},"export":{"otlp":{"enabled":false}}}`))
		assert.NotContains(t, outBuf.String(), "OTLP")
	})
}

func TestTelemetryEnabledCategories(t *testing.T) {
	var cfg kernel.BrowserTelemetryConfig
	raw := `{"browser":{"control":{"enabled":true},"system":{"enabled":true},"network":{"enabled":false}}}`
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assert.Equal(t, []string{"control", "system"}, telemetryEnabledCategories(cfg))
}

func TestTelemetryStream_RejectsInvalidReplay(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}, telemetry: &FakeBrowserTelemetryService{}}
	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{Identifier: "br-1", Seq: -1, Replay: "oldest"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --replay")
}

func TestTelemetryEvents_Table(t *testing.T) {
	buf := capturePtermOutput(t)
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: "sess-1"}, nil
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{
		EventsFunc: func(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error) {
			assert.Equal(t, "sess-1", id, "events should query the resolved session id")
			return &pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{
				Items: []kernel.BrowserTelemetryEventsResponse{
					{Seq: 7, Event: kernel.BrowserTelemetryEventUnion{Category: "network", Type: "network_response", Ts: 0}},
				},
			}, nil
		},
	}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}
	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1"})
	assert.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "7")
	assert.Contains(t, out, "network")
	assert.Contains(t, out, "network_response")
}

func TestTelemetryEvents_EmptyJSON(t *testing.T) {
	out := captureStdout(t, func() {
		fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return &kernel.BrowserGetResponse{SessionID: "sess-1"}, nil
		}}
		b := BrowsersCmd{browsers: fakeBrowsers, telemetry: &FakeBrowserTelemetryService{}}
		err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1", Output: "json"})
		assert.NoError(t, err)
	})
	// JSON mode emits an envelope so scripted callers can read the pagination
	// cursor; with no events and no next page it is just an empty events list.
	assert.JSONEq(t, `{"events":[]}`, out)
}

func TestTelemetryEvents_OffsetIgnoresSinceKeepsUntil(t *testing.T) {
	buf := capturePtermOutput(t)
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: "sess-1"}, nil
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{
		EventsFunc: func(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error) {
			// Paging by the opaque offset cursor: since is ignored, but until still
			// bounds the page (per the API contract).
			assert.True(t, query.Offset.Valid(), "offset should be forwarded")
			assert.False(t, query.Since.Valid(), "since must be omitted when --offset is set")
			assert.True(t, query.Until.Valid(), "until still bounds the page when --offset is set")
			return &pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{}, nil
		},
	}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}
	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1", Offset: 5, Since: "5m", Until: "1m"})
	assert.NoError(t, err)
	_ = buf
}

func TestTelemetryEvents_UnknownCategoryErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}, telemetry: &FakeBrowserTelemetryService{}}

	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1", Categories: []string{"netowrk"}})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --categories value")
}

func TestTelemetryEvents_InvalidLimitErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}, telemetry: &FakeBrowserTelemetryService{}}

	for _, lim := range []int64{-1, 101} {
		err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1", Limit: lim})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --limit value")
	}
}

// Categories are filtered server-side, but the SDK comma-joins a []string field
// into one value the endpoint won't match, so they go out as repeated query
// params instead of the typed Category field.
func TestTelemetryEvents_CategoriesSentAsRepeatedQueryParams(t *testing.T) {
	buf := capturePtermOutput(t)
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: "sess-1"}, nil
	}}
	var gotQuery kernel.BrowserTelemetryEventsParams
	var gotOpts []option.RequestOption
	fakeTelemetry := &FakeBrowserTelemetryService{
		EventsFunc: func(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error) {
			gotQuery, gotOpts = query, opts
			return &pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{}, nil
		},
	}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1", Categories: []string{"console", "network"}})

	assert.NoError(t, err)
	assert.Empty(t, gotQuery.Category, "categories must not use the comma-joined typed field")
	// Two category query params plus the response-capture option for the cursor.
	assert.Len(t, gotOpts, 3)
	_ = buf
}

// The events archive outlives the session, so a 404 from Get (e.g. an ended
// session) must not stop the read: the command falls back to the raw identifier.
func TestTelemetryEvents_FallsBackToIdentifierOn404(t *testing.T) {
	buf := capturePtermOutput(t)
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return nil, &kernel.Error{StatusCode: http.StatusNotFound}
	}}
	var gotID string
	fakeTelemetry := &FakeBrowserTelemetryService{
		EventsFunc: func(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error) {
			gotID = id
			return &pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{}, nil
		},
	}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "ended-session-id"})

	assert.NoError(t, err)
	assert.Equal(t, "ended-session-id", gotID)
	_ = buf
}

// A non-404 Get failure (auth, 5xx, transient) is a real error, not an ended
// session — it must surface, not get swallowed by the fallback.
func TestTelemetryEvents_SurfacesNonNotFoundGetError(t *testing.T) {
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return nil, &kernel.Error{StatusCode: http.StatusInternalServerError}
	}}
	fakeTelemetry := &FakeBrowserTelemetryService{
		EventsFunc: func(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error) {
			t.Fatalf("Events must not be called when Get fails with a non-404 error")
			return nil, nil
		},
	}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1"})

	assert.Error(t, err)
}

// A --types filter is client-side, so it must scan every page in the window to be
// complete. Setting --types (without --all) must therefore route through the
// auto-pager, not the single-page fetch that could drop matches on later pages.
func TestTelemetryEvents_TypesFilterWalksAllPages(t *testing.T) {
	buf := capturePtermOutput(t)
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: "sess-1"}, nil
	}}
	autoPaged := false
	fakeTelemetry := &FakeBrowserTelemetryService{
		EventsFunc: func(ctx context.Context, id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse], error) {
			t.Fatalf("single-page Events must not be called when --types is set")
			return nil, nil
		},
		EventsAutoPagingFunc: func(id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) *pagination.OffsetPaginationAutoPager[kernel.BrowserTelemetryEventsResponse] {
			autoPaged = true
			return pagination.NewOffsetPaginationAutoPager(&pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{}, nil)
		},
	}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1", Types: []string{"network_response"}})

	assert.NoError(t, err)
	assert.True(t, autoPaged, "--types must walk every page so the client-side filter is complete")
	_ = buf
}

// A full-window scan (--all/--types) must ignore the manual --offset cursor and
// walk from --since; forwarding the offset would start mid-window and drop
// earlier pages, contradicting the documented behavior.
func TestTelemetryEvents_FullScanIgnoresOffsetUsesSince(t *testing.T) {
	buf := capturePtermOutput(t)
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: "sess-1"}, nil
	}}
	var gotQuery kernel.BrowserTelemetryEventsParams
	fakeTelemetry := &FakeBrowserTelemetryService{
		EventsAutoPagingFunc: func(id string, query kernel.BrowserTelemetryEventsParams, opts ...option.RequestOption) *pagination.OffsetPaginationAutoPager[kernel.BrowserTelemetryEventsResponse] {
			gotQuery = query
			return pagination.NewOffsetPaginationAutoPager(&pagination.OffsetPagination[kernel.BrowserTelemetryEventsResponse]{}, nil)
		},
	}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: fakeTelemetry}

	err := b.TelemetryEvents(context.Background(), BrowsersTelemetryEventsInput{Identifier: "br-1", All: true, Offset: 100, Since: "5m"})

	assert.NoError(t, err)
	assert.False(t, gotQuery.Offset.Valid(), "--all must not forward --offset")
	assert.Equal(t, "5m", gotQuery.Since.Value, "--all walks the window from --since")
	_ = buf
}

func TestParseTelemetryCategories_Platform(t *testing.T) {
	p, err := parseTelemetryCategories("platform")

	assert.NoError(t, err)
	assert.True(t, p.Platform.Enabled.Valid())
	assert.True(t, p.Platform.Enabled.Value)
	// platform is opt-in only, so it must be offered by the flag's error message too.
	_, err = parseTelemetryCategories("bogus")
	assert.ErrorContains(t, err, "platform")
}

func TestTelemetryEnabledCategories_Platform(t *testing.T) {
	cfg := kernel.BrowserTelemetryConfig{Browser: kernel.BrowserTelemetryCategoriesConfig{}}
	cfg.Browser.Platform.Enabled = true

	assert.Equal(t, []string{"platform"}, telemetryEnabledCategories(cfg))
}

func TestParseTelemetryCdpExcludedMethods(t *testing.T) {
	t.Run("canonicalizes and trims", func(t *testing.T) {
		got, err := parseTelemetryCdpExcludedMethods(" input.dispatchmouseevent , Page.captureScreenshot ")
		assert.NoError(t, err)
		assert.Equal(t, []kernel.BrowserCdpCommandMethod{
			kernel.BrowserCdpCommandMethodInputDispatchMouseEvent,
			kernel.BrowserCdpCommandMethodPageCaptureScreenshot,
		}, got)
	})
	t.Run("none clears the list", func(t *testing.T) {
		got, err := parseTelemetryCdpExcludedMethods("none")
		assert.NoError(t, err)
		assert.NotNil(t, got, "an empty list must still be sent, so the API reports every method again")
		assert.Empty(t, got)
	})
	t.Run("rejects unknown methods", func(t *testing.T) {
		_, err := parseTelemetryCdpExcludedMethods("Page.doesNotExist")
		assert.ErrorContains(t, err, "unknown CDP method")
	})
}

func TestBuildTelemetryParam_CdpExclude(t *testing.T) {
	t.Run("merges into control without enabling it", func(t *testing.T) {
		p, err := buildNewTelemetryParam("", "Input.dispatchMouseEvent", "")
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Valid())
		assert.False(t, p.Browser.Control.Enabled.Valid(), "exclusions must not silently flip the control category")
		assert.Equal(t, []kernel.BrowserCdpCommandMethod{
			kernel.BrowserCdpCommandMethodInputDispatchMouseEvent,
		}, p.Browser.Control.Cdp.ExcludedMethods)
	})
	t.Run("combines with a category selection", func(t *testing.T) {
		p, err := buildUpdateTelemetryParam("control,network", "Page.captureScreenshot")
		assert.NoError(t, err)
		assert.True(t, p.Browser.Control.Enabled.Value)
		assert.Equal(t, []kernel.BrowserCdpCommandMethod{
			kernel.BrowserCdpCommandMethodPageCaptureScreenshot,
		}, p.Browser.Control.Cdp.ExcludedMethods)
	})
	t.Run("rejects combining with telemetry off", func(t *testing.T) {
		_, err := buildNewTelemetryParam("off", "Page.captureScreenshot", "")
		assert.ErrorContains(t, err, "cannot combine --telemetry=off with --telemetry-cdp-exclude")
	})
}

func TestBuildManagedAuthTelemetryParam_CdpExcludeNeedsCategories(t *testing.T) {
	// The connection stores the config verbatim, so exclusions on their own would
	// replace its category selection — allowed on create, rejected on update/login.
	_, err := buildManagedAuthTelemetryParam("", "Page.navigate", "", false)
	assert.ErrorContains(t, err, "also requires --telemetry in the same command")

	p, err := buildManagedAuthTelemetryParam("", "Page.navigate", "", true)
	assert.NoError(t, err)
	assert.Equal(t, []kernel.BrowserCdpCommandMethod{
		kernel.BrowserCdpCommandMethodPageNavigate,
	}, p.Browser.Control.Cdp.ExcludedMethods)

	_, err = buildManagedAuthTelemetryParam("control", "Page.navigate", "", false)
	assert.NoError(t, err)
}
