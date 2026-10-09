package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kernel/cli/pkg/util"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executePlaywrightCommand(t *testing.T, handler http.HandlerFunc, args ...string) (string, string, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
	root := &cobra.Command{Use: "kernel", SilenceErrors: true, SilenceUsage: true}
	root.SetContext(context.WithValue(context.Background(), util.KernelClientKey, client))
	root.AddCommand(newBrowsersPlaywrightCommand())
	root.SetArgs(append([]string{"playwright"}, args...))
	buf := capturePtermOutput(t)
	var err error
	stdout := captureStdout(t, func() { err = root.Execute() })
	return stdout, buf.String(), err
}

const playwrightExecutorsFixture = `{"executors":[{"name":"default","busy":false,"created_at":"2026-01-02T03:04:05Z","last_used_at":"2026-01-02T03:05:05Z"},{"name":"checkout","busy":true,"created_at":"2026-01-02T03:06:05Z","last_used_at":"2026-01-02T03:07:05Z","target_id":"ABCDEF0123456789","url":"https://example.com/cart"}]}`

func TestPlaywrightCommandWiring(t *testing.T) {
	for _, path := range [][]string{{"execute"}, {"executors", "list"}, {"executors", "delete"}} {
		name := path[len(path)-1]
		cmd, remaining, err := rootCmd.Find(append([]string{"browsers", "playwright"}, path...))
		require.NoError(t, err)
		require.Empty(t, remaining)
		assert.Equal(t, name, cmd.Name())
		assert.NotNil(t, cmd.RunE)
		assert.False(t, isAuthExempt(cmd))
	}

	execute, _, err := rootCmd.Find([]string{"browsers", "playwright", "execute"})
	require.NoError(t, err)
	executor := execute.Flags().Lookup("executor")
	require.NotNil(t, executor)
	assert.Equal(t, "", executor.DefValue)
	assert.Contains(t, execute.Long, "8 named executors")
	assert.NotNil(t, execute.Flags().Lookup("timeout"))
	assert.NotNil(t, execute.Flags().Lookup("output"))

	del, _, err := rootCmd.Find([]string{"browsers", "playwright", "executors", "delete"})
	require.NoError(t, err)
	closeTab := del.Flags().Lookup("close-tab")
	require.NotNil(t, closeTab)
	assert.Equal(t, "true", closeTab.DefValue)
	assert.Contains(t, del.Long, "default")

	list, _, err := rootCmd.Find([]string{"browsers", "playwright", "executors", "list"})
	require.NoError(t, err)
	assert.NotNil(t, list.Flags().Lookup("output"))
}

func TestPlaywrightExecuteExecutorParam(t *testing.T) {
	for _, tc := range []struct {
		name     string
		flags    []string
		executor string
	}{
		{"omitted", nil, ""},
		{"named", []string{"--executor", "checkout"}, "checkout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body struct {
				Code     string  `json:"code"`
				Executor *string `json:"executor"`
				Timeout  *int64  `json:"timeout_sec"`
			}
			calls := 0
			stdout, table, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/browsers/my-browser":
					fmt.Fprint(w, `{"session_id":"session123"}`)
				case r.Method == http.MethodPost && r.URL.Path == "/browsers/session123/playwright/execute":
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					fmt.Fprint(w, `{"success":true,"result":{"title":"Example"},"tab":{"target_id":"ABCDEF0123456789","created":true}}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			}, append([]string{"execute", "my-browser", "return await page.title()", "--timeout", "30"}, tc.flags...)...)
			require.NoError(t, err)
			assert.Equal(t, 2, calls)
			assert.Equal(t, "return await page.title()", body.Code)
			if tc.executor == "" {
				assert.Nil(t, body.Executor)
			} else {
				require.NotNil(t, body.Executor)
				assert.Equal(t, tc.executor, *body.Executor)
			}
			require.NotNil(t, body.Timeout)
			assert.Equal(t, int64(30), *body.Timeout)
			for _, value := range []string{"Success", "true", "Tab Target ID", "ABCDEF0123456789", "Tab Created"} {
				assert.Contains(t, table, value)
			}
			assert.Contains(t, stdout, `"title": "Example"`)
		})
	}
}

func TestPlaywrightExecuteOutput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		json     bool
		wantTab  bool
	}{
		{"without tab", `{"success":false,"error":"boom"}`, false, false},
		{"with tab", `{"success":true,"tab":{"target_id":"ABCDEF0123456789","created":false}}`, false, true},
		{"json", `{"success":true,"result":42,"tab":{"target_id":"ABCDEF0123456789","created":true}}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"execute", "session123", "return 1"}
			if tc.json {
				args = append(args, "-o", "json")
			}
			stdout, table, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					fmt.Fprint(w, `{"session_id":"session123"}`)
					return
				}
				fmt.Fprint(w, tc.response)
			}, args...)
			require.NoError(t, err)
			if tc.json {
				assert.JSONEq(t, tc.response, stdout)
				assert.Empty(t, table)
				return
			}
			if tc.wantTab {
				assert.Contains(t, table, "Tab Target ID")
				assert.Contains(t, table, "ABCDEF0123456789")
				assert.Contains(t, table, "Tab Created")
			} else {
				assert.NotContains(t, table, "Tab Target ID")
				assert.Contains(t, table, "boom")
			}
		})
	}
}

