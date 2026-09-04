package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kernel/cli/pkg/interactive"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeVaultsService implements VaultsService
type FakeVaultsService struct {
	GetFunc    func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Vault, error)
	ListFunc   func(ctx context.Context, query kernel.VaultListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Vault], error)
	DeleteFunc func(ctx context.Context, idOrName string, opts ...option.RequestOption) error
	UpsertFunc func(ctx context.Context, body kernel.VaultUpsertParams, opts ...option.RequestOption) (*kernel.Vault, error)
}

func (f *FakeVaultsService) Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Vault, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, idOrName, opts...)
	}
	return &kernel.Vault{ID: "v1", Name: idOrName, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}

func (f *FakeVaultsService) List(ctx context.Context, query kernel.VaultListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Vault], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.Vault]{Items: []kernel.Vault{}}, nil
}

func (f *FakeVaultsService) Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, idOrName, opts...)
	}
	return nil
}

func (f *FakeVaultsService) Upsert(ctx context.Context, body kernel.VaultUpsertParams, opts ...option.RequestOption) (*kernel.Vault, error) {
	if f.UpsertFunc != nil {
		return f.UpsertFunc(ctx, body, opts...)
	}
	return &kernel.Vault{ID: "v1", Name: body.Name, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}

// FakeVaultItemsService implements VaultItemsService
type FakeVaultItemsService struct {
	GetFunc              func(ctx context.Context, key string, params kernel.VaultItemGetParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error)
	UpdateFunc           func(ctx context.Context, key string, params kernel.VaultItemUpdateParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error)
	ListFunc             func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*[]kernel.VaultItemUnion, error)
	DeleteFunc           func(ctx context.Context, key string, body kernel.VaultItemDeleteParams, opts ...option.RequestOption) error
	EventsFunc           func(ctx context.Context, key string, params kernel.VaultItemEventsParams, opts ...option.RequestOption) (*[]kernel.VaultItemEvent, error)
	PerformOperationFunc func(ctx context.Context, key string, params kernel.VaultItemPerformOperationParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error)
	UpsertFunc           func(ctx context.Context, key string, params kernel.VaultItemUpsertParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error)
}

func (f *FakeVaultItemsService) Get(ctx context.Context, key string, params kernel.VaultItemGetParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, key, params, opts...)
	}
	return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "wallet"}, nil
}

func (f *FakeVaultItemsService) Update(ctx context.Context, key string, params kernel.VaultItemUpdateParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, key, params, opts...)
	}
	return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "card"}, nil
}

func (f *FakeVaultItemsService) List(ctx context.Context, idOrName string, opts ...option.RequestOption) (*[]kernel.VaultItemUnion, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, idOrName, opts...)
	}
	items := []kernel.VaultItemUnion{}
	return &items, nil
}

func (f *FakeVaultItemsService) Delete(ctx context.Context, key string, body kernel.VaultItemDeleteParams, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, key, body, opts...)
	}
	return nil
}

func (f *FakeVaultItemsService) Events(ctx context.Context, key string, params kernel.VaultItemEventsParams, opts ...option.RequestOption) (*[]kernel.VaultItemEvent, error) {
	if f.EventsFunc != nil {
		return f.EventsFunc(ctx, key, params, opts...)
	}
	events := []kernel.VaultItemEvent{}
	return &events, nil
}

func (f *FakeVaultItemsService) PerformOperation(ctx context.Context, key string, params kernel.VaultItemPerformOperationParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
	if f.PerformOperationFunc != nil {
		return f.PerformOperationFunc(ctx, key, params, opts...)
	}
	return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "card"}, nil
}

func (f *FakeVaultItemsService) Upsert(ctx context.Context, key string, params kernel.VaultItemUpsertParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
	if f.UpsertFunc != nil {
		return f.UpsertFunc(ctx, key, params, opts...)
	}
	return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "wallet"}, nil
}

// vaultItemFromJSON builds a VaultItemUnion the way the SDK does, so tests
// exercise the same union field population the API responses produce.
func vaultItemFromJSON(t *testing.T, raw string) kernel.VaultItemUnion {
	t.Helper()
	var item kernel.VaultItemUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &item))
	return item
}

func TestVaultsList_Empty(t *testing.T) {
	buf := capturePtermOutput(t)
	v := VaultsCmd{vaults: &FakeVaultsService{}}
	require.NoError(t, v.List(context.Background(), VaultsListInput{Page: 1, PerPage: 20}))
	assert.Contains(t, buf.String(), "No vaults found")
}

