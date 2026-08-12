package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeBrowserPoolsService is a configurable fake implementing BrowserPoolsService.
type FakeBrowserPoolsService struct {
	AcquireFunc func(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error)
	ListFunc    func(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserPool], error)
	NewFunc     func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error)
	UpdateFunc  func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error)
}

func (f *FakeBrowserPoolsService) List(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserPool], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.BrowserPool]{Items: []kernel.BrowserPool{}}, nil
}

func (f *FakeBrowserPoolsService) New(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.BrowserPool{}, nil
}

func (f *FakeBrowserPoolsService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	return &kernel.BrowserPool{}, nil
}

func (f *FakeBrowserPoolsService) Update(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserPool{}, nil
}

func (f *FakeBrowserPoolsService) Delete(ctx context.Context, id string, body kernel.BrowserPoolDeleteParams, opts ...option.RequestOption) error {
	return nil
}

func (f *FakeBrowserPoolsService) Acquire(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error) {
	if f.AcquireFunc != nil {
		return f.AcquireFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserPoolAcquireResponse{}, nil
}

func (f *FakeBrowserPoolsService) Release(ctx context.Context, id string, body kernel.BrowserPoolReleaseParams, opts ...option.RequestOption) error {
	return nil
}

func (f *FakeBrowserPoolsService) Flush(ctx context.Context, id string, opts ...option.RequestOption) error {
	return nil
}

func TestBrowserPoolsAcquire_WithNameAndTags(t *testing.T) {
	setupStdoutCapture(t)

	var capturedID string
	var captured kernel.BrowserPoolAcquireParams
	fake := &FakeBrowserPoolsService{
		AcquireFunc: func(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error) {
			capturedID = id
			captured = body
			return &kernel.BrowserPoolAcquireResponse{
				SessionID: "sess-acq",
				CdpWsURL:  "ws://cdp-acq",
				Name:      "lease-name",
				Tags:      kernel.Tags{"env": "prod"},
			}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Acquire(context.Background(), BrowserPoolsAcquireInput{
		IDOrName: "my-pool",
		Name:     "lease-name",
		Tags:     map[string]string{"env": "prod"},
	})
	assert.NoError(t, err)

	// Pool lookup is by id or name; name + tags are forwarded per-lease.
	assert.Equal(t, "my-pool", capturedID)
	assert.True(t, captured.Name.Valid())
	assert.Equal(t, "lease-name", captured.Name.Value)
	assert.Equal(t, "prod", captured.Tags["env"])

	// And surfaced in the acquired-session table.
	out := outBuf.String()
	assert.Contains(t, out, "lease-name")
	assert.Contains(t, out, "Tags")
	assert.Contains(t, out, "env=prod")
}

func TestBrowserPoolsList_ForwardsLimitOffset(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolListParams
	fake := &FakeBrowserPoolsService{
		ListFunc: func(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserPool], error) {
			captured = query
			return &pagination.OffsetPagination[kernel.BrowserPool]{Items: []kernel.BrowserPool{}}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.List(context.Background(), BrowserPoolsListInput{Limit: 4, Offset: 8})

	assert.NoError(t, err)
	assert.Equal(t, int64(4), captured.Limit.Value)
	assert.Equal(t, int64(8), captured.Offset.Value)
}

func TestBrowserPoolsList_ForwardsRegion(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolListParams
	fake := &FakeBrowserPoolsService{
		ListFunc: func(ctx context.Context, query kernel.BrowserPoolListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserPool], error) {
			captured = query
			return &pagination.OffsetPagination[kernel.BrowserPool]{Items: []kernel.BrowserPool{}}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.List(context.Background(), BrowserPoolsListInput{Region: "eu-west"})

	assert.NoError(t, err)
	assert.Equal(t, kernel.BrowserPoolListParamsRegionEuWest, captured.Region)

	// An unknown region is rejected before the request is made.
	err = c.List(context.Background(), BrowserPoolsListInput{Region: "emea"})
	assert.Error(t, err)
}

func TestBrowserPoolsCreate_WithRegion(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-1", Region: kernel.BrowserPoolRegionEuWest}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	require.NoError(t, c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, Region: "eu-west"}))
	assert.Equal(t, kernel.BrowserPoolNewParamsRegionEuWest, captured.Region)

	// Omitting the flag leaves the region unset so the API default applies.
	require.NoError(t, c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1}))
	assert.Empty(t, string(captured.Region))

	assert.Error(t, c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, Region: "emea"}))
}

func TestBrowserPoolsCreate_PrivateHostNormalization(t *testing.T) {
	setupStdoutCapture(t)

	var gotJSON []byte
	var marshalErr error
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			gotJSON, marshalErr = json.Marshal(body)
			return &kernel.BrowserPool{ID: "pool-1"}, nil
		},
	}
	c := BrowserPoolsCmd{client: fake}

	// Blank entries from a trailing comma are dropped before the request.
	require.NoError(t, c.Create(context.Background(), BrowserPoolsCreateInput{
		Size:         1,
		PrivateHosts: []string{"*.example.ts.net", " "},
	}))
	require.NoError(t, marshalErr)
	assert.Contains(t, string(gotJSON), `"network":{"private_hosts":["*.example.ts.net"]}`)

	// Omitting the flag leaves network out of the request entirely, so the API
	// keeps its default private ranges.
	require.NoError(t, c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1}))
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(gotJSON), "network")

	// The API's 32-entry cap is enforced client-side.
	tooMany := make([]string, maxPrivateHosts+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("host-%d.internal", i)
	}
	assert.Error(t, c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, PrivateHosts: tooMany}))
}