func TestPlaywrightExecuteExecutorLimit(t *testing.T) {
	_, _, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"session_id":"session123"}`)
			return
		}
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"message":"Browser already has 8 named Playwright executors.",`+
			`"executors":[{"name":"default","busy":false,"created_at":"2026-01-02T03:04:05Z","last_used_at":"2026-01-02T03:04:05Z"}]}`)
	}, "execute", "my-browser", "return 1", "--executor", "ninth")
	require.EqualError(t, err, "Browser already has 8 named Playwright executors. "+
		"See them with 'kernel browsers playwright executors list my-browser' and "+
		"free one with 'kernel browsers playwright executors delete my-browser <executor>'")
	assert.NotContains(t, err.Error(), "\n")
	// The root error handler wraps command errors again before printing them.
	assert.Equal(t, err.Error(), util.CleanedUpSdkError{Err: err}.Error())
}

func TestPlaywrightExecuteOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  []string
		status int
		body   string
		want   string
	}{
		{"not conflict", []string{"--executor", "bad name"}, http.StatusBadRequest, `{"code":"invalid_request","message":"Invalid executor name"}`, "invalid_request: Invalid executor name"},
		{"conflict without message", []string{"--executor", "ninth"}, http.StatusConflict, `{"code":"conflict","message":""}`, "conflict: "},
		{"conflict without executor", nil, http.StatusConflict, `{"code":"conflict","message":"Browser is busy"}`, "conflict: Browser is busy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					fmt.Fprint(w, `{"session_id":"session123"}`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}, append([]string{"execute", "session123", "return 1"}, tc.flags...)...)
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestPlaywrightExecutorsList(t *testing.T) {
	for _, identifier := range []string{"my-browser", "session123"} {
		for _, flags := range [][]string{nil, {"-o", "json"}, {"--output", "json"}} {
			t.Run(identifier+strings.Join(flags, ""), func(t *testing.T) {
				calls := 0
				stdout, table, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "/browsers/"+identifier+"/playwright/executors", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, playwrightExecutorsFixture)
				}, append([]string{"executors", "list", identifier}, flags...)...)
				require.NoError(t, err)
				assert.Equal(t, 1, calls)
				if len(flags) > 0 {
					assert.JSONEq(t, playwrightExecutorsFixture, stdout)
					assert.Empty(t, table)
					return
				}
				for _, value := range []string{"Name", "Busy", "Created At", "Last Used At", "Target ID", "URL", "default", "false", "checkout", "true", "ABCDEF0123456789", "https://example.com/cart"} {
					assert.Contains(t, table, value)
				}
				rows := strings.Split(strings.TrimSpace(table), "\n")
				require.Len(t, rows, 3)
				assert.Contains(t, rows[1], "default")
				assert.NotContains(t, rows[1], "ABCDEF0123456789")
				assert.Contains(t, rows[2], "checkout")
			})
		}
	}
}

func TestPlaywrightExecutorsListEmpty(t *testing.T) {
	_, table, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"executors":[]}`)
	}, "executors", "list", "my-browser")
	require.NoError(t, err)
	assert.Contains(t, table, "No Playwright executors found")
}

func TestPlaywrightExecutorsDelete(t *testing.T) {
	for _, tc := range []struct {
		name     string
		executor string
		flags    []string
		closeTab string
		want     string
	}{
		{"default close", "checkout", nil, "", `Deleted Playwright executor "checkout"`},
		{"keep tab", "checkout", []string{"--close-tab=false"}, "false", `Deleted Playwright executor "checkout"`},
		{"explicit close", "checkout", []string{"--close-tab"}, "true", `Deleted Playwright executor "checkout"`},
		{"default executor", "default", nil, "", "Restarted the default Playwright executor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			stdout, out, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/browsers/my-browser/playwright/executors/"+tc.executor, r.URL.Path)
				assert.Equal(t, tc.closeTab, r.URL.Query().Get("close_tab"))
				w.WriteHeader(http.StatusNoContent)
			}, append([]string{"executors", "delete", "my-browser", tc.executor}, tc.flags...)...)
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.Empty(t, stdout)
			assert.Contains(t, out, tc.want)
		})
	}
}

func TestPlaywrightExecutorsErrors(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		status int
		body   string
		want   string
	}{
		{[]string{"executors", "list", "missing"}, http.StatusNotFound, `{"code":"not_found","message":"Browser not found"}`, "not_found: Browser not found"},
		{[]string{"executors", "delete", "my-browser", "missing"}, http.StatusNotFound, `{"code":"not_found","message":"Executor not found"}`, "not_found: Executor not found"},
		{[]string{"executors", "delete", "my-browser", "bad name"}, http.StatusBadRequest, `{"code":"invalid_request","message":"Invalid executor name"}`, "invalid_request: Invalid executor name"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, _, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}, tc.args...)
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestPlaywrightExecutorsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"executors", "list"}, "accepts 1 arg"},
		{[]string{"executors", "list", "browser", "-o", "yaml"}, "unsupported --output"},
		{[]string{"executors", "delete", "browser"}, "accepts 2 arg"},
		{[]string{"executors", "delete", "browser", "a", "b"}, "accepts 2 arg"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, _, err := executePlaywrightCommand(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid input reached API")
			}, tc.args...)
			require.ErrorContains(t, err, tc.want)
		})
	}
}
