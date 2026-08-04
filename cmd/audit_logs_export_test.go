package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeAuditLogsExportService struct {
	CreateFunc func(ctx context.Context, body createAuditLogExportDestinationRequest) (*auditLogExportDestination, error)
	ListFunc   func(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error)
	GetFunc    func(ctx context.Context, id string) (*auditLogExportDestination, error)
	UpdateFunc func(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error)
	DeleteFunc func(ctx context.Context, id string) error
	TestFunc   func(ctx context.Context, id string) (*auditLogExportTestResult, error)
}

func (f *FakeAuditLogsExportService) Create(ctx context.Context, body createAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
	if f.CreateFunc != nil {
		return f.CreateFunc(ctx, body)
	}
	return nil, errors.New("Create not implemented")
}

func (f *FakeAuditLogsExportService) List(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, limit, offset)
	}
	return nil, auditLogExportListPageInfo{}, errors.New("List not implemented")
}

func (f *FakeAuditLogsExportService) Get(ctx context.Context, id string) (*auditLogExportDestination, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id)
	}
	return nil, errors.New("Get not implemented")
}

func (f *FakeAuditLogsExportService) Update(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
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

func (f *FakeAuditLogsExportService) Test(ctx context.Context, id string) (*auditLogExportTestResult, error) {
	if f.TestFunc != nil {
		return f.TestFunc(ctx, id)
	}
	return nil, errors.New("Test not implemented")
}

func stringPtr(s string) *string {
	return &s
}

func auditLogExportDestinationFromJSON(raw string) auditLogExportDestination {
	var d auditLogExportDestination
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		panic(err)
	}
	return d
}

func sampleAuditLogExportDestination() auditLogExportDestination {
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
		CreateFunc: func(ctx context.Context, body createAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			assert.Equal(t, "s3", body.Type)
			assert.Equal(t, "jsonl.gz", body.Format)
			assert.Equal(t, "us-east-1", body.Region)
			assert.Equal(t, "acme-audit-logs", body.Bucket)
			assert.Equal(t, "kernel/audit", body.Prefix)
			assert.Equal(t, "arn:aws:iam::123456789012:role/audit-export", body.RoleARN)
			assert.Nil(t, body.KMSKeyID)
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
		CreateFunc: func(ctx context.Context, body createAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			require.NotNil(t, body.KMSKeyID)
			assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/abc-def", *body.KMSKeyID)
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
		CreateFunc: func(ctx context.Context, body createAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
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
		ListFunc: func(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error) {
			assert.Equal(t, 20, limit)
			assert.Equal(t, 0, offset)
			return []auditLogExportDestination{sampleAuditLogExportDestination()}, auditLogExportListPageInfo{HasMore: true, NextOffset: 20}, nil
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
		ListFunc: func(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error) {
			assert.Equal(t, 50, limit)
			assert.Equal(t, 40, offset)
			return []auditLogExportDestination{sampleAuditLogExportDestination()}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 50, Offset: 40})
	require.NoError(t, err)
}

func TestAuditLogsExportListTruncatesLongLastError(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error) {
			dest := sampleAuditLogExportDestination()
			dest.LastError = "AccessDenied: this is a very long error message that exceeds sixty characters and must be truncated"
			return []auditLogExportDestination{dest}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 20})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "...")
	assert.NotContains(t, out, "must be truncated")
}

func TestAuditLogsExportListPrintsEmptyMessage(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error) {
			return []auditLogExportDestination{}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	err := c.List(context.Background(), AuditLogsExportListInput{Limit: 20})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No audit log export destinations found")
}