// TestBuildAcquireParams covers the shared name/tags/timeout/telemetry/start-url
// forwarding used by both `browser-pools acquire` and the `browsers create
// --pool-id` lease path.
func TestBuildAcquireParams(t *testing.T) {
	p, err := buildAcquireParams("lease", map[string]string{"env": "prod"}, 30, "console,network", "https://example.com")
	assert.NoError(t, err)
	assert.True(t, p.Name.Valid())
	assert.Equal(t, "lease", p.Name.Value)
	assert.Equal(t, "prod", p.Tags["env"])
	assert.True(t, p.AcquireTimeoutSeconds.Valid())
	assert.Equal(t, int64(30), p.AcquireTimeoutSeconds.Value)
	assert.True(t, p.StartURL.Valid())
	assert.Equal(t, "https://example.com", p.StartURL.Value)
	assert.True(t, p.Telemetry.Browser.Console.Enabled.Value)
	assert.True(t, p.Telemetry.Browser.Network.Enabled.Value)

	// Unset inputs produce an empty params struct (nothing forwarded).
	empty, err := buildAcquireParams("", nil, 0, "", "")
	assert.NoError(t, err)
	assert.False(t, empty.Name.Valid())
	assert.Len(t, empty.Tags, 0)
	assert.False(t, empty.AcquireTimeoutSeconds.Valid())
	assert.False(t, empty.StartURL.Valid())

	// An invalid category surfaces an error rather than a partial param.
	_, err = buildAcquireParams("", nil, 0, "bogus", "")
	assert.Error(t, err)
}

func TestBrowserPoolsCreate_WithRefreshOnProfileUpdate(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-ropu"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{
		Size:                   1,
		RefreshOnProfileUpdate: BoolFlag{Set: true, Value: true},
	})
	assert.NoError(t, err)
	assert.True(t, captured.RefreshOnProfileUpdate.Valid())
	assert.True(t, captured.RefreshOnProfileUpdate.Value)
}

func TestBrowserPoolsUpdate_WithRefreshOnProfileUpdate(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolUpdateParams
	fake := &FakeBrowserPoolsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: id}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Update(context.Background(), BrowserPoolsUpdateInput{
		IDOrName:               "pool-1",
		RefreshOnProfileUpdate: BoolFlag{Set: true, Value: false},
	})
	assert.NoError(t, err)
	assert.True(t, captured.RefreshOnProfileUpdate.Valid())
	assert.False(t, captured.RefreshOnProfileUpdate.Value)
}

