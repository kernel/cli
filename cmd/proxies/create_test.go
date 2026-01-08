package proxies

import (
	"context"
	"errors"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
)

func TestProxyCreate_Datacenter_Success(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			// Verify the request
			assert.Equal(t, kernel.ProxyNewParamsTypeDatacenter, body.Type)
			assert.Equal(t, "My DC Proxy", body.Name.Value)

			// Check config
			dcConfig := body.Config.OfProxyNewsConfigDatacenterProxyConfig
			assert.NotNil(t, dcConfig)
			assert.Equal(t, "US", dcConfig.Country.Value)

			return &kernel.ProxyNewResponse{
				ID:   "dc-new",
				Name: "My DC Proxy",
				Type: kernel.ProxyNewResponseTypeDatacenter,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name:    "My DC Proxy",
		Type:    "datacenter",
		Country: "US",
	})

	assert.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "Creating datacenter proxy")
	assert.Contains(t, output, "Successfully created proxy")
	assert.Contains(t, output, "dc-new")
	assert.Contains(t, output, "My DC Proxy")
}

func TestProxyCreate_Datacenter_WithoutCountry(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			// Verify the request
			assert.Equal(t, kernel.ProxyNewParamsTypeDatacenter, body.Type)
			assert.Equal(t, "My DC Proxy", body.Name.Value)

			// Check config - country should not be set (it should be zero/nil)
			dcConfig := body.Config.OfProxyNewsConfigDatacenterProxyConfig
			assert.NotNil(t, dcConfig)

			return &kernel.ProxyNewResponse{
				ID:   "dc-new",
				Name: "My DC Proxy",
				Type: kernel.ProxyNewResponseTypeDatacenter,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name: "My DC Proxy",
		Type: "datacenter",
		// Country is now optional
	})

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Creating datacenter proxy")
	assert.Contains(t, output, "Successfully created proxy")
}

func TestProxyCreate_Residential_Success(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			// Verify residential config
			resConfig := body.Config.OfProxyNewsConfigResidentialProxyConfig
			assert.NotNil(t, resConfig)
			assert.Equal(t, "US", resConfig.Country.Value)
			assert.Equal(t, "sanfrancisco", resConfig.City.Value)
			assert.Equal(t, "CA", resConfig.State.Value)
			assert.Equal(t, "94107", resConfig.Zip.Value)
			assert.Equal(t, "AS15169", resConfig.Asn.Value)
			assert.Equal(t, "windows", resConfig.Os)

			return &kernel.ProxyNewResponse{
				ID:   "res-new",
				Name: "SF Residential",
				Type: kernel.ProxyNewResponseTypeResidential,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name:    "SF Residential",
		Type:    "residential",
		Country: "US",
		City:    "sanfrancisco",
		State:   "CA",
		Zip:     "94107",
		ASN:     "AS15169",
		OS:      "windows",
	})

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Successfully created proxy")
}

func TestProxyCreate_Residential_CityWithoutCountry(t *testing.T) {
	fake := &FakeProxyService{}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Type: "residential",
		City: "sanfrancisco",
		// Missing country
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--country is required when --city is specified")
}

func TestProxyCreate_Residential_InvalidOS(t *testing.T) {
	fake := &FakeProxyService{}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Type: "residential",
		OS:   "linux", // Invalid OS
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid OS value: linux (must be windows, macos, or android)")
}

func TestProxyCreate_Mobile_Success(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			// Verify mobile config
			mobConfig := body.Config.OfProxyNewsConfigMobileProxyConfig
			assert.NotNil(t, mobConfig)
			assert.Equal(t, "US", mobConfig.Country.Value)
			assert.Equal(t, "verizon", mobConfig.Carrier)

			return &kernel.ProxyNewResponse{
				ID:   "mobile-new",
				Name: "Mobile Proxy",
				Type: kernel.ProxyNewResponseTypeMobile,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name:    "Mobile Proxy",
		Type:    "mobile",
		Country: "US",
		Carrier: "verizon",
	})

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Creating mobile proxy")
	assert.Contains(t, output, "Successfully created proxy")
}

