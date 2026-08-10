package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeAuditLogsExportService struct {
	CreateFunc func(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error)
	ListFunc   func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error)
	GetFunc    func(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error)
	UpdateFunc func(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error)
	DeleteFunc func(ctx context.Context, id string) error
	TestFunc   func(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error)
}

func (f *FakeAuditLogsExportService) Create(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
	if f.CreateFunc != nil {
		return f.CreateFunc(ctx, body)
	}
	return nil, errors.New("Create not implemented")
}

func (f *FakeAuditLogsExportService) List(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query)
	}
	return nil, auditLogExportListPageInfo{}, errors.New("List not implemented")
}

func (f *FakeAuditLogsExportService) Get(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id)
	}
	return nil, errors.New("Get not implemented")
}

func (f *FakeAuditLogsExportService) Update(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body)
	}
	return nil, errors.New("Update not implemented")
}

func (f *FakeAuditLogsExportService) Delete(ctx context.Context, id string) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, id)
	}
	return errors.New("Delete not implemented")
}

func (f *FakeAuditLogsExportService) Test(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error) {
	if f.TestFunc != nil {
		return f.TestFunc(ctx, id)
	}
	return nil, errors.New("Test not implemented")
}

func stringPtr(s string) *string {
	return &s
}

func auditLogExportDestinationFromJSON(raw string) kernel.AuditLogExportDestination {
	var d kernel.AuditLogExportDestination
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		panic(err)
	}
	return d
}

func sampleAuditLogExportDestination() kernel.AuditLogExportDestination {
	return auditLogExportDestinationFromJSON(`{
		"id": "dest_123",
		"type": "s3",
		"region": "us-east-1",
		"bucket": "acme-audit-logs",
		"prefix": "kernel/audit",
		"role_arn": "arn:aws:iam::123456789012:role/audit-export",
		"external_id": "ext_abc123",
		"kernel_role_arn": "arn:aws:iam::210987654321:role/kernel-exporter",
		"kms_key_id": "arn:aws:kms:us-east-1:123456789012:key/abc-def",
		"format": "jsonl.gz",
		"status": "active",
		"last_exported_cursor": "cursor_v1_abc",
		"last_success_at": "2026-07-01T12:00:00Z",
		"last_error": "AccessDenied: not authorized to perform s3:PutObject",
		"last_error_at": "2026-07-01T11:00:00Z",
		"consecutive_failures": 3,
		"next_attempt_at": "2026-07-01T12:05:00Z",
		"created_at": "2026-06-30T00:00:00Z",
		"updated_at": "2026-07-01T00:00:00Z"
	}`)
}

func auditLogExportTestResultFromJSON(raw string) kernel.AuditLogExportDestinationTestResult {
	var result kernel.AuditLogExportDestinationTestResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		panic(err)
	}
	return result
}

func auditLogExportAPIError(status int) *kernel.Error {
	return &kernel.Error{
		StatusCode: status,
		Request:    &http.Request{Method: http.MethodPatch, URL: &url.URL{Path: "/audit-logs/export/destinations/dest_123"}},
		Response:   &http.Response{StatusCode: status},
	}
}

