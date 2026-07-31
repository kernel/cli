package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeAuthConnectionService struct {
	NewFunc             func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error)
	GetFunc             func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error)
	UpdateFunc          func(ctx context.Context, id string, body kernel.AuthConnectionUpdateParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error)
	ListFunc            func(ctx context.Context, query kernel.AuthConnectionListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuth], error)
	DeleteFunc          func(ctx context.Context, id string, opts ...option.RequestOption) error
	LoginFunc           func(ctx context.Context, id string, body kernel.AuthConnectionLoginParams, opts ...option.RequestOption) (*kernel.LoginResponse, error)
	SubmitFunc          func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error)
	TimelineFunc        func(ctx context.Context, id string, query kernel.AuthConnectionTimelineParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent], error)
	FollowStreamingFunc func(ctx context.Context, id string, opts ...option.RequestOption) *ssestream.Stream[kernel.AuthConnectionFollowResponseUnion]
}

func (f *FakeAuthConnectionService) Timeline(ctx context.Context, id string, query kernel.AuthConnectionTimelineParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent], error) {
	if f.TimelineFunc != nil {
		return f.TimelineFunc(ctx, id, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent]{}, nil
}

func (f *FakeAuthConnectionService) New(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.ManagedAuth{}, nil
}

func (f *FakeAuthConnectionService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return nil, errors.New("not found")
}

func (f *FakeAuthConnectionService) Update(ctx context.Context, id string, body kernel.AuthConnectionUpdateParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return &kernel.ManagedAuth{ID: id}, nil
}

func (f *FakeAuthConnectionService) List(ctx context.Context, query kernel.AuthConnectionListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuth], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.ManagedAuth]{Items: []kernel.ManagedAuth{}}, nil
}

func (f *FakeAuthConnectionService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, id, opts...)
	}
	return nil
}

func (f *FakeAuthConnectionService) Login(ctx context.Context, id string, body kernel.AuthConnectionLoginParams, opts ...option.RequestOption) (*kernel.LoginResponse, error) {
	if f.LoginFunc != nil {
		return f.LoginFunc(ctx, id, body, opts...)
	}
	return &kernel.LoginResponse{}, nil
}

func (f *FakeAuthConnectionService) Submit(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
	if f.SubmitFunc != nil {
		return f.SubmitFunc(ctx, id, body, opts...)
	}
	return &kernel.SubmitFieldsResponse{Accepted: true}, nil
}

func (f *FakeAuthConnectionService) FollowStreaming(ctx context.Context, id string, opts ...option.RequestOption) *ssestream.Stream[kernel.AuthConnectionFollowResponseUnion] {
	if f.FollowStreamingFunc != nil {
		return f.FollowStreamingFunc(ctx, id, opts...)
	}
	return nil
}

func TestAuthConnectionsGet_PrintsSubmissionHints(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeAuthConnectionService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			return &kernel.ManagedAuth{
				ID:          id,
				Domain:      "auth.leaseweb.com",
				ProfileName: "raf-leaseweb",
				Status:      kernel.ManagedAuthStatusNeedsAuth,
				FlowStatus:  kernel.ManagedAuthFlowStatusInProgress,
				FlowStep:    kernel.ManagedAuthFlowStepAwaitingInput,
				DiscoveredFields: []kernel.ManagedAuthDiscoveredField{
					{Name: "username", Type: "text", Required: true},
					{Name: "password", Type: "password", Required: true},
				},
				MfaOptions: []kernel.ManagedAuthMfaOption{
					{Label: "Text message", Type: "sms"},
				},
				PendingSSOButtons: []kernel.ManagedAuthPendingSSOButton{
					{Label: "Continue with Google", Provider: "google"},
				},
			}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}

	err := c.Get(context.Background(), AuthConnectionGetInput{ID: "e0x3vbw4z66kpwny3k5k46tj"})
	require.NoError(t, err)

	out := outBuf.String()
	assert.Contains(t, out, "Discovered Fields")
	assert.Contains(t, out, "username")
	assert.Contains(t, out, "password")
	assert.Contains(t, out, "MFA Options")
	assert.Contains(t, out, "Text message")
	assert.Contains(t, out, "Pending SSO Buttons")
	assert.Contains(t, out, "Continue with Google")
}

