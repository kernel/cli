package cmd

import (
	"context"
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
