package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/pterm/pterm"
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

func testOrgEntitlementsWithUnlimitedValues(t *testing.T) *kernel.OrgEntitlements {
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

func TestOrgEntitlementRows_CompleteProjection(t *testing.T) {
	var entitlements kernel.OrgEntitlements
	err := json.Unmarshal([]byte(`{
		"plan":{"id":"HOBBYIST","effective_id":"START_UP","status":"ACTIVE","is_trialing":true,"trial_ends_at":"2030-01-02T03:04:05Z"},
		"features":{
			"profiles":{"enabled":true},
			"file_io":{"enabled":false},
			"browser_replays":{"enabled":true,"retention_days":17},
			"browser_extensions":{"enabled":false,"max_stored_per_org":23},
			"browser_pools":{"enabled":true},
			"managed_auth":{"enabled":false,"max_connections":29,"health_check_interval_min_seconds":31,"health_check_interval_default_seconds":37,"health_check_interval_max_seconds":41},
			"credentials":{"enabled":true},
			"credential_providers":{"enabled":false},
			"managed_proxies":{"enabled":true},
			"custom_proxies":{"enabled":false},
			"proxy_bypass_hosts":{"enabled":true},
			"gpu":{"enabled":false}
		},
		"limits":{"max_concurrent_browsers":43,"max_concurrent_invocations":47,"default_max_concurrent_invocations_per_app":53}
	}`), &entitlements)
	assert.NoError(t, err)

	assert.Equal(t, pterm.TableData{
		{"Category", "Entitlement", "Value"},
		{"Plan", "Contractual plan", "HOBBYIST"},
		{"Plan", "Effective plan", "START_UP"},
		{"Plan", "Status", "ACTIVE"},
		{"Plan", "Trialing", "true"},
		{"Plan", "Trial ends at", util.FormatLocal(entitlements.Plan.TrialEndsAt)},
		{"Feature", "Profiles", "true"},
		{"Feature", "File I/O", "false"},
		{"Feature", "Browser replays", "true"},
		{"Feature", "Browser replay retention (days)", "17"},
		{"Feature", "Browser extensions", "false"},
		{"Feature", "Max stored extensions", "23"},
		{"Feature", "Browser pools", "true"},
		{"Feature", "Managed auth", "false"},
		{"Feature", "Max managed auth connections", "29"},
		{"Feature", "Health check minimum (seconds)", "31"},
		{"Feature", "Health check default (seconds)", "37"},
		{"Feature", "Health check maximum (seconds)", "41"},
		{"Feature", "Credentials", "true"},
		{"Feature", "Credential providers", "false"},
		{"Feature", "Managed proxies", "true"},
		{"Feature", "Custom proxies", "false"},
		{"Feature", "Proxy bypass hosts", "true"},
		{"Feature", "GPU", "false"},
		{"Limit", "Max concurrent browsers", "43"},
		{"Limit", "Max concurrent invocations", "47"},
		{"Limit", "Default max concurrent invocations per app", "53"},
	}, orgEntitlementRows(&entitlements))
}

func TestOrgEntitlementRows_BooleanFieldProvenance(t *testing.T) {
	tests := []struct {
		entitlement string
		set         func(*kernel.OrgEntitlements)
	}{
		{"Trialing", func(e *kernel.OrgEntitlements) { e.Plan.IsTrialing = true }},
		{"Profiles", func(e *kernel.OrgEntitlements) { e.Features.Profiles.Enabled = true }},
		{"File I/O", func(e *kernel.OrgEntitlements) { e.Features.FileIo.Enabled = true }},
		{"Browser replays", func(e *kernel.OrgEntitlements) { e.Features.BrowserReplays.Enabled = true }},
		{"Browser extensions", func(e *kernel.OrgEntitlements) { e.Features.BrowserExtensions.Enabled = true }},
		{"Browser pools", func(e *kernel.OrgEntitlements) { e.Features.BrowserPools.Enabled = true }},
		{"Managed auth", func(e *kernel.OrgEntitlements) { e.Features.ManagedAuth.Enabled = true }},
		{"Credentials", func(e *kernel.OrgEntitlements) { e.Features.Credentials.Enabled = true }},
		{"Credential providers", func(e *kernel.OrgEntitlements) { e.Features.CredentialProviders.Enabled = true }},
		{"Managed proxies", func(e *kernel.OrgEntitlements) { e.Features.ManagedProxies.Enabled = true }},
		{"Custom proxies", func(e *kernel.OrgEntitlements) { e.Features.CustomProxies.Enabled = true }},
		{"Proxy bypass hosts", func(e *kernel.OrgEntitlements) { e.Features.ProxyBypassHosts.Enabled = true }},
		{"GPU", func(e *kernel.OrgEntitlements) { e.Features.GPU.Enabled = true }},
	}

	for _, tt := range tests {
		t.Run(tt.entitlement, func(t *testing.T) {
			var entitlements kernel.OrgEntitlements
			tt.set(&entitlements)
			rows := orgEntitlementRows(&entitlements)
			values := make(map[string]string, len(rows))
			for _, row := range rows {
				values[row[1]] = row[2]
			}

			for _, candidate := range tests {
				expected := "false"
				if candidate.entitlement == tt.entitlement {
					expected = "true"
				}
				assert.Equal(t, expected, values[candidate.entitlement], candidate.entitlement)
			}
		})
	}
}

func TestOrgEntitlementRows_NullableFieldStates(t *testing.T) {
	populatedTrialEnd, err := time.Parse(time.RFC3339, "2031-02-03T04:05:06Z")
	assert.NoError(t, err)

	tests := []struct {
		name                 string
		payload              string
		expectedStatus       string
		expectedTrialEnd     string
		expectedMaxStored    string
		expectedMaxAuthConns string
	}{
		{
			name:                 "populated",
			payload:              `{"plan":{"status":"ACTIVE","trial_ends_at":"2031-02-03T04:05:06Z"},"features":{"browser_extensions":{"max_stored_per_org":11},"managed_auth":{"max_connections":13}}}`,
			expectedStatus:       "ACTIVE",
			expectedTrialEnd:     util.FormatLocal(populatedTrialEnd),
			expectedMaxStored:    "11",
			expectedMaxAuthConns: "13",
		},
		{
			name:                 "explicit null",
			payload:              `{"plan":{"status":null,"trial_ends_at":null},"features":{"browser_extensions":{"max_stored_per_org":null},"managed_auth":{"max_connections":null}}}`,
			expectedStatus:       "none",
			expectedTrialEnd:     "none",
			expectedMaxStored:    "unlimited",
			expectedMaxAuthConns: "unlimited",
		},
		{
			name:                 "omitted",
			payload:              `{"plan":{},"features":{"browser_extensions":{},"managed_auth":{}}}`,
			expectedStatus:       "unknown",
			expectedTrialEnd:     "unknown",
			expectedMaxStored:    "unknown",
			expectedMaxAuthConns: "unknown",
		},
		{
			name:                 "malformed",
			payload:              `{"plan":{"status":7,"trial_ends_at":"not-a-date"},"features":{"browser_extensions":{"max_stored_per_org":"many"},"managed_auth":{"max_connections":"many"}}}`,
			expectedStatus:       "unknown",
			expectedTrialEnd:     "unknown",
			expectedMaxStored:    "unknown",
			expectedMaxAuthConns: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entitlements kernel.OrgEntitlements
			assert.NoError(t, json.Unmarshal([]byte(tt.payload), &entitlements))

			rows := orgEntitlementRows(&entitlements)
			assert.Equal(t, pterm.TableData{
				{"Plan", "Status", tt.expectedStatus},
				{"Plan", "Trial ends at", tt.expectedTrialEnd},
				{"Feature", "Max stored extensions", tt.expectedMaxStored},
				{"Feature", "Max managed auth connections", tt.expectedMaxAuthConns},
			}, pterm.TableData{rows[3], rows[5], rows[11], rows[14]})
		})
	}
}

