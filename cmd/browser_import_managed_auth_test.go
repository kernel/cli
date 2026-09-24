package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kernel/cli/internal/passwordmanager"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeImportedCredentials struct {
	newFunc    func(kernel.CredentialNewParams) (*kernel.Credential, error)
	getFunc    func(string) (*kernel.Credential, error)
	updateFunc func(string, kernel.CredentialUpdateParams) (*kernel.Credential, error)
}

func (f fakeImportedCredentials) New(_ context.Context, params kernel.CredentialNewParams, _ ...option.RequestOption) (*kernel.Credential, error) {
	return f.newFunc(params)
}

func (f fakeImportedCredentials) Get(_ context.Context, name string, _ ...option.RequestOption) (*kernel.Credential, error) {
	return f.getFunc(name)
}

func (f fakeImportedCredentials) Update(_ context.Context, name string, params kernel.CredentialUpdateParams, _ ...option.RequestOption) (*kernel.Credential, error) {
	return f.updateFunc(name, params)
}

type fakeImportedConnections struct {
	newFunc  func(kernel.AuthConnectionNewParams) (*kernel.ManagedAuth, error)
	listFunc func(kernel.AuthConnectionListParams) (*pagination.OffsetPagination[kernel.ManagedAuth], error)
}

func (f fakeImportedConnections) New(_ context.Context, params kernel.AuthConnectionNewParams, _ ...option.RequestOption) (*kernel.ManagedAuth, error) {
	return f.newFunc(params)
}

func (f fakeImportedConnections) List(_ context.Context, params kernel.AuthConnectionListParams, _ ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuth], error) {
	return f.listFunc(params)
}

