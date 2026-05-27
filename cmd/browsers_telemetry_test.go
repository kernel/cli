package cmd

import (
	"context"
	"encoding/json"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/stretchr/testify/assert"
)

type FakeBrowserTelemetryService struct{}

func (f *FakeBrowserTelemetryService) StreamStreaming(ctx context.Context, id string, query kernel.BrowserTelemetryStreamParams, opts ...option.RequestOption) *ssestream.Stream[kernel.BrowserTelemetryStreamResponse] {
	return makeStream([]kernel.BrowserTelemetryStreamResponse{})
}

func TestBrowsersTelemetryStart_SendsEnablePayload(t *testing.T) {
	setupStdoutCapture(t)
	var capturedID string
	var captured kernel.BrowserUpdateParams
	fake := &FakeBrowsersService{UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserUpdateParams, opts ...option.RequestOption) (*kernel.BrowserUpdateResponse, error) {
		capturedID = id
		captured = body
		return &kernel.BrowserUpdateResponse{SessionID: id}, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.TelemetryStart(context.Background(), BrowsersTelemetryStartInput{Identifier: "session123"})

	assert.NoError(t, err)
	assert.Equal(t, "session123", capturedID)
	assert.True(t, captured.Telemetry.Enabled.Valid())
	assert.True(t, captured.Telemetry.Enabled.Value)
	assert.Contains(t, outBuf.String(), "Started telemetry for browser session123")
}

func TestBrowsersTelemetryStop_SendsDisablePayload(t *testing.T) {
	setupStdoutCapture(t)
	var capturedID string
	var captured kernel.BrowserUpdateParams
	fake := &FakeBrowsersService{UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserUpdateParams, opts ...option.RequestOption) (*kernel.BrowserUpdateResponse, error) {
		capturedID = id
		captured = body
		return &kernel.BrowserUpdateResponse{SessionID: id}, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.TelemetryStop(context.Background(), BrowsersTelemetryStopInput{Identifier: "session123"})

	assert.NoError(t, err)
	assert.Equal(t, "session123", capturedID)
	assert.True(t, captured.Telemetry.Enabled.Valid())
	assert.False(t, captured.Telemetry.Enabled.Value)
	assert.Contains(t, outBuf.String(), "Stopped telemetry for browser session123")
}

func TestBrowsersTelemetrySet_PartialCategories(t *testing.T) {
	setupStdoutCapture(t)
	var captured kernel.BrowserUpdateParams
	fake := &FakeBrowsersService{UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserUpdateParams, opts ...option.RequestOption) (*kernel.BrowserUpdateResponse, error) {
		captured = body
		return &kernel.BrowserUpdateResponse{SessionID: id}, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.TelemetrySet(context.Background(), BrowsersTelemetrySetInput{Identifier: "session123", Categories: "network=on,page=off"})

	assert.NoError(t, err)
	assert.False(t, captured.Telemetry.Enabled.Valid())
	assert.True(t, captured.Telemetry.Browser.Network.Enabled.Valid())
	assert.True(t, captured.Telemetry.Browser.Network.Enabled.Value)
	assert.True(t, captured.Telemetry.Browser.Page.Enabled.Valid())
	assert.False(t, captured.Telemetry.Browser.Page.Enabled.Value)
	// Unspecified categories omitted — server retains their state
	assert.False(t, captured.Telemetry.Browser.Console.Enabled.Valid())
	assert.False(t, captured.Telemetry.Browser.Interaction.Enabled.Valid())
}

func TestBrowsersTelemetrySet_InvalidCategory(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetrySet(context.Background(), BrowsersTelemetrySetInput{Identifier: "session123", Categories: "foo=on"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
}

func TestBrowsersTelemetrySet_InvalidValue(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetrySet(context.Background(), BrowsersTelemetrySetInput{Identifier: "session123", Categories: "network=yes"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 'on' or 'off'")
}

func TestBrowsersTelemetryStart_UnsupportedOutputErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetryStart(context.Background(), BrowsersTelemetryStartInput{Identifier: "session123", Output: "yaml"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestBrowsersTelemetryStop_UnsupportedOutputErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetryStop(context.Background(), BrowsersTelemetryStopInput{Identifier: "session123", Output: "yaml"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestBrowsersTelemetrySet_UnsupportedOutputErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetrySet(context.Background(), BrowsersTelemetrySetInput{Identifier: "session123", Categories: "network=on", Output: "yaml"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestBrowsersTelemetryStatus_UnsupportedOutputErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetryStatus(context.Background(), BrowsersTelemetryStatusInput{Identifier: "session123", Output: "yaml"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestBrowsersTelemetrySet_WhitespaceTolerance(t *testing.T) {
	setupStdoutCapture(t)
	var captured kernel.BrowserUpdateParams
	fake := &FakeBrowsersService{UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserUpdateParams, opts ...option.RequestOption) (*kernel.BrowserUpdateResponse, error) {
		captured = body
		return &kernel.BrowserUpdateResponse{SessionID: id}, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.TelemetrySet(context.Background(), BrowsersTelemetrySetInput{Identifier: "session123", Categories: " network = on , page = off "})

	assert.NoError(t, err)
	assert.True(t, captured.Telemetry.Browser.Network.Enabled.Valid())
	assert.True(t, captured.Telemetry.Browser.Network.Enabled.Value)
	assert.True(t, captured.Telemetry.Browser.Page.Enabled.Valid())
	assert.False(t, captured.Telemetry.Browser.Page.Enabled.Value)
}

func TestBrowsersTelemetryStatus_PrintsCategories(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		var resp kernel.BrowserGetResponse
		if err := json.Unmarshal([]byte(`{"session_id":"session123","telemetry":{"browser":{"console":{"enabled":true},"interaction":{"enabled":false},"network":{"enabled":true},"page":{"enabled":false}}}}`), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return &resp, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.TelemetryStatus(context.Background(), BrowsersTelemetryStatusInput{Identifier: "session123"})

	assert.NoError(t, err)
	out := outBuf.String()
	assert.Contains(t, out, "enabled:     on")
	assert.Contains(t, out, "console:     on")
	assert.Contains(t, out, "interaction: off")
	assert.Contains(t, out, "network:     on")
	assert.Contains(t, out, "page:        off")
}

func TestBrowsersTelemetryStatus_VMDefaultsAllOn(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		// API returns browser:{} when using VM defaults — enabled field absent means true per SDK
		var resp kernel.BrowserGetResponse
		if err := json.Unmarshal([]byte(`{"session_id":"session123","telemetry":{"browser":{}}}`), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return &resp, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.TelemetryStatus(context.Background(), BrowsersTelemetryStatusInput{Identifier: "session123"})

	assert.NoError(t, err)
	out := outBuf.String()
	assert.Contains(t, out, "enabled:     on")
	assert.Contains(t, out, "console:     on")
	assert.Contains(t, out, "interaction: on")
	assert.Contains(t, out, "network:     on")
	assert.Contains(t, out, "page:        on")
}

func TestBrowsersTelemetryStatus_StoppedShowsDisabled(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		// API returns {} (no browser key) after telemetry stop
		var resp kernel.BrowserGetResponse
		if err := json.Unmarshal([]byte(`{"session_id":"session123","telemetry":{}}`), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return &resp, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.TelemetryStatus(context.Background(), BrowsersTelemetryStatusInput{Identifier: "session123"})

	assert.NoError(t, err)
	out := outBuf.String()
	assert.Contains(t, out, "enabled:     off")
	assert.NotContains(t, out, "console:")
	assert.NotContains(t, out, "network:")
}

func TestTelemetryStream_UnknownCategoryErrors(t *testing.T) {
	b := BrowsersCmd{browsers: &FakeBrowsersService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Categories: []string{"invalid"},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
}

func TestTelemetryStream_UnknownTypeWarns(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeBrowsersService{GetFunc: func(ctx context.Context, id string, query kernel.BrowserGetParams, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
		return &kernel.BrowserGetResponse{SessionID: id}, nil
	}}
	b := BrowsersCmd{browsers: fake, telemetry: &FakeBrowserTelemetryService{}}

	err := b.TelemetryStream(context.Background(), BrowsersTelemetryStreamInput{
		Identifier: "session123",
		Types:      []string{"invalid_type"},
	})

	assert.NoError(t, err)
	assert.Contains(t, outBuf.String(), "unrecognized event type")
}

func makeEvent(t *testing.T, raw string) kernel.BrowserTelemetryEventUnion {
	t.Helper()
	var ev kernel.BrowserTelemetryEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("makeEvent: %v", err)
	}
	return ev
}

func TestEventCategoryFromRaw(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		// real category field present — used directly
		{`{"type":"monitor_screenshot","category":"system","ts":0}`, "system"},
		{`{"type":"network_response","category":"network","ts":0}`, "network"},
		// no category field — returns ""
		{`{"type":"console_log","ts":0}`, ""},
		{`{"type":"page_navigation","ts":0}`, ""},
		{`{"type":"nounderscore","ts":0}`, ""},
	}
	for _, tc := range cases {
		ev := makeEvent(t, tc.raw)
		assert.Equal(t, tc.want, eventCategoryFromRaw(ev), "raw=%s", tc.raw)
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
			assert.Equal(t, tc.want, shouldEmit(ev, tc.categories, tc.types))
		})
	}
}
