package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const webMCPCustomToolsFixture = `{"tools":[{"id":"ct_abcdefghijklmnopqrstuvwx","namespace":"helpers","kind":"page","match":{"url_patterns":["https://example.com/*","https://example.org/*"]},"tool":{"name":"search","title":"Search","description":"Search the page","inputSchema":{"type":"object"},"outputSchema":{"type":"object"},"annotations":{"readOnlyHint":true}}},{"id":"ct_bcdefghijklmnopqrstuvwxy","namespace":"helpers","kind":"cdp","match":{"url_patterns":["https://example.net/*"]},"tool":{"name":"inspect","description":"Inspect the page","inputSchema":{"type":"object"}}}],"future_field":true}`

func TestWebMCPListExcludeCustom(t *testing.T) {
	for _, tc := range []struct {
		flag, query string
	}{
		{"", ""},
		{"--exclude-custom", "exclude_custom=true"},
		{"--exclude-custom=false", "exclude_custom=false"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			args := []string{"list", "browser"}
			if tc.flag != "" {
				args = append(args, tc.flag)
			}
			_, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.query, r.URL.RawQuery)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"tools":[]}`)
			}, "", args...)
			require.NoError(t, err)
		})
	}
}

func TestWebMCPCustomToolsList(t *testing.T) {
	for _, fixture := range []string{webMCPCustomToolsFixture, `{"tools":[]}`} {
		for _, flags := range [][]string{nil, {"--json"}, {"-o", "json"}, {"--output", "json"}} {
			t.Run(fixture+strings.Join(flags, " "), func(t *testing.T) {
				calls := 0
				stdout, table, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					assert.Equal(t, http.MethodGet, r.Method)
					assert.Equal(t, "/browsers/my-browser/webmcp/custom-tools", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, fixture)
				}, "", append([]string{"custom-tools", "list", "my-browser"}, flags...)...)
				require.NoError(t, err)
				assert.Equal(t, 1, calls)
				if len(flags) > 0 {
					assert.JSONEq(t, fixture, stdout)
					assert.Empty(t, table)
				} else if fixture == `{"tools":[]}` {
					assert.Contains(t, table, "No custom WebMCP tools found")
				} else {
					for _, value := range []string{"ID", "Namespace", "Kind", "Name", "URL Patterns", "ct_abcdefghijklmnopqrstuvwx", "helpers", "page", "cdp", "search", "inspect", "https://example.com/*, https://example.org/*"} {
						assert.Contains(t, table, value)
					}
				}
			})
		}
	}
}

func TestWebMCPCustomToolsAdd(t *testing.T) {
	source := `[{kind: "page", match: {url_patterns: ["https://example.com/*"]}, tool: {name: "search", description: "café", inputSchema: {type: "object"}}, execute: async () => ({ok: true})}]`
	file := filepath.Join(t.TempDir(), "tools.js")
	require.NoError(t, os.WriteFile(file, []byte(source), 0600))
	forceTrue, forceFalse := true, false
	for _, tc := range []struct {
		name, path, stdin string
		flags             []string
		force             *bool
	}{
		{name: "file", path: file},
		{name: "stdin", path: "-", stdin: source, flags: []string{"--json"}},
		{name: "overwrite", path: file, flags: []string{"--force-overwrite-namespace", "-o", "json"}, force: &forceTrue},
		{name: "explicit false", path: file, flags: []string{"--force-overwrite-namespace=false", "--output", "json"}, force: &forceFalse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			stdout, table, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/browsers/my-browser/webmcp/custom-tools", r.URL.Path)
				var body struct {
					Namespace string `json:"namespace"`
					Source    string `json:"source"`
					Force     *bool  `json:"force_overwrite_namespace"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "helpers", body.Namespace)
				assert.Equal(t, source, body.Source)
				assert.Equal(t, tc.force, body.Force)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprint(w, webMCPCustomToolsFixture)
			}, tc.stdin, append([]string{"custom-tools", "add", "my-browser", "--namespace", "helpers", "--source-file", tc.path}, tc.flags...)...)
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			if len(tc.flags) > 0 {
				assert.JSONEq(t, webMCPCustomToolsFixture, stdout)
				assert.Empty(t, table)
			} else {
				assert.Contains(t, table, "ct_abcdefghijklmnopqrstuvwx")
				assert.Contains(t, table, "search")
			}
		})
	}
}

