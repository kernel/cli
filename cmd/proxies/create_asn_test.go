package proxies

import (
	"context"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyCreate_ISP_SendsASN(t *testing.T) {
	captureOutput(t)

	called := false
	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			called = true
			ispConfig := body.Config.OfIsp
			require.NotNil(t, ispConfig)
			extras := ispConfig.ExtraFields()
			assert.Contains(t, extras, "asn")
			assert.Equal(t, "AS6079", extras["asn"])

			return &kernel.ProxyNewResponse{ID: "isp-new", Name: "RCN ISP", Type: kernel.ProxyNewResponseTypeIsp}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name: "RCN ISP",
		Type: "isp",
		ASN:  "AS6079",
	})

	require.NoError(t, err)
	assert.True(t, called, "expected the create request to be sent")
}

// Omitting --asn must not start sending an empty ASN, which the API would reject as
// malformed rather than treating as "no preference".
func TestProxyCreate_ISP_OmitsEmptyASN(t *testing.T) {
	captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			ispConfig := body.Config.OfIsp
			require.NotNil(t, ispConfig)
			assert.NotContains(t, ispConfig.ExtraFields(), "asn", "asn should be absent when not requested")

			return &kernel.ProxyNewResponse{ID: "isp-new", Name: "Any ISP", Type: kernel.ProxyNewResponseTypeIsp}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{Name: "Any ISP", Type: "isp"})
	require.NoError(t, err)
}

// The flag is registered on every type, so the types that cannot honour it have to say so
// rather than drop it and hand back a proxy on an arbitrary network.
func TestProxyCreate_ASNRejectedForUnsupportedTypes(t *testing.T) {
	tests := []struct {
		proxyType string
		input     ProxyCreateInput
	}{
		{"datacenter", ProxyCreateInput{Name: "dc", Type: "datacenter", ASN: "AS6079"}},
		{"mobile", ProxyCreateInput{Name: "mob", Type: "mobile", Country: "US", ASN: "AS6079"}},
		{"custom", ProxyCreateInput{Name: "cus", Type: "custom", Host: "proxy.example.com", Port: 8080, ASN: "AS6079"}},
	}

	for _, tt := range tests {
		t.Run(tt.proxyType, func(t *testing.T) {
			captureOutput(t)

			fake := &FakeProxyService{
				NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
					require.Failf(t, "unexpected request", "create should not be attempted for %s with --asn", tt.proxyType)
					return nil, nil
				},
			}

			p := ProxyCmd{proxies: fake}
			err := p.Create(context.Background(), tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--asn is not supported")
		})
	}
}

func TestProxyCreate_ZipRejectedForMobile(t *testing.T) {
	captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			require.Fail(t, "unexpected request", "create should not be attempted for mobile with --zip")
			return nil, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{Name: "mob", Type: "mobile", Country: "US", Zip: "94107"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--zip is not supported")
}
