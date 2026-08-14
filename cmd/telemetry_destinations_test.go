package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeTelemetryDestinationsService implements TelemetryDestinationsService.
type FakeTelemetryDestinationsService struct {
	NewFunc    func(ctx context.Context, body kernel.TelemetryDestinationNewParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error)
	GetFunc    func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.OtlpDestination, error)
	UpdateFunc func(ctx context.Context, idOrName string, body kernel.TelemetryDestinationUpdateParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error)
	ListFunc   func(ctx context.Context, query kernel.TelemetryDestinationListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.OtlpDestination], error)
	DeleteFunc func(ctx context.Context, idOrName string, opts ...option.RequestOption) error
}

func (f *FakeTelemetryDestinationsService) New(ctx context.Context, body kernel.TelemetryDestinationNewParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.OtlpDestination{ID: "d1", Name: body.Name, Endpoint: body.Endpoint, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}

func (f *FakeTelemetryDestinationsService) Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.OtlpDestination, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, idOrName, opts...)
	}
	return &kernel.OtlpDestination{ID: idOrName, Name: idOrName, Endpoint: "https://api.honeycomb.io", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}

func (f *FakeTelemetryDestinationsService) Update(ctx context.Context, idOrName string, body kernel.TelemetryDestinationUpdateParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, idOrName, body, opts...)
	}
	return &kernel.OtlpDestination{ID: idOrName, Name: idOrName, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}

func (f *FakeTelemetryDestinationsService) List(ctx context.Context, query kernel.TelemetryDestinationListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.OtlpDestination], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.OtlpDestination]{Items: []kernel.OtlpDestination{}}, nil
}

func (f *FakeTelemetryDestinationsService) Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, idOrName, opts...)
	}
	return nil
}

func TestTelemetryDestinationsList_Empty(t *testing.T) {
	buf := capturePtermOutput(t)
	c := TelemetryDestinationsCmd{destinations: &FakeTelemetryDestinationsService{}}
	require.NoError(t, c.List(context.Background(), TelemetryDestinationsListInput{Page: 1, PerPage: 20}))
	assert.Contains(t, buf.String(), "No OTLP destinations found")
}

func TestTelemetryDestinationsList_WithRows(t *testing.T) {
	buf := capturePtermOutput(t)
	created := time.Unix(0, 0)
	items := []kernel.OtlpDestination{{
		ID:          "d1",
		Name:        "honeycomb",
		Endpoint:    "https://api.honeycomb.io",
		Description: "prod",
		// Values come back redacted, so only the names are renderable.
		Headers:   map[string]string{"X-Honeycomb-Team": "", "Authorization": ""},
		CreatedAt: created,
		UpdatedAt: created,
	}}
	fake := &FakeTelemetryDestinationsService{ListFunc: func(ctx context.Context, query kernel.TelemetryDestinationListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.OtlpDestination], error) {
		return &pagination.OffsetPagination[kernel.OtlpDestination]{Items: items}, nil
	}}
	c := TelemetryDestinationsCmd{destinations: fake}
	require.NoError(t, c.List(context.Background(), TelemetryDestinationsListInput{Page: 1, PerPage: 20}))
	out := buf.String()
	assert.Contains(t, out, "d1")
	assert.Contains(t, out, "honeycomb")
	assert.Contains(t, out, "Authorization, X-Honeycomb-Team")
	assert.Contains(t, out, "Has more: no")
}

func TestTelemetryDestinationsList_HasMore(t *testing.T) {
	buf := capturePtermOutput(t)
	created := time.Unix(0, 0)
	perPage := 2
	items := make([]kernel.OtlpDestination, perPage+1)
	for i := range items {
		items[i] = kernel.OtlpDestination{ID: fmt.Sprintf("d%d", i), Name: fmt.Sprintf("dest%d", i), CreatedAt: created, UpdatedAt: created}
	}
	var gotQuery kernel.TelemetryDestinationListParams
	fake := &FakeTelemetryDestinationsService{ListFunc: func(ctx context.Context, query kernel.TelemetryDestinationListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.OtlpDestination], error) {
		gotQuery = query
		return &pagination.OffsetPagination[kernel.OtlpDestination]{Items: items}, nil
	}}
	c := TelemetryDestinationsCmd{destinations: fake}
	require.NoError(t, c.List(context.Background(), TelemetryDestinationsListInput{Page: 2, PerPage: perPage, Query: "honey comb"}))
	out := buf.String()
	// The extra item is requested to detect the next page, then dropped.
	assert.Equal(t, int64(perPage+1), gotQuery.Limit.Value)
	assert.Equal(t, int64(perPage), gotQuery.Offset.Value)
	assert.Contains(t, out, "Has more: yes")
	assert.Contains(t, out, `Next: kernel telemetry destinations list --page 3 --per-page 2 --query "honey comb"`)
	assert.NotContains(t, out, "dest2")
}

