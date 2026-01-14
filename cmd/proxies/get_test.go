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

func TestProxyGet_Datacenter(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
			return &kernel.ProxyGetResponse{
				ID:   "dc-1",
				Name: "US Datacenter",
				Type: kernel.ProxyGetResponseTypeDatacenter,
				Config: kernel.ProxyGetResponseConfigUnion{
					Country: "US",
				},
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Get(context.Background(), ProxyGetInput{ID: "dc-1"})

	assert.NoError(t, err)
	output := buf.String()

	// Check all fields are displayed
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "dc-1")
	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "US Datacenter")
	assert.Contains(t, output, "Type")
	assert.Contains(t, output, "datacenter")
	assert.Contains(t, output, "Country")
	assert.Contains(t, output, "US")
}

func TestProxyGet_Residential(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
			return &kernel.ProxyGetResponse{
				ID:   "res-1",
				Name: "SF Residential",
				Type: kernel.ProxyGetResponseTypeResidential,
				Config: kernel.ProxyGetResponseConfigUnion{
					Country: "US",
					City:    "sanfrancisco",
					State:   "CA",
					Zip:     "94107",
					Asn:     "AS15169",
					Os:      "windows",
				},
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Get(context.Background(), ProxyGetInput{ID: "res-1"})

	assert.NoError(t, err)
	output := buf.String()

	// Check all residential-specific fields
	assert.Contains(t, output, "Country")
	assert.Contains(t, output, "US")
	assert.Contains(t, output, "City")
	assert.Contains(t, output, "sanfrancisco")
	assert.Contains(t, output, "State")
	assert.Contains(t, output, "CA")
	assert.Contains(t, output, "ZIP")
	assert.Contains(t, output, "94107")
	assert.Contains(t, output, "ASN")
	assert.Contains(t, output, "AS15169")
	assert.Contains(t, output, "OS")
	assert.Contains(t, output, "windows")
}

func TestProxyGet_Mobile(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
			return &kernel.ProxyGetResponse{
				ID:   "mobile-1",
				Name: "Mobile Proxy",
				Type: kernel.ProxyGetResponseTypeMobile,
				Config: kernel.ProxyGetResponseConfigUnion{
					Country: "US",
					Carrier: "verizon",
				},
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Get(context.Background(), ProxyGetInput{ID: "mobile-1"})

	assert.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "Carrier")
	assert.Contains(t, output, "verizon")
}

func TestProxyGet_Custom(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
			return &kernel.ProxyGetResponse{
				ID:   "custom-1",
				Name: "My Proxy",
				Type: kernel.ProxyGetResponseTypeCustom,
				Config: kernel.ProxyGetResponseConfigUnion{
					Host:        "proxy.example.com",
					Port:        8080,
					Username:    "user123",
					HasPassword: true,
				},
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Get(context.Background(), ProxyGetInput{ID: "custom-1"})

	assert.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "Host")
	assert.Contains(t, output, "proxy.example.com")
	assert.Contains(t, output, "Port")
	assert.Contains(t, output, "8080")
	assert.Contains(t, output, "Username")
	assert.Contains(t, output, "user123")
	assert.Contains(t, output, "Has Password")
	assert.Contains(t, output, "Yes")
}

func TestProxyGet_EmptyName(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
			return &kernel.ProxyGetResponse{
				ID:   "proxy-1",
				Name: "", // Empty name
				Type: kernel.ProxyGetResponseTypeIsp,
				Config: kernel.ProxyGetResponseConfigUnion{
					Country: "US",
				},
			}, nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Get(context.Background(), ProxyGetInput{ID: "proxy-1"})

	assert.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "-") // Empty name shows as "-"
}

func TestProxyGet_NotFound(t *testing.T) {
	_ = captureOutput(t)

	fake := &FakeProxyService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
			return nil, &kernel.Error{StatusCode: http.StatusNotFound}
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Get(context.Background(), ProxyGetInput{ID: "not-found"})

	assert.Error(t, err)
}

func TestProxyGet_Error(t *testing.T) {
	_ = captureOutput(t)

	fake := &FakeProxyService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
			return nil, errors.New("API error")
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Get(context.Background(), ProxyGetInput{ID: "proxy-1"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}
