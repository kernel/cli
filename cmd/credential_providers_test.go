package cmd

import (
	"context"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
)

// FakeCredentialProvidersService is a configurable fake implementing CredentialProvidersService.
type FakeCredentialProvidersService struct {
	ListFunc func(ctx context.Context, query kernel.CredentialProviderListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.CredentialProvider], error)
}

func (f *FakeCredentialProvidersService) New(ctx context.Context, body kernel.CredentialProviderNewParams, opts ...option.RequestOption) (*kernel.CredentialProvider, error) {
	return &kernel.CredentialProvider{}, nil
}

func (f *FakeCredentialProvidersService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.CredentialProvider, error) {
	return &kernel.CredentialProvider{}, nil
}

func (f *FakeCredentialProvidersService) Update(ctx context.Context, id string, body kernel.CredentialProviderUpdateParams, opts ...option.RequestOption) (*kernel.CredentialProvider, error) {
	return &kernel.CredentialProvider{}, nil
}

func (f *FakeCredentialProvidersService) List(ctx context.Context, query kernel.CredentialProviderListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.CredentialProvider], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.CredentialProvider]{Items: []kernel.CredentialProvider{}}, nil
}

func (f *FakeCredentialProvidersService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	return nil
}

func (f *FakeCredentialProvidersService) Test(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.CredentialProviderTestResult, error) {
	return &kernel.CredentialProviderTestResult{}, nil
}

func (f *FakeCredentialProvidersService) ListItems(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.CredentialProviderListItemsResponse, error) {
	return &kernel.CredentialProviderListItemsResponse{}, nil
}

func TestCredentialProvidersList_ForwardsLimitOffset(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.CredentialProviderListParams
	fake := &FakeCredentialProvidersService{
		ListFunc: func(ctx context.Context, query kernel.CredentialProviderListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.CredentialProvider], error) {
			captured = query
			return &pagination.OffsetPagination[kernel.CredentialProvider]{Items: []kernel.CredentialProvider{}}, nil
		},
	}

	c := CredentialProvidersCmd{providers: fake}
	err := c.List(context.Background(), CredentialProvidersListInput{Limit: 5, Offset: 10})

	assert.NoError(t, err)
	assert.Equal(t, int64(5), captured.Limit.Value)
	assert.Equal(t, int64(10), captured.Offset.Value)
}