func TestManagedAuthProvisionFreshPathUsesThreeRequests(t *testing.T) {
	calls := make([]string, 0, 3)
	provisioner := kernelManagedAuthProvisioner{
		credentials: fakeImportedCredentials{
			newFunc: func(params kernel.CredentialNewParams) (*kernel.Credential, error) {
				calls = append(calls, "credential.new")
				return &kernel.Credential{Name: params.CreateCredentialRequest.Name}, nil
			},
			getFunc: func(string) (*kernel.Credential, error) { t.Fatal("unexpected credential get"); return nil, nil },
			updateFunc: func(string, kernel.CredentialUpdateParams) (*kernel.Credential, error) {
				t.Fatal("unexpected credential update")
				return nil, nil
			},
		},
		connections: fakeImportedConnections{
			listFunc: func(kernel.AuthConnectionListParams) (*pagination.OffsetPagination[kernel.ManagedAuth], error) {
				calls = append(calls, "connection.list")
				return &pagination.OffsetPagination[kernel.ManagedAuth]{Items: []kernel.ManagedAuth{}}, nil
			},
			newFunc: func(kernel.AuthConnectionNewParams) (*kernel.ManagedAuth, error) {
				calls = append(calls, "connection.new")
				return &kernel.ManagedAuth{ID: "auth_1"}, nil
			},
		},
	}

	ids, err := provisioner.Provision(t.Context(), "imported", []passwordmanager.Record{{Provider: "bitwarden", ID: "item", Domain: "example.com", Username: "me", Password: "secret"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"auth_1"}, ids)
	assert.Equal(t, []string{"connection.list", "credential.new", "connection.new"}, calls)
}

func TestManagedAuthProvisionRetryRefreshesCredentialAndRemovesTOTP(t *testing.T) {
	record := passwordmanager.Record{Provider: "bitwarden", ID: "item", Domain: "example.com", Username: "me", Password: "new"}
	name := importedCredentialName(record)
	var update kernel.CredentialUpdateParams
	provisioner := kernelManagedAuthProvisioner{
		credentials: fakeImportedCredentials{
			newFunc: func(kernel.CredentialNewParams) (*kernel.Credential, error) {
				t.Fatal("unexpected credential create")
				return nil, nil
			},
			getFunc: func(string) (*kernel.Credential, error) { t.Fatal("unexpected credential get"); return nil, nil },
			updateFunc: func(_ string, params kernel.CredentialUpdateParams) (*kernel.Credential, error) {
				update = params
				return &kernel.Credential{Name: name}, nil
			},
		},
		connections: fakeImportedConnections{
			listFunc: func(kernel.AuthConnectionListParams) (*pagination.OffsetPagination[kernel.ManagedAuth], error) {
				return &pagination.OffsetPagination[kernel.ManagedAuth]{Items: []kernel.ManagedAuth{{ID: "auth_existing", Credential: kernel.ManagedAuthCredential{Name: name}}}}, nil
			},
			newFunc: func(kernel.AuthConnectionNewParams) (*kernel.ManagedAuth, error) {
				t.Fatal("unexpected connection create")
				return nil, nil
			},
		},
	}

	ids, err := provisioner.Provision(t.Context(), "imported", []passwordmanager.Record{record})
	require.NoError(t, err)
	assert.Equal(t, []string{"auth_existing"}, ids)
	require.True(t, update.UpdateCredentialRequest.TotpSecret.Valid())
	assert.Empty(t, update.UpdateCredentialRequest.TotpSecret.Value)
}

func TestManagedAuthProvisionDoesNotMutateCredentialOnConnectionConflict(t *testing.T) {
	provisioner := kernelManagedAuthProvisioner{
		credentials: fakeImportedCredentials{
			newFunc: func(kernel.CredentialNewParams) (*kernel.Credential, error) {
				t.Fatal("unexpected credential create")
				return nil, nil
			},
			getFunc: func(string) (*kernel.Credential, error) { t.Fatal("unexpected credential get"); return nil, nil },
			updateFunc: func(string, kernel.CredentialUpdateParams) (*kernel.Credential, error) {
				t.Fatal("unexpected credential update")
				return nil, nil
			},
		},
		connections: fakeImportedConnections{
			listFunc: func(kernel.AuthConnectionListParams) (*pagination.OffsetPagination[kernel.ManagedAuth], error) {
				return &pagination.OffsetPagination[kernel.ManagedAuth]{Items: []kernel.ManagedAuth{{ID: "auth_other", Credential: kernel.ManagedAuthCredential{Name: "user-owned"}}}}, nil
			},
			newFunc: func(kernel.AuthConnectionNewParams) (*kernel.ManagedAuth, error) {
				return nil, errors.New("unexpected connection create")
			},
		},
	}

	_, err := provisioner.Provision(t.Context(), "imported", []passwordmanager.Record{{Provider: "bitwarden", ID: "item", Domain: "example.com"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `already uses credential "user-owned"`)
}

func TestImportedCredentialNameFitsAPINameLimit(t *testing.T) {
	name := importedCredentialName(passwordmanager.Record{Provider: "bitwarden", ID: "item", Domain: strings.Repeat("a", 253)})
	assert.LessOrEqual(t, len(name), 255)
	assert.Contains(t, name, "import-bitwarden-")
	assert.Regexp(t, `-[a-f0-9]{10}$`, name)
}

func TestManagedAuthProvisionFindsMatchingConnectionAfterSiblingAccount(t *testing.T) {
	record := passwordmanager.Record{Provider: "bitwarden", ID: "item", Domain: "example.com", Username: "me"}
	name := importedCredentialName(record)
	updated := false
	provisioner := kernelManagedAuthProvisioner{
		credentials: fakeImportedCredentials{
			newFunc: func(kernel.CredentialNewParams) (*kernel.Credential, error) {
				t.Fatal("unexpected credential create")
				return nil, nil
			},
			getFunc: func(string) (*kernel.Credential, error) { t.Fatal("unexpected credential get"); return nil, nil },
			updateFunc: func(string, kernel.CredentialUpdateParams) (*kernel.Credential, error) {
				updated = true
				return &kernel.Credential{Name: name}, nil
			},
		},
		connections: fakeImportedConnections{
			listFunc: func(kernel.AuthConnectionListParams) (*pagination.OffsetPagination[kernel.ManagedAuth], error) {
				return &pagination.OffsetPagination[kernel.ManagedAuth]{Items: []kernel.ManagedAuth{
					{ID: "auth_other", Credential: kernel.ManagedAuthCredential{Name: "another-account"}},
					{ID: "auth_match", Credential: kernel.ManagedAuthCredential{Name: name}},
				}}, nil
			},
			newFunc: func(kernel.AuthConnectionNewParams) (*kernel.ManagedAuth, error) {
				t.Fatal("unexpected connection create")
				return nil, nil
			},
		},
	}

	ids, err := provisioner.Provision(t.Context(), "imported", []passwordmanager.Record{record})
	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, []string{"auth_match"}, ids)
}
