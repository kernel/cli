package proxies

import (
	"context"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
)

func TestProxyCheck_ShowsBypassHosts(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		CheckFunc: func(ctx context.Context, id string, body kernel.ProxyCheckParams, opts ...option.RequestOption) (*kernel.ProxyCheckResponse, error) {
			return &kernel.ProxyCheckResponse{
				ID:          id,
				Name:        "Proxy 1",
				Type:        kernel.ProxyCheckResponseTypeDatacenter,
				BypassHosts: []string{"localhost", "internal.service.local"},
				Status:      kernel.ProxyCheckResponseStatusAvailable,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Check(context.Background(), ProxyCheckInput{ID: "proxy-1"})

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Bypass Hosts")
	assert.Contains(t, output, "localhost")
	assert.Contains(t, output, "internal.service.local")
	assert.Contains(t, output, "Proxy health check passed")
}

func TestProxyCheck_PassesURL(t *testing.T) {
	buf := captureOutput(t)
	var captured kernel.ProxyCheckParams

	fake := &FakeProxyService{
		CheckFunc: func(ctx context.Context, id string, body kernel.ProxyCheckParams, opts ...option.RequestOption) (*kernel.ProxyCheckResponse, error) {
			captured = body
			return &kernel.ProxyCheckResponse{
				ID:     id,
				Name:   "Proxy 1",
				Type:   kernel.ProxyCheckResponseTypeDatacenter,
				Status: kernel.ProxyCheckResponseStatusAvailable,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Check(context.Background(), ProxyCheckInput{ID: "proxy-1", URL: "https://example.com"})

	assert.NoError(t, err)
	assert.True(t, captured.URL.Valid())
	assert.Equal(t, "https://example.com", captured.URL.Value)
	assert.Contains(t, buf.String(), "Proxy health check passed")
}
