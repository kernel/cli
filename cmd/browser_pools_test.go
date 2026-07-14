package cmd

import (
	"context"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeBrowserPoolsService is a configurable fake implementing BrowserPoolsService.
type FakeBrowserPoolsService struct {
	AcquireFunc func(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error)
	ListFunc    func(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserPool], error)
	NewFunc     func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error)
	UpdateFunc  func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error)
}

func (f *FakeBrowserPoolsService) List(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserPool], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.BrowserPool]{Items: []kernel.BrowserPool{}}, nil
}

func (f *FakeBrowserPoolsService) New(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.BrowserPool{}, nil
}

func (f *FakeBrowserPoolsService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	return &kernel.BrowserPool{}, nil
}

func (f *FakeBrowserPoolsService) Update(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserPool{}, nil
}

func (f *FakeBrowserPoolsService) Delete(ctx context.Context, id string, body kernel.BrowserPoolDeleteParams, opts ...option.RequestOption) error {
	return nil
}

func (f *FakeBrowserPoolsService) Acquire(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error) {
	if f.AcquireFunc != nil {
		return f.AcquireFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserPoolAcquireResponse{}, nil
}

func (f *FakeBrowserPoolsService) Release(ctx context.Context, id string, body kernel.BrowserPoolReleaseParams, opts ...option.RequestOption) error {
	return nil
}

func (f *FakeBrowserPoolsService) Flush(ctx context.Context, id string, opts ...option.RequestOption) error {
	return nil
}

func TestBrowserPoolsAcquire_WithNameAndTags(t *testing.T) {
	setupStdoutCapture(t)

	var capturedID string
	var captured kernel.BrowserPoolAcquireParams
	fake := &FakeBrowserPoolsService{
		AcquireFunc: func(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error) {
			capturedID = id
			captured = body
			return &kernel.BrowserPoolAcquireResponse{
				SessionID: "sess-acq",
				CdpWsURL:  "ws://cdp-acq",
				Name:      "lease-name",
				Tags:      kernel.Tags{"env": "prod"},
			}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Acquire(context.Background(), BrowserPoolsAcquireInput{
		IDOrName: "my-pool",
		Name:     "lease-name",
		Tags:     map[string]string{"env": "prod"},
	})
	assert.NoError(t, err)

	// Pool lookup is by id or name; name + tags are forwarded per-lease.
	assert.Equal(t, "my-pool", capturedID)
	assert.True(t, captured.Name.Valid())
	assert.Equal(t, "lease-name", captured.Name.Value)
	assert.Equal(t, "prod", captured.Tags["env"])

	// And surfaced in the acquired-session table.
	out := outBuf.String()
	assert.Contains(t, out, "lease-name")
	assert.Contains(t, out, "Tags")
	assert.Contains(t, out, "env=prod")
}

func TestBrowserPoolsList_ForwardsLimitOffset(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolListParams
	fake := &FakeBrowserPoolsService{
		ListFunc: func(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserPool], error) {
			captured = query
			return &pagination.OffsetPagination[kernel.BrowserPool]{Items: []kernel.BrowserPool{}}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.List(context.Background(), BrowserPoolsListInput{Limit: 4, Offset: 8})

	assert.NoError(t, err)
	assert.Equal(t, int64(4), captured.Limit.Value)
	assert.Equal(t, int64(8), captured.Offset.Value)
}

// TestBuildAcquireParams covers the shared name/tags/timeout forwarding used by
// both `browser-pools acquire` and the `browsers create --pool-id` lease path.
func TestBuildAcquireParams(t *testing.T) {
	p := buildAcquireParams("lease", map[string]string{"env": "prod"}, 30)
	assert.True(t, p.Name.Valid())
	assert.Equal(t, "lease", p.Name.Value)
	assert.Equal(t, "prod", p.Tags["env"])
	assert.True(t, p.AcquireTimeoutSeconds.Valid())
	assert.Equal(t, int64(30), p.AcquireTimeoutSeconds.Value)

	// Unset inputs produce an empty params struct (nothing forwarded).
	empty := buildAcquireParams("", nil, 0)
	assert.False(t, empty.Name.Valid())
	assert.Len(t, empty.Tags, 0)
	assert.False(t, empty.AcquireTimeoutSeconds.Valid())
}

func TestBrowserPoolsCreate_WithRefreshOnProfileUpdate(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-ropu"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{
		Size:                   1,
		RefreshOnProfileUpdate: BoolFlag{Set: true, Value: true},
	})
	assert.NoError(t, err)
	assert.True(t, captured.RefreshOnProfileUpdate.Valid())
	assert.True(t, captured.RefreshOnProfileUpdate.Value)
}

func TestBrowserPoolsCreate_DefaultFillRate(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-default-fill-rate"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, FillRate: 25})
	require.NoError(t, err)
	assert.True(t, captured.FillRatePerMinute.Valid())
	assert.Equal(t, int64(25), captured.FillRatePerMinute.Value)
}

func TestBrowserPoolsUpdate_WithRefreshOnProfileUpdate(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolUpdateParams
	fake := &FakeBrowserPoolsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: id}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Update(context.Background(), BrowserPoolsUpdateInput{
		IDOrName:               "pool-1",
		RefreshOnProfileUpdate: BoolFlag{Set: true, Value: false},
	})
	assert.NoError(t, err)
	assert.True(t, captured.RefreshOnProfileUpdate.Valid())
	assert.False(t, captured.RefreshOnProfileUpdate.Value)
}

func TestBrowserPoolsCreate_WithChromePolicy(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-cp"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{
		Size:         1,
		ChromePolicy: `{"BookmarkBarEnabled": false}`,
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"BookmarkBarEnabled": false}, captured.ChromePolicy)
}

func TestBrowserPoolsCreate_ChromePolicyEmptyObjectOmitted(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-cp"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, ChromePolicy: "{}"})
	assert.NoError(t, err)
	assert.Nil(t, captured.ChromePolicy)
}

func TestBrowserPoolsUpdate_WithChromePolicy(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolUpdateParams
	fake := &FakeBrowserPoolsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: id}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Update(context.Background(), BrowserPoolsUpdateInput{
		IDOrName:     "pool-1",
		ChromePolicy: `{"BookmarkBarEnabled": false}`,
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"BookmarkBarEnabled": false}, captured.ChromePolicy)
}

func TestBrowserPoolsUpdate_EmptyChromePolicyWarnsAndDoesNotClear(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolUpdateParams
	fake := &FakeBrowserPoolsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: id}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Update(context.Background(), BrowserPoolsUpdateInput{
		IDOrName:     "pool-1",
		ChromePolicy: "{}",
	})
	assert.NoError(t, err)
	assert.Nil(t, captured.ChromePolicy)
	assert.Contains(t, outBuf.String(), "does not clear")
}

func TestBrowserPoolsUpdate_EmptyChromePolicyQuietInJSONMode(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowserPoolsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			return &kernel.BrowserPool{ID: id}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Update(context.Background(), BrowserPoolsUpdateInput{
		IDOrName:     "pool-1",
		ChromePolicy: "{}",
		Output:       "json",
	})
	assert.NoError(t, err)
	// The warning must not leak onto stdout in json mode, where it would corrupt the payload.
	assert.NotContains(t, outBuf.String(), "does not clear")
}
