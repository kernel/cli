package proxies

import (
	"context"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProxyIPService struct {
	listFunc func(context.Context, kernel.ProxyIPListParams, ...option.RequestOption) (*pagination.OffsetPagination[kernel.ProxyIP], error)
	getFunc  func(context.Context, string, ...option.RequestOption) (*kernel.ProxyIPDetail, error)
}

func (f *fakeProxyIPService) List(ctx context.Context, params kernel.ProxyIPListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ProxyIP], error) {
	if f.listFunc != nil {
		return f.listFunc(ctx, params, opts...)
	}
	return &pagination.OffsetPagination[kernel.ProxyIP]{}, nil
}

func (f *fakeProxyIPService) Get(ctx context.Context, ipAddress string, opts ...option.RequestOption) (*kernel.ProxyIPDetail, error) {
	if f.getFunc != nil {
		return f.getFunc(ctx, ipAddress, opts...)
	}
	return &kernel.ProxyIPDetail{IPAddress: ipAddress}, nil
}

func TestProxyIPListUsesPagePaginationAndPreservesFilters(t *testing.T) {
	buf := captureOutput(t)
	var captured kernel.ProxyIPListParams
	fake := &fakeProxyIPService{
		listFunc: func(_ context.Context, params kernel.ProxyIPListParams, _ ...option.RequestOption) (*pagination.OffsetPagination[kernel.ProxyIP], error) {
			captured = params
			return &pagination.OffsetPagination[kernel.ProxyIP]{Items: []kernel.ProxyIP{
				{IPAddress: "203.0.113.10", Country: "US", Status: kernel.ProxyIPStatusActive},
				{IPAddress: "203.0.113.11", Country: "US", Status: kernel.ProxyIPStatusExcluded},
				{IPAddress: "203.0.113.12", Country: "US", Status: kernel.ProxyIPStatusActive},
			}}, nil
		},
	}

	err := ProxyIPCmd{proxyIPs: fake}.List(context.Background(), ProxyIPListInput{
		Country: "US",
		Status:  "active",
		Page:    2,
		PerPage: 2,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(3), captured.Limit.Value)
	assert.Equal(t, int64(2), captured.Offset.Value)
	assert.Equal(t, "US", captured.Country.Value)
	assert.Equal(t, kernel.ProxyIPListParamsStatusActive, captured.Status)

	out := buf.String()
	assert.Contains(t, out, "203.0.113.10")
	assert.Contains(t, out, "203.0.113.11")
	assert.NotContains(t, out, "203.0.113.12")
	assert.Contains(t, out, "Page: 2")
	assert.Contains(t, out, "Per-page: 2")
	assert.Contains(t, out, "Has more: yes")
	assert.Contains(t, out, `Next: kernel proxy-ips list --page 3 --per-page 2 --country "US" --status "active"`)
}

func TestProxyIPListPrintsFooterWhenEmpty(t *testing.T) {
	buf := captureOutput(t)

	err := ProxyIPCmd{proxyIPs: &fakeProxyIPService{}}.List(context.Background(), ProxyIPListInput{})

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No reserved ISP IPs found")
	assert.Contains(t, buf.String(), "Page: 1")
	assert.Contains(t, buf.String(), "Has more: no")
}

func TestProxyIPListRejectsInvalidStatus(t *testing.T) {
	called := false
	fake := &fakeProxyIPService{
		listFunc: func(_ context.Context, _ kernel.ProxyIPListParams, _ ...option.RequestOption) (*pagination.OffsetPagination[kernel.ProxyIP], error) {
			called = true
			return nil, nil
		},
	}

	err := ProxyIPCmd{proxyIPs: fake}.List(context.Background(), ProxyIPListInput{Status: "unavailable"})

	assert.EqualError(t, err, `invalid --status "unavailable": must be active or excluded`)
	assert.False(t, called)
}

func TestProxyIPGetShowsOrganizationAndProjectUsage(t *testing.T) {
	buf := captureOutput(t)
	fake := &fakeProxyIPService{
		getFunc: func(_ context.Context, ipAddress string, _ ...option.RequestOption) (*kernel.ProxyIPDetail, error) {
			assert.Equal(t, "203.0.113.10", ipAddress)
			return &kernel.ProxyIPDetail{
				IPAddress:  ipAddress,
				Country:    "US",
				Status:     kernel.ProxyIPDetailStatusActive,
				ProxyCount: 2,
				Proxies: []kernel.ProxyIPDetailProxy{
					{ID: "proxy-1", Name: "Primary"},
					{ID: "proxy-2"},
				},
			}, nil
		},
	}

	err := ProxyIPCmd{proxyIPs: fake}.Get(context.Background(), ProxyIPGetInput{IPAddress: "203.0.113.10"})

	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "203.0.113.10")
	assert.Contains(t, out, "Organization Proxy Count")
	assert.Contains(t, out, "2")
	assert.Contains(t, out, "Primary (proxy-1)")
	assert.Contains(t, out, "proxy-2")
}

func TestProxyIPCommandsExposePageBasedFlags(t *testing.T) {
	assert.NotNil(t, ProxyIPsCmd.Commands())
	assert.NotNil(t, proxyIPsListCmd.Flags().Lookup("page"))
	assert.NotNil(t, proxyIPsListCmd.Flags().Lookup("per-page"))
	assert.Nil(t, proxyIPsListCmd.Flags().Lookup("limit"))
	assert.Nil(t, proxyIPsListCmd.Flags().Lookup("offset"))
	assert.NotNil(t, proxyIPsListCmd.Flags().Lookup("country"))
	assert.NotNil(t, proxyIPsListCmd.Flags().Lookup("status"))
}
