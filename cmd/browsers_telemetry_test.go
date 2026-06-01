package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
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
	StreamFunc func() *ssestream.Stream[kernel.BrowserTelemetryStreamResponse]
}

func (f *FakeBrowserTelemetryService) StreamStreaming(ctx context.Context, id string, query kernel.BrowserTelemetryStreamParams, opts ...option.RequestOption) *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
	if f.StreamFunc != nil {
		return f.StreamFunc()
	}
	return makeStream([]kernel.BrowserTelemetryStreamResponse{})
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

type capturingTelemetryService struct {
	captured kernel.BrowserTelemetryStreamParams
}

func (c *capturingTelemetryService) StreamStreaming(ctx context.Context, id string, query kernel.BrowserTelemetryStreamParams, opts ...option.RequestOption) *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
	c.captured = query
	return makeStream([]kernel.BrowserTelemetryStreamResponse{})
}

func TestTelemetryStream_SeqZeroSetsLastEventID(t *testing.T) {
	fakeBrowsers := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	capSvc := &capturingTelemetryService{}
	b := BrowsersCmd{browsers: fakeBrowsers, telemetry: capSvc}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Seq:        0,
	})

	assert.NoError(t, err)
	assert.True(t, capSvc.captured.LastEventID.Valid())
	assert.Equal(t, "0", capSvc.captured.LastEventID.Value)
}

func makeEvent(t *testing.T, raw string) kernel.BrowserTelemetryEventUnion {
	t.Helper()
	var ev kernel.BrowserTelemetryEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("makeEvent: %v", err)
	}
	return ev
}

func TestEventCategory(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		// Wire category wins when present.
		{`{"type":"network_response","category":"network","ts":0}`, "network"},
		{`{"type":"monitor_screenshot","category":"system","ts":0}`, "system"},
		// Wire category overrides what a naive type-prefix split would return,
		// e.g. cdp_* events the server classifies as system.
		{`{"type":"cdp_attached","category":"system","ts":0}`, "system"},
		// Fallback to prefix when wire category is absent.
		{`{"type":"monitor_screenshot","ts":0}`, "system"},
		{`{"type":"monitor_disconnected","ts":0}`, "system"},
		{`{"type":"network_response","ts":0}`, "network"},
		{`{"type":"console_log","ts":0}`, "console"},
		{`{"type":"page_navigation","ts":0}`, "page"},
		{`{"type":"interaction_click","ts":0}`, "interaction"},
		{`{"type":"nounderscore","ts":0}`, "nounderscore"},
	}
	for _, tc := range cases {
		ev := makeEvent(t, tc.raw)
		assert.Equal(t, tc.want, eventCategory(ev), "type=%s", ev.Type)
	}
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
		{"system category matches monitor_screenshot", `{"type":"monitor_screenshot","category":"system","ts":0}`, []string{"system"}, nil, true},
		{"matching type passes", `{"type":"console_log","category":"console","ts":0}`, nil, []string{"console_log"}, true},
		{"non-matching type drops", `{"type":"network_response","category":"network","ts":0}`, nil, []string{"console_log"}, false},
		{"both filters pass when both match", `{"type":"network_response","category":"network","ts":0}`, []string{"network"}, []string{"network_response"}, true},
		{"both filters drop when type misses", `{"type":"network_response","category":"network","ts":0}`, []string{"network"}, []string{"console_log"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := makeEvent(t, tc.raw)
			assert.Equal(t, tc.want, shouldEmit(eventCategory(ev), ev.Type, tc.categories, tc.types))
		})
	}
}

func TestParseTelemetryCategories_PartialCategories(t *testing.T) {
	p, err := parseTelemetryCategories("network=on,page=off")

	assert.NoError(t, err)
	assert.True(t, p.Network.Enabled.Valid())
	assert.True(t, p.Network.Enabled.Value)
	assert.True(t, p.Page.Enabled.Valid())
	assert.False(t, p.Page.Enabled.Value)
	// Unspecified categories omitted — server retains their state
	assert.False(t, p.Console.Enabled.Valid())
	assert.False(t, p.Interaction.Enabled.Valid())
}

func TestParseTelemetryCategories_InvalidCategory(t *testing.T) {
	_, err := parseTelemetryCategories("foo=on")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
}

func TestParseTelemetryCategories_InvalidValue(t *testing.T) {
	_, err := parseTelemetryCategories("network=yes")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `must be "on" or "off"`)
}

func TestParseTelemetryCategories_WhitespaceTolerance(t *testing.T) {
	p, err := parseTelemetryCategories(" network = on , page = off ")

	assert.NoError(t, err)
	assert.True(t, p.Network.Enabled.Valid())
	assert.True(t, p.Network.Enabled.Value)
	assert.True(t, p.Page.Enabled.Valid())
	assert.False(t, p.Page.Enabled.Value)
}

// TestBuildTelemetryParam_WireEncoding locks in the three distinct wire shapes
// the API expects: enable-all sets Enabled=true without Browser, disable-all
// sets Enabled=false without Browser, and per-category sets only Browser so the
// API treats it as a merge rather than a replace.
func TestBuildTelemetryParam_WireEncoding(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		p, err := buildNewTelemetryParam("all")
		assert.NoError(t, err)
		assert.True(t, p.Enabled.Valid())
		assert.True(t, p.Enabled.Value)
		assert.False(t, p.Browser.Network.Enabled.Valid())
	})
	t.Run("off", func(t *testing.T) {
		p, err := buildNewTelemetryParam("off")
		assert.NoError(t, err)
		assert.True(t, p.Enabled.Valid())
		assert.False(t, p.Enabled.Value)
		assert.False(t, p.Browser.Network.Enabled.Valid())
	})
	t.Run("per-category omits Enabled so API merges", func(t *testing.T) {
		p, err := buildNewTelemetryParam("network=off")
		assert.NoError(t, err)
		assert.False(t, p.Enabled.Valid(), "Enabled must be unset so API takes the merge path")
		assert.True(t, p.Browser.Network.Enabled.Valid())
		assert.False(t, p.Browser.Network.Enabled.Value)
	})
}
