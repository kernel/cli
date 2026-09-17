package cmd

import (
	"context"
	"encoding/json"
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

func executeWebMCPCommand(t *testing.T, handler http.HandlerFunc, stdin string, args ...string) (string, string, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"))
	root := &cobra.Command{Use: "kernel", SilenceErrors: true, SilenceUsage: true}
	root.SetContext(context.WithValue(context.Background(), util.KernelClientKey, client))
	root.SetIn(strings.NewReader(stdin))
	root.AddCommand(newBrowsersWebMCPCommand())
	root.SetArgs(append([]string{"webmcp"}, args...))
	buf := capturePtermOutput(t)
	var err error
	stdout := captureStdout(t, func() { err = root.Execute() })
	return stdout, buf.String(), err
}

const webMCPToolsFixture = `{"tools":[{"name":"search","tool_ref":"opaque/ref+==","description":"Search the page","input_schema":{"type":"object"},"annotations":{"read_only":true,"autosubmit":false,"consequential":false,"untrusted_content":true},"source":{"window_id":1,"tab_id":42,"page_url":"https://example.com","page_title":"Example","frame":null}}],"future_field":true}`

func TestWebMCPCommandWiring(t *testing.T) {
	for _, name := range []string{"list", "invoke"} {
		cmd, remaining, err := rootCmd.Find([]string{"browsers", "webmcp", name})
		require.NoError(t, err)
		require.Empty(t, remaining)
		assert.Equal(t, name, cmd.Name())
		assert.NotNil(t, cmd.RunE)
		assert.False(t, isAuthExempt(cmd))
	}
}

func TestWebMCPList(t *testing.T) {
	for _, identifier := range []string{"my-browser", "session123"} {
		for _, flags := range [][]string{nil, {"--json"}, {"-o", "json"}, {"--output", "json"}} {
			t.Run(identifier+strings.Join(flags, ""), func(t *testing.T) {
				calls := 0
				stdout, table, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "/browsers/"+identifier+"/webmcp/tools", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, webMCPToolsFixture)
				}, "", append([]string{"list", identifier}, flags...)...)
				require.NoError(t, err)
				assert.Equal(t, 1, calls)
				if len(flags) > 0 {
					assert.JSONEq(t, webMCPToolsFixture, stdout)
					assert.Empty(t, table)
				} else {
					for _, value := range []string{"Name", "Tool Ref", "Page URL", "Tab ID", "Read Only", "search", "opaque/ref+==", "https://example.com", "42", "true"} {
						assert.Contains(t, table, value)
					}
				}
			})
		}
	}
}

func TestWebMCPListEmpty(t *testing.T) {
	for _, flags := range [][]string{nil, {"--json"}} {
		stdout, table, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"tools":[]}`)
		}, "", append([]string{"list", "my-browser"}, flags...)...)
		require.NoError(t, err)
		if len(flags) == 0 {
			assert.Contains(t, table, "No WebMCP tools found")
		} else {
			assert.JSONEq(t, `{"tools":[]}`, stdout)
		}
	}
}

func TestWebMCPListAnnotations(t *testing.T) {
	for _, tc := range []struct{ annotation, want string }{
		{`{"read_only":true}`, "true"},
		{`{"read_only":false}`, "false"},
		{`{}`, "-"},
		{`null`, "-"},
	} {
		t.Run(tc.annotation, func(t *testing.T) {
			_, table, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"tools":[{"name":"search","annotations":%s}]}`, tc.annotation)
			}, "", "list", "my-browser")
			require.NoError(t, err)
			rows := strings.Split(strings.TrimSpace(table), "\n")
			assert.True(t, strings.HasSuffix(strings.TrimSpace(rows[len(rows)-1]), tc.want), table)
		})
	}
}

func TestWebMCPInvoke(t *testing.T) {
	input := `{"id":9007199254740993,"nested":{"items":[1,true,null]},"query":"example"}`
	file := filepath.Join(t.TempDir(), "input.json")
	require.NoError(t, os.WriteFile(file, []byte(input), 0600))
	for _, tc := range []struct {
		name, stdin string
		flags       []string
		timeout     bool
	}{
		{"inline", "", []string{"--input", input}, false},
		{"file", "", []string{"--input-file", file, "--timeout-sec", "30"}, true},
		{"stdin", input, []string{"--input-file", "-"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			stdout, table, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/browsers/my-browser/webmcp/invoke", r.URL.Path)
				var body struct {
					ToolRef string          `json:"tool_ref"`
					Input   json.RawMessage `json:"input"`
					Timeout *int64          `json:"timeout_sec"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "opaque/ref+==", body.ToolRef)
				assert.JSONEq(t, input, string(body.Input))
				assert.Contains(t, string(body.Input), "9007199254740993")
				if tc.timeout {
					require.NotNil(t, body.Timeout)
					assert.Equal(t, int64(30), *body.Timeout)
				} else {
					assert.Nil(t, body.Timeout)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"invocation_id":"inv-1","status":"completed","output":{"id":9007199254740993,"ok":true}}`)
			}, tc.stdin, append([]string{"invoke", "my-browser", "--tool-ref", "opaque/ref+=="}, tc.flags...)...)
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			assert.Equal(t, "{\n  \"id\": 9007199254740993,\n  \"ok\": true\n}\n", stdout)
			assert.Empty(t, table)
		})
	}
}