func TestBrowserPoolsCreate_WithChromePolicy(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-cp"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{
		Size:         1,
		ChromePolicy: `{"BookmarkBarEnabled": false}`,
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"BookmarkBarEnabled": false}, captured.ChromePolicy)
}

func TestBrowserPoolsCreate_ChromePolicyEmptyObjectOmitted(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-cp"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, ChromePolicy: "{}"})
	assert.NoError(t, err)
	assert.Nil(t, captured.ChromePolicy)
}

func TestBrowserPoolsUpdate_WithChromePolicy(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolUpdateParams
	fake := &FakeBrowserPoolsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: id}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Update(context.Background(), BrowserPoolsUpdateInput{
		IDOrName:     "pool-1",
		ChromePolicy: `{"BookmarkBarEnabled": false}`,
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"BookmarkBarEnabled": false}, captured.ChromePolicy)
}

func TestBrowserPoolsUpdate_DurableClearAndZeroStates(t *testing.T) {
	policyDir := t.TempDir()
	emptyObjectPolicyFile := filepath.Join(policyDir, "empty-object.json")
	require.NoError(t, os.WriteFile(emptyObjectPolicyFile, []byte(`{}`), 0o600))
	blankPolicyFile := filepath.Join(policyDir, "blank.json")
	require.NoError(t, os.WriteFile(blankPolicyFile, []byte("\n"), 0o600))

	tests := []struct {
		name     string
		input    BrowserPoolsUpdateInput
		wantJSON string
	}{
		{
			name:     "durable fields omitted",
			input:    BrowserPoolsUpdateInput{},
			wantJSON: `{}`,
		},
		{
			name: "fill rate zero",
			input: BrowserPoolsUpdateInput{
				FillRate: Int64Flag{Set: true, Value: 0},
			},
			wantJSON: `{"fill_rate_per_minute":0}`,
		},
		{
			name: "clear proxy",
			input: BrowserPoolsUpdateInput{
				ClearProxy: true,
			},
			wantJSON: `{"proxy_id":""}`,
		},
		{
			name: "clear profile",
			input: BrowserPoolsUpdateInput{
				ClearProfile: true,
			},
			wantJSON: `{"profile":{"id":""}}`,
		},
		{
			name: "clear start URL",
			input: BrowserPoolsUpdateInput{
				ClearStartURL: true,
			},
			wantJSON: `{"start_url":""}`,
		},
		{
			name: "clear extensions",
			input: BrowserPoolsUpdateInput{
				ClearExtensions: true,
			},
			wantJSON: `{"extensions":[]}`,
		},
		{
			name: "clear Chrome policy",
			input: BrowserPoolsUpdateInput{
				ClearChromePolicy: true,
			},
			wantJSON: `{"chrome_policy":{}}`,
		},
		{
			name: "empty inline Chrome policy",
			input: BrowserPoolsUpdateInput{
				ChromePolicy: `{}`,
			},
			wantJSON: `{"chrome_policy":{}}`,
		},
		{
			name: "empty object Chrome policy file",
			input: BrowserPoolsUpdateInput{
				ChromePolicyFile: emptyObjectPolicyFile,
			},
			wantJSON: `{"chrome_policy":{}}`,
		},
		{
			name: "blank Chrome policy file",
			input: BrowserPoolsUpdateInput{
				ChromePolicyFile: blankPolicyFile,
			},
			wantJSON: `{}`,
		},
		{
			name: "replace private hosts",
			input: BrowserPoolsUpdateInput{
				PrivateHosts: []string{" preview.internal ", "", "10.0.0.0/8"},
			},
			wantJSON: `{"network":{"private_hosts":["preview.internal","10.0.0.0/8"]}}`,
		},
		{
			// An empty object removes the network configuration entirely, so the
			// default private ranges come back.
			name: "clear private hosts",
			input: BrowserPoolsUpdateInput{
				ClearPrivateHosts: true,
			},
			wantJSON: `{"network":{}}`,
		},
		{
			name: "all durable clear states",
			input: BrowserPoolsUpdateInput{
				FillRate:          Int64Flag{Set: true, Value: 0},
				ClearProfile:      true,
				ClearProxy:        true,
				ClearStartURL:     true,
				ClearExtensions:   true,
				ClearChromePolicy: true,
				ClearPrivateHosts: true,
			},
			wantJSON: `{"fill_rate_per_minute":0,"profile":{"id":""},"proxy_id":"","start_url":"","extensions":[],"chrome_policy":{},"network":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupStdoutCapture(t)

			var gotJSON []byte
			var marshalErr error
			fake := &FakeBrowserPoolsService{
				UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
					gotJSON, marshalErr = json.Marshal(body)
					return &kernel.BrowserPool{ID: id}, nil
				},
			}

			tt.input.IDOrName = "pool-1"
			err := (BrowserPoolsCmd{client: fake}).Update(context.Background(), tt.input)
			require.NoError(t, err)
			require.NoError(t, marshalErr)
			assert.JSONEq(t, tt.wantJSON, string(gotJSON))
		})
	}
}

func TestBrowserPoolsUpdate_RejectsInvalidDurableInputs(t *testing.T) {
	tests := []struct {
		name    string
		input   BrowserPoolsUpdateInput
		wantErr string
	}{
		{
			name:    "conflicting proxy flags",
			input:   BrowserPoolsUpdateInput{ProxyID: "proxy-1", ClearProxy: true},
			wantErr: "cannot specify both --proxy-id and --clear-proxy",
		},
		{
			name:    "conflicting start URL flags",
			input:   BrowserPoolsUpdateInput{StartURL: "https://example.com", ClearStartURL: true},
			wantErr: "cannot specify both --start-url and --clear-start-url",
		},
		{
			name:    "conflicting profile flags",
			input:   BrowserPoolsUpdateInput{ProfileID: "profile-1", ClearProfile: true},
			wantErr: "cannot specify --clear-profile with --profile-id or --profile-name",
		},
		{
			name:    "conflicting extension flags",
			input:   BrowserPoolsUpdateInput{Extensions: []string{"extension-1"}, ClearExtensions: true},
			wantErr: "cannot specify both --extension and --clear-extensions",
		},
		{
			name:    "conflicting Chrome policy flags",
			input:   BrowserPoolsUpdateInput{ChromePolicy: `{}`, ClearChromePolicy: true},
			wantErr: "cannot specify --clear-chrome-policy with --chrome-policy or --chrome-policy-file",
		},
		{
			name:    "negative fill rate",
			input:   BrowserPoolsUpdateInput{FillRate: Int64Flag{Set: true, Value: -1}},
			wantErr: "--fill-rate must be zero or greater",
		},
		{
			name:    "conflicting private host modes",
			input:   BrowserPoolsUpdateInput{PrivateHosts: []string{"internal.example"}, ClearPrivateHosts: true},
			wantErr: "cannot specify both --private-host and --clear-private-hosts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &FakeBrowserPoolsService{
				UpdateFunc: func(context.Context, string, kernel.BrowserPoolUpdateParams, ...option.RequestOption) (*kernel.BrowserPool, error) {
					t.Fatal("Update should not be called for invalid input")
					return nil, nil
				},
			}

			tt.input.IDOrName = "pool-1"
			err := (BrowserPoolsCmd{client: fake}).Update(context.Background(), tt.input)
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestBrowserPoolsCreate_WithPrivateHosts(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-network"}, nil
		},
	}

	err := (BrowserPoolsCmd{client: fake}).Create(context.Background(), BrowserPoolsCreateInput{
		Size:         1,
		PrivateHosts: []string{"*.example.ts.net", "100.64.0.0/10"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"*.example.ts.net", "100.64.0.0/10"}, captured.Network.PrivateHosts)
}

func TestBrowserPoolsUpdate_PrivateHostModes(t *testing.T) {
	tests := []struct {
		name     string
		input    BrowserPoolsUpdateInput
		wantJSON string
	}{
		{
			name:     "replace",
			input:    BrowserPoolsUpdateInput{PrivateHosts: []string{"*.example.ts.net"}},
			wantJSON: `"network":{"private_hosts":["*.example.ts.net"]}`,
		},
		{
			name:     "restore defaults",
			input:    BrowserPoolsUpdateInput{ClearPrivateHosts: true},
			wantJSON: `"network":{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupStdoutCapture(t)
			var captured kernel.BrowserPoolUpdateParams
			fake := &FakeBrowserPoolsService{
				UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
					captured = body
					return &kernel.BrowserPool{ID: id}, nil
				},
			}
			tt.input.IDOrName = "pool-network"
			require.NoError(t, (BrowserPoolsCmd{client: fake}).Update(context.Background(), tt.input))
			raw, err := captured.MarshalJSON()
			require.NoError(t, err)
			assert.Contains(t, string(raw), tt.wantJSON)
		})
	}
}