func TestProxyCreate_Custom_Success(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			// Verify custom config
			customConfig := body.Config.OfProxyNewsConfigCreateCustomProxyConfig
			assert.NotNil(t, customConfig)
			assert.Equal(t, "proxy.example.com", customConfig.Host)
			assert.Equal(t, int64(8080), customConfig.Port)
			assert.Equal(t, "user123", customConfig.Username.Value)
			assert.Equal(t, "secret", customConfig.Password.Value)

			return &kernel.ProxyNewResponse{
				ID:   "custom-new",
				Name: "My Custom Proxy",
				Type: kernel.ProxyNewResponseTypeCustom,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name:     "My Custom Proxy",
		Type:     "custom",
		Host:     "proxy.example.com",
		Port:     8080,
		Username: "user123",
		Password: "secret",
	})

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Creating custom proxy")
	assert.Contains(t, output, "Successfully created proxy")
}

func TestProxyCreate_Custom_MissingHost(t *testing.T) {
	fake := &FakeProxyService{}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Type: "custom",
		Port: 8080,
		// Missing required host
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--host is required for custom proxy type")
}

func TestProxyCreate_Custom_MissingPort(t *testing.T) {
	fake := &FakeProxyService{}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Type: "custom",
		Host: "proxy.example.com",
		// Missing required port (will be 0)
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--port is required for custom proxy type")
}

func TestProxyCreate_InvalidType(t *testing.T) {
	fake := &FakeProxyService{}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Type: "invalid",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid proxy type: invalid")
}

func TestProxyCreate_Protocol_Valid(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
	}{
		{"http protocol", "http"},
		{"https protocol", "https"},
		{"empty protocol", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &FakeProxyService{
				NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
					return &kernel.ProxyNewResponse{
						ID:   "test-proxy",
						Name: "Test Proxy",
						Type: kernel.ProxyNewResponseTypeDatacenter,
					}, nil
				},
			}

			p := ProxyCmd{proxies: fake}
			err := p.Create(context.Background(), ProxyCreateInput{
				Type:     "datacenter",
				Country:  "US",
				Protocol: tt.protocol,
			})

			assert.NoError(t, err)
		})
	}
}

func TestProxyCreate_Protocol_Invalid(t *testing.T) {
	fake := &FakeProxyService{}
	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Type:     "datacenter",
		Country:  "US",
		Protocol: "ftp",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid protocol: ftp")
}

func TestProxyCreate_APIError(t *testing.T) {
	_ = captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			return nil, errors.New("API error")
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name:    "Test",
		Type:    "datacenter",
		Country: "US",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestProxyCreate_ISP_Success(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			// Verify ISP config
			ispConfig := body.Config.OfProxyNewsConfigIspProxyConfig
			assert.NotNil(t, ispConfig)
			assert.Equal(t, "EU", ispConfig.Country.Value)

			return &kernel.ProxyNewResponse{
				ID:   "isp-new",
				Name: "EU ISP",
				Type: kernel.ProxyNewResponseTypeIsp,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name:    "EU ISP",
		Type:    "isp",
		Country: "EU",
	})

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Creating isp proxy")
	assert.Contains(t, output, "Successfully created proxy")
}

func TestProxyCreate_ISP_WithoutCountry(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		NewFunc: func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
			// Verify ISP config
			ispConfig := body.Config.OfProxyNewsConfigIspProxyConfig
			assert.NotNil(t, ispConfig)

			return &kernel.ProxyNewResponse{
				ID:   "isp-new",
				Name: "ISP Proxy",
				Type: kernel.ProxyNewResponseTypeIsp,
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Create(context.Background(), ProxyCreateInput{
		Name: "ISP Proxy",
		Type: "isp",
		// Country is now optional
	})

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Creating isp proxy")
	assert.Contains(t, output, "Successfully created proxy")
}
