package proxies

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
)

// captureOutput sets pterm writers for tests
func captureOutput(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer
	pterm.SetDefaultOutput(&buf)
	pterm.Info.Writer = &buf
	pterm.Error.Writer = &buf
	pterm.Success.Writer = &buf
	pterm.Warning.Writer = &buf
	pterm.Debug.Writer = &buf
	pterm.Fatal.Writer = &buf
	pterm.DefaultTable = *pterm.DefaultTable.WithWriter(&buf)
	t.Cleanup(func() {
		pterm.SetDefaultOutput(os.Stdout)
		pterm.Info.Writer = os.Stdout
		pterm.Error.Writer = os.Stdout
		pterm.Success.Writer = os.Stdout
		pterm.Warning.Writer = os.Stdout
		pterm.Debug.Writer = os.Stdout
		pterm.Fatal.Writer = os.Stdout
		pterm.DefaultTable = *pterm.DefaultTable.WithWriter(os.Stdout)
	})
	return &buf
}

// FakeProxyService implements ProxyService for testing
type FakeProxyService struct {
	ListFunc   func(ctx context.Context, opts ...option.RequestOption) (*[]kernel.ProxyListResponse, error)
	GetFunc    func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error)
	NewFunc    func(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error)
	DeleteFunc func(ctx context.Context, id string, opts ...option.RequestOption) error
}

func (f *FakeProxyService) List(ctx context.Context, opts ...option.RequestOption) (*[]kernel.ProxyListResponse, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, opts...)
	}
	empty := []kernel.ProxyListResponse{}
	return &empty, nil
}

func (f *FakeProxyService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProxyGetResponse, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return &kernel.ProxyGetResponse{ID: id, Type: kernel.ProxyGetResponseTypeDatacenter}, nil
}

func (f *FakeProxyService) New(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (*kernel.ProxyNewResponse, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.ProxyNewResponse{ID: "new-proxy", Type: kernel.ProxyNewResponseTypeDatacenter}, nil
}

func (f *FakeProxyService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, id, opts...)
	}
	return nil
}

// Helper function to create test proxy responses
func createDatacenterProxy(id, name, country string) kernel.ProxyListResponse {
	return kernel.ProxyListResponse{
		ID:   id,
		Name: name,
		Type: kernel.ProxyListResponseTypeDatacenter,
		Config: kernel.ProxyListResponseConfigUnion{
			Country: country,
		},
	}
}

func createResidentialProxy(id, name, country, city, state string) kernel.ProxyListResponse {
	return kernel.ProxyListResponse{
		ID:   id,
		Name: name,
		Type: kernel.ProxyListResponseTypeResidential,
		Config: kernel.ProxyListResponseConfigUnion{
			Country: country,
			City:    city,
			State:   state,
		},
	}
}

func createCustomProxy(id, name, host string, port int64) kernel.ProxyListResponse {
	return kernel.ProxyListResponse{
		ID:   id,
		Name: name,
		Type: kernel.ProxyListResponseTypeCustom,
		Config: kernel.ProxyListResponseConfigUnion{
			Host: host,
			Port: port,
		},
	}
}