func TestAuthConnectionsGet_JSONOutputIncludesDiscoveredFields(t *testing.T) {
	setupStdoutCapture(t)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	fake := &FakeAuthConnectionService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			jsonData := `{
				"id":"e0x3vbw4z66kpwny3k5k46tj",
				"domain":"auth.leaseweb.com",
				"profile_name":"raf-leaseweb",
				"save_credentials":true,
				"status":"NEEDS_AUTH",
				"flow_status":"IN_PROGRESS",
				"flow_step":"AWAITING_INPUT",
				"discovered_fields":[
					{"label":"Email","name":"email","selector":"#email","type":"email","required":true}
				]
			}`
			var auth kernel.ManagedAuth
			require.NoError(t, json.Unmarshal([]byte(jsonData), &auth))
			return &auth, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}

	err := c.Get(context.Background(), AuthConnectionGetInput{
		ID:     "e0x3vbw4z66kpwny3k5k46tj",
		Output: "json",
	})
	require.NoError(t, err)

	w.Close()
	var stdoutBuf bytes.Buffer
	_, _ = io.Copy(&stdoutBuf, r)
	out := stdoutBuf.String()
	assert.Contains(t, out, "\"discovered_fields\"")
	assert.Contains(t, out, "\"selector\"")
	assert.Contains(t, out, "\"email\"")
}

func TestAuthConnectionsList_JSONOutput_PrintsRawResponse(t *testing.T) {
	setupStdoutCapture(t)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	fake := &FakeAuthConnectionService{
		ListFunc: func(ctx context.Context, query kernel.AuthConnectionListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuth], error) {
			jsonData := `[{
				"id":"e0x3vbw4z66kpwny3k5k46tj",
				"domain":"auth.leaseweb.com",
				"profile_name":"raf-leaseweb",
				"save_credentials":true,
				"status":"NEEDS_AUTH"
			}]`
			var page pagination.OffsetPagination[kernel.ManagedAuth]
			require.NoError(t, json.Unmarshal([]byte(jsonData), &page))
			return &page, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}

	err := c.List(context.Background(), AuthConnectionListInput{Output: "json"})
	require.NoError(t, err)

	w.Close()
	var stdoutBuf bytes.Buffer
	_, _ = io.Copy(&stdoutBuf, r)
	out := stdoutBuf.String()
	assert.Contains(t, out, "\"profile_name\"")
	assert.Contains(t, out, "\"raf-leaseweb\"")
}

