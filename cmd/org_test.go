package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/stretchr/testify/assert"
)

type FakeOrgLimitsService struct {
	GetFunc    func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgLimits, error)
	UpdateFunc func(ctx context.Context, body kernel.OrganizationLimitUpdateParams, opts ...option.RequestOption) (*kernel.OrgLimits, error)
}

type FakeOrgEntitlementsService struct {
	GetFunc func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error)
}

func (f *FakeOrgEntitlementsService) Get(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, opts...)
	}
	return nil, nil
}

func testOrgEntitlements(t *testing.T) *kernel.OrgEntitlements {
	t.Helper()
	var entitlements kernel.OrgEntitlements
	err := json.Unmarshal([]byte(`{
		"plan":{"id":"FREE","effective_id":"START_UP","status":null,"is_trialing":true,"trial_ends_at":null},
		"features":{
			"profiles":{"enabled":true},
			"file_io":{"enabled":true},
			"browser_replays":{"enabled":true,"retention_days":30},
			"browser_extensions":{"enabled":true,"max_stored_per_org":null},
			"browser_pools":{"enabled":true},
			"managed_auth":{"enabled":true,"max_connections":null,"health_check_interval_min_seconds":1200,"health_check_interval_default_seconds":3600,"health_check_interval_max_seconds":86400},
			"credentials":{"enabled":true},
			"credential_providers":{"enabled":true},
			"managed_proxies":{"enabled":true},
			"custom_proxies":{"enabled":true},
			"proxy_bypass_hosts":{"enabled":true},
			"gpu":{"enabled":false}
		},
		"limits":{"max_concurrent_browsers":150,"max_concurrent_invocations":150,"default_max_concurrent_invocations_per_app":20}
	}`), &entitlements)
	assert.NoError(t, err)
	return &entitlements
}

func TestOrgEntitlements_RendersEffectiveValues(t *testing.T) {
	buf := capturePtermOutput(t)
	c := OrgCmd{entitlements: &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			return testOrgEntitlements(t), nil
		},
	}}

	assert.NoError(t, c.Entitlements(context.Background(), OrgEntitlementsInput{}))
	out := buf.String()
	assert.Contains(t, out, "Contractual plan")
	assert.Contains(t, out, "Effective plan")
	assert.Contains(t, out, "START_UP")
	assert.Contains(t, out, "Browser replay retention (days)")
	assert.Contains(t, out, "Max managed auth connections")
	assert.Contains(t, out, "unlimited")
	assert.Contains(t, out, "Max concurrent browsers")
}

func TestOrgEntitlements_JSONPreservesNullUnlimitedValues(t *testing.T) {
	c := OrgCmd{entitlements: &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			return testOrgEntitlements(t), nil
		},
	}}

	out := captureStdout(t, func() {
		assert.NoError(t, c.Entitlements(context.Background(), OrgEntitlementsInput{Output: "json"}))
	})
	assert.Contains(t, out, `"max_stored_per_org": null`)
	assert.Contains(t, out, `"max_connections": null`)
}

func TestOrgEntitlements_SurfacesAPIError(t *testing.T) {
	capturePtermOutput(t)
	c := OrgCmd{entitlements: &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			return nil, errors.New("boom")
		},
	}}

	assert.Error(t, c.Entitlements(context.Background(), OrgEntitlementsInput{}))
}

func (f *FakeOrgLimitsService) Get(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, opts...)
	}
	return &kernel.OrgLimits{MaxConcurrentSessions: 100}, nil
}

func (f *FakeOrgLimitsService) Update(ctx context.Context, body kernel.OrganizationLimitUpdateParams, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, body, opts...)
	}
	return &kernel.OrgLimits{MaxConcurrentSessions: 100}, nil
}

func TestOrgLimitsGet_RendersLimits(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeOrgLimitsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
			limits := &kernel.OrgLimits{MaxConcurrentSessions: 100, DefaultProjectMaxConcurrentSessions: 25}
			limits.JSON.DefaultProjectMaxConcurrentSessions = respjson.NewField("25")
			return limits, nil
		},
	}
	c := OrgCmd{limits: fake}
	assert.NoError(t, c.LimitsGet(context.Background(), OrgLimitsGetInput{}))

	out := buf.String()
	assert.Contains(t, out, "Max Concurrent Sessions")
	assert.Contains(t, out, "100")
	assert.Contains(t, out, "25")
}

func TestOrgLimitsGet_NullDefaultShownAsUnlimited(t *testing.T) {
	buf := capturePtermOutput(t)
	c := OrgCmd{limits: &FakeOrgLimitsService{}}
	assert.NoError(t, c.LimitsGet(context.Background(), OrgLimitsGetInput{}))
	assert.Contains(t, buf.String(), "unlimited")
}

