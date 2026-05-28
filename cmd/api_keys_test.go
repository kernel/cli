package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeAPIKeysService struct {
	NewFunc    func(ctx context.Context, body kernel.APIKeyNewParams, opts ...option.RequestOption) (*kernel.CreatedAPIKey, error)
	GetFunc    func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.APIKey, error)
	UpdateFunc func(ctx context.Context, id string, body kernel.APIKeyUpdateParams, opts ...option.RequestOption) (*kernel.APIKey, error)
	ListFunc   func(ctx context.Context, query kernel.APIKeyListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.APIKey], error)
	DeleteFunc func(ctx context.Context, id string, opts ...option.RequestOption) error
}

func (f *FakeAPIKeysService) New(ctx context.Context, body kernel.APIKeyNewParams, opts ...option.RequestOption) (*kernel.CreatedAPIKey, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return createdAPIKeyFromJSON(`{"id":"key_123","name":"default","key":"sk_test","masked_key":"sk_...test","created_at":"2026-05-27T12:00:00Z","created_by":{"id":"user_123","email":"dev@example.com","name":"Dev"},"expires_at":null,"project_id":null,"project_name":null}`), nil
}

func (f *FakeAPIKeysService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.APIKey, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return apiKeyFromJSON(`{"id":"` + id + `","name":"default","masked_key":"sk_...test","created_at":"2026-05-27T12:00:00Z","created_by":{"id":"user_123","email":"dev@example.com","name":"Dev"},"expires_at":null,"project_id":null,"project_name":null}`), nil
}

func (f *FakeAPIKeysService) Update(ctx context.Context, id string, body kernel.APIKeyUpdateParams, opts ...option.RequestOption) (*kernel.APIKey, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return apiKeyFromJSON(`{"id":"` + id + `","name":"` + body.Name + `","masked_key":"sk_...test","created_at":"2026-05-27T12:00:00Z","created_by":{"id":"user_123","email":"dev@example.com","name":"Dev"},"expires_at":null,"project_id":null,"project_name":null}`), nil
}

func (f *FakeAPIKeysService) List(ctx context.Context, query kernel.APIKeyListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.APIKey], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.APIKey]{Items: []kernel.APIKey{}}, nil
}

func (f *FakeAPIKeysService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, id, opts...)
	}
	return nil
}

func createdAPIKeyFromJSON(raw string) *kernel.CreatedAPIKey {
	var key kernel.CreatedAPIKey
	if err := json.Unmarshal([]byte(raw), &key); err != nil {
		panic(err)
	}
	return &key
}

func apiKeyFromJSON(raw string) *kernel.APIKey {
	var key kernel.APIKey
	if err := json.Unmarshal([]byte(raw), &key); err != nil {
		panic(err)
	}
	return &key
}