// Regression test for the 1Password auto-lookup UX gap: before this fix,
// `kernel auth connections create --credential-provider foo` sent a
// CredentialReference of { provider } with no auto flag, which the API accepted
// as valid-but-inert — the managed auth session would never fetch credentials
// and would prompt the user for manual input. The dashboard already defaults
// to auto: true for this case (see packages/dashboard/src/components/create-managed-auth-dialog.tsx);
// the CLI now matches that UX.
func TestAuthConnectionsCreate_ProviderWithoutPath_DefaultsAutoTrue(t *testing.T) {
	var captured kernel.AuthConnectionNewParams
	fake := &FakeAuthConnectionService{
		NewFunc: func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{ID: "conn-new"}, nil
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Create(context.Background(), AuthConnectionCreateInput{
		Domain:             "google.com",
		ProfileName:        "my-profile",
		CredentialProvider: "my-1p",
		Output:             "json",
	})
	require.NoError(t, err)

	cred := captured.ManagedAuthCreateRequest.Credential
	require.True(t, cred.Provider.Valid())
	assert.Equal(t, "my-1p", cred.Provider.Value)
	assert.False(t, cred.Path.Valid(), "path should not be set when only --credential-provider is given")
	require.True(t, cred.Auto.Valid(), "auto should default to true when provider is set without path")
	assert.True(t, cred.Auto.Value)
}

// Explicit --credential-path should keep the credential reference as a pinned
// path lookup (no implicit auto).
func TestAuthConnectionsCreate_ProviderWithPath_DoesNotSetAuto(t *testing.T) {
	var captured kernel.AuthConnectionNewParams
	fake := &FakeAuthConnectionService{
		NewFunc: func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{ID: "conn-new"}, nil
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Create(context.Background(), AuthConnectionCreateInput{
		Domain:             "google.com",
		ProfileName:        "my-profile",
		CredentialProvider: "my-1p",
		CredentialPath:     "Employees/Google Workspace",
		Output:             "json",
	})
	require.NoError(t, err)

	cred := captured.ManagedAuthCreateRequest.Credential
	require.True(t, cred.Provider.Valid())
	assert.Equal(t, "my-1p", cred.Provider.Value)
	require.True(t, cred.Path.Valid())
	assert.Equal(t, "Employees/Google Workspace", cred.Path.Value)
	assert.False(t, cred.Auto.Valid(), "auto should remain unset when --credential-path is explicit")
}

// --credential-auto should still be honored (it was a no-op redundant flag
// before the default changed, but callers may pass it for clarity).
func TestAuthConnectionsCreate_ProviderWithExplicitAuto_SetsAuto(t *testing.T) {
	var captured kernel.AuthConnectionNewParams
	fake := &FakeAuthConnectionService{
		NewFunc: func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{ID: "conn-new"}, nil
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Create(context.Background(), AuthConnectionCreateInput{
		Domain:             "google.com",
		ProfileName:        "my-profile",
		CredentialProvider: "my-1p",
		CredentialAuto:     true,
		Output:             "json",
	})
	require.NoError(t, err)

	cred := captured.ManagedAuthCreateRequest.Credential
	require.True(t, cred.Auto.Valid())
	assert.True(t, cred.Auto.Value)
}

// --credential-name references a Kernel-managed credential and should never
// carry a provider/auto/path — the default-auto logic must not kick in here.
func TestAuthConnectionsCreate_CredentialName_UnaffectedByAutoDefault(t *testing.T) {
	var captured kernel.AuthConnectionNewParams
	fake := &FakeAuthConnectionService{
		NewFunc: func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{ID: "conn-new"}, nil
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Create(context.Background(), AuthConnectionCreateInput{
		Domain:         "google.com",
		ProfileName:    "my-profile",
		CredentialName: "my-google-creds",
		Output:         "json",
	})
	require.NoError(t, err)

	cred := captured.ManagedAuthCreateRequest.Credential
	require.True(t, cred.Name.Valid())
	assert.Equal(t, "my-google-creds", cred.Name.Value)
	assert.False(t, cred.Provider.Valid())
	assert.False(t, cred.Auto.Valid())
	assert.False(t, cred.Path.Valid())
}

func TestAuthConnectionsCreate_RecordSession(t *testing.T) {
	tests := []struct {
		name string
		flag BoolFlag
	}{
		{name: "omitted"},
		{name: "enabled", flag: BoolFlag{Set: true, Value: true}},
		{name: "disabled", flag: BoolFlag{Set: true, Value: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured kernel.AuthConnectionNewParams
			fake := &FakeAuthConnectionService{
				NewFunc: func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
					captured = body
					return &kernel.ManagedAuth{ID: "conn-new"}, nil
				},
			}

			c := AuthConnectionCmd{svc: fake}
			require.NoError(t, c.Create(context.Background(), AuthConnectionCreateInput{
				Domain:        "example.com",
				ProfileName:   "profile-1",
				RecordSession: tt.flag,
				Output:        "json",
			}))

			assert.Equal(t, tt.flag.Set, captured.ManagedAuthCreateRequest.RecordSession.Valid())
			if tt.flag.Set {
				assert.Equal(t, tt.flag.Value, captured.ManagedAuthCreateRequest.RecordSession.Value)
			}
		})
	}
}

func TestAuthConnectionsUpdate_MapsParams(t *testing.T) {
	var captured kernel.AuthConnectionUpdateParams
	fake := &FakeAuthConnectionService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuthConnectionUpdateParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{
				ID:          id,
				Domain:      "example.com",
				ProfileName: "profile-1",
				Status:      kernel.ManagedAuthStatusAuthenticated,
			}, nil
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Update(context.Background(), AuthConnectionUpdateInput{
		ID:                     "conn-1",
		LoginURL:               "https://login.example.com",
		LoginURLSet:            true,
		AllowedDomains:         []string{"example.com", "login.example.com"},
		AllowedDomainsSet:      true,
		CredentialProvider:     "vault-provider",
		CredentialProviderSet:  true,
		CredentialPath:         "Vault/Item",
		CredentialPathSet:      true,
		CredentialAuto:         BoolFlag{Set: true, Value: true},
		ProxyID:                "proxy-123",
		ProxyIDSet:             true,
		SaveCredentials:        BoolFlag{Set: true, Value: false},
		RecordSession:          BoolFlag{Set: true, Value: false},
		HealthCheckInterval:    900,
		HealthCheckIntervalSet: true,
	})
	require.NoError(t, err)
	require.True(t, captured.ManagedAuthUpdateRequest.LoginURL.Valid())
	assert.Equal(t, "https://login.example.com", captured.ManagedAuthUpdateRequest.LoginURL.Value)
	assert.Equal(t, []string{"example.com", "login.example.com"}, captured.ManagedAuthUpdateRequest.AllowedDomains)
	require.True(t, captured.ManagedAuthUpdateRequest.Credential.Provider.Valid())
	assert.Equal(t, "vault-provider", captured.ManagedAuthUpdateRequest.Credential.Provider.Value)
	require.True(t, captured.ManagedAuthUpdateRequest.Credential.Path.Valid())
	assert.Equal(t, "Vault/Item", captured.ManagedAuthUpdateRequest.Credential.Path.Value)
	require.True(t, captured.ManagedAuthUpdateRequest.Credential.Auto.Valid())
	assert.True(t, captured.ManagedAuthUpdateRequest.Credential.Auto.Value)
	require.True(t, captured.ManagedAuthUpdateRequest.Proxy.ID.Valid())
	assert.Equal(t, "proxy-123", captured.ManagedAuthUpdateRequest.Proxy.ID.Value)
	require.True(t, captured.ManagedAuthUpdateRequest.SaveCredentials.Valid())
	assert.False(t, captured.ManagedAuthUpdateRequest.SaveCredentials.Value)
	require.True(t, captured.ManagedAuthUpdateRequest.RecordSession.Valid())
	assert.False(t, captured.ManagedAuthUpdateRequest.RecordSession.Value)
	require.True(t, captured.ManagedAuthUpdateRequest.HealthCheckInterval.Valid())
	assert.Equal(t, int64(900), captured.ManagedAuthUpdateRequest.HealthCheckInterval.Value)
}

func TestAuthConnectionsLogin_RecordSession(t *testing.T) {
	tests := []struct {
		name string
		flag BoolFlag
	}{
		{name: "inherits connection default"},
		{name: "enabled", flag: BoolFlag{Set: true, Value: true}},
		{name: "disabled", flag: BoolFlag{Set: true, Value: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured kernel.AuthConnectionLoginParams
			fake := &FakeAuthConnectionService{
				LoginFunc: func(ctx context.Context, id string, body kernel.AuthConnectionLoginParams, opts ...option.RequestOption) (*kernel.LoginResponse, error) {
					captured = body
					return &kernel.LoginResponse{}, nil
				},
			}

			c := AuthConnectionCmd{svc: fake}
			require.NoError(t, c.Login(context.Background(), AuthConnectionLoginInput{
				ID:            "conn-1",
				RecordSession: tt.flag,
				Output:        "json",
			}))

			assert.Equal(t, tt.flag.Set, captured.RecordSession.Valid())
			if tt.flag.Set {
				assert.Equal(t, tt.flag.Value, captured.RecordSession.Value)
			}
		})
	}
}

func newFakeWithMfaOptions(options []kernel.ManagedAuthMfaOption) *FakeAuthConnectionService {
	return &FakeAuthConnectionService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			return &kernel.ManagedAuth{
				ID:         id,
				MfaOptions: options,
			}, nil
		},
		SubmitFunc: func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
			return &kernel.SubmitFieldsResponse{Accepted: true}, nil
		},
	}
}