func TestBrowserPoolsCreate_WithTelemetry(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-tel"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{
		Size:      1,
		Telemetry: "console,network",
	})
	assert.NoError(t, err)
	assert.True(t, captured.Telemetry.Browser.Console.Enabled.Value)
	assert.True(t, captured.Telemetry.Browser.Network.Enabled.Value)
	assert.False(t, captured.Telemetry.Enabled.Valid())
}

func TestBrowserPoolsCreate_TelemetryAllAndOff(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolNewParams
	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: "pool-tel"}, nil
		},
	}
	c := BrowserPoolsCmd{client: fake}

	assert.NoError(t, c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, Telemetry: "all"}))
	assert.True(t, captured.Telemetry.Enabled.Value)

	assert.NoError(t, c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, Telemetry: "off"}))
	assert.True(t, captured.Telemetry.Enabled.Valid())
	assert.False(t, captured.Telemetry.Enabled.Value)
}

func TestBrowserPoolsCreate_TelemetryInvalidCategory(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowserPoolsService{
		NewFunc: func(ctx context.Context, body kernel.BrowserPoolNewParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			t.Fatal("New should not be called when telemetry parsing fails")
			return nil, nil
		},
	}
	c := BrowserPoolsCmd{client: fake}
	err := c.Create(context.Background(), BrowserPoolsCreateInput{Size: 1, Telemetry: "bogus"})
	assert.Error(t, err)
}

