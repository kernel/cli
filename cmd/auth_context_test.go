package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
)

type FakeAuthContextService struct {
	GetFunc func(ctx context.Context, opts ...option.RequestOption) (*kernel.AuthContext, error)
}

func (f *FakeAuthContextService) Get(ctx context.Context, opts ...option.RequestOption) (*kernel.AuthContext, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, opts...)
	}
	return &kernel.AuthContext{}, nil
}

// sampleAuthContext unmarshals from raw JSON so RawJSON() is populated, which
// the --output json path relies on.
func sampleAuthContext(t *testing.T, projectID string) *kernel.AuthContext {
	t.Helper()
	scope := "null"
	if projectID != "" {
		scope = `"` + projectID + `"`
	}
	raw := `{
		"principal": {"id": "key_123", "type": "api_key"},
		"organization": {"id": "org_456"},
		"authentication": {"credential_id": "key_123", "method": "api_key", "source": "api_key"},
		"authorization": {
			"credential_scope": {"project_id": ` + scope + `},
			"effective_scope": {"project_id": ` + scope + `}
		}
	}`
	authCtx := &kernel.AuthContext{}
	if err := authCtx.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	return authCtx
}

func TestAuthContextGet_RendersContext(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuthContextService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.AuthContext, error) {
			return sampleAuthContext(t, "proj_789"), nil
		},
	}
	c := AuthContextCmd{svc: fake}
	assert.NoError(t, c.Get(context.Background(), AuthContextGetInput{}))

	out := buf.String()
	assert.Contains(t, out, "Principal ID")
	assert.Contains(t, out, "key_123")
	assert.Contains(t, out, "org_456")
	assert.Contains(t, out, "proj_789")
}

func TestAuthContextGet_NullFieldsRenderPlaceholders(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeAuthContextService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.AuthContext, error) {
			// An org-wide credential has a null project_id in both scopes.
			return sampleAuthContext(t, ""), nil
		},
	}
	c := AuthContextCmd{svc: fake}
	assert.NoError(t, c.Get(context.Background(), AuthContextGetInput{}))
	assert.Contains(t, buf.String(), "organization-wide")
}

func TestAuthContextGet_JSONOutput(t *testing.T) {
	fake := &FakeAuthContextService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.AuthContext, error) {
			return sampleAuthContext(t, "proj_789"), nil
		},
	}
	c := AuthContextCmd{svc: fake}
	out := captureStdout(t, func() {
		assert.NoError(t, c.Get(context.Background(), AuthContextGetInput{Output: "json"}))
	})
	assert.Contains(t, out, "org_456")
}

func TestAuthContextGet_SurfacesAPIError(t *testing.T) {
	capturePtermOutput(t)
	fake := &FakeAuthContextService{
		GetFunc: func(ctx context.Context, opts ...option.RequestOption) (*kernel.AuthContext, error) {
			return nil, errors.New("boom")
		},
	}
	c := AuthContextCmd{svc: fake}
	assert.Error(t, c.Get(context.Background(), AuthContextGetInput{}))
}