func TestSubmit_MfaOptionResolvesType(t *testing.T) {
	fake := newFakeWithMfaOptions([]kernel.ManagedAuthMfaOption{
		{Label: "Get a text", Type: "sms"},
		{Label: "Have us call you", Type: "call"},
	})

	var submittedID string
	fake.SubmitFunc = func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
		submittedID = body.SubmitFieldsRequest.MfaOptionID.Value
		return &kernel.SubmitFieldsResponse{Accepted: true}, nil
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:          "conn-1",
		MfaOptionID: "sms",
		Output:      "json",
	})
	require.NoError(t, err)
	assert.Equal(t, "sms", submittedID)
}

func TestSubmit_SSOProviderMapped(t *testing.T) {
	var provider string
	fake := &FakeAuthConnectionService{
		SubmitFunc: func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
			provider = body.SubmitFieldsRequest.SSOProvider.Value
			return &kernel.SubmitFieldsResponse{Accepted: true}, nil
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:          "conn-1",
		SSOProvider: "google",
		Output:      "json",
	})
	require.NoError(t, err)
	assert.Equal(t, "google", provider)
}

func TestSubmit_SignInOptionMapped(t *testing.T) {
	var signInOption string
	fake := &FakeAuthConnectionService{
		SubmitFunc: func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
			signInOption = body.SubmitFieldsRequest.SignInOptionID.Value
			return &kernel.SubmitFieldsResponse{Accepted: true}, nil
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:             "conn-1",
		SignInOptionID: "pick-account",
		Output:         "json",
	})
	require.NoError(t, err)
	assert.Equal(t, "pick-account", signInOption)
}