func TestAuditLogsExportCreateBuildsRequestAndPrintsOnboarding(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		CreateFunc: func(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
			req := body.CreateAuditLogExportDestinationRequest
			assert.Equal(t, kernel.CreateAuditLogExportDestinationRequestTypeS3, req.Type)
			assert.Equal(t, kernel.CreateAuditLogExportDestinationRequestFormatJSONLGz, req.Format)
			assert.Equal(t, "us-east-1", req.Region)
			assert.Equal(t, "acme-audit-logs", req.Bucket)
			assert.Equal(t, "kernel/audit", req.Prefix)
			assert.Equal(t, "arn:aws:iam::123456789012:role/audit-export", req.RoleArn)
			assert.False(t, req.KmsKeyID.Valid())
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Create(context.Background(), AuditLogsExportCreateInput{
		Region:  "us-east-1",
		Bucket:  "acme-audit-logs",
		Prefix:  "kernel/audit",
		RoleARN: "arn:aws:iam::123456789012:role/audit-export",
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Created audit log export destination dest_123")
	assert.Contains(t, out, "paused")
	assert.Contains(t, out, "ext_abc123")
	assert.Contains(t, out, "arn:aws:iam::210987654321:role/kernel-exporter")
	assert.Contains(t, out, "kernel audit-logs export test dest_123")
	assert.Contains(t, out, "kernel audit-logs export resume dest_123")
}

func TestAuditLogsExportCreateIncludesKMSKeyWhenSet(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		CreateFunc: func(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
			req := body.CreateAuditLogExportDestinationRequest
			require.True(t, req.KmsKeyID.Valid())
			assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/abc-def", req.KmsKeyID.Value)
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Create(context.Background(), AuditLogsExportCreateInput{
		Region:   "us-east-1",
		Bucket:   "acme-audit-logs",
		Prefix:   "kernel/audit",
		RoleARN:  "arn:aws:iam::123456789012:role/audit-export",
		KMSKeyID: "arn:aws:kms:us-east-1:123456789012:key/abc-def",
	})
	require.NoError(t, err)
}

func TestAuditLogsExportCreateJSONPrintsObject(t *testing.T) {
	fake := &FakeAuditLogsExportService{
		CreateFunc: func(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	var err error
	out := captureStdout(t, func() {
		err = c.Create(context.Background(), AuditLogsExportCreateInput{
			Region:  "us-east-1",
			Bucket:  "acme-audit-logs",
			Prefix:  "kernel/audit",
			RoleARN: "arn:aws:iam::123456789012:role/audit-export",
			Output:  "json",
		})
	})
	require.NoError(t, err)

	assert.Contains(t, out, `"id": "dest_123"`)
	assert.Contains(t, out, `"kernel_role_arn": "arn:aws:iam::210987654321:role/kernel-exporter"`)
	assert.NotContains(t, out, "Created audit log export destination")
}

func TestAuditLogsExportListRendersTableAndPaginationHint(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
			require.True(t, query.Limit.Valid())
			assert.Equal(t, int64(20), query.Limit.Value)
			assert.False(t, query.Offset.Valid())
			return []kernel.AuditLogExportDestination{sampleAuditLogExportDestination()}, auditLogExportListPageInfo{HasMore: true, NextOffset: 20}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 20})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "dest_123")
	assert.Contains(t, out, "acme-audit-logs")
	assert.Contains(t, out, "kernel/audit")
	assert.Contains(t, out, "us-east-1")
	assert.Contains(t, out, "active")
	assert.Contains(t, out, "AccessDenied")
	assert.Contains(t, out, "--offset 20")
}

func TestAuditLogsExportListPassesLimitAndOffset(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
			require.True(t, query.Limit.Valid())
			assert.Equal(t, int64(50), query.Limit.Value)
			require.True(t, query.Offset.Valid())
			assert.Equal(t, int64(40), query.Offset.Value)
			return []kernel.AuditLogExportDestination{sampleAuditLogExportDestination()}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 50, Offset: 40})
	require.NoError(t, err)
}

func TestAuditLogsExportListTruncatesLongLastError(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
			dest := sampleAuditLogExportDestination()
			dest.LastError = "AccessDenied: this is a very long error message that exceeds sixty characters and must be truncated"
			return []kernel.AuditLogExportDestination{dest}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 20})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "...")
	assert.NotContains(t, out, "must be truncated")
}

func TestTruncateAuditLogExportErrorPreservesUTF8(t *testing.T) {
	input := strings.Repeat("a", 56) + "界" + strings.Repeat("b", 10)
	got := truncateAuditLogExportError(input)

	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, strings.Repeat("a", 56)+"界...", got)
	assert.Len(t, []rune(got), 60)
}

func TestAuditLogsExportListPrintsEmptyMessage(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
			return []kernel.AuditLogExportDestination{}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 20})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No audit log export destinations found")
}

func TestAuditLogsExportListJSONEmptyPrintsEnvelope(t *testing.T) {
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
			return []kernel.AuditLogExportDestination{}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	var err error
	out := captureStdout(t, func() {
		err = c.List(context.Background(), AuditLogsExportListInput{Limit: 20, Output: "json"})
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"destinations":[]}`, out)
}

func TestAuditLogsExportListJSONIncludesNextOffset(t *testing.T) {
	destination := sampleAuditLogExportDestination()
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
			return []kernel.AuditLogExportDestination{destination}, auditLogExportListPageInfo{HasMore: true, NextOffset: 20}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	var err error
	out := captureStdout(t, func() {
		err = c.List(context.Background(), AuditLogsExportListInput{Limit: 20, Output: "json"})
	})
	require.NoError(t, err)

	var payload struct {
		Destinations []json.RawMessage `json:"destinations"`
		NextOffset   int               `json:"next_offset"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Len(t, payload.Destinations, 1)
	assert.JSONEq(t, destination.RawJSON(), string(payload.Destinations[0]))
	assert.Equal(t, 20, payload.NextOffset)
}

func TestAuditLogsExportClientListCapturesPaginationHeaders(t *testing.T) {
	destination := sampleAuditLogExportDestination()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/audit-logs/export/destinations", r.URL.Path)
		assert.Equal(t, "2", r.URL.Query().Get("limit"))
		assert.Equal(t, "40", r.URL.Query().Get("offset"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Has-More", "true")
		w.Header().Set("X-Next-Offset", "42")
		_, _ = w.Write([]byte("[" + destination.RawJSON() + "]"))
	}))
	defer server.Close()

	client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"))
	export := auditLogsExportClient{svc: &client.AuditLogs.ExportDestinations}
	items, pageInfo, err := export.List(context.Background(), kernel.AuditLogExportDestinationListParams{
		Limit:  kernel.Opt(int64(2)),
		Offset: kernel.Opt(int64(40)),
	})

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, destination.ID, items[0].ID)
	assert.Equal(t, auditLogExportListPageInfo{HasMore: true, NextOffset: 42}, pageInfo)
}

