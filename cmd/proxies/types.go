package proxies

import (
	"context"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
)

// ProxyService defines the subset of the Kernel SDK proxy client that we use.
type ProxyService interface {
	List(ctx context.Context, opts ...option.RequestOption) (res *[]kernel.ProxyListResponse, err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.ProxyGetResponse, err error)
	New(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (res *kernel.ProxyNewResponse, err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
}

// ProxyCmd handles proxy operations independent of cobra.
type ProxyCmd struct {
	proxies ProxyService
}

// Input types for proxy operations
type ProxyListInput struct {
	Output string
}

type ProxyGetInput struct {
	ID     string
	Output string
}

type ProxyCreateInput struct {
	Name     string
	Type     string
	Protocol string
	// Datacenter/ISP config
	Country string
	// Residential/Mobile config
	City  string
	State string
	Zip   string
	ASN   string
	OS    string
	// Mobile specific
	Carrier string
	// Custom proxy config
	Host     string
	Port     int
	Username string
	Password string
	Output   string
}

type ProxyDeleteInput struct {
	ID          string
	SkipConfirm bool
}