func TestSubmit_MfaOptionResolvesLabel(t *testing.T) {
	fake := newFakeWithMfaOptions([]kernel.ManagedAuthMfaOption{
		{Label: "Get a text", Type: "sms"},
		{Label: "Have us call you", Type: "call"},
	})

	var submittedID string
	fake.SubmitFunc = func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
		submittedID = body.SubmitFieldsRequest.MfaOptionID.Value
		return &kernel.SubmitFieldsResponse{Accepted: true}, nil
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:          "conn-1",
		MfaOptionID: "Get a text",
		Output:      "json",
	})
	require.NoError(t, err)
	assert.Equal(t, "sms", submittedID)
}

func TestSubmit_MfaOptionResolvesDisplayString(t *testing.T) {
	fake := newFakeWithMfaOptions([]kernel.ManagedAuthMfaOption{
		{Label: "Get a text", Type: "sms"},
	})

	var submittedID string
	fake.SubmitFunc = func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
		submittedID = body.SubmitFieldsRequest.MfaOptionID.Value
		return &kernel.SubmitFieldsResponse{Accepted: true}, nil
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:          "conn-1",
		MfaOptionID: "Get a text (sms)",
		Output:      "json",
	})
	require.NoError(t, err)
	assert.Equal(t, "sms", submittedID)
}

