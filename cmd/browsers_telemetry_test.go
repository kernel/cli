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
	assert.Contains(t, err.Error(), "must be 'on' or 'off'")
}

func TestParseTelemetryCategories_WhitespaceTolerance(t *testing.T) {
	p, err := parseTelemetryCategories(" network = on , page = off ")

	assert.NoError(t, err)
	assert.True(t, p.Network.Enabled.Valid())
	assert.True(t, p.Network.Enabled.Value)
	assert.True(t, p.Page.Enabled.Valid())
	assert.False(t, p.Page.Enabled.Value)
}
