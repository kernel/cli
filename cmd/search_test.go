package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeSearchCommand(t *testing.T, handler http.HandlerFunc, stdin string, args ...string) (string, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithProject("project-test"))
	root := &cobra.Command{Use: "kernel", SilenceErrors: true, SilenceUsage: true}
	root.SetContext(context.WithValue(context.Background(), util.KernelClientKey, client))
	root.SetIn(strings.NewReader(stdin))
	root.AddCommand(newSearchCommand())
	root.SetArgs(append([]string{"search"}, args...))
	var err error
	stdout := captureStdout(t, func() { err = root.Execute() })
	return stdout, err
}

func TestSearchCreate(t *testing.T) {
	const advanced = `{"query":"test","strategy":{"type":"fallback","providers":[{"provider":"exa","options":{"type":"auto"}},{"provider":"brave"}],"fallback_on":["error","timeout","empty"]},"content":true,"strict_params":true,"include_raw":true,"include_domains":["example.com"]}`
	path := filepath.Join(t.TempDir(), "request.json")
	require.NoError(t, os.WriteFile(path, []byte(advanced), 0600))
	for _, tc := range []struct {
		name        string
		args        []string
		stdin, want string
	}{
		{"auto", []string{"test"}, "", `{"query":"test"}`},
		{"pinned", []string{"test", "--provider", "exa", "--max-results", "5"}, "", `{"query":"test","max_results":5,"strategy":{"type":"pinned","provider":{"provider":"exa"}}}`},
		{"inline", []string{"--request", advanced}, "", advanced},
		{"stdin", []string{"--request-file", "-"}, advanced, advanced},
		{"file", []string{"--request-file", path}, "", advanced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			const response = `{"id":"srch_test","results":[],"warnings":[{"code":"billing_unavailable"}],"attempts":[],"future_field":9007199254740993}`
			stdout, err := executeSearchCommand(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/search", r.URL.Path)
				assert.Equal(t, "Bearer test", r.Header.Get("Authorization"))
				assert.Equal(t, "project-test", r.Header.Get("X-Kernel-Project"))
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, tc.want, string(data))
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, response)
			}, tc.stdin, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.JSONEq(t, response, stdout)
			assert.Contains(t, stdout, "9007199254740993")
		})
	}
}

func TestSearchReadCommands(t *testing.T) {
	for _, tc := range []struct {
		args                  []string
		path, query, response string
	}{
		{[]string{"get", "srch_test"}, "/search/srch_test", "", `{"id":"srch_test","results":[]}`},
		{[]string{"providers"}, "/search/providers", "", `[]`},
		{[]string{"providers", "--slug", "exa&other=1"}, "/search/providers", "slug=exa%26other%3D1", `[]`},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			stdout, err := executeSearchCommand(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, tc.path, r.URL.Path)
				assert.Equal(t, tc.query, r.URL.RawQuery)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.Empty(t, body)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.response)
			}, "", tc.args...)
			require.NoError(t, err)
			assert.JSONEq(t, tc.response, stdout)
		})
	}
}

func TestSearchInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{}, {" "}, {strings.Repeat("x", 2049)}, {"a", "b"}, {"test", "--provider", ""},
		{"test", "--max-results", "0"}, {"test", "--max-results", "101"},
		{"--request", "null"}, {"--request", "[]"}, {"--request", "{"}, {"--request", `{}`}, {"--request", `{"query":1}`},
		{"test", "--request", `{"query":"test"}`}, {"--request", `{}`, "--provider", "exa"},
		{"--request", `{}`, "--max-results", "5"}, {"--request", `{}`, "--request-file", "-"},
		{"--request-file", "/nonexistent/search-request.json"}, {"get"}, {"providers", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := executeSearchCommand(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected API call") }, "", args...)
			require.Error(t, err)
		})
	}
}

func TestSearchErrorsDoNotRetryCreate(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			stdout, err := executeSearchCommand(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"search failed"}`)
			}, "", "test")
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestSearchWiring(t *testing.T) {
	for _, path := range [][]string{{"search"}, {"search", "get"}, {"search", "providers"}} {
		cmd, remaining, err := rootCmd.Find(path)
		require.NoError(t, err)
		require.Empty(t, remaining)
		assert.NotNil(t, cmd.RunE)
		assert.False(t, isAuthExempt(cmd))
	}
}
