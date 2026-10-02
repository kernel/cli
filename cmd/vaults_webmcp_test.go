package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const resyCredentialFixture = `{"id":"credential-1","key":"resy","type":"credential","version":1,"spec":{"provider":"kernel","description":"Resy","fields":[{"name":"email","type":"email","sensitive":false},{"name":"password","type":"password","sensitive":true}]},"state":{"status":"ready"},"available_operations":[{"type":"webmcp_invoke","description":"Invoke a live WebMCP tool with selected credential fields."}]}`
const resyWebMCPRequest = `{"type":"webmcp_invoke","browser_id":"browser-session-id","tool_ref":"wmcp_tool_ref_1","page_url":"https://resy.com/login","input":{"email":null,"password":null,"remember":true},"bindings":[{"field":"email","input_path":"/email"},{"field":"password","input_path":"/password"}],"timeout_sec":30}`
const resyWebMCPResult = `{"type":"webmcp_invoke","status":"completed","invocation_id":"invoke-1","output":{"authenticated":true,"user_id":12345678901234567890}}`

func vaultWebMCPArgs(extra ...string) []string {
	return append([]string{"vaults", "items", "webmcp", "invoke", "user-vault", "resy",
		"--browser-id", "browser-session-id", "--tool-ref", "wmcp_tool_ref_1", "--page-url", "https://resy.com/login",
		"--input", `{"email":null,"password":null,"remember":true}`,
		"--bind", "email=/email", "--bind", "password=/password", "--timeout-sec", "30"}, extra...)
}

func vaultWebMCPClient(t *testing.T, item string, status int, result string, posts *int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			assert.Equal(t, "/vaults/user-vault/items/resy", r.URL.Path)
			_, _ = io.WriteString(w, item)
			return
		}
		*posts++
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/vaults/user-vault/items/resy/operations", r.URL.Path)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, result)
	}
}

func TestVaultWebMCPInvokeResyLogin(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	specFile := filepath.Join(t.TempDir(), "params.json")
	require.NoError(t, os.WriteFile(specFile, []byte(`{"browser_id":"browser-session-id","tool_ref":"wmcp_tool_ref_1","page_url":"https://resy.com/login","input":{"email":null,"password":null,"remember":true},"bindings":[{"field":"email","input_path":"/email"},{"field":"password","input_path":"/password"}],"timeout_sec":30}`), 0o600))
	for name, args := range map[string][]string{
		"flags":     vaultWebMCPArgs("-o", "json"),
		"spec-file": {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--spec-file", specFile, "-o", "json"},
	} {
		t.Run(name, func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.JSONEq(t, resyWebMCPRequest, string(body))
				}
				vaultWebMCPClient(t, resyCredentialFixture, 200, resyWebMCPResult, &posts)(w, r)
			})
			out, human, err := executeVaultCommand(t, client, args...)
			require.NoError(t, err)
			assert.Equal(t, 1, posts)
			assert.JSONEq(t, resyWebMCPResult, out)
			assert.Contains(t, out, "12345678901234567890")
			assert.Empty(t, human)
		})
	}
}

func TestVaultWebMCPInvokeHumanOutput(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	posts := 0
	client := vaultTestClient(t, vaultWebMCPClient(t, resyCredentialFixture, 200, resyWebMCPResult, &posts))
	out, human, err := executeVaultCommand(t, client, vaultWebMCPArgs()...)
	require.NoError(t, err)
	assert.Empty(t, out)
	assert.Contains(t, human, "WebMCP invoke: completed")
	assert.Contains(t, human, `Invocation ID: "invoke-1"`)
	assert.Contains(t, human, `"authenticated": true`)
	assert.Contains(t, human, "does not confirm the website accepted the action")
}

