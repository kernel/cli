package proxies

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kernel/cli/pkg/interactive"
	"github.com/kernel/cli/pkg/util"
	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyDelete_SkipConfirm_Success(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			assert.Equal(t, "proxy-1", id)
			return nil
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Delete(context.Background(), ProxyDeleteInput{
		ID:          "proxy-1",
		SkipConfirm: true,
	})

	assert.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "Deleting proxy: proxy-1")
	assert.Contains(t, output, "Successfully deleted proxy: proxy-1")
}

func TestProxyDelete_SkipConfirm_NotFound(t *testing.T) {
	buf := captureOutput(t)

	fake := &FakeProxyService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			return &kernel.Error{StatusCode: http.StatusNotFound}
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Delete(context.Background(), ProxyDeleteInput{
		ID:          "not-found",
		SkipConfirm: true,
	})

	assert.Error(t, err)
	assert.NotContains(t, buf.String(), "Successfully deleted")
}

func TestProxyDeleteAPIErrors(t *testing.T) {
	for _, skip := range []bool{false, true} {
		for _, response := range []struct {
			status int
			code   string
		}{
			{404, "project_not_found"}, {404, "not_found"}, {403, "forbidden"}, {409, "conflict"}, {500, "internal_error"},
		} {
			t.Run(fmt.Sprintf("skip=%t/%s", skip, response.code), func(t *testing.T) {
				buf := captureOutput(t)
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, "selected-project", r.Header.Get("X-Kernel-Project"))
					if skip {
						assert.Equal(t, http.MethodDelete, r.Method)
					} else {
						assert.Equal(t, http.MethodGet, r.Method)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(response.status)
					_, _ = io.WriteString(w, fmt.Sprintf(`{"code":%q,"message":"API error details"}`, response.code))
				}))
				defer server.Close()
				client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithProject("selected-project"), option.WithMaxRetries(0))
				p := ProxyCmd{proxies: &client.Proxies, prompter: interactive.NewPrompterWithTerminal(true)}
				err := p.Delete(context.Background(), ProxyDeleteInput{ID: "resource", SkipConfirm: skip})
				var apiErr *kernel.Error
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, response.status, apiErr.StatusCode)
				assert.Equal(t, response.code+": API error details", util.CleanedUpSdkError{Err: err}.Error())
				assert.NotContains(t, buf.String(), "Successfully deleted")
				assert.Equal(t, 1, calls)
			})
		}
	}
}

func TestProxyDelete_SkipConfirm_APIError(t *testing.T) {
	_ = captureOutput(t)

	fake := &FakeProxyService{
		DeleteFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			return errors.New("API error")
		},
	}

	p := ProxyCmd{proxies: fake}
	err := p.Delete(context.Background(), ProxyDeleteInput{
		ID:          "proxy-1",
		SkipConfirm: true,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}