func TestAuditLogsExportListJSONEmptyPrintsEmptyArray(t *testing.T) {
	fake := &FakeAuditLogsExportService{
		ListFunc: func(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error) {
			return []auditLogExportDestination{}, auditLogExportListPageInfo{}, nil
		},
	}
	c := AuditLogsExportCmd{export: fake}

	var err error
	out := captureStdout(t, func() {
		err = c.List(context.Background(), AuditLogsExportListInput{Limit: 20, Output: "json"})
	})
	require.NoError(t, err)
	assert.Contains(t, out, "[]")
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
		GetFunc: func(ctx context.Context, id string) (*auditLogExportDestination, error) {
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
		GetFunc: func(ctx context.Context, id string) (*auditLogExportDestination, error) {
			return &auditLogExportDestination{ID: id, Type: "s3", Status: "paused"}, nil
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
		GetFunc: func(ctx context.Context, id string) (*auditLogExportDestination, error) {
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
		UpdateFunc: func(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			assert.Equal(t, "dest_123", id)
			require.NotNil(t, body.Bucket)
			assert.Equal(t, "new-bucket", *body.Bucket)
			require.NotNil(t, body.Prefix)
			assert.Equal(t, "new/prefix", *body.Prefix)
			assert.Nil(t, body.Region)
			assert.Nil(t, body.RoleARN)
			assert.Nil(t, body.KMSKeyID)
			assert.Nil(t, body.Status)
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
		UpdateFunc: func(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			require.NotNil(t, body.KMSKeyID)
			assert.Equal(t, "", *body.KMSKeyID)
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
	raw, err := json.Marshal(updateAuditLogExportDestinationRequest{KMSKeyID: stringPtr("")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"kms_key_id":""}`, string(raw))

	raw, err = json.Marshal(updateAuditLogExportDestinationRequest{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(raw))

	raw, err = json.Marshal(updateAuditLogExportDestinationRequest{Status: stringPtr("paused")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"paused"}`, string(raw))
}

func TestAuditLogsExportUpdateHintsOnConflict(t *testing.T) {
	fake := &FakeAuditLogsExportService{
		UpdateFunc: func(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
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
		UpdateFunc: func(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			assert.Equal(t, "dest_123", id)
			require.NotNil(t, body.Status)
			assert.Equal(t, "paused", *body.Status)
			assert.Nil(t, body.Region)
			assert.Nil(t, body.Bucket)
			assert.Nil(t, body.Prefix)
			assert.Nil(t, body.RoleARN)
			assert.Nil(t, body.KMSKeyID)
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
		UpdateFunc: func(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			require.NotNil(t, body.Status)
			assert.Equal(t, "active", *body.Status)
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
		TestFunc: func(ctx context.Context, id string) (*auditLogExportTestResult, error) {
			assert.Equal(t, "dest_123", id)
			return &auditLogExportTestResult{Success: true, Stage: "complete"}, nil
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
		TestFunc: func(ctx context.Context, id string) (*auditLogExportTestResult, error) {
			return &auditLogExportTestResult{
				Success: false,
				Stage:   "assume_role",
				Error:   &auditLogExportTestResultError{Code: "assume_role_failed", Message: "AccessDenied: not authorized to perform sts:AssumeRole"},
			}, nil
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
		TestFunc: func(ctx context.Context, id string) (*auditLogExportTestResult, error) {
			return &auditLogExportTestResult{
				Success: false,
				Stage:   "put_object",
				Error:   &auditLogExportTestResultError{Code: "put_object_failed", Message: "NoSuchBucket"},
			}, nil
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
		CreateFunc: func(ctx context.Context, body createAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			return nil, boom
		},
		ListFunc: func(ctx context.Context, limit, offset int) ([]auditLogExportDestination, auditLogExportListPageInfo, error) {
			return nil, auditLogExportListPageInfo{}, boom
		},
		GetFunc: func(ctx context.Context, id string) (*auditLogExportDestination, error) {
			return nil, boom
		},
		UpdateFunc: func(ctx context.Context, id string, body updateAuditLogExportDestinationRequest) (*auditLogExportDestination, error) {
			return nil, boom
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			return boom
		},
		TestFunc: func(ctx context.Context, id string) (*auditLogExportTestResult, error) {
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