func TestSubmit_MfaOptionResolvesLabelCaseInsensitive(t *testing.T) {
	fake := newFakeWithMfaOptions([]kernel.ManagedAuthMfaOption{
		{Label: "Get a text", Type: "sms"},
	})

	var submittedID string
	fake.SubmitFunc = func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
		submittedID = body.SubmitFieldsRequest.MfaOptionID.Value
		return &kernel.SubmitFieldsResponse{Accepted: true}, nil
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:          "conn-1",
		MfaOptionID: "get a TEXT",
		Output:      "json",
	})
	require.NoError(t, err)
	assert.Equal(t, "sms", submittedID)
}

func TestSubmit_MfaOptionGetErrorSurfaced(t *testing.T) {
	fake := &FakeAuthConnectionService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			return nil, errors.New("connection not found")
		},
	}

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:          "conn-1",
		MfaOptionID: "sms",
		Output:      "json",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection not found")
}

func TestSubmit_MfaOptionRejectsUnknown(t *testing.T) {
	fake := newFakeWithMfaOptions([]kernel.ManagedAuthMfaOption{
		{Label: "Get a text", Type: "sms"},
		{Label: "Have us call you", Type: "call"},
	})

	c := AuthConnectionCmd{svc: fake}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:          "conn-1",
		MfaOptionID: "carrier pigeon",
		Output:      "json",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown MFA option")
	assert.Contains(t, err.Error(), "carrier pigeon")
	assert.Contains(t, err.Error(), "Get a text (sms)")
}