func TestOrgEntitlements_RendersTrialEndInLocalTime(t *testing.T) {
	var entitlements kernel.OrgEntitlements
	err := json.Unmarshal([]byte(`{
		"plan":{"id":"FREE","effective_id":"START_UP","status":"ACTIVE","is_trialing":true,"trial_ends_at":"2030-01-02T03:04:05Z"},
		"features":{},
		"limits":{}
	}`), &entitlements)
	assert.NoError(t, err)

	buf := capturePtermOutput(t)
	renderOrgEntitlements(&entitlements)

	assert.Contains(t, buf.String(), util.FormatLocal(entitlements.Plan.TrialEndsAt))
}

func TestOrgEntitlements_JSONPreservesNullUnlimitedValues(t *testing.T) {
	c := OrgCmd{entitlements: &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			return testOrgEntitlementsWithUnlimitedValues(t), nil
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

type FakeOrgEntitlementsService struct {
	GetFunc func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error)
}

func (f *FakeOrgEntitlementsService) Get(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, opts...)
	}
	return &kernel.OrgEntitlements{}, nil
}

// populatedEntitlements builds an entitlements payload with every nullable field
// present, so renders exercise the non-"unlimited" branches.
func populatedEntitlements() *kernel.OrgEntitlements {
	ent := &kernel.OrgEntitlements{}

	ent.Plan.ID = "START_UP"
	ent.Plan.EffectiveID = "START_UP"
	ent.Plan.IsTrialing = true
	ent.Plan.Status = "ACTIVE"
	ent.Plan.TrialEndsAt = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	ent.Plan.JSON.Status = respjson.NewField(`"ACTIVE"`)
	ent.Plan.JSON.TrialEndsAt = respjson.NewField(`"2030-01-02T03:04:05Z"`)

	ent.Features.BrowserExtensions.Enabled = true
	ent.Features.BrowserExtensions.MaxStoredPerOrg = 25
	ent.Features.BrowserExtensions.JSON.MaxStoredPerOrg = respjson.NewField("25")
	ent.Features.BrowserPools.Enabled = true
	ent.Features.BrowserReplays.Enabled = true
	ent.Features.BrowserReplays.RetentionDays = 7
	ent.Features.BrowserReplays.JSON.RetentionDays = respjson.NewField("7")
	ent.Features.CredentialProviders.Enabled = true
	ent.Features.Credentials.Enabled = true
	ent.Features.CustomProxies.Enabled = false
	ent.Features.FileIo.Enabled = true
	ent.Features.GPU.Enabled = false
	ent.Features.ManagedAuth.Enabled = true
	ent.Features.ManagedAuth.MaxConnections = 10
	ent.Features.ManagedAuth.HealthCheckIntervalDefaultSeconds = 600
	ent.Features.ManagedAuth.HealthCheckIntervalMinSeconds = 300
	ent.Features.ManagedAuth.HealthCheckIntervalMaxSeconds = 86400
	ent.Features.ManagedAuth.JSON.MaxConnections = respjson.NewField("10")
	ent.Features.ManagedProxies.Enabled = true
	ent.Features.Profiles.Enabled = true
	ent.Features.ProxyBypassHosts.Enabled = true

	ent.Limits.MaxConcurrentBrowsers = 50
	ent.Limits.MaxConcurrentInvocations = 20
	ent.Limits.DefaultMaxConcurrentInvocationsPerApp = 5
	ent.Limits.JSON.MaxConcurrentBrowsers = respjson.NewField("50")
	ent.Limits.JSON.MaxConcurrentInvocations = respjson.NewField("20")
	ent.Limits.JSON.DefaultMaxConcurrentInvocationsPerApp = respjson.NewField("5")

	return ent
}

