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
	ListFunc            func(ctx context.Context, query kernel.AuthConnectionListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.ManagedAuth], error)
	DeleteFunc          func(ctx context.Context, id string, opts ...option.RequestOption) error
	LoginFunc           func(ctx context.Context, id string, body kernel.AuthConnectionLoginParams, opts ...option.RequestOption) (*kernel.LoginResponse, error)
	SubmitFunc          func(ctx context.Context, id string, body kernel.AuthConnectionSubmitParams, opts ...option.RequestOption) (*kernel.SubmitFieldsResponse, error)
	FollowStreamingFunc func(ctx context.Context, id string, opts ...option.RequestOption) *ssestream.Stream[kernel.AuthConnectionFollowResponseUnion]
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
