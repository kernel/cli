package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeAuditLogsService struct {
	ListAutoPagingFunc func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry]
}

func (f *FakeAuditLogsService) ListAutoPaging(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
	if f.ListAutoPagingFunc != nil {
		return f.ListAutoPagingFunc(ctx, query, opts...)
	}
	return auditLogPager()
}

func auditLogPager(entries ...kernel.AuditLogEntry) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
	page := &pagination.PageTokenPagination[kernel.AuditLogEntry]{Items: entries}
	page.SetPageConfig(nil, &http.Response{Header: http.Header{}})
	return pagination.NewPageTokenPaginationAutoPager(page, nil)
}

func auditLogEntryFromJSON(raw string) kernel.AuditLogEntry {
	var entry kernel.AuditLogEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		panic(err)
	}
	return entry
}

func sampleAuditLogEntry(method, path string) kernel.AuditLogEntry {
	return auditLogEntryFromJSON(`{"timestamp":"2026-07-01T12:00:00Z","method":"` + method + `","status":201,"path":"` + path + `","route":"/browsers","domain":"api.onkernel.com","email":"dev@example.com","user_id":"user_123","auth_strategy":"api_key","client_ip":"203.0.113.7","user_agent":"kernel-cli","duration_ms":42}`)
}

func TestAuditLogsSearchBuildsParamsAndPrintsTable(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), query.Start)
			assert.Equal(t, time.Date(2026, 7, 2, 15, 0, 0, 0, time.UTC), query.End)
			assert.Equal(t, "browsers", query.Search.Value)
			assert.Equal(t, "POST", query.Method.Value)
			assert.Equal(t, []string{"OPTIONS"}, query.ExcludeMethod)
			assert.Equal(t, "api", query.Service.Value)
			assert.Equal(t, "api_key", query.AuthStrategy.Value)
			assert.Equal(t, []string{"user_123", "user_456"}, query.SearchUserID)
			assert.Equal(t, int64(50), query.Limit.Value)
			return auditLogPager(sampleAuditLogEntry("POST", "/browsers"))
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{
		Start:         "2026-07-01",
		End:           "2026-07-02T15:00:00Z",
		Search:        "browsers",
		Method:        "POST",
		ExcludeMethod: "OPTIONS",
		Service:       "api",
		AuthStrategy:  "api_key",
		UserIDs:       []string{"user_123", "user_456"},
		Limit:         50,
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "/browsers")
	assert.Contains(t, out, "POST")
	assert.Contains(t, out, "201")
	assert.Contains(t, out, "dev@example.com")
	assert.Contains(t, out, "203.0.113.7")
}

func TestAuditLogsSearchRejectsInvalidStart(t *testing.T) {
	c := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}

	err := c.Search(context.Background(), AuditLogsSearchInput{Start: "yesterday", Limit: 100})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--start")
	assert.Contains(t, err.Error(), "invalid time")
}

func TestAuditLogsSearchRejectsStartAfterEnd(t *testing.T) {
	c := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}

	err := c.Search(context.Background(), AuditLogsSearchInput{
		Start: "2026-07-02",
		End:   "2026-07-01",
		Limit: 100,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--start must be before --end")
}

func TestAuditLogsSearchDefaultsWindowToLast24Hours(t *testing.T) {
	capturePtermOutput(t)
	var gotStart, gotEnd time.Time
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			gotStart = query.Start
			gotEnd = query.End
			return auditLogPager()
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{Limit: 100})
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC().Add(-24*time.Hour), gotStart, time.Minute)
	assert.WithinDuration(t, time.Now().UTC(), gotEnd, time.Minute)
}

func TestAuditLogsSearchDateOnlyEndIsExclusive(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Equal(t, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), query.Start)
			assert.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), query.End)
			return auditLogPager()
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{Start: "2026-06-30", End: "2026-07-01", Limit: 100})
	require.NoError(t, err)
}

func TestAuditLogsSearchNoTruncationNoticeWhenResultsEndAtLimit(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			return auditLogPager(sampleAuditLogEntry("POST", "/first"), sampleAuditLogEntry("POST", "/second"))
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{Start: "2026-07-01", Limit: 2})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "/second")
	assert.NotContains(t, out, "Showing first")
}

func TestAuditLogsSearchExcludesGetByDefault(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Equal(t, []string{"GET"}, query.ExcludeMethod)
			return auditLogPager()
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{Limit: 100})
	require.NoError(t, err)
}

func TestAuditLogExcludeMethodsDeduplicatesDefaultGetCaseInsensitively(t *testing.T) {
	assert.Equal(t, []string{"GET"}, auditLogExcludeMethods("", "get", false))
}

func TestAuditLogsSearchIncludeGetDisablesDefaultExclusion(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Empty(t, query.ExcludeMethod)
			return auditLogPager()
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{IncludeGet: true, Limit: 100})
	require.NoError(t, err)
}

func TestAuditLogsSearchMethodDisablesDefaultExclusion(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Equal(t, "GET", query.Method.Value)
			assert.Empty(t, query.ExcludeMethod)
			return auditLogPager()
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{Method: "GET", Limit: 100})
	require.NoError(t, err)
}

func TestAuditLogsSearchExcludeMethodStacksWithDefaultGetExclusion(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Equal(t, []string{"GET", "post"}, query.ExcludeMethod)
			values, err := query.URLQuery()
			require.NoError(t, err)
			assert.Equal(t, "GET,post", values.Get("exclude_method"))
			return auditLogPager(sampleAuditLogEntry("DELETE", "/deleted"))
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{ExcludeMethod: "post", Limit: 100})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "/deleted")
}

func TestAuditLogsSearchIncludeGetWithExcludeMethodSendsUserExclusion(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Equal(t, []string{"POST"}, query.ExcludeMethod)
			return auditLogPager()
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{IncludeGet: true, ExcludeMethod: "POST", Limit: 100})
	require.NoError(t, err)
}

func TestAuditLogsSearchLimitTruncatesResults(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			assert.Equal(t, int64(2), query.Limit.Value)
			return auditLogPager(
				sampleAuditLogEntry("POST", "/first"),
				sampleAuditLogEntry("POST", "/second"),
				sampleAuditLogEntry("POST", "/third"),
			)
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{Start: "2026-07-01", Limit: 2})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "/first")
	assert.Contains(t, out, "/second")
	assert.NotContains(t, out, "/third")
	assert.Contains(t, out, "Showing first 2 results")
}

func TestAuditLogsSearchPrintsEmptyMessage(t *testing.T) {
	buf := capturePtermOutput(t)
	c := AuditLogsCmd{auditLogs: &FakeAuditLogsService{}}

	err := c.Search(context.Background(), AuditLogsSearchInput{Start: "2026-07-01", Limit: 100})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No audit log entries found")
}

func TestAuditLogsSearchPropagatesAPIError(t *testing.T) {
	fake := &FakeAuditLogsService{
		ListAutoPagingFunc: func(ctx context.Context, query kernel.AuditLogListParams, opts ...option.RequestOption) *pagination.PageTokenPaginationAutoPager[kernel.AuditLogEntry] {
			return pagination.NewPageTokenPaginationAutoPager[kernel.AuditLogEntry](nil, errors.New("boom"))
		},
	}
	c := AuditLogsCmd{auditLogs: fake}

	err := c.Search(context.Background(), AuditLogsSearchInput{Start: "2026-07-01", Limit: 100})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
