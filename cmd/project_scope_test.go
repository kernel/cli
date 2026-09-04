package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kernel/cli/pkg/auth"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func projectTestCommand(t *testing.T, serverURL, project string) *cobra.Command {
	t.Helper()
	t.Setenv("KERNEL_BASE_URL", serverURL)
	t.Setenv("KERNEL_API_KEY", "test-api-key")
	cmd := &cobra.Command{Use: "request"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("project", project, "")
	cmd.Flags().String("log-level", "warn", "")
	cmd.Flags().Bool("no-color", false, "")
	return cmd
}

func TestProjectSelectionHeaders(t *testing.T) {
	for _, tt := range []struct {
		name, flag, env, want string
	}{
		{name: "flag ID", flag: "proj_123", want: "proj_123"},
		{name: "flag exact name", flag: "Release QA", want: "Release QA"},
		{name: "environment ID", env: "proj_123", want: "proj_123"},
		{name: "environment exact name", env: "Release QA", want: "Release QA"},
		{name: "flag wins", flag: "Release QA", env: "proj_other", want: "Release QA"},
		{name: "legacy name delimiters", flag: "QA/100%", want: "QA/100%"},
		{name: "no selection"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KERNEL_PROJECT", tt.env)
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				assert.Equal(t, tt.want, r.Header.Get("X-Kernel-Project"))
				assert.Empty(t, r.Header.Get("X-Kernel-Project-Id"))
				assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()
			cmd := projectTestCommand(t, server.URL, tt.flag)
			require.NoError(t, rootCmd.PersistentPreRunE(cmd, nil))
			client := getKernelClient(cmd)
			_, err := client.Browsers.Get(cmd.Context(), "browser_123", kernel.BrowserGetParams{})
			require.NoError(t, err)
			_, err = client.Profiles.Get(cmd.Context(), "profile_123")
			require.NoError(t, err)
			_, err = client.Proxies.Get(cmd.Context(), "proxy_123")
			require.NoError(t, err)
			_, err = client.Auth.Connections.Get(cmd.Context(), "connection_123")
			require.NoError(t, err)
			assert.Equal(t, []string{"/browsers/browser_123", "/profiles/profile_123", "/proxies/proxy_123", "/auth/connections/connection_123"}, paths)
		})
	}
}

func TestDeployGithubProjectScope(t *testing.T) {
	for _, tt := range []struct {
		name, flag, env, want string
		oauth                 bool
		status                int
	}{
		{name: "flag ID", flag: "proj_123", want: "proj_123"},
		{name: "flag exact name", flag: "Release QA", want: "Release QA"},
		{name: "environment ID", env: "proj_123", want: "proj_123"},
		{name: "environment exact name", env: "Release QA", want: "Release QA"},
		{name: "flag wins", flag: "Release QA", env: "proj_other", want: "Release QA"},
		{name: "no selection"},
		{name: "project OAuth", flag: "Release QA", want: "Release QA", oauth: true},
		{name: "server error without retry", flag: "proj_123", want: "proj_123", status: http.StatusInternalServerError},
		{name: "invalid ID", flag: "proj_missing", want: "proj_missing", status: http.StatusNotFound},
		{name: "invalid name", env: "missing project", want: "missing project", status: http.StatusNotFound},
		{name: "credential scope mismatch", flag: "Other Project", want: "Other Project", oauth: true, status: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KERNEL_PROJECT", tt.env)
			wantAuth := "Bearer test-api-key"
			if tt.oauth {
				wantAuth = "Bearer test-oauth-token"
			}
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.Method+" "+r.URL.Path)
				assert.Equal(t, tt.want, r.Header.Get("X-Kernel-Project"))
				assert.Empty(t, r.Header.Get("X-Kernel-Project-Id"))
				assert.Equal(t, wantAuth, r.Header.Get("Authorization"))
				assert.Equal(t, metadata.Version, r.Header.Get("X-Kernel-Cli-Version"))
				switch r.URL.Path {
				case "/deployments":
					assert.Equal(t, http.MethodPost, r.Method)
					if err := r.ParseMultipartForm(1 << 20); !assert.NoError(t, err) {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					defer func() { assert.NoError(t, r.MultipartForm.RemoveAll()) }()
					assert.Equal(t, "aws.us-east-1a", r.FormValue("region"))
					var source map[string]string
					assert.NoError(t, json.Unmarshal([]byte(r.FormValue("source")), &source))
					assert.Equal(t, map[string]string{"type": "github", "url": "https://github.com/example/app", "ref": "main", "entrypoint": "app.ts"}, source)
					w.Header().Set("Content-Type", "application/json")
					if tt.status != 0 {
						w.WriteHeader(tt.status)
						_, _ = io.WriteString(w, `{"code":"project_error","message":"project selector rejected"}`)
						return
					}
					_, _ = io.WriteString(w, `{"id":"deployment_123"}`)
				case "/deployments/deployment_123/events":
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"event\":\"deployment_state\",\"deployment\":{\"status\":\"running\"}}\n\n")
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			cmd := projectTestCommand(t, server.URL, tt.flag)
			if tt.oauth {
				t.Setenv("KERNEL_API_KEY", "")
				keyring.MockInit()
				t.Cleanup(keyring.MockInit)
				require.NoError(t, auth.SaveTokens(&auth.TokenStorage{
					AccessToken: "test-oauth-token", ExpiresAt: time.Now().Add(time.Hour),
					AccessScope: "project", ProjectID: "proj_123",
				}))
			}
			cmd.Flags().String("url", "https://github.com/example/app", "")
			cmd.Flags().String("ref", "main", "")
			cmd.Flags().String("entrypoint", "app.ts", "")
			cmd.Flags().String("output", "json", "")
			require.NoError(t, rootCmd.PersistentPreRunE(cmd, nil))
			err := runDeployGithub(cmd, nil)
			if tt.status != 0 {
				var apiErr *kernel.Error
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tt.status, apiErr.StatusCode)
				assert.Contains(t, err.Error(), "project selector rejected")
				assert.Equal(t, []string{"POST /deployments"}, paths)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []string{"POST /deployments", "GET /deployments/deployment_123/events"}, paths)
			}
		})
	}
}