func TestVaultsList_PaginationFooter(t *testing.T) {
	buf := capturePtermOutput(t)
	// Three vaults for a page size of two: the extra item is what signals that
	// another page exists, and must not be displayed.
	rows := []kernel.Vault{
		{ID: "v1", Name: "alpha", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)},
		{ID: "v2", Name: "beta", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)},
		{ID: "v3", Name: "gamma", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)},
	}
	var got kernel.VaultListParams
	fake := &FakeVaultsService{ListFunc: func(ctx context.Context, query kernel.VaultListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Vault], error) {
		got = query
		return &pagination.OffsetPagination[kernel.Vault]{Items: rows}, nil
	}}
	v := VaultsCmd{vaults: fake}
	require.NoError(t, v.List(context.Background(), VaultsListInput{Page: 2, PerPage: 2}))

	assert.Equal(t, int64(3), got.Limit.Value, "requests one extra item to detect the next page")
	assert.Equal(t, int64(2), got.Offset.Value)

	out := buf.String()
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "beta")
	assert.NotContains(t, out, "gamma", "the extra item is trimmed before display")
	assert.Contains(t, out, "Page: 2  Per-page: 2  Items this page: 2  Has more: yes")
	assert.Contains(t, out, "Next: kernel vaults list --page 3 --per-page 2")
}

func TestVaultsList_DefaultsPageAndPerPage(t *testing.T) {
	_ = capturePtermOutput(t)
	var got kernel.VaultListParams
	fake := &FakeVaultsService{ListFunc: func(ctx context.Context, query kernel.VaultListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Vault], error) {
		got = query
		return &pagination.OffsetPagination[kernel.Vault]{Items: []kernel.Vault{}}, nil
	}}
	v := VaultsCmd{vaults: fake}
	require.NoError(t, v.List(context.Background(), VaultsListInput{}))

	assert.Equal(t, int64(21), got.Limit.Value, "defaults to 20 per page plus the lookahead item")
	assert.Equal(t, int64(0), got.Offset.Value)
}

