package proxies

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ASN currently travels as an extra field rather than a generated one, so assert on
// the serialized request: that is what the API sees, and it stays valid once the field
// becomes typed.
func ispConfigJSON(t *testing.T, body kernel.ProxyNewParams) map[string]any {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	var decoded struct {
		Config map[string]any `json:"config"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded.Config
}

func TestProxyCreate_ISP_SendsASN(t *testing.T) {
	captureOutput(t)

	called := false
	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			called = true
			assert.Equal(t, "AS6079", ispConfigJSON(t, body)["asn"])

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

func TestProxyCreate_ISP_SendsASNAlongsideCountry(t *testing.T) {
	captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			config := ispConfigJSON(t, body)
			assert.Equal(t, "AS6079", config["asn"])
			assert.Equal(t, "US", config["country"], "the extra field must not displace country")

			return &kernel.ProxyNewResponse{ID: "isp-new", Name: "RCN ISP", Type: kernel.ProxyNewResponseTypeIsp}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name:    "RCN ISP",
		Type:    "isp",
		Country: "US",
		ASN:     "AS6079",
	})
	require.NoError(t, err)
}

// Omitting --asn must not start sending an empty ASN, which the API would reject as
// malformed rather than treating as "no preference".
func TestProxyCreate_ISP_OmitsEmptyASN(t *testing.T) {
	captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			_, present := ispConfigJSON(t, body)["asn"]
			assert.False(t, present, "asn should be absent when not requested")

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
