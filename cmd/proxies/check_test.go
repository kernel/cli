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
		CheckFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyCheckResponse, error) {
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
