package cmd

import (
	"context"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
)

type fakeBrowserPoolsService struct {
	newFunc func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error)
}

func (f *fakeBrowserPoolsService) List(ctx context.Context, opts ...option.RequestOption) (*[]kernel.BrowserPool, error) {
	return &[]kernel.BrowserPool{}, nil
}

func (f *fakeBrowserPoolsService) New(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	if f.newFunc != nil {
		return f.newFunc(ctx, body, opts...)
	}
	return &kernel.BrowserPool{}, nil
}

func (f *fakeBrowserPoolsService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	return &kernel.BrowserPool{}, nil
}

func (f *fakeBrowserPoolsService) Update(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	return &kernel.BrowserPool{}, nil
}

func (f *fakeBrowserPoolsService) Delete(ctx context.Context, id string, body kernel.BrowserPoolDeleteParams, opts ...option.RequestOption) error {
	return nil
}

func (f *fakeBrowserPoolsService) Acquire(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error) {
	return &kernel.BrowserPoolAcquireResponse{}, nil
}

func (f *fakeBrowserPoolsService) Release(ctx context.Context, id string, body kernel.BrowserPoolReleaseParams, opts ...option.RequestOption) error {
	return nil
}

func (f *fakeBrowserPoolsService) Flush(ctx context.Context, id string, opts ...option.RequestOption) error {
	return nil
}

func TestBrowserPoolsCreate_MapsChromePolicy(t *testing.T) {
	setupStdoutCapture(t)

	fake := &fakeBrowserPoolsService{
		newFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			assert.Equal(t, map[string]any{
				"HomepageLocation": "https://example.com",
				"ShowHomeButton":   true,
			}, body.ChromePolicy)
			return &kernel.BrowserPool{ID: "pool_123", Name: "test-pool"}, nil
		},
	}

	cmd := BrowserPoolsCmd{client: fake}
	err := cmd.Create(context.Background(), BrowserPoolsCreateInput{
		Name:         "test-pool",
		Size:         1,
		ChromePolicy: `{"HomepageLocation":"https://example.com","ShowHomeButton":true}`,
	})

	assert.NoError(t, err)
}