func TestVaultWebMCPInvokeInputPreservedAndCardFormat(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	card := strings.Replace(readyFillCardFixture, `[{"type":"fill","description":"Fill checkout fields."}]`, `[{"type":"webmcp_invoke","description":"Invoke a live WebMCP tool."}]`, 1)
	posts := 0
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"webmcp_invoke","browser_id":"b","tool_ref":"t","page_url":"https://shop.example/checkout?step=2","input":{"card":{"numbers":[null],"expiry":null},"amount":12345678901234567890,"a/b":null,"t~":null},"bindings":[{"field":"number","input_path":"/card/numbers/0"},{"field":"expiration","format":"MM/YY","input_path":"/card/expiry"},{"field":"cvc","input_path":"/a~1b"},{"field":"billing_name","input_path":"/t~0"}]}`, string(body))
			assert.Contains(t, string(body), "12345678901234567890")
		}
		vaultWebMCPClient(t, card, 200, `{"type":"webmcp_invoke","status":"awaiting_submission","invocation_id":"invoke-2"}`, &posts)(w, r)
	})
	out, human, err := executeVaultCommand(t, client, "vaults", "items", "webmcp", "invoke", "user-vault", "resy",
		"--browser-id", "b", "--tool-ref", "t", "--page-url", "https://shop.example/checkout?step=2",
		"--input", `{"card":{"numbers":[null],"expiry":null},"amount":12345678901234567890,"a/b":null,"t~":null}`,
		"--bind", "number=/card/numbers/0", "--bind", "expiration:MM/YY=/card/expiry", "--bind", "cvc=/a~1b", "--bind", "billing_name=/t~0")
	require.NoError(t, err)
	assert.Equal(t, 1, posts)
	assert.Empty(t, out)
	assert.Contains(t, human, "WebMCP invoke: awaiting_submission")
	assert.Contains(t, human, "do not invoke the tool again")
}

func TestVaultWebMCPInvokeValidation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s request", r.Method)
	})
	flag := func(name, value string) []string {
		args := vaultWebMCPArgs()
		for i, arg := range args {
			if arg == name {
				args[i+1] = value
				return args
			}
		}
		return append(args, name, value)
	}
	input := func(value string) []string { return flag("--input", value) }
	for name, args := range map[string][]string{
		"non-null slot":     input(`{"email":"credential-sentinel","password":null}`),
		"missing slot":      input(`{"password":null}`),
		"slot through text": flag("--bind", "password=/password/x"),
		"input array":       input(`["credential-sentinel"]`),
		"input null":        input(`null`),
		"input trailing":    input(`{"email":null,"password":null} {}`),
		"input malformed":   input(`{"credential-sentinel":`),
		"root path":         {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", `{"browser_id":"b","tool_ref":"t","page_url":"https://resy.com/login","input":{"email":null},"bindings":[{"field":"email","input_path":""}]}`},
		"append path":       append(input(`{"email":[],"password":null}`), "--bind", "x=/email/-"),
		"noncanonical":      append(input(`{"email":null,"password":null,"list":[null,null]}`), "--bind", "x=/list/01"),
		"out of range":      append(input(`{"email":null,"password":null,"list":[null]}`), "--bind", "x=/list/1"),
		"bad escape":        append(input(`{"email":null,"password":null,"a~2":null}`), "--bind", "x=/a~2"),
		"duplicate field":   append(input(`{"email":null,"password":null,"other":null}`), "--bind", "email=/other"),
		"duplicate path":    append(vaultWebMCPArgs(), "--bind", "username=/email"),
		"bind syntax":       append(vaultWebMCPArgs(), "--bind", "credential-sentinel"),
		"bind no slash":     append(vaultWebMCPArgs(), "--bind", "x=email"),
		"bad format":        append(input(`{"email":null,"password":null,"exp":null}`), "--bind", "expiration:YYYY=/exp"),
		"empty format":      append(input(`{"email":null,"password":null,"exp":null}`), "--bind", "expiration:=/exp"),
		"fragment URL":      flag("--page-url", "https://resy.com/login#top"),
		"relative URL":      flag("--page-url", "/login"),
		"blank browser":     flag("--browser-id", " "),
		"blank tool":        flag("--tool-ref", " "),
		"long tool":         flag("--tool-ref", strings.Repeat("t", 129)),
		"timeout zero":      flag("--timeout-sec", "0"),
		"timeout high":      flag("--timeout-sec", "121"),
		"input conflict":    append(vaultWebMCPArgs(), "--input-file", "-"),
		"too large":         input(`{"email":null,"password":null,"pad":"` + strings.Repeat("x", 64*1024) + `"}`),
		"params open":       {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", `{}`, "--open"},
		"params missing":    {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke"},
		"params type":       {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", `{"type":"fill","browser_id":"b"}`},
		"params unknown":    {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", strings.Replace(resyWebMCPRequest, `"type":"webmcp_invoke"`, `"credential-sentinel":1`, 1)},
		"params values":     {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", `{"browser_id":"b","tool_ref":"t","page_url":"https://resy.com/login","input":{"email":null},"bindings":[{"field":"email","input_path":"/email"}],"values":{"password":"credential-sentinel"}}`},
		"params bindings":   {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", `{"browser_id":"b","tool_ref":"t","page_url":"https://resy.com/login","input":{"email":null},"bindings":[]}`},
		"params too many":   {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", `{"browser_id":"b","tool_ref":"t","page_url":"https://resy.com/login","input":{"a":[` + strings.TrimSuffix(strings.Repeat("null,", 33), ",") + `]},"bindings":[` + vaultWebMCPManyBindings(33) + `]}`},
		"params timeout":    {"vaults", "items", "invoke", "user-vault", "resy", "webmcp_invoke", "--params", `{"browser_id":"b","tool_ref":"t","page_url":"https://resy.com/login","input":{"email":null},"bindings":[{"field":"email","input_path":"/email"}],"timeout_sec":1.5}`},
	} {
		t.Run(name, func(t *testing.T) {
			out, human, err := executeVaultCommand(t, client, args...)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "credential-sentinel")
			assert.Empty(t, out)
			assert.Empty(t, human)
		})
	}
}

func vaultWebMCPManyBindings(n int) string {
	bindings := make([]string, n)
	for i := range bindings {
		bindings[i] = fmt.Sprintf(`{"field":"f%d","input_path":"/a/%d"}`, i, i)
	}
	return strings.Join(bindings, ",")
}

func TestVaultWebMCPInvokeOutcomes(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		name, result, human string
		ok                  bool
	}{
		{"error", `{"type":"webmcp_invoke","status":"error","invocation_id":"invoke-3","error_text":"Invalid password for credential-sentinel\u001b[31m"}`, "may have had side effects", false},
		{"canceled", `{"type":"webmcp_invoke","status":"canceled","invocation_id":"invoke-4"}`, "do not retry automatically", false},
		{"unknown", `{"type":"webmcp_invoke","status":"unknown"}`, "the tool may have run", false},
		{"null output", `{"type":"webmcp_invoke","status":"completed","invocation_id":"invoke-5","output":null}`, "Output (untrusted", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, vaultWebMCPClient(t, resyCredentialFixture, 200, tc.result, &posts))
			out, human, err := executeVaultCommand(t, client, vaultWebMCPArgs("-o", "json")...)
			assert.Equal(t, 1, posts)
			assert.JSONEq(t, tc.result, out)
			assert.Empty(t, human)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.True(t, err.(interface{ Silent() bool }).Silent())
			}

			posts = 0
			out, human, err = executeVaultCommand(t, client, vaultWebMCPArgs()...)
			assert.Equal(t, 1, posts)
			assert.Empty(t, out)
			assert.Contains(t, human, tc.human)
			assert.NotContains(t, human, "\x1b[31m")
			assert.Equal(t, tc.ok, err == nil)
		})
	}
}

func TestVaultWebMCPInvokeRequestErrors(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"target changed", `{"code":"target_changed","message":"credential-sentinel"}`, "target_changed (HTTP 400): the tool_ref is no longer live", 400},
		{"destination", `{"code":"destination_denied"}`, "destination_denied (HTTP 403)", 403},
		{"conflict", `{"code":"conflict"}`, "conflict (HTTP 409)", 409},
		{"unrecognized", `{"code":"credential-sentinel"}`, "webmcp_invoke request failed (HTTP 400); the tool was not invoked", 400},
		{"server", `{"code":"execution_failed"}`, "execution_failed (HTTP 500): WebMCP invocation failed; the tool may have run", 500},
		{"gateway", `credential-sentinel`, "webmcp_invoke request failed (HTTP 502); the tool may have run", 502},
		{"invalid result", `{"type":"fill","status":"completed","fields":[]}`, "invalid webmcp_invoke result; the tool may have run", 200},
		{"invalid status", `{"type":"webmcp_invoke","status":"credential-sentinel"}`, "invalid webmcp_invoke result", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			client := vaultTestClient(t, vaultWebMCPClient(t, resyCredentialFixture, tc.status, tc.body, &posts))
			out, _, err := executeVaultCommand(t, client, vaultWebMCPArgs("-o", "json")...)
			require.Error(t, err)
			assert.Equal(t, 1, posts, "webmcp_invoke must not be retried")
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, err.Error(), "retry")
			assert.NotContains(t, err.Error(), "credential-sentinel")
			assert.Empty(t, out)
		})
	}
}

func TestVaultWebMCPInvokeRequiresAdvertisedOperation(t *testing.T) {
	t.Setenv("KERNEL_PROJECT", "")
	posts := 0
	item := strings.Replace(resyCredentialFixture, `"webmcp_invoke"`, `"fill"`, 1)
	client := vaultTestClient(t, vaultWebMCPClient(t, item, 200, resyWebMCPResult, &posts))
	_, _, err := executeVaultCommand(t, client, vaultWebMCPArgs()...)
	require.ErrorContains(t, err, `operation "webmcp_invoke" is not advertised`)
	assert.Zero(t, posts)

	client = vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"not_found"}`)
	})
	_, _, err = executeVaultCommand(t, client, vaultWebMCPArgs()...)
	require.ErrorContains(t, err, "webmcp_invoke was not invoked")
}