func TestWebMCPInvokeOutputTypes(t *testing.T) {
	for _, output := range []string{`null`, `false`, `42`, `"text"`, `[]`, `{}`} {
		t.Run(output, func(t *testing.T) {
			stdout, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"tool_ref":"ref","input":{}}`, string(data))
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"invocation_id":"inv-1","status":"completed","output":%s}`, output)
			}, "", "invoke", "session123", "--tool-ref", "ref", "--input", "{}")
			require.NoError(t, err)
			assert.JSONEq(t, output, stdout)
		})
	}
}

func TestWebMCPInvalidInput(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"list"}, "accepts 1 arg"},
		{[]string{"list", "browser", "extra"}, "accepts 1 arg"},
		{[]string{"list", "browser", "--json", "-o", "yaml"}, "unsupported --output"},
		{[]string{"invoke", "browser", "--input", "{}"}, "required flag"},
		{[]string{"invoke", "browser", "--tool-ref", "ref"}, "at least one"},
		{[]string{"invoke", "browser", "--tool-ref", "ref", "--input", "{}", "--input-file", "-"}, "none of the others can be"},
		{[]string{"invoke", "browser", "--tool-ref=", "--input", "{}"}, "missing --tool-ref"},
		{[]string{"invoke", "browser", "--tool-ref", "ref", "--input-file", filepath.Join(t.TempDir(), "missing")}, "failed to read input file"},
		{[]string{"invoke", "browser", "--tool-ref", "ref", "--input-file", "-"}, "expected a JSON object"},
	}
	for _, input := range []string{"", "null", "[]", "1", "true", `"text"`, "{", "{} {}", "{} trailing"} {
		tests = append(tests, struct {
			args []string
			want string
		}{[]string{"invoke", "browser", "--tool-ref", "ref", "--input", input}, "invalid input"})
	}
	for _, timeout := range []string{"0", "-1"} {
		tests = append(tests, struct {
			args []string
			want string
		}{[]string{"invoke", "browser", "--tool-ref", "ref", "--input", "{}", "--timeout-sec", timeout}, "invalid --timeout-sec"})
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid input reached API")
			}, "", tc.args...)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestWebMCPInvokeNeverRetries(t *testing.T) {
	for _, status := range []int{504, 500, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			stdout, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Should-Retry", "true")
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"code":"outcome_unknown","invocation_id":"inv-unknown","message":"Tool may have completed"}`)
			}, "", "invoke", "browser", "--tool-ref", "ref", "--input", "{}")
			require.Error(t, err)
			assert.Equal(t, 1, calls)
			assert.Empty(t, stdout)
			// The root error handler wraps command errors again before printing them.
			assert.EqualError(t, util.CleanedUpSdkError{Err: err}, "outcome_unknown (invocation_id: inv-unknown): Tool may have completed")
			var apiErr *kernel.Error
			assert.ErrorAs(t, err, &apiErr)
		})
	}
}

func TestWebMCPInvokeFailedStatus(t *testing.T) {
	for _, status := range []string{"error", "canceled"} {
		t.Run(status, func(t *testing.T) {
			stdout, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"invocation_id":"inv-1","status":%q,"error_text":"Tool did not complete"}`, status)
			}, "", "invoke", "browser", "--tool-ref", "ref", "--input", "{}")
			require.ErrorContains(t, err, "inv-1: "+status+": Tool did not complete")
			assert.Empty(t, stdout)
		})
	}
}

func TestWebMCPInvokeAwaitingSubmission(t *testing.T) {
	stdout, warning, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"invocation_id":"inv-1","status":"awaiting_submission","output":{"filled":["email"]}}`)
	}, "", "invoke", "browser", "--tool-ref", "ref", "--input", "{}")
	require.NoError(t, err)
	assert.JSONEq(t, `{"filled":["email"]}`, stdout)
	assert.Contains(t, warning, "inv-1")
	assert.Contains(t, warning, "without submitting it")
}

func TestWebMCPListAPIError(t *testing.T) {
	_, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"code":"not_found","message":"Browser not found"}`)
	}, "", "list", "missing")
	require.EqualError(t, err, "not_found: Browser not found")
}
