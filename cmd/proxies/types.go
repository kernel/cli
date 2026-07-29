package proxies

import (
	"context"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
)

// ProxyService defines the subset of the Kernel SDK proxy client that we use.
type ProxyService interface {
	List(ctx context.Context, query kernel.ProxyListParams, opts ...option.RequestOption) (res *pagination.OffsetPagination[kernel.ProxyListResponse], err error)
	Get(ctx context.Context, id string, opts ...option.RequestOption) (res *kernel.ProxyGetResponse, err error)
	New(ctx context.Context, body kernel.ProxyNewParams, opts ...option.RequestOption) (res *kernel.ProxyNewResponse, err error)
	Update(ctx context.Context, id string, body kernel.ProxyUpdateParams, opts ...option.RequestOption) (res *kernel.ProxyUpdateResponse, err error)
	Delete(ctx context.Context, id string, opts ...option.RequestOption) (err error)
	Check(ctx context.Context, id string, body kernel.ProxyCheckParams, opts ...option.RequestOption) (res *kernel.ProxyCheckResponse, err error)
}

// ProxyCmd handles proxy operations independent of cobra.
type ProxyCmd struct {
	proxies  ProxyService
	prompter interactive.Prompter
}

// Input types for proxy operations
type ProxyListInput struct {
	Limit  int
	Offset int
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
	// Hostnames that should bypass the parent proxy and connect directly.
	BypassHosts []string
	// Datacenter/ISP config
	Country string
	// Residential/Mobile config
	City  string
	State string
	Zip   string
	ASN   string
	OS    string
	// Custom proxy config
	Host     string
	Port     int
	Username string
	Password string
	// PEM-encoded CA bundle for MITM proxies, provided inline or read from a file.
	CaBundle     string
	CaBundleFile string
	Output       string
}

type ProxyUpdateInput struct {
	ID     string
	Name   string
	Output string
}

type ProxyDeleteInput struct {
	ID          string
	SkipConfirm bool
}

type ProxyCheckInput struct {
	ID     string
	URL    string
	Output string
}
