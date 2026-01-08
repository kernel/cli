package proxies

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
)

func TestProxyDelete_SkipConfirm_Success(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			assert.Equal(t, "proxy-1", id)
			return nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Delete(context.Background(), ProxyDeleteInput{
		ID:          "proxy-1",
		SkipConfirm: true,
	})

	assert.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "Deleting proxy: proxy-1")
	assert.Contains(t, output, "Successfully deleted proxy: proxy-1")
}

func TestProxyDelete_SkipConfirm_NotFound(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			return &kernel.Error{StatusCode: http.StatusNotFound}
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Delete(context.Background(), ProxyDeleteInput{
		ID:          "not-found",
		SkipConfirm: true,
	})

	assert.NoError(t, err) // Not found returns nil
	output := buf.String()

	assert.Contains(t, output, "Proxy 'not-found' not found")
}

func TestProxyDelete_SkipConfirm_APIError(t *testing.T) {
	_ = captureOutput(t)

	fake := &FakeProxyService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			return errors.New("API error")
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Delete(context.Background(), ProxyDeleteInput{
		ID:          "proxy-1",
		SkipConfirm: true,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}