func TestAPIKeysCreateBuildsParamsAndPrintsPlaintextKey(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAPIKeysService{
		NewFunc: func(ctx context.Context, body kernel.APIKeyNewParams, opts ...option.RequestOption) (*kernel.CreatedAPIKey, error) {
			assert.Equal(t, "ci", body.Name)
			assert.True(t, body.DaysToExpire.Valid())
			assert.Equal(t, int64(30), body.DaysToExpire.Value)
			assert.True(t, body.ProjectID.Valid())
			assert.Equal(t, "proj_123", body.ProjectID.Value)
			return createdAPIKeyFromJSON(`{"id":"key_123","name":"ci","key":"sk_live_123","masked_key":"sk_...123","created_at":"2026-05-27T12:00:00Z","created_by":{"id":"user_123","email":"dev@example.com","name":"Dev"},"expires_at":null,"project_id":"proj_123","project_name":"Prod"}`), nil
		},
	}
	c := APIKeysCmd{apiKeys: fake}

	err := c.Create(context.Background(), APIKeysCreateInput{
		Name: "ci",
		DaysToExpire: Int64Flag{
			Set:   true,
			Value: 30,
		},
		ProjectID: "proj_123",
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Created API key: key_123")
	assert.Contains(t, out, "key_123")
	assert.Contains(t, out, "sk_live_123")
	assert.Contains(t, out, "Prod")
}

func TestAPIKeysCreateRejectsInvalidDaysToExpire(t *testing.T) {
	c := APIKeysCmd{apiKeys: &FakeAPIKeysService{}}

	err := c.Create(context.Background(), APIKeysCreateInput{
		Name: "ci",
		DaysToExpire: Int64Flag{
			Set:   true,
			Value: 0,
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--days-to-expire must be between 1 and 3650")
}

func TestAPIKeysRejectInvalidOutputBeforeCallingAPI(t *testing.T) {
	fake := &FakeAPIKeysService{
		NewFunc: func(ctx context.Context, body kernel.APIKeyNewParams, opts ...option.RequestOption) (*kernel.CreatedAPIKey, error) {
			t.Fatal("New should not be called")
			return nil, nil
		},
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.APIKey, error) {
			t.Fatal("Get should not be called")
			return nil, nil
		},
		UpdateFunc: func(ctx context.Context, id string, body kernel.APIKeyUpdateParams, opts ...option.RequestOption) (*kernel.APIKey, error) {
			t.Fatal("Update should not be called")
			return nil, nil
		},
		ListFunc: func(ctx context.Context, query kernel.APIKeyListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.APIKey], error) {
			t.Fatal("List should not be called")
			return nil, nil
		},
	}
	c := APIKeysCmd{apiKeys: fake}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "create",
			run:  func() error { return c.Create(context.Background(), APIKeysCreateInput{Name: "ci", Output: "yaml"}) },
		},
		{
			name: "list",
			run:  func() error { return c.List(context.Background(), APIKeysListInput{Output: "yaml"}) },
		},
		{
			name: "get",
			run:  func() error { return c.Get(context.Background(), APIKeysGetInput{ID: "key_123", Output: "yaml"}) },
		},
		{
			name: "update",
			run: func() error {
				return c.Update(context.Background(), APIKeysUpdateInput{ID: "key_123", Name: "ci", Output: "yaml"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported --output value")
		})
	}
}

func TestAPIKeysListPassesPaginationAndRendersRows(t *testing.T) {
	buf := capturePtermOutput(t)
	key := *apiKeyFromJSON(`{"id":"key_123","name":"ci","masked_key":"sk_...123","created_at":"2026-05-27T12:00:00Z","created_by":{"id":"user_123","email":"dev@example.com","name":"Dev"},"expires_at":null,"project_id":null,"project_name":null}`)
	fake := &FakeAPIKeysService{
		ListFunc: func(ctx context.Context, query kernel.APIKeyListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.APIKey], error) {
			assert.True(t, query.Limit.Valid())
			assert.Equal(t, int64(10), query.Limit.Value)
			assert.True(t, query.Offset.Valid())
			assert.Equal(t, int64(20), query.Offset.Value)
			return &pagination.OffsetPagination[kernel.APIKey]{Items: []kernel.APIKey{key}}, nil
		},
	}
	c := APIKeysCmd{apiKeys: fake}

	err := c.List(context.Background(), APIKeysListInput{Limit: 10, Offset: 20})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "key_123")
	assert.Contains(t, out, "ci")
	assert.Contains(t, out, "Never")
}

func TestAPIKeysUpdateRequiresName(t *testing.T) {
	c := APIKeysCmd{apiKeys: &FakeAPIKeysService{}}
	err := c.Update(context.Background(), APIKeysUpdateInput{ID: "key_123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name is required")
}

func TestAPIKeysUpdatePrintsTerseSuccess(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAPIKeysService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.APIKeyUpdateParams, opts ...option.RequestOption) (*kernel.APIKey, error) {
			assert.Equal(t, "key_123", id)
			assert.Equal(t, "renamed", body.Name)
			return apiKeyFromJSON(`{"id":"key_123","name":"renamed","masked_key":"sk_...123","created_at":"2026-05-27T12:00:00Z","created_by":{"id":"user_123","email":"dev@example.com","name":"Dev"},"expires_at":null,"project_id":null,"project_name":null}`), nil
		},
	}
	c := APIKeysCmd{apiKeys: fake}

	err := c.Update(context.Background(), APIKeysUpdateInput{ID: "key_123", Name: "renamed"})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Updated API key: key_123")
	assert.NotContains(t, out, "Masked Key")
	assert.NotContains(t, out, "sk_...123")
}

func TestAPIKeysDeleteSkipsConfirmation(t *testing.T) {
	buf := capturePtermOutput(t)
	deleted := false
	fake := &FakeAPIKeysService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			assert.Equal(t, "key_123", id)
			deleted = true
			return nil
		},
	}
	c := APIKeysCmd{apiKeys: fake}

	err := c.Delete(context.Background(), APIKeysDeleteInput{ID: "key_123", SkipConfirm: true})
	require.NoError(t, err)
	assert.True(t, deleted)
	assert.Contains(t, buf.String(), "Deleted API key: key_123")
}

func TestAPIKeysDeleteReturnsAPIError(t *testing.T) {
	fake := &FakeAPIKeysService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			return errors.New("API error")
		},
	}
	c := APIKeysCmd{apiKeys: fake}

	err := c.Delete(context.Background(), APIKeysDeleteInput{ID: "key_123", SkipConfirm: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}
