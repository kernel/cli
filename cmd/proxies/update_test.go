package proxies

import (
	"context"
	"errors"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
)

func TestProxyUpdate_RenamesProxy(t *testing.T) {
	buf := captureOutput(t)

	var capturedID string
	var captured kernel.ProxyUpdateParams
	fake := &FakeProxyService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProxyUpdateParams, opts ...option.RequestOption) (*kernel.ProxyUpdateResponse, error) {
			capturedID = id
			captured = body
			return &kernel.ProxyUpdateResponse{
				ID:     id,
				Name:   body.Name.Value,
				Type:   kernel.ProxyUpdateResponseTypeIsp,
				Status: kernel.ProxyUpdateResponseStatusAvailable,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Update(context.Background(), ProxyUpdateInput{ID: "proxy-1", Name: "New Name"})

	assert.NoError(t, err)
	assert.Equal(t, "proxy-1", capturedID)
	assert.True(t, captured.Name.Valid())
	assert.Equal(t, "New Name", captured.Name.Value)

	out := buf.String()
	assert.Contains(t, out, "Updated proxy proxy-1")
	assert.Contains(t, out, "isp")
}

func TestProxyUpdate_MovesISPToReservedIP(t *testing.T) {
	var captured kernel.ProxyUpdateParams
	fake := &FakeProxyService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProxyUpdateParams, opts ...option.RequestOption) (*kernel.ProxyUpdateResponse, error) {
			captured = body
			return &kernel.ProxyUpdateResponse{
				ID:        id,
				Type:      kernel.ProxyUpdateResponseTypeIsp,
				IPAddress: body.Config.IPAddress,
			}, nil
		},
	}

	err := ProxyCmd{proxies: fake}.Update(context.Background(), ProxyUpdateInput{ID: "proxy-1", IPAddress: "203.0.113.10"})

	assert.NoError(t, err)
	assert.False(t, captured.Name.Valid())
	assert.Equal(t, "203.0.113.10", captured.Config.IPAddress)
}

func TestProxyUpdate_RequiresNameOrIPAddress(t *testing.T) {
	p := ProxyCmd{proxies: &FakeProxyService{}}
	err := p.Update(context.Background(), ProxyUpdateInput{ID: "proxy-1"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of --name or --ip-address is required")
}

func TestProxyUpdate_SurfacesAPIError(t *testing.T) {
	fake := &FakeProxyService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProxyUpdateParams, opts ...option.RequestOption) (*kernel.ProxyUpdateResponse, error) {
			return nil, errors.New("proxy not found")
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Update(context.Background(), ProxyUpdateInput{ID: "missing", Name: "New Name"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "proxy not found")
}

func TestProxyUpdate_InvalidOutput(t *testing.T) {
	p := ProxyCmd{proxies: &FakeProxyService{}}
	err := p.Update(context.Background(), ProxyUpdateInput{ID: "proxy-1", Name: "New Name", Output: "yaml"})

	assert.Error(t, err)
}
