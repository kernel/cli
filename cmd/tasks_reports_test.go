package cmd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTasksService struct {
	newFunc  func(kernel.TaskNewParams) *kernel.Task
	listFunc func(kernel.TaskListParams) *pagination.OffsetPagination[kernel.Task]
	linkFunc func(string, kernel.TaskLinkBrowsersParams) *kernel.Task
}

func (f fakeTasksService) New(_ context.Context, body kernel.TaskNewParams, _ ...option.RequestOption) (*kernel.Task, error) {
	return f.newFunc(body), nil
}
func (f fakeTasksService) Get(_ context.Context, id string, _ ...option.RequestOption) (*kernel.Task, error) {
	return &kernel.Task{ID: id}, nil
}
func (f fakeTasksService) List(_ context.Context, query kernel.TaskListParams, _ ...option.RequestOption) (*pagination.OffsetPagination[kernel.Task], error) {
	return f.listFunc(query), nil
}
func (f fakeTasksService) LinkBrowsers(_ context.Context, id string, body kernel.TaskLinkBrowsersParams, _ ...option.RequestOption) (*kernel.Task, error) {
	return f.linkFunc(id, body), nil
}

func TestTasksCreateMapsAllFields(t *testing.T) {
	startedAt := "2026-09-01T12:00:00Z"
	var captured kernel.TaskNewParams
	service := fakeTasksService{newFunc: func(params kernel.TaskNewParams) *kernel.Task {
		captured = params
		parsed, _ := time.Parse(time.RFC3339, startedAt)
		return &kernel.Task{ID: "task-1", Description: params.Description, StartedAt: parsed, CreatedAt: parsed}
	}}
	buf := capturePtermOutput(t)
	err := (TasksCmd{tasks: service}).Create(context.Background(), TasksCreateInput{
		Description: "Book a hotel", StartedAt: startedAt, TargetWebsite: "booking.example",
		Metadata: map[string]string{"thread_id": "thread-1"}, SessionIDs: []string{"browser-1"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Book a hotel", captured.Description)
	assert.Equal(t, "booking.example", captured.TargetWebsite.Value)
	assert.Equal(t, "thread-1", captured.Metadata["thread_id"])
	assert.Equal(t, []string{"browser-1"}, captured.SessionIDs)
	assert.True(t, captured.StartedAt.Valid())
	assert.Contains(t, buf.String(), "task-1")
}

func TestListPageUXRejectsValuesThatCannotUseLookahead(t *testing.T) {
	_, _, err := normalizePageInput(1, 100)
	require.ErrorContains(t, err, "between 1 and 99")
	_, _, err = normalizePageInput(-1, 20)
	require.ErrorContains(t, err, "--page")
}

func TestTasksListUsesPageUXAndPreservesFilter(t *testing.T) {
	var captured kernel.TaskListParams
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := []kernel.Task{
		{ID: "task-1", Description: "one", StartedAt: now, CreatedAt: now},
		{ID: "task-2", Description: "two", StartedAt: now, CreatedAt: now},
		{ID: "task-3", Description: "three", StartedAt: now, CreatedAt: now},
	}
	service := fakeTasksService{listFunc: func(params kernel.TaskListParams) *pagination.OffsetPagination[kernel.Task] {
		captured = params
		return &pagination.OffsetPagination[kernel.Task]{Items: items}
	}}
	buf := capturePtermOutput(t)
	err := (TasksCmd{tasks: service}).List(context.Background(), TasksListInput{Page: 2, PerPage: 2, SessionID: "browser one"})
	require.NoError(t, err)
	assert.Equal(t, int64(3), captured.Limit.Value)
	assert.Equal(t, int64(2), captured.Offset.Value)
	assert.Equal(t, "browser one", captured.SessionID.Value)
	assert.Contains(t, buf.String(), "Has more: yes")
	assert.Contains(t, buf.String(), `Next: kernel tasks list --page 3 --per-page 2 --session-id "browser one"`)
	assert.NotContains(t, buf.String(), "task-3")
}

type fakeReportsService struct {
	newFunc  func(kernel.ReportNewParams) *kernel.Report
	listFunc func(kernel.ReportListParams) *pagination.OffsetPagination[kernel.Report]
}

func (f fakeReportsService) New(_ context.Context, body kernel.ReportNewParams, _ ...option.RequestOption) (*kernel.Report, error) {
	return f.newFunc(body), nil
}
func (f fakeReportsService) Get(_ context.Context, id string, _ ...option.RequestOption) (*kernel.Report, error) {
	return &kernel.Report{ID: id, Data: map[string]any{}}, nil
}
func (f fakeReportsService) List(_ context.Context, query kernel.ReportListParams, _ ...option.RequestOption) (*pagination.OffsetPagination[kernel.Report], error) {
	return f.listFunc(query), nil
}

func TestReportsCreateMapsAllFields(t *testing.T) {
	occurredAt := "2026-09-01T12:00:00Z"
	var captured kernel.ReportNewParams
	service := fakeReportsService{newFunc: func(params kernel.ReportNewParams) *kernel.Report {
		captured = params
		parsed, _ := time.Parse(time.RFC3339, occurredAt)
		return &kernel.Report{ID: "report-1", TaskID: params.TaskID, Type: params.Type, SchemaVersion: params.SchemaVersion.Value, SessionID: params.SessionID.Value, Data: params.Data, OccurredAt: parsed, CreatedAt: parsed}
	}}
	buf := capturePtermOutput(t)
	err := (ReportsCmd{reports: service}).Create(context.Background(), ReportsCreateInput{
		TaskID: "task-1", Type: "task.result", Data: `{"result":"succeeded","count":2}`,
		OccurredAt: occurredAt, SchemaVersion: 2, SessionID: "browser-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "task-1", captured.TaskID)
	assert.Equal(t, "task.result", captured.Type)
	assert.Equal(t, "succeeded", captured.Data["result"])
	assert.Equal(t, int64(2), captured.SchemaVersion.Value)
	assert.Equal(t, "browser-1", captured.SessionID.Value)
	assert.True(t, captured.OccurredAt.Valid())
	assert.Contains(t, buf.String(), "report-1")
}

func TestReportsListUsesPageUXAndPreservesFilters(t *testing.T) {
	var captured kernel.ReportListParams
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := []kernel.Report{
		{ID: "report-1", Type: "task.result", OccurredAt: now, CreatedAt: now},
		{ID: "report-2", Type: "task.result", OccurredAt: now, CreatedAt: now},
	}
	service := fakeReportsService{listFunc: func(params kernel.ReportListParams) *pagination.OffsetPagination[kernel.Report] {
		captured = params
		return &pagination.OffsetPagination[kernel.Report]{Items: items}
	}}
	buf := capturePtermOutput(t)
	err := (ReportsCmd{reports: service}).List(context.Background(), ReportsListInput{TaskID: "task one", Type: "task.result", Page: 1, PerPage: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(2), captured.Limit.Value)
	assert.Equal(t, int64(0), captured.Offset.Value)
	assert.Equal(t, "task.result", captured.Type.Value)
	assert.Contains(t, buf.String(), `--task-id "task one" --type "task.result"`)
	assert.NotContains(t, buf.String(), "report-2")
}

type fakeReportTypesService struct {
	listFunc func(kernel.ReportTypeListParams) *pagination.OffsetPagination[kernel.ReportTypeSummary]
}

func (f fakeReportTypesService) Get(_ context.Context, typeName string, _ ...option.RequestOption) (*kernel.ReportType, error) {
	return &kernel.ReportType{Type: typeName, Schema: map[string]any{}}, nil
}
func (f fakeReportTypesService) List(_ context.Context, query kernel.ReportTypeListParams, _ ...option.RequestOption) (*pagination.OffsetPagination[kernel.ReportTypeSummary], error) {
	return f.listFunc(query), nil
}

type fakeReportTypeVersionsService struct {
	getFunc func(int64, kernel.ReportTypeVersionGetParams) *kernel.ReportTypeVersion
}

func (f fakeReportTypeVersionsService) Get(_ context.Context, version int64, query kernel.ReportTypeVersionGetParams, _ ...option.RequestOption) (*kernel.ReportTypeVersion, error) {
	return f.getFunc(version, query), nil
}

func TestReportTypesListAndVersionGet(t *testing.T) {
	var listParams kernel.ReportTypeListParams
	listService := fakeReportTypesService{listFunc: func(params kernel.ReportTypeListParams) *pagination.OffsetPagination[kernel.ReportTypeSummary] {
		listParams = params
		return &pagination.OffsetPagination[kernel.ReportTypeSummary]{Items: []kernel.ReportTypeSummary{{Type: "task.result", LatestVersion: 2, Description: "Task outcome"}}}
	}}
	buf := capturePtermOutput(t)
	err := (ReportTypesCmd{reportTypes: listService}).List(context.Background(), ReportTypesListInput{TaskID: "task-1", Page: 1, PerPage: 20})
	require.NoError(t, err)
	assert.Equal(t, "task-1", listParams.TaskID)
	assert.Equal(t, int64(21), listParams.Limit.Value)
	assert.Contains(t, buf.String(), "task.result")
	assert.Contains(t, buf.String(), "Has more: no")

	var gotType string
	var gotVersion int64
	versionService := fakeReportTypeVersionsService{getFunc: func(version int64, params kernel.ReportTypeVersionGetParams) *kernel.ReportTypeVersion {
		gotType, gotVersion = params.Type, version
		return &kernel.ReportTypeVersion{Type: params.Type, Version: version, Schema: map[string]any{"type": "object"}}
	}}
	err = (ReportTypesCmd{versions: versionService}).GetVersion(context.Background(), ReportTypeVersionsGetInput{Type: "task.result", Version: 2})
	require.NoError(t, err)
	assert.Equal(t, "task.result", gotType)
	assert.Equal(t, int64(2), gotVersion)
}

func TestTasksListJSONIncludesPageMetadata(t *testing.T) {
	var items []kernel.Task
	require.NoError(t, json.Unmarshal([]byte(`[{"id":"task-1"},{"id":"task-2"},{"id":"task-3"}]`), &items))
	service := fakeTasksService{listFunc: func(kernel.TaskListParams) *pagination.OffsetPagination[kernel.Task] {
		return &pagination.OffsetPagination[kernel.Task]{Items: items}
	}}
	for _, tc := range []struct {
		name    string
		perPage int
		want    string
	}{
		{"more pages", 2, `{"items":[{"id":"task-1"},{"id":"task-2"}],"page":3,"per_page":2,"has_more":true}`},
		{"last page", 3, `{"items":[{"id":"task-1"},{"id":"task-2"},{"id":"task-3"}],"page":3,"per_page":3,"has_more":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				require.NoError(t, (TasksCmd{tasks: service}).List(context.Background(), TasksListInput{Page: 3, PerPage: tc.perPage, Output: "json"}))
			})
			assert.JSONEq(t, tc.want, out)
		})
	}
}