func TestVaultsCreate_RequiresName(t *testing.T) {
	_ = capturePtermOutput(t)
	v := VaultsCmd{vaults: &FakeVaultsService{}}
	err := v.Create(context.Background(), VaultsCreateInput{Name: "  "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name is required")
}

func TestVaultsCreate_PassesName(t *testing.T) {
	buf := capturePtermOutput(t)
	var got kernel.VaultUpsertParams
	fake := &FakeVaultsService{UpsertFunc: func(ctx context.Context, body kernel.VaultUpsertParams, opts ...option.RequestOption) (*kernel.Vault, error) {
		got = body
		return &kernel.Vault{ID: "v1", Name: body.Name, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
	}}
	v := VaultsCmd{vaults: fake}
	require.NoError(t, v.Create(context.Background(), VaultsCreateInput{Name: "payments"}))

	assert.Equal(t, "payments", got.Name)
	out := buf.String()
	assert.Contains(t, out, "v1")
	assert.Contains(t, out, "payments")
}

func TestVaultsDelete_FailsFastWhenNonInteractive(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeVaultsService{DeleteFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
		t.Fatal("delete must not be called without confirmation")
		return nil
	}}
	v := VaultsCmd{vaults: fake, prompter: interactive.NewPrompterWithTerminal(false)}

	err := v.Delete(context.Background(), VaultsDeleteInput{Identifier: "payments"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete vault 'payments'")
	assert.Contains(t, err.Error(), "--yes")
}

func TestVaultsDelete_SkipConfirm(t *testing.T) {
	buf := capturePtermOutput(t)
	var deleted string
	fake := &FakeVaultsService{DeleteFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
		deleted = idOrName
		return nil
	}}
	v := VaultsCmd{vaults: fake, prompter: interactive.NewPrompterWithTerminal(false)}

	require.NoError(t, v.Delete(context.Background(), VaultsDeleteInput{Identifier: "payments", SkipConfirm: true}))
	assert.Equal(t, "payments", deleted)
	assert.Contains(t, buf.String(), "Deleted vault: payments")
}

func TestVaultItemsDelete_FailsFastWhenNonInteractive(t *testing.T) {
	_ = capturePtermOutput(t)
	fake := &FakeVaultItemsService{DeleteFunc: func(ctx context.Context, key string, body kernel.VaultItemDeleteParams, opts ...option.RequestOption) error {
		t.Fatal("delete must not be called without confirmation")
		return nil
	}}
	v := VaultsCmd{items: fake, prompter: interactive.NewPrompterWithTerminal(false)}

	err := v.ItemsDelete(context.Background(), VaultItemsDeleteInput{Vault: "payments", Key: "wallet"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete vault item 'wallet'")
	assert.Contains(t, err.Error(), "--yes")
}

func TestVaultItemsDelete_ScopesToVault(t *testing.T) {
	buf := capturePtermOutput(t)
	var got kernel.VaultItemDeleteParams
	fake := &FakeVaultItemsService{DeleteFunc: func(ctx context.Context, key string, body kernel.VaultItemDeleteParams, opts ...option.RequestOption) error {
		got = body
		return nil
	}}
	v := VaultsCmd{items: fake, prompter: interactive.NewPrompterWithTerminal(false)}

	require.NoError(t, v.ItemsDelete(context.Background(), VaultItemsDeleteInput{Vault: "payments", Key: "wallet", SkipConfirm: true}))
	assert.Equal(t, "payments", got.IDOrName)
	assert.Contains(t, buf.String(), "Deleted vault item: wallet")
}

func TestVaultItemsGet_SendsWaitAndExpand(t *testing.T) {
	_ = capturePtermOutput(t)
	var got kernel.VaultItemGetParams
	fake := &FakeVaultItemsService{GetFunc: func(ctx context.Context, key string, params kernel.VaultItemGetParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		got = params
		return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "wallet"}, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsGet(context.Background(), VaultItemsGetInput{
		Vault:  "payments",
		Key:    "wallet",
		Wait:   30,
		Expand: []string{"payment_methods, ", ""},
	}))

	assert.Equal(t, "payments", got.IDOrName)
	assert.Equal(t, int64(30), got.Wait.Value)
	assert.Equal(t, []string{"payment_methods"}, got.Expand, "comma-separated entries are split and blanks dropped")
}

func TestVaultItemsGet_OmitsUnsetWaitAndExpand(t *testing.T) {
	_ = capturePtermOutput(t)
	var got kernel.VaultItemGetParams
	fake := &FakeVaultItemsService{GetFunc: func(ctx context.Context, key string, params kernel.VaultItemGetParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		got = params
		return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "wallet"}, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsGet(context.Background(), VaultItemsGetInput{Vault: "payments", Key: "wallet"}))

	assert.False(t, got.Wait.Valid())
	assert.Empty(t, got.Expand)
}

func TestVaultItemsGet_RendersOperationsAndAction(t *testing.T) {
	buf := capturePtermOutput(t)
	item := vaultItemFromJSON(t, `{
		"id":"i1",
		"key":"wallet",
		"type":"wallet",
		"spec":{"provider":"agentcard"},
		"state":{"provider":"agentcard","status":"pending_authorization"},
		"action":{"name":"card_enrollment","url":"https://vault.example.com/enroll"},
		"available_operations":[{"type":"authorize","description":"Authorize the wallet."}],
		"available_expansions":[{"type":"payment_methods","description":"Live payment methods."}],
		"created_at":"1970-01-01T00:00:00Z",
		"updated_at":"1970-01-01T00:00:00Z"
	}`)
	fake := &FakeVaultItemsService{GetFunc: func(ctx context.Context, key string, params kernel.VaultItemGetParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		return &item, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsGet(context.Background(), VaultItemsGetInput{Vault: "payments", Key: "wallet"}))

	out := buf.String()
	assert.Contains(t, out, "pending_authorization")
	assert.Contains(t, out, "card_enrollment")
	assert.Contains(t, out, "https://vault.example.com/enroll")
	assert.Contains(t, out, "authorize")
	assert.Contains(t, out, "payment_methods")
	assert.Contains(t, out, "Authorize the wallet.", "operation descriptions say which operation to invoke next")
}

func TestVaultItemsCreate_RejectsUnknownType(t *testing.T) {
	_ = capturePtermOutput(t)
	v := VaultsCmd{items: &FakeVaultItemsService{}}
	err := v.ItemsCreate(context.Background(), VaultItemsCreateInput{Vault: "payments", Key: "k", Type: "cheque", Spec: "{}"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --type")
}

func TestVaultItemsCreate_RequiresSpec(t *testing.T) {
	_ = capturePtermOutput(t)
	v := VaultsCmd{items: &FakeVaultItemsService{}}
	err := v.ItemsCreate(context.Background(), VaultItemsCreateInput{Vault: "payments", Key: "k", Type: "wallet"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must specify one of --spec or --spec-file")
}

func TestVaultItemsCreate_RejectsInvalidJSON(t *testing.T) {
	_ = capturePtermOutput(t)
	v := VaultsCmd{items: &FakeVaultItemsService{}}
	err := v.ItemsCreate(context.Background(), VaultItemsCreateInput{Vault: "payments", Key: "k", Type: "wallet", Spec: "{not json"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON in spec")
}

func TestVaultItemsCreate_WalletSpec(t *testing.T) {
	_ = capturePtermOutput(t)
	var got kernel.VaultItemUpsertParams
	fake := &FakeVaultItemsService{UpsertFunc: func(ctx context.Context, key string, params kernel.VaultItemUpsertParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		got = params
		return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "wallet"}, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsCreate(context.Background(), VaultItemsCreateInput{
		Vault: "payments",
		Key:   "wallet",
		Type:  "WALLET",
		Spec:  `{"provider":"link","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`,
	}))

	assert.Equal(t, "payments", got.IDOrName)
	require.NotNil(t, got.OfWallet)
	assert.Nil(t, got.OfCard)

	body, err := json.Marshal(got.OfWallet.Spec)
	require.NoError(t, err)
	assert.JSONEq(t, `{"provider":"link","authorization":{"method":"oauth","client":{"type":"kernel_managed"}}}`, string(body))
}

func TestVaultItemsCreate_CardSpec(t *testing.T) {
	_ = capturePtermOutput(t)
	var got kernel.VaultItemUpsertParams
	fake := &FakeVaultItemsService{UpsertFunc: func(ctx context.Context, key string, params kernel.VaultItemUpsertParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		got = params
		return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "card"}, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsCreate(context.Background(), VaultItemsCreateInput{
		Vault: "payments",
		Key:   "card",
		Type:  "card",
		Spec:  `{"provider":"agentcard","wallet":"my-wallet","merchant":"Acme","amount":1250,"currency":"USD"}`,
	}))

	require.NotNil(t, got.OfCard)
	assert.Nil(t, got.OfWallet)

	body, err := json.Marshal(got.OfCard.Spec)
	require.NoError(t, err)
	assert.JSONEq(t, `{"provider":"agentcard","wallet":"my-wallet","merchant":"Acme","amount":1250,"currency":"USD"}`, string(body))
}

func TestVaultItemsCreate_SpecFile(t *testing.T) {
	_ = capturePtermOutput(t)
	path := filepath.Join(t.TempDir(), "spec.json")
	require.NoError(t, os.WriteFile(path, []byte("  {\"provider\":\"agentcard\",\"user_id\":\"usr_123\"}\n"), 0o600))

	var got kernel.VaultItemUpsertParams
	fake := &FakeVaultItemsService{UpsertFunc: func(ctx context.Context, key string, params kernel.VaultItemUpsertParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		got = params
		return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "wallet"}, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsCreate(context.Background(), VaultItemsCreateInput{
		Vault:    "payments",
		Key:      "wallet",
		Type:     "wallet",
		SpecFile: path,
	}))

	require.NotNil(t, got.OfWallet)
	body, err := json.Marshal(got.OfWallet.Spec)
	require.NoError(t, err)
	assert.JSONEq(t, `{"provider":"agentcard","user_id":"usr_123"}`, string(body))
}

func TestVaultItemsUpdate_SendsCardSpec(t *testing.T) {
	_ = capturePtermOutput(t)
	var got kernel.VaultItemUpdateParams
	fake := &FakeVaultItemsService{UpdateFunc: func(ctx context.Context, key string, params kernel.VaultItemUpdateParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		got = params
		return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "card"}, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsUpdate(context.Background(), VaultItemsUpdateInput{
		Vault: "payments",
		Key:   "card",
		Spec:  `{"provider":"agentcard","wallet":"my-wallet","merchant":"Acme","amount":500,"currency":"USD"}`,
	}))

	assert.Equal(t, "payments", got.IDOrName)
	body, err := json.Marshal(got.Spec)
	require.NoError(t, err)
	assert.JSONEq(t, `{"provider":"agentcard","wallet":"my-wallet","merchant":"Acme","amount":500,"currency":"USD"}`, string(body))
}

func TestVaultItemsUpdate_RequiresSpec(t *testing.T) {
	_ = capturePtermOutput(t)
	v := VaultsCmd{items: &FakeVaultItemsService{}}
	err := v.ItemsUpdate(context.Background(), VaultItemsUpdateInput{Vault: "payments", Key: "card"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must specify one of --spec or --spec-file")
}

func TestVaultItemsPerformOperation_SendsType(t *testing.T) {
	_ = capturePtermOutput(t)
	var got kernel.VaultItemPerformOperationParams
	fake := &FakeVaultItemsService{PerformOperationFunc: func(ctx context.Context, key string, params kernel.VaultItemPerformOperationParams, opts ...option.RequestOption) (*kernel.VaultItemUnion, error) {
		got = params
		return &kernel.VaultItemUnion{ID: "i1", Key: key, Type: "card"}, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsPerformOperation(context.Background(), VaultItemsPerformOperationInput{
		Vault: "payments",
		Key:   "card",
		Type:  "Authorize",
	}))

	assert.Equal(t, "payments", got.IDOrName)
	assert.Equal(t, kernel.VaultItemPerformOperationParamsTypeAuthorize, got.Type)
}

func TestVaultItemsPerformOperation_RequiresType(t *testing.T) {
	_ = capturePtermOutput(t)
	v := VaultsCmd{items: &FakeVaultItemsService{}}
	err := v.ItemsPerformOperation(context.Background(), VaultItemsPerformOperationInput{Vault: "payments", Key: "card", Type: " "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--type is required")
}

func TestVaultItemsEvents_SendsAfterAndWait(t *testing.T) {
	buf := capturePtermOutput(t)
	var got kernel.VaultItemEventsParams
	fake := &FakeVaultItemsService{EventsFunc: func(ctx context.Context, key string, params kernel.VaultItemEventsParams, opts ...option.RequestOption) (*[]kernel.VaultItemEvent, error) {
		got = params
		events := []kernel.VaultItemEvent{{
			ID:        "e1",
			Name:      "wallet_authorization_started",
			BrowserID: "b1",
			CreatedAt: time.Unix(0, 0),
		}}
		return &events, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsEvents(context.Background(), VaultItemsEventsInput{
		Vault: "payments",
		Key:   "wallet",
		After: "e0",
		Wait:  10,
	}))

	assert.Equal(t, "payments", got.IDOrName)
	assert.Equal(t, "e0", got.After.Value)
	assert.Equal(t, int64(10), got.Wait.Value)

	out := buf.String()
	assert.Contains(t, out, "e1")
	assert.Contains(t, out, "wallet_authorization_started")
	assert.Contains(t, out, "b1")
}

func TestVaultItemsEvents_Empty(t *testing.T) {
	buf := capturePtermOutput(t)
	v := VaultsCmd{items: &FakeVaultItemsService{}}
	require.NoError(t, v.ItemsEvents(context.Background(), VaultItemsEventsInput{Vault: "payments", Key: "wallet"}))
	assert.Contains(t, buf.String(), "No events found for item 'wallet'")
}

func TestVaultItemsList_RendersRows(t *testing.T) {
	buf := capturePtermOutput(t)
	item := vaultItemFromJSON(t, `{
		"id":"i1",
		"key":"card",
		"type":"card",
		"spec":{"provider":"agentcard","merchant":"Acme"},
		"state":{"provider":"agentcard","status":"ready"},
		"created_at":"1970-01-01T00:00:00Z",
		"updated_at":"1970-01-01T00:00:00Z"
	}`)
	fake := &FakeVaultItemsService{ListFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*[]kernel.VaultItemUnion, error) {
		items := []kernel.VaultItemUnion{item}
		return &items, nil
	}}
	v := VaultsCmd{items: fake}
	require.NoError(t, v.ItemsList(context.Background(), VaultItemsListInput{Vault: "payments"}))

	out := buf.String()
	assert.Contains(t, out, "card")
	assert.Contains(t, out, "agentcard")
	assert.Contains(t, out, "ready")
}

func TestVaultItemsList_Empty(t *testing.T) {
	buf := capturePtermOutput(t)
	v := VaultsCmd{items: &FakeVaultItemsService{}}
	require.NoError(t, v.ItemsList(context.Background(), VaultItemsListInput{Vault: "payments"}))
	assert.Contains(t, buf.String(), "No items found in vault 'payments'")
}

func TestFormatVaultReferences(t *testing.T) {
	assert.Empty(t, formatVaultReferences(nil), "no vaults means the row is omitted entirely")
	assert.Equal(t, "payments, v2", formatVaultReferences([]kernel.VaultReference{
		{ID: "v1", Name: "payments"},
		{ID: "v2"},
	}), "names are preferred, IDs are the fallback")
}