func TestOrgLimitsGet_RendersManagedAuthLimits(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeOrgLimitsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
			limits := &kernel.OrgLimits{
				MaxConcurrentSessions:         100,
				MaxAuthConnections:            10,
				AuthConnectionsUsed:           3,
				MinHealthCheckIntervalSeconds: 300,
			}
			limits.JSON.MaxAuthConnections = respjson.NewField("10")
			limits.JSON.AuthConnectionsUsed = respjson.NewField("3")
			limits.JSON.MinHealthCheckIntervalSeconds = respjson.NewField("300")
			return limits, nil
		},
	}
	c := OrgCmd{limits: fake}
	assert.NoError(t, c.LimitsGet(context.Background(), OrgLimitsGetInput{}))

	out := buf.String()
	assert.Contains(t, out, "Max Auth Connections")
	assert.Contains(t, out, "Auth Connections Used")
	assert.Contains(t, out, "Min Health Check Interval")
	assert.Contains(t, out, "300s")
}

func TestOrgLimitsGet_NullMaxAuthConnectionsShownAsUnlimited(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeOrgLimitsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
			limits := &kernel.OrgLimits{MaxConcurrentSessions: 100, DefaultProjectMaxConcurrentSessions: 25}
			limits.JSON.DefaultProjectMaxConcurrentSessions = respjson.NewField("25")
			// Null (not omitted) means the plan allows unlimited connections.
			limits.JSON.MaxAuthConnections = respjson.NewField(respjson.Null)
			return limits, nil
		},
	}
	c := OrgCmd{limits: fake}
	assert.NoError(t, c.LimitsGet(context.Background(), OrgLimitsGetInput{}))

	out := buf.String()
	assert.Contains(t, out, "Max Auth Connections")
	assert.Contains(t, out, "unlimited")
}

func TestOrgLimitsGet_OmitsManagedAuthRowsWhenAbsent(t *testing.T) {
	buf := capturePtermOutput(t)
	c := OrgCmd{limits: &FakeOrgLimitsService{}}
	assert.NoError(t, c.LimitsGet(context.Background(), OrgLimitsGetInput{}))

	out := buf.String()
	assert.NotContains(t, out, "Max Auth Connections")
	assert.NotContains(t, out, "Auth Connections Used")
	assert.NotContains(t, out, "Min Health Check Interval")
}

func TestOrgLimitsGet_SurfacesAPIError(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeOrgLimitsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
			return nil, errors.New("boom")
		},
	}
	c := OrgCmd{limits: fake}
	assert.Error(t, c.LimitsGet(context.Background(), OrgLimitsGetInput{}))
}

func TestOrgLimitsSet_SendsDefault(t *testing.T) {
	buf := capturePtermOutput(t)
	var captured kernel.OrganizationLimitUpdateParams
	fake := &FakeOrgLimitsService{
		UpdateFunc: func(ctx context.Context, body kernel.OrganizationLimitUpdateParams, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
			captured = body
			limits := &kernel.OrgLimits{MaxConcurrentSessions: 100, DefaultProjectMaxConcurrentSessions: 50}
			limits.JSON.DefaultProjectMaxConcurrentSessions = respjson.NewField("50")
			return limits, nil
		},
	}
	c := OrgCmd{limits: fake}
	assert.NoError(t, c.LimitsSet(context.Background(), OrgLimitsSetInput{
		DefaultProjectMaxConcurrentSessions: Int64Flag{Set: true, Value: 50},
	}))

	req := captured.UpdateOrgLimitsRequest
	assert.True(t, req.DefaultProjectMaxConcurrentSessions.Valid())
	assert.Equal(t, int64(50), req.DefaultProjectMaxConcurrentSessions.Value)
	assert.Contains(t, buf.String(), "Organization limits updated")
}

func TestOrgLimitsSet_ZeroRemovesDefault(t *testing.T) {
	capturePtermOutput(t)
	var captured kernel.OrganizationLimitUpdateParams
	fake := &FakeOrgLimitsService{
		UpdateFunc: func(ctx context.Context, body kernel.OrganizationLimitUpdateParams, opts ...option.RequestOption) (*kernel.OrgLimits, error) {
			captured = body
			return &kernel.OrgLimits{MaxConcurrentSessions: 100}, nil
		},
	}
	c := OrgCmd{limits: fake}
	assert.NoError(t, c.LimitsSet(context.Background(), OrgLimitsSetInput{
		DefaultProjectMaxConcurrentSessions: Int64Flag{Set: true, Value: 0},
	}))
	// 0 must still be sent explicitly — it is how the default gets removed.
	assert.True(t, captured.UpdateOrgLimitsRequest.DefaultProjectMaxConcurrentSessions.Valid())
	assert.Equal(t, int64(0), captured.UpdateOrgLimitsRequest.DefaultProjectMaxConcurrentSessions.Value)
}

func TestOrgLimitsSet_RequiresFlag(t *testing.T) {
	c := OrgCmd{limits: &FakeOrgLimitsService{}}
	err := c.LimitsSet(context.Background(), OrgLimitsSetInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must provide --default-project-max-concurrent-sessions")
}

func TestOrgLimitsSet_RejectsNegative(t *testing.T) {
	c := OrgCmd{limits: &FakeOrgLimitsService{}}
	err := c.LimitsSet(context.Background(), OrgLimitsSetInput{
		DefaultProjectMaxConcurrentSessions: Int64Flag{Set: true, Value: -1},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}