func TestTelemetryDestinationsCreate_SendsHeaders(t *testing.T) {
	capturePtermOutput(t)
	var got kernel.TelemetryDestinationNewParams
	fake := &FakeTelemetryDestinationsService{NewFunc: func(ctx context.Context, body kernel.TelemetryDestinationNewParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error) {
		got = body
		return &kernel.OtlpDestination{ID: "d1", Name: body.Name, Endpoint: body.Endpoint, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
	}}
	c := TelemetryDestinationsCmd{destinations: fake}
	require.NoError(t, c.Create(context.Background(), TelemetryDestinationsCreateInput{
		Name:        "honeycomb",
		Endpoint:    "https://api.honeycomb.io",
		Description: "prod",
		Headers:     map[string]string{"x-honeycomb-team": "secret"},
	}))
	assert.Equal(t, "honeycomb", got.Name)
	assert.Equal(t, "https://api.honeycomb.io", got.Endpoint)
	assert.Equal(t, "prod", got.Description.Value)
	assert.Equal(t, map[string]string{"x-honeycomb-team": "secret"}, got.Headers)
}

func TestTelemetryDestinationsCreate_RequiresNameAndEndpoint(t *testing.T) {
	capturePtermOutput(t)
	c := TelemetryDestinationsCmd{destinations: &FakeTelemetryDestinationsService{}}
	err := c.Create(context.Background(), TelemetryDestinationsCreateInput{Endpoint: "https://api.honeycomb.io"})
	assert.ErrorContains(t, err, "--name is required")
	err = c.Create(context.Background(), TelemetryDestinationsCreateInput{Name: "honeycomb"})
	assert.ErrorContains(t, err, "--endpoint is required")
}

func TestTelemetryDestinationsUpdate_NothingToUpdate(t *testing.T) {
	capturePtermOutput(t)
	c := TelemetryDestinationsCmd{destinations: &FakeTelemetryDestinationsService{}}
	err := c.Update(context.Background(), TelemetryDestinationsUpdateInput{Identifier: "d1"})
	assert.ErrorContains(t, err, "nothing to update")
}

func TestTelemetryDestinationsUpdate_ClearsDescription(t *testing.T) {
	capturePtermOutput(t)
	empty := ""
	var got kernel.TelemetryDestinationUpdateParams
	fake := &FakeTelemetryDestinationsService{UpdateFunc: func(ctx context.Context, idOrName string, body kernel.TelemetryDestinationUpdateParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error) {
		got = body
		return &kernel.OtlpDestination{ID: idOrName, Name: idOrName, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
	}}
	c := TelemetryDestinationsCmd{destinations: fake}
	require.NoError(t, c.Update(context.Background(), TelemetryDestinationsUpdateInput{Identifier: "d1", Description: &empty}))
	assert.True(t, got.Description.Valid())
	assert.Equal(t, "", got.Description.Value)
}

// Header removals ride the SDK's extra-fields escape hatch because a JSON null
// cannot be expressed through the typed map[string]string field.
func TestTelemetryDestinationsUpdate_RemoveHeaderSendsNull(t *testing.T) {
	capturePtermOutput(t)
	var got kernel.TelemetryDestinationUpdateParams
	fake := &FakeTelemetryDestinationsService{UpdateFunc: func(ctx context.Context, idOrName string, body kernel.TelemetryDestinationUpdateParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error) {
		got = body
		return &kernel.OtlpDestination{ID: idOrName, Name: idOrName, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
	}}
	c := TelemetryDestinationsCmd{destinations: fake}
	require.NoError(t, c.Update(context.Background(), TelemetryDestinationsUpdateInput{
		Identifier:    "d1",
		Headers:       map[string]string{"x-honeycomb-team": "rotated"},
		RemoveHeaders: []string{"x-old"},
	}))

	body, err := json.Marshal(got)
	require.NoError(t, err)
	var payload struct {
		Headers map[string]*string `json:"headers"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Contains(t, payload.Headers, "x-old")
	assert.Nil(t, payload.Headers["x-old"])
	require.NotNil(t, payload.Headers["x-honeycomb-team"])
	assert.Equal(t, "rotated", *payload.Headers["x-honeycomb-team"])
}

// A header named by both flags keeps its new value: removals are merged first.
func TestTelemetryDestinationsUpdate_SetWinsOverRemove(t *testing.T) {
	capturePtermOutput(t)
	var got kernel.TelemetryDestinationUpdateParams
	fake := &FakeTelemetryDestinationsService{UpdateFunc: func(ctx context.Context, idOrName string, body kernel.TelemetryDestinationUpdateParams, opts ...option.RequestOption) (*kernel.OtlpDestination, error) {
		got = body
		return &kernel.OtlpDestination{ID: idOrName, Name: idOrName, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
	}}
	c := TelemetryDestinationsCmd{destinations: fake}
	require.NoError(t, c.Update(context.Background(), TelemetryDestinationsUpdateInput{
		Identifier:    "d1",
		Headers:       map[string]string{"authorization": "new"},
		RemoveHeaders: []string{"authorization"},
	}))

	body, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"authorization":"new"`)
}

func TestTelemetryDestinationsDelete_NotFound(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeTelemetryDestinationsService{DeleteFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
		return &kernel.Error{StatusCode: http.StatusNotFound}
	}}
	c := TelemetryDestinationsCmd{destinations: fake}
	require.NoError(t, c.Delete(context.Background(), TelemetryDestinationsDeleteInput{Identifier: "nope", SkipConfirm: true}))
	assert.Contains(t, buf.String(), "not found")
}

func TestTelemetryDestinationsDelete_NonInteractiveWithoutYes(t *testing.T) {
	capturePtermOutput(t)
	deleted := false
	fake := &FakeTelemetryDestinationsService{DeleteFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
		deleted = true
		return nil
	}}
	c := TelemetryDestinationsCmd{destinations: fake, prompter: interactive.NewPrompterWithTerminal(false)}
	err := c.Delete(context.Background(), TelemetryDestinationsDeleteInput{Identifier: "d1"})
	assert.Error(t, err)
	assert.False(t, deleted)
}

func TestFormatOtlpDestinationHeaders(t *testing.T) {
	assert.Equal(t, "", formatOtlpDestinationHeaders(nil))
	assert.Equal(t, "Authorization, X-Api-Key", formatOtlpDestinationHeaders(map[string]string{"X-Api-Key": "", "Authorization": ""}))
}
