package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteAPIErrors(t *testing.T) {
	commands := []struct {
		name   string
		lookup bool
		run    func(context.Context, kernel.Client) error
	}{
		{"browsers", false, func(ctx context.Context, client kernel.Client) error {
			return (BrowsersCmd{browsers: &client.Browsers}).Delete(ctx, BrowsersDeleteInput{Identifier: "resource"})
		}},
		{"profiles lookup", true, func(ctx context.Context, client kernel.Client) error {
			return (ProfilesCmd{profiles: &client.Profiles}).Delete(ctx, ProfilesDeleteInput{Identifier: "resource", SkipConfirm: true})
		}},
		{"profiles delete", false, func(ctx context.Context, client kernel.Client) error {
			return (ProfilesCmd{profiles: &client.Profiles}).Delete(ctx, ProfilesDeleteInput{Identifier: "resource", SkipConfirm: true})
		}},
		{"extensions", false, func(ctx context.Context, client kernel.Client) error {
			return (ExtensionsCmd{extensions: &client.Extensions}).Delete(ctx, ExtensionsDeleteInput{Identifier: "resource", SkipConfirm: true})
		}},
		{"credentials", false, func(ctx context.Context, client kernel.Client) error {
			return (CredentialsCmd{credentials: &client.Credentials}).Delete(ctx, CredentialsDeleteInput{Identifier: "resource", SkipConfirm: true})
		}},
		{"credential providers", false, func(ctx context.Context, client kernel.Client) error {
			return (CredentialProvidersCmd{providers: &client.CredentialProviders}).Delete(ctx, CredentialProvidersDeleteInput{ID: "resource", SkipConfirm: true})
		}},
		{"managed auth", false, func(ctx context.Context, client kernel.Client) error {
			return (AuthConnectionCmd{svc: &client.Auth.Connections}).Delete(ctx, AuthConnectionDeleteInput{ID: "resource", SkipConfirm: true})
		}},
		{"telemetry destinations", false, func(ctx context.Context, client kernel.Client) error {
			return (TelemetryDestinationsCmd{destinations: &client.Telemetry.Destinations}).Delete(ctx, TelemetryDestinationsDeleteInput{Identifier: "resource", SkipConfirm: true})
		}},
		{"api keys", false, func(ctx context.Context, client kernel.Client) error {
			return (APIKeysCmd{apiKeys: &client.APIKeys}).Delete(ctx, APIKeysDeleteInput{ID: "resource", SkipConfirm: true})
		}},
	}
	responses := []struct {
		status        int
		body, message string
	}{
		{404, `{"code":"project_not_found","message":"Project not found or inactive"}`, "project_not_found: Project not found or inactive"},
		{404, `{"code":"not_found","message":"Resource not found"}`, "not_found: Resource not found"},
		{403, `{"code":"forbidden","message":"Access denied"}`, "forbidden: Access denied"},
		{409, `{"code":"conflict","message":"Resource is still in use"}`, "conflict: Resource is still in use"},
		{500, `{"code":"internal_error","message":"Deletion failed"}`, "internal_error: Deletion failed"},
		{204, "", ""},
	}
	for _, command := range commands {
		for _, response := range responses {
			if command.lookup && response.status == http.StatusNoContent {
				continue
			}
			t.Run(fmt.Sprintf("%s/%d/%s", command.name, response.status, response.message), func(t *testing.T) {
				buf := capturePtermOutput(t)
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, "selected-project", r.Header.Get("X-Kernel-Project"))
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet && !command.lookup {
						assert.Equal(t, "profiles delete", command.name)
						_, _ = io.WriteString(w, `{"id":"resource","name":"resource"}`)
						return
					}
					if command.lookup {
						assert.Equal(t, http.MethodGet, r.Method)
					} else {
						assert.Equal(t, http.MethodDelete, r.Method)
					}
					w.WriteHeader(response.status)
					_, _ = io.WriteString(w, response.body)
				}))
				defer server.Close()
				client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithProject("selected-project"), option.WithMaxRetries(0))
				err := command.run(context.Background(), client)
				if response.status == http.StatusNoContent {
					require.NoError(t, err)
					assert.Regexp(t, "[Dd]eleted", buf.String())
				} else {
					var apiErr *kernel.Error
					require.ErrorAs(t, err, &apiErr)
					assert.Equal(t, response.status, apiErr.StatusCode)
					assert.Equal(t, response.message, util.CleanedUpSdkError{Err: err}.Error())
					assert.Empty(t, buf.String())
				}
				expectedCalls := 1
				if command.name == "profiles delete" {
					expectedCalls = 2
				}
				assert.Equal(t, expectedCalls, calls)
			})
		}
	}
}