func TestParseAuditLogExportListPagination(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		want     auditLogExportListPageInfo
		wantErr  string
	}{
		{
			name:     "more results",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}, "X-Next-Offset": []string{"120"}}},
			want:     auditLogExportListPageInfo{HasMore: true, NextOffset: 120},
		},
		{
			name:     "terminal page",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"false"}, "X-Next-Offset": []string{"0"}}},
			want:     auditLogExportListPageInfo{},
		},
		{name: "missing response", wantErr: "missing pagination headers"},
		{
			name:     "missing has more",
			response: &http.Response{Header: http.Header{"X-Next-Offset": []string{"120"}}},
			wantErr:  "invalid X-Has-More",
		},
		{
			name:     "has more with missing offset",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}}},
			wantErr:  "invalid X-Next-Offset",
		},
		{
			name:     "has more with malformed offset",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}, "X-Next-Offset": []string{"next"}}},
			wantErr:  "invalid X-Next-Offset",
		},
		{
			name:     "has more with terminal offset",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}, "X-Next-Offset": []string{"0"}}},
			wantErr:  "X-Next-Offset is not positive",
		},
		{
			name:     "terminal page with offset",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"false"}, "X-Next-Offset": []string{"120"}}},
			wantErr:  "X-Has-More is false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAuditLogExportListPagination(tt.response)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuditLogsExportListRejectsInvalidLimitAndOffset(t *testing.T) {
	c := AuditLogsExportCmd{export: &FakeAuditLogsExportService{}}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--limit")

	err = c.List(context.Background(), AuditLogsExportListInput{Limit: 101})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--limit")

	err = c.List(context.Background(), AuditLogsExportListInput{Limit: 20, Offset: -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--offset")
}

func TestAuditLogsExportGetRendersDeliveryStatus(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		GetFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
			assert.Equal(t, "dest_123", id)
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Get(context.Background(), AuditLogsExportGetInput{ID: "dest_123"})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "dest_123")
	assert.Contains(t, out, "active")
	assert.Contains(t, out, "cursor_v1_abc")
	assert.Contains(t, out, "ago)")
	assert.Contains(t, out, "AccessDenied: not authorized to perform s3:PutObject")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "2026-07-01")
	assert.Contains(t, out, "arn:aws:kms:us-east-1:123456789012:key/abc-def")
}

func TestAuditLogsExportGetRendersDashWhenNeverDelivered(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		GetFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
			return &kernel.AuditLogExportDestination{ID: id, Type: "s3", Status: "paused"}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Get(context.Background(), AuditLogsExportGetInput{ID: "dest_123"})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Last Success")
	assert.NotContains(t, out, "ago)")
	assert.NotContains(t, out, "0001-01-01")
}

