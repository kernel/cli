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
				Name:   body.Name,
				Type:   kernel.ProxyUpdateResponseTypeDatacenter,
				Status: kernel.ProxyUpdateResponseStatusAvailable,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Update(context.Background(), ProxyUpdateInput{ID: "proxy-1", Name: "New Name"})

	assert.NoError(t, err)
	assert.Equal(t, "proxy-1", capturedID)
	assert.Equal(t, "New Name", captured.Name)

	out := buf.String()
	assert.Contains(t, out, "Renamed proxy proxy-1 to New Name")
	assert.Contains(t, out, "datacenter")
}

func TestProxyUpdate_RequiresName(t *testing.T) {
	p := ProxyCmd{proxies: &FakeProxyService{}}
	err := p.Update(context.Background(), ProxyUpdateInput{ID: "proxy-1"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--name is required")
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