func TestBrowserPoolsUpdate_WithTelemetry(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolUpdateParams
	fake := &FakeBrowserPoolsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.BrowserPoolUpdateParams, opts ...option.RequestOption) (*kernel.BrowserPool, error) {
			captured = body
			return &kernel.BrowserPool{ID: id}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Update(context.Background(), BrowserPoolsUpdateInput{
		IDOrName:  "pool-1",
		Telemetry: "off",
	})
	assert.NoError(t, err)
	assert.True(t, captured.Telemetry.Enabled.Valid())
	assert.False(t, captured.Telemetry.Enabled.Value)
}

func TestBrowserPoolsAcquire_WithTelemetryOverride(t *testing.T) {
	setupStdoutCapture(t)

	var captured kernel.BrowserPoolAcquireParams
	fake := &FakeBrowserPoolsService{
		AcquireFunc: func(ctx context.Context, id string, body kernel.BrowserPoolAcquireParams, opts ...option.RequestOption) (*kernel.BrowserPoolAcquireResponse, error) {
			captured = body
			return &kernel.BrowserPoolAcquireResponse{SessionID: "sess-1"}, nil
		},
	}

	c := BrowserPoolsCmd{client: fake}
	err := c.Acquire(context.Background(), BrowserPoolsAcquireInput{
		IDOrName:  "pool-1",
		Telemetry: "page",
	})
	assert.NoError(t, err)
	assert.True(t, captured.Telemetry.Browser.Page.Enabled.Value)
}