func TestAuditLogsExportGetJSONPrintsObject(t *testing.T) {
	fake := &FakeAuditLogsExportService{
		GetFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	var err error
	out := captureStdout(t, func() {
		err = c.Get(context.Background(), AuditLogsExportGetInput{ID: "dest_123", Output: "json"})
	})
	require.NoError(t, err)

	assert.Contains(t, out, `"id": "dest_123"`)
	assert.Contains(t, out, `"consecutive_failures": 3`)
	assert.NotContains(t, out, "Property")
}

func TestAuditLogsExportUpdateBuildsPartialRequest(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			req := body.UpdateAuditLogExportDestinationRequest
			assert.Equal(t, "dest_123", id)
			require.True(t, req.Bucket.Valid())
			assert.Equal(t, "new-bucket", req.Bucket.Value)
			require.True(t, req.Prefix.Valid())
			assert.Equal(t, "new/prefix", req.Prefix.Value)
			assert.False(t, req.Region.Valid())
			assert.False(t, req.RoleArn.Valid())
			assert.False(t, req.KmsKeyID.Valid())
			assert.Empty(t, req.Status)
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Update(context.Background(), AuditLogsExportUpdateInput{
		ID:     "dest_123",
		Bucket: stringPtr("new-bucket"),
		Prefix: stringPtr("new/prefix"),
	})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Updated audit log export destination dest_123")
}

func TestAuditLogsExportUpdateClearKMSKeySendsEmptyString(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			req := body.UpdateAuditLogExportDestinationRequest
			require.True(t, req.KmsKeyID.Valid())
			assert.Equal(t, "", req.KmsKeyID.Value)
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Update(context.Background(), AuditLogsExportUpdateInput{ID: "dest_123", ClearKMSKey: true})
	require.NoError(t, err)
}

func TestAuditLogsExportUpdateRejectsKMSKeyAndClear(t *testing.T) {
	c := AuditLogsExportCmd{export: &FakeAuditLogsExportService{}}

	err := c.Update(context.Background(), AuditLogsExportUpdateInput{
		ID:          "dest_123",
		KMSKeyID:    stringPtr("arn:aws:kms:us-east-1:123456789012:key/abc-def"),
		ClearKMSKey: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--kms-key-id")
	assert.Contains(t, err.Error(), "--clear-kms-key")
}

func TestAuditLogsExportUpdateRequiresAChange(t *testing.T) {
	c := AuditLogsExportCmd{export: &FakeAuditLogsExportService{}}

	err := c.Update(context.Background(), AuditLogsExportUpdateInput{ID: "dest_123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to update")
}

func TestAuditLogsExportUpdateRequestSerialization(t *testing.T) {
	raw, err := json.Marshal(kernel.UpdateAuditLogExportDestinationRequestParam{KmsKeyID: kernel.String("")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"kms_key_id":""}`, string(raw))

	raw, err = json.Marshal(kernel.UpdateAuditLogExportDestinationRequestParam{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(raw))

	raw, err = json.Marshal(kernel.UpdateAuditLogExportDestinationRequestParam{Status: kernel.UpdateAuditLogExportDestinationRequestStatusPaused})
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"paused"}`, string(raw))
}

func TestAuditLogsExportUpdateHintsOnConflict(t *testing.T) {
	fake := &FakeAuditLogsExportService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			return nil, auditLogExportAPIError(http.StatusConflict)
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Update(context.Background(), AuditLogsExportUpdateInput{ID: "dest_123", Bucket: stringPtr("new-bucket")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concurrently")
}

func TestAuditLogsExportPauseSendsStatusPaused(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			req := body.UpdateAuditLogExportDestinationRequest
			assert.Equal(t, "dest_123", id)
			assert.Equal(t, kernel.UpdateAuditLogExportDestinationRequestStatusPaused, req.Status)
			assert.False(t, req.Region.Valid())
			assert.False(t, req.Bucket.Valid())
			assert.False(t, req.Prefix.Valid())
			assert.False(t, req.RoleArn.Valid())
			assert.False(t, req.KmsKeyID.Valid())
			dest := sampleAuditLogExportDestination()
			dest.Status = "paused"
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.SetStatus(context.Background(), AuditLogsExportStatusInput{ID: "dest_123", Status: "paused"})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Paused audit log export destination dest_123")
	assert.Contains(t, out, "in progress")
}

func TestAuditLogsExportResumeSendsStatusActive(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			req := body.UpdateAuditLogExportDestinationRequest
			assert.Equal(t, kernel.UpdateAuditLogExportDestinationRequestStatusActive, req.Status)
			dest := sampleAuditLogExportDestination()
			return &dest, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.SetStatus(context.Background(), AuditLogsExportStatusInput{ID: "dest_123", Status: "active"})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Resumed audit log export destination dest_123")
	assert.NotContains(t, out, "in progress")
}

func TestAuditLogsExportSetStatusRejectsInvalidStatus(t *testing.T) {
	c := AuditLogsExportCmd{export: &FakeAuditLogsExportService{}}

	err := c.SetStatus(context.Background(), AuditLogsExportStatusInput{ID: "dest_123", Status: "stopped"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestAuditLogsExportDeletePrintsSuccess(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		DeleteFunc: func(ctx context.Context, id string) error {
			assert.Equal(t, "dest_123", id)
			return nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Delete(context.Background(), AuditLogsExportDeleteInput{ID: "dest_123"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Deleted audit log export destination dest_123")
}

func TestAuditLogsExportTestPassesPrintsSuccess(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		TestFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error) {
			assert.Equal(t, "dest_123", id)
			result := auditLogExportTestResultFromJSON(`{"success":true,"stage":"complete"}`)
			return &result, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Test(context.Background(), AuditLogsExportTestInput{ID: "dest_123"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Test passed (stage: complete)")
}

func TestAuditLogsExportTestFailurePrintsDetailsAndReturnsError(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		TestFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error) {
			result := auditLogExportTestResultFromJSON(`{"success":false,"stage":"assume_role","error":{"code":"assume_role_failed","message":"AccessDenied: not authorized to perform sts:AssumeRole"}}`)
			return &result, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.Test(context.Background(), AuditLogsExportTestInput{ID: "dest_123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assume_role")

	out := buf.String()
	assert.Contains(t, out, "assume_role")
	assert.Contains(t, out, "assume_role_failed")
	assert.Contains(t, out, "AccessDenied")
}

func TestAuditLogsExportTestJSONFailurePrintsResultAndReturnsError(t *testing.T) {
	fake := &FakeAuditLogsExportService{
		TestFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error) {
			result := auditLogExportTestResultFromJSON(`{"success":false,"stage":"put_object","error":{"code":"put_object_failed","message":"NoSuchBucket"}}`)
			return &result, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	var err error
	out := captureStdout(t, func() {
		err = c.Test(context.Background(), AuditLogsExportTestInput{ID: "dest_123", Output: "json"})
	})
	require.Error(t, err)

	assert.Contains(t, out, `"success": false`)
	assert.Contains(t, out, `"stage": "put_object"`)
	assert.Contains(t, out, `"code": "put_object_failed"`)
}

func TestAuditLogsExportRejectsInvalidJSONOutput(t *testing.T) {
	c := AuditLogsExportCmd{export: &FakeAuditLogsExportService{}}
	ctx := context.Background()

	errs := []error{
		c.Create(ctx, AuditLogsExportCreateInput{Output: "yaml"}),
		c.List(ctx, AuditLogsExportListInput{Limit: 20, Output: "yaml"}),
		c.Get(ctx, AuditLogsExportGetInput{ID: "dest_123", Output: "yaml"}),
		c.Update(ctx, AuditLogsExportUpdateInput{ID: "dest_123", Bucket: stringPtr("b"), Output: "yaml"}),
		c.SetStatus(ctx, AuditLogsExportStatusInput{ID: "dest_123", Status: "paused", Output: "yaml"}),
		c.Test(ctx, AuditLogsExportTestInput{ID: "dest_123", Output: "yaml"}),
	}
	for _, err := range errs {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported --output value")
	}
}

func TestAuditLogsExportPropagatesAPIErrors(t *testing.T) {
	boom := errors.New("boom")
	c := AuditLogsExportCmd{export: &FakeAuditLogsExportService{
		CreateFunc: func(ctx context.Context, body kernel.AuditLogExportDestinationNewParams) (*kernel.AuditLogExportDestination, error) {
			return nil, boom
		},
		ListFunc: func(ctx context.Context, query kernel.AuditLogExportDestinationListParams) ([]kernel.AuditLogExportDestination, auditLogExportListPageInfo, error) {
			return nil, auditLogExportListPageInfo{}, boom
		},
		GetFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestination, error) {
			return nil, boom
		},
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuditLogExportDestinationUpdateParams) (*kernel.AuditLogExportDestination, error) {
			return nil, boom
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			return boom
		},
		TestFunc: func(ctx context.Context, id string) (*kernel.AuditLogExportDestinationTestResult, error) {
			return nil, boom
		},
	}}
	ctx := context.Background()

	assert.ErrorContains(t, c.Create(ctx, AuditLogsExportCreateInput{}), "boom")
	assert.ErrorContains(t, c.List(ctx, AuditLogsExportListInput{Limit: 20}), "boom")
	assert.ErrorContains(t, c.Get(ctx, AuditLogsExportGetInput{ID: "dest_123"}), "boom")
	assert.ErrorContains(t, c.Update(ctx, AuditLogsExportUpdateInput{ID: "dest_123", Bucket: stringPtr("b")}), "boom")
	assert.ErrorContains(t, c.SetStatus(ctx, AuditLogsExportStatusInput{ID: "dest_123", Status: "paused"}), "boom")
	assert.ErrorContains(t, c.Delete(ctx, AuditLogsExportDeleteInput{ID: "dest_123"}), "boom")
	assert.ErrorContains(t, c.Test(ctx, AuditLogsExportTestInput{ID: "dest_123"}), "boom")
}