func TestWebMCPCustomToolsRemove(t *testing.T) {
	calls := 0
	stdout, output, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/browsers/my-browser/webmcp/custom-tools/ct_abcdefghijklmnopqrstuvwx", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}, "", "custom-tools", "remove", "my-browser", "ct_abcdefghijklmnopqrstuvwx")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Empty(t, stdout)
	assert.Contains(t, output, "Removed custom WebMCP tool: ct_abcdefghijklmnopqrstuvwx")
}

func TestWebMCPCustomToolsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"list"}, "accepts 1 arg"},
		{[]string{"list", "browser", "extra"}, "accepts 1 arg"},
		{[]string{"list", "browser", "--json", "-o", "yaml"}, "unsupported --output"},
		{[]string{"add", "browser"}, "required flag"},
		{[]string{"add", "browser", "--namespace", "helpers"}, "required flag"},
		{[]string{"add", "browser", "--source-file", "-"}, "required flag"},
		{[]string{"add", "browser", "--namespace", "helpers", "--source-file", "-", "--json", "-o", "yaml"}, "unsupported --output"},
		{[]string{"add", "browser", "--namespace", "helpers", "--source-file", filepath.Join(t.TempDir(), "missing")}, "failed to read source file"},
		{[]string{"remove", "browser"}, "accepts 2 arg"},
		{[]string{"remove", "browser", "not-an-id"}, "invalid custom tool ID"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid input reached API")
			}, "", append([]string{"custom-tools"}, tc.args...)...)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestWebMCPCustomToolsSourceValidation(t *testing.T) {
	for _, tc := range []struct{ name, namespace, source, want string }{
		{"empty namespace", "", "[]", "invalid --namespace"},
		{"invalid namespace", "bad/name", "[]", "invalid --namespace"},
		{"long namespace", strings.Repeat("a", 129), "[]", "invalid --namespace"},
		{"empty source", "helpers", "", "must not be empty"},
		{"blank source", "helpers", " \n\t", "must not be empty"},
		{"oversized", "helpers", strings.Repeat("é", webMCPCustomSourceMaxBytes/2+1), "exceeds 8000000 UTF-8 bytes"},
		{"invalid UTF-8", "helpers", string([]byte{0xff}), "must be valid UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid input reached API")
			}, tc.source, "custom-tools", "add", "browser", "--namespace", tc.namespace, "--source-file", "-")
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestWebMCPCustomToolsSourceLimit(t *testing.T) {
	source := "/*" + strings.Repeat("é", (webMCPCustomSourceMaxBytes-6)/2) + "*/[]"
	namespace := "A_-." + strings.Repeat("a", 124)
	calls := 0
	_, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Source    string `json:"source"`
			Namespace string `json:"namespace"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, source, body.Source)
		assert.Len(t, body.Source, webMCPCustomSourceMaxBytes)
		assert.Equal(t, namespace, body.Namespace)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, webMCPCustomToolsFixture)
	}, source, "custom-tools", "add", "browser", "--namespace", namespace, "--source-file", "-")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestWebMCPCustomToolsAPIErrors(t *testing.T) {
	for _, args := range [][]string{
		{"list", "browser"},
		{"add", "browser", "--namespace", "helpers", "--source-file", "-"},
		{"remove", "browser", "ct_abcdefghijklmnopqrstuvwx"},
	} {
		t.Run(args[0], func(t *testing.T) {
			stdout, output, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"code":"invalid_request","message":"Invalid custom tool"}`)
			}, "[]", append([]string{"custom-tools"}, args...)...)
			require.EqualError(t, err, "invalid_request: Invalid custom tool")
			assert.Empty(t, stdout)
			assert.Empty(t, output)
		})
	}
}

func TestWebMCPCustomToolsAddNeverRetries(t *testing.T) {
	calls := 0
	_, _, err := executeWebMCPCommand(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Should-Retry", "true")
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"Response lost"}`)
	}, "[]", "custom-tools", "add", "browser", "--namespace", "helpers", "--source-file", "-")
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}