func TestCreate_TelemetryCategoriesOptIn(t *testing.T) {
	capturePtermOutput(t)
	var captured kernel.AuthConnectionNewParams
	fake := &FakeAuthConnectionService{
		NewFunc: func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{ID: "auth_1"}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	err := c.Create(context.Background(), AuthConnectionCreateInput{
		Domain:      "example.com",
		ProfileName: "prof",
		Telemetry:   "console,network",
	})
	require.NoError(t, err)

	tel := captured.ManagedAuthCreateRequest.BrowserTelemetry
	assert.False(t, tel.Enabled.Valid())
	assert.True(t, tel.Browser.Console.Enabled.Value)
	assert.True(t, tel.Browser.Network.Enabled.Value)
	assert.False(t, tel.Browser.Page.Enabled.Valid())
}

func TestCreate_TelemetryOff(t *testing.T) {
	capturePtermOutput(t)
	var captured kernel.AuthConnectionNewParams
	fake := &FakeAuthConnectionService{
		NewFunc: func(ctx context.Context, body kernel.AuthConnectionNewParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{ID: "auth_1"}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	require.NoError(t, c.Create(context.Background(), AuthConnectionCreateInput{
		Domain: "example.com", ProfileName: "prof", Telemetry: "off",
	}))

	tel := captured.ManagedAuthCreateRequest.BrowserTelemetry
	require.True(t, tel.Enabled.Valid())
	assert.False(t, tel.Enabled.Value)
}

func TestCreate_TelemetryUnknownCategoryErrors(t *testing.T) {
	capturePtermOutput(t)
	c := AuthConnectionCmd{svc: &FakeAuthConnectionService{}}
	err := c.Create(context.Background(), AuthConnectionCreateInput{
		Domain: "example.com", ProfileName: "prof", Telemetry: "bogus",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
}

func TestUpdate_TelemetryCountsAsChange(t *testing.T) {
	capturePtermOutput(t)
	var captured kernel.AuthConnectionUpdateParams
	fake := &FakeAuthConnectionService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.AuthConnectionUpdateParams, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			captured = body
			return &kernel.ManagedAuth{ID: id}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	// --telemetry alone must satisfy the "at least one field" guard.
	require.NoError(t, c.Update(context.Background(), AuthConnectionUpdateInput{ID: "auth_1", Telemetry: "all"}))

	tel := captured.ManagedAuthUpdateRequest.BrowserTelemetry
	require.True(t, tel.Enabled.Valid())
	assert.True(t, tel.Enabled.Value)
}

func TestLogin_TelemetryOverride(t *testing.T) {
	capturePtermOutput(t)
	var captured kernel.AuthConnectionLoginParams
	fake := &FakeAuthConnectionService{
		LoginFunc: func(ctx context.Context, id string, body kernel.AuthConnectionLoginParams, opts ...option.RequestOption) (*kernel.LoginResponse, error) {
			captured = body
			return &kernel.LoginResponse{ID: id}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	require.NoError(t, c.Login(context.Background(), AuthConnectionLoginInput{ID: "auth_1", Telemetry: "screenshot"}))
	assert.True(t, captured.BrowserTelemetry.Browser.Screenshot.Enabled.Value)
}

func TestSubmit_CanonicalChoiceID(t *testing.T) {
	capturePtermOutput(t)
	var captured kernel.AuthConnectionSubmitParams
	fake := &FakeAuthConnectionService{
		SubmitFunc: func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
			captured = body
			return &kernel.SubmitFieldsResponse{Accepted: true}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	require.NoError(t, c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:               "auth_1",
		SelectedChoiceID: "choice_sms",
	}))
	require.True(t, captured.SubmitFieldsRequest.SelectedChoiceID.Valid())
	assert.Equal(t, "choice_sms", captured.SubmitFieldsRequest.SelectedChoiceID.Value)
	// Legacy fields must stay absent so the API sees exactly one submit mode.
	assert.Nil(t, captured.SubmitFieldsRequest.Fields)
}

func TestSubmit_CanonicalFieldValues(t *testing.T) {
	capturePtermOutput(t)
	var captured kernel.AuthConnectionSubmitParams
	fake := &FakeAuthConnectionService{
		SubmitFunc: func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error) {
			captured = body
			return &kernel.SubmitFieldsResponse{Accepted: true}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	require.NoError(t, c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:                   "auth_1",
		CanonicalFieldValues: map[string]string{"field_email": "me@example.com"},
	}))
	assert.Equal(t, map[string]string{"field_email": "me@example.com"}, captured.SubmitFieldsRequest.FieldValues)
	assert.Nil(t, captured.SubmitFieldsRequest.Fields)
}

func TestSubmit_CanonicalAndLegacyAreMutuallyExclusive(t *testing.T) {
	capturePtermOutput(t)
	c := AuthConnectionCmd{svc: &FakeAuthConnectionService{}}
	err := c.Submit(context.Background(), AuthConnectionSubmitInput{
		ID:                   "auth_1",
		FieldValues:          map[string]string{"email": "a@b.com"},
		CanonicalFieldValues: map[string]string{"field_email": "a@b.com"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provide exactly one of")
}

func TestTimeline_RendersEventsAndPagination(t *testing.T) {
	buf := capturePtermOutput(t)
	var captured kernel.AuthConnectionTimelineParams
	// Decoded from JSON so the telemetry_captured field registers as present;
	// the CLI distinguishes "reported false" from "not reported at all".
	var loginEvent kernel.ManagedAuthTimelineEvent
	require.NoError(t, json.Unmarshal([]byte(`{
		"id": "e1",
		"type": "login",
		"status": "SUCCESS",
		"browser_session_id": "browser_1",
		"telemetry_captured": true
	}`), &loginEvent))
	fake := &FakeAuthConnectionService{
		TimelineFunc: func(ctx context.Context, id string, query kernel.AuthConnectionTimelineParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent], error) {
			captured = query
			// Return perPage+1 events so the CLI reports another page exists.
			return &pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent]{
				Items: []kernel.ManagedAuthTimelineEvent{
					loginEvent,
					{ID: "e2", Type: "reauth", Status: "FAILED", ErrorMessage: "boom"},
					{ID: "e3", Type: "health_check", Status: "SUCCESS"},
				},
			}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	require.NoError(t, c.Timeline(context.Background(), AuthConnectionTimelineInput{ID: "auth_1", Page: 1, PerPage: 2}))

	// The +1 trick: request one more than a page to detect hasMore.
	require.True(t, captured.Limit.Valid())
	assert.Equal(t, int64(3), captured.Limit.Value)
	assert.Equal(t, int64(0), captured.Offset.Value)

	out := buf.String()
	assert.Contains(t, out, "browser_1")
	assert.Contains(t, out, "boom")
	// Telemetry capture is reported for events that have a browser session.
	assert.Contains(t, out, "Telemetry")
	assert.Regexp(t, `browser_1.*yes`, out)
	// The third event is truncated off the page.
	assert.NotContains(t, out, "health_check")
	assert.Contains(t, out, "Has more: yes")
	assert.Contains(t, out, "kernel auth connections timeline auth_1 --page 2 --per-page 2")
}

func TestTimeline_TypeFilterValidated(t *testing.T) {
	capturePtermOutput(t)
	c := AuthConnectionCmd{svc: &FakeAuthConnectionService{}}
	err := c.Timeline(context.Background(), AuthConnectionTimelineInput{ID: "auth_1", Type: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --type")

	var captured kernel.AuthConnectionTimelineParams
	fake := &FakeAuthConnectionService{
		TimelineFunc: func(ctx context.Context, id string, query kernel.AuthConnectionTimelineParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent], error) {
			captured = query
			return &pagination.OffsetPagination[kernel.ManagedAuthTimelineEvent]{}, nil
		},
	}
	c = AuthConnectionCmd{svc: fake}
	require.NoError(t, c.Timeline(context.Background(), AuthConnectionTimelineInput{ID: "auth_1", Type: "health_check"}))
	assert.Equal(t, kernel.AuthConnectionTimelineParamsTypeHealthCheck, captured.Type)
}

func TestGet_ShowsCanonicalFieldsAndChoices(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuthConnectionService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			return &kernel.ManagedAuth{
				ID:     id,
				Domain: "example.com",
				Fields: []kernel.ManagedAuthField{
					{ID: "field_email", Ref: "email", Type: "identifier", Label: "Email", Required: true},
				},
				Choices: []kernel.ManagedAuthChoice{
					{ID: "choice_sms", Label: "Text me", Type: "mfa_method"},
				},
			}, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}
	require.NoError(t, c.Get(context.Background(), AuthConnectionGetInput{ID: "auth_1"}))

	out := buf.String()
	assert.Contains(t, out, "field_email")
	assert.Contains(t, out, "ref=email")
	assert.Contains(t, out, "choice_sms")
}

func TestAuthConnectionsGet_TelemetryEnabledWithoutCategories(t *testing.T) {
	setupStdoutCapture(t)
	// The API preserves the create-browser config verbatim, so an enabled
	// connection can come back as {"enabled": true} with no category object.
	// Rendering that as "disabled" would invert the connection's real state.
	fake := &FakeAuthConnectionService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			var auth kernel.ManagedAuth
			require.NoError(t, json.Unmarshal([]byte(`{"id":"conn-1","browser_telemetry":{"enabled":true}}`), &auth))
			return &auth, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}

	require.NoError(t, c.Get(context.Background(), AuthConnectionGetInput{ID: "conn-1"}))
	assert.Contains(t, outBuf.String(), "enabled (default categories)")
}

func TestAuthConnectionsGet_TelemetryRowOmittedWhenOff(t *testing.T) {
	setupStdoutCapture(t)
	// Telemetry that is off is not reported at all, rather than shown as a
	// "disabled" row alongside the connection's real settings.
	fake := &FakeAuthConnectionService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ManagedAuth, error) {
			var auth kernel.ManagedAuth
			require.NoError(t, json.Unmarshal([]byte(`{"id":"conn-1","browser_telemetry":{"enabled":false}}`), &auth))
			return &auth, nil
		},
	}
	c := AuthConnectionCmd{svc: fake}

	require.NoError(t, c.Get(context.Background(), AuthConnectionGetInput{ID: "conn-1"}))
	assert.NotContains(t, outBuf.String(), "Browser Telemetry")
}