func TestOrgEntitlementsGet_RendersPlanFeaturesAndLimits(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			return populatedEntitlements(), nil
		},
	}
	c := OrgCmd{entitlements: fake}
	assert.NoError(t, c.EntitlementsGet(context.Background(), OrgEntitlementsGetInput{}))

	out := buf.String()
	// Plan section
	assert.Contains(t, out, "START_UP")
	assert.Contains(t, out, "Effective Plan")
	assert.Contains(t, out, "Trialing")
	assert.Contains(t, out, "ACTIVE")
	// Features section — every feature should get a row.
	for _, feature := range []string{
		"Browser Extensions", "Browser Pools", "Browser Replays", "Credential Providers",
		"Credentials", "Custom Proxies", "File I/O", "GPU", "Managed Auth",
		"Managed Proxies", "Profiles", "Proxy Bypass Hosts",
	} {
		assert.Contains(t, out, feature)
	}
	assert.Contains(t, out, "max stored per org: 25")
	assert.Contains(t, out, "retention: 7 days")
	assert.Contains(t, out, "max connections: 10")
	assert.Contains(t, out, "600s default (300s-86400s)")
	// Limits section
	assert.Contains(t, out, "Max Concurrent Browsers")
	assert.Contains(t, out, "Max Concurrent Invocations")
	assert.Contains(t, out, "Default Max Concurrent Invocations Per App")
}

func TestOrgEntitlementsGet_NullConstraintsShownAsUnlimited(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			ent := populatedEntitlements()
			// Null (not omitted) constraints mean unlimited.
			ent.Features.BrowserExtensions.JSON.MaxStoredPerOrg = respjson.NewField(respjson.Null)
			ent.Features.ManagedAuth.JSON.MaxConnections = respjson.NewField(respjson.Null)
			ent.Limits.JSON.MaxConcurrentBrowsers = respjson.NewField(respjson.Null)
			return ent, nil
		},
	}
	c := OrgCmd{entitlements: fake}
	assert.NoError(t, c.EntitlementsGet(context.Background(), OrgEntitlementsGetInput{}))

	out := buf.String()
	assert.Contains(t, out, "max stored per org: unlimited")
	assert.Contains(t, out, "max connections: unlimited")
	assert.Contains(t, out, "unlimited")
}

func TestOrgEntitlementsGet_NullPlanFieldsShownAsDash(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			ent := populatedEntitlements()
			ent.Plan.IsTrialing = false
			ent.Plan.JSON.Status = respjson.NewField(respjson.Null)
			ent.Plan.JSON.TrialEndsAt = respjson.NewField(respjson.Null)
			return ent, nil
		},
	}
	c := OrgCmd{entitlements: fake}
	assert.NoError(t, c.EntitlementsGet(context.Background(), OrgEntitlementsGetInput{}))

	out := buf.String()
	assert.Contains(t, out, "Billing Status")
	assert.Contains(t, out, "Trial Ends At")
	assert.NotContains(t, out, "ACTIVE")
}

func TestOrgEntitlementsGet_RejectsUnknownOutput(t *testing.T) {
	c := OrgCmd{entitlements: &FakeOrgEntitlementsService{}}
	assert.Error(t, c.EntitlementsGet(context.Background(), OrgEntitlementsGetInput{Output: "yaml"}))
}

func TestOrgEntitlementsGet_SurfacesAPIError(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeOrgEntitlementsService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.OrgEntitlements, error) {
			return nil, errors.New("boom")
		},
	}
	c := OrgCmd{entitlements: fake}
	assert.Error(t, c.EntitlementsGet(context.Background(), OrgEntitlementsGetInput{}))
}
