package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fillParamsFixture = `{"browser_id":"browser-session-id","page_url":"https://shop.example/checkout?step=2#payment","fields":[{"field":"number","selector":"#card-number"},{"field":"expiration","format":"MM/YY","selector":"#expiry"},{"field":"cvc","selector":"#security-code"}],"timeout_ms":10000}`
const readyFillCardFixture = `{"id":"item-1","key":"order-1","type":"card","spec":{"provider":"link"},"state":{"provider":"link","status":"ready"},"available_operations":[{"type":"fill","description":"Fill checkout fields."}]}`
const completedFillFixture = `{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled"},{"index":1,"status":"filled"},{"index":2,"status":"filled"}]}`
const failedFillFixture = `{"type":"fill","status":"failed","fields":[{"index":0,"status":"filled"},{"index":1,"status":"failed","error_code":"element_not_found"},{"index":2,"status":"not_attempted"}]}`
const unknownFillFixture = `{"type":"fill","status":"unknown","fields":[{"index":0,"status":"filled"},{"index":1,"status":"unknown","error_code":"timeout"},{"index":2,"status":"not_attempted"}]}`

func TestVaultFillParamsValidation(t *testing.T) {
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid params reached API") })
	replace := func(old, value string) string { return strings.Replace(fillParamsFixture, old, value, 1) }
	for name, raw := range map[string]string{
		"empty": "", "null": "null", "array": "[]", "scalar": `"credential-sentinel"`,
		"malformed": `{"credential-sentinel":`, "trailing": fillParamsFixture + ` {}`,
		"type override":             replace(`"browser_id":`, `"type":"authorize","browser_id":`),
		"same type":                 replace(`"browser_id":`, `"type":"fill","browser_id":`),
		"case variant":              replace(`"browser_id":`, `"Type":"authorize","browser_id":`),
		"duplicate":                 replace(`"browser_id":`, `"browser_id":"credential-sentinel","browser_id":`),
		"unknown key":               replace(`"browser_id":`, `"credential-sentinel":"secret","browser_id":`),
		"missing browser":           replace(`"browser_id":"browser-session-id",`, ""),
		"null browser":              replace(`"browser-session-id"`, `null`),
		"numeric browser":           replace(`"browser-session-id"`, `123`),
		"blank browser":             replace(`"browser-session-id"`, `" "`),
		"http URL":                  replace(`https://`, `http://`),
		"credentials URL":           replace(`https://`, `https://credential-sentinel:secret@`),
		"glob host":                 replace(`shop.example`, `*.example`),
		"missing host":              replace(`shop.example`, ``),
		"bad port":                  replace(`shop.example`, `shop.example:secret`),
		"URL whitespace":            replace(`checkout?`, `checkout ?`),
		"URL type":                  replace(`"https://shop.example/checkout?step=2#payment"`, `123`),
		"empty fields":              replace(`[{"field":"number","selector":"#card-number"},{"field":"expiration","format":"MM/YY","selector":"#expiry"},{"field":"cvc","selector":"#security-code"}]`, `[]`),
		"fields object":             `{"browser_id":"id","page_url":"https://shop.example/","fields":{}}`,
		"missing fields":            `{"browser_id":"id","page_url":"https://shop.example/"}`,
		"null fields":               `{"browser_id":"id","page_url":"https://shop.example/","fields":null}`,
		"null binding":              `{"browser_id":"id","page_url":"https://shop.example/","fields":[null]}`,
		"array binding":             `{"browser_id":"id","page_url":"https://shop.example/","fields":[[]]}`,
		"too many":                  `{"browser_id":"id","page_url":"https://shop.example/","fields":[` + strings.Repeat(`{"field":"number","selector":"#n"},`, 32) + `{"field":"cvc","selector":"#c"}]}`,
		"missing field":             replace(`"field":"number",`, ``),
		"unknown field":             replace(`"number"`, `"credential-sentinel"`),
		"field type":                replace(`"number"`, `{}`),
		"missing selector":          replace(`,"selector":"#card-number"`, ``),
		"null selector":             replace(`"#card-number"`, `null`),
		"empty selector":            replace(`"#card-number"`, `" "`),
		"selector type":             replace(`"#card-number"`, `123`),
		"duplicate field":           replace(`"field":"number"`, `"field":"cvc","field":"number"`),
		"frame ID":                  replace(`"selector":"#card-number"`, `"selector":"#card-number","frame_id":"secret"`),
		"literal value":             replace(`"selector":"#card-number"`, `"selector":"#card-number","value":"credential-sentinel"`),
		"format on stored":          replace(`"field":"number"`, `"field":"number","format":"MM/YY"`),
		"null stored format":        replace(`"field":"number"`, `"field":"number","format":null`),
		"missing expiration format": replace(`,"format":"MM/YY"`, ``),
		"bad format":                replace(`"MM/YY"`, `"credential-sentinel"`),
		"null format":               replace(`"MM/YY"`, `null`),
		"timeout zero":              replace(`10000`, `0`), "timeout high": replace(`10000`, `30001`),
		"negative timeout": replace(`10000`, `-1`), "fractional timeout": replace(`10000`, `1.5`),
		"timeout string": replace(`10000`, `"credential-sentinel"`), "timeout null": replace(`10000`, `null`),
	} {
		t.Run(name, func(t *testing.T) {
			out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", raw, "-o", "json")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "credential-sentinel")
			assert.NotContains(t, err.Error(), "#card-number")
			assert.Empty(t, out)
			assert.Empty(t, human)
		})
	}
	for _, args := range [][]string{
		{"fill"}, {"fill", "--params", fillParamsFixture, "--open"},
		{"fill", "--params", fillParamsFixture, "--open=false"},
		{"authorize", "--params", `{}`}, {"authorize", "--params", ``},
		{"refresh", "--params", `{}`}, {"refresh", "--open"},
	} {
		_, _, err := executeVaultCommand(t, client, append([]string{"vaults", "items", "invoke", "checkout", "order-1"}, args...)...)
		require.Error(t, err)
	}
}

func TestVaultFillBindingsSerialization(t *testing.T) {
	fields := make([]vaultFillField, 0)
	for _, field := range strings.Fields("number cvc exp_month exp_year billing_name billing_line1 billing_line2 billing_city billing_state billing_postal_code billing_country") {
		fields = append(fields, vaultFillField{Field: field, Selector: "#" + field})
	}
	for _, format := range []string{"MM/YY", "MM/YYYY"} {
		fields = append(fields, vaultFillField{Field: "expiration", Format: format, Selector: "#expiry"})
	}
	for len(fields) < 32 {
		fields = append(fields, vaultFillField{Field: "billing_name", Selector: fmt.Sprintf("#billing-name-%d", len(fields))})
	}
	for _, timeout := range []int{0, 1, 30000} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			params := vaultFillParams{BrowserID: "Session-ID", PageURL: "https://shop.example/checkout?step=2#payment", Fields: fields}
			if timeout != 0 {
				params.TimeoutMS = &timeout
			}
			raw, err := json.Marshal(params)
			require.NoError(t, err)
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "Bearer test", r.Header.Get("Authorization"))
				assert.Equal(t, "chosen-project", r.Header.Get("X-Kernel-Project"))
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, readyFillCardFixture)
					return
				}
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/vaults/checkout/items/order-1/operations", r.URL.Path)
				assert.Contains(t, r.Header.Get("Content-Type"), "application/json")
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"type":"fill",`+string(raw[1:]), string(body))
				results := make([]map[string]any, 0, len(fields))
				for i := range fields {
					results = append(results, map[string]any{"index": i, "status": "filled"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"type": "fill", "status": "completed", "fields": results})
			})
			t.Setenv("KERNEL_PROJECT", "other-project")
			out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", string(raw), "--project", "chosen-project", "-o", "json")
			require.NoError(t, err)
			assert.True(t, json.Valid([]byte(out)))
			assert.Empty(t, human)
			assert.Equal(t, 2, calls)
		})
	}
}

func TestVaultFillAdvertisedAvailability(t *testing.T) {
	for name, fixture := range map[string]string{
		"requested":                requestedCardFixture,
		"wallet":                   connectedWalletFixture,
		"agentcard":                strings.ReplaceAll(strings.ReplaceAll(readyFillCardFixture, "link", "agentcard"), `[{"type":"fill","description":"Fill checkout fields."}]`, `[]`),
		"ready unadvertised":       strings.ReplaceAll(readyFillCardFixture, `[{"type":"fill","description":"Fill checkout fields."}]`, `[]`),
		"recovery stale operation": strings.ReplaceAll(readyFillCardFixture, `"ready"`, `"recovery_required"`),
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, fixture)
			})
			out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", fillParamsFixture, "-o", "json")
			require.Error(t, err)
			assert.Empty(t, out)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestVaultFillHumanOutputAndRedaction(t *testing.T) {
	for _, result := range []string{completedFillFixture, failedFillFixture, unknownFillFixture} {
		t.Run(result, func(t *testing.T) {
			response := strings.ReplaceAll(result, `"index":`, `"value":"credential-sentinel","selector":"#secret-selector","dom":{"text":"secret"},"index":`)
			response = strings.Replace(response, `"type":`, `"values":["credential-sentinel"],"type":`, 1)
			client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				body := response
				if r.Method == http.MethodGet {
					body = readyFillCardFixture
				}
				_, _ = io.WriteString(w, body)
			})
			for _, output := range []string{"", "json"} {
				out, human, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", fillParamsFixture, "-o", output)
				if result == completedFillFixture {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				assert.NotContains(t, out+human, "credential-sentinel")
				assert.NotContains(t, out+human, "#secret-selector")
				assert.NotContains(t, out+human, "#card-number")
				if output == "json" {
					assert.JSONEq(t, result, out)
					assert.Empty(t, human)
				} else {
					assert.Contains(t, human, "Field index")
					assert.Contains(t, human, "filled")
					if result != completedFillFixture {
						assert.Contains(t, human, "not_attempted")
						assert.Contains(t, human, "do not retry")
					}
				}
			}
		})
	}
}

func runVaultFillCLI(t *testing.T, serverURL string, args ...string) (string, string, int) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, exe, append([]string{"vaults", "items", "invoke", "checkout", "order-1"}, args...)...)
	command.Env = append(os.Environ(), "KERNEL_CLI_E2E_EXEC=1", "KERNEL_NO_UPDATE_CHECK=1", "KERNEL_API_KEY=test", "KERNEL_BASE_URL="+serverURL, "KERNEL_PROJECT=test-project", "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	require.NoError(t, ctx.Err(), "CLI hung: %s", stderr.String())
	code := 0
	if err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		code = exit.ExitCode()
	}
	return stdout.String(), ansiEscapes.ReplaceAllString(stderr.String(), ""), code
}

func TestVaultFillCLIOutcomesAndFailures(t *testing.T) {
	type testCase struct {
		name, response string
		status, exit   int
		getFailure     bool
	}
	tests := []testCase{
		{"completed", completedFillFixture, 200, 0, false},
		{"failed", failedFillFixture, 200, 1, false},
		{"unknown", unknownFillFixture, 200, 1, false},
		{"wrong union", requestedCardFixture, 200, 1, false},
		{"malformed response", `{"credential-sentinel":`, 200, 1, false},
		{"empty response", `null`, 200, 1, false},
		{"empty item", `null`, 200, 1, true},
		{"malformed item", `{"credential-sentinel":`, 200, 1, true},
		{"malformed operations", strings.Replace(readyFillCardFixture, `"type":"fill"`, `"type":{"credential-sentinel":true}`, 1), 200, 1, true},
		{"empty fields", `{"type":"fill","status":"completed","fields":[]}`, 200, 1, false},
		{"invalid status", strings.ReplaceAll(completedFillFixture, "completed", "credential-sentinel"), 200, 1, false},
		{"missing index", strings.Replace(completedFillFixture, `"index":0,`, ``, 1), 200, 1, false},
		{"wrong index", strings.Replace(completedFillFixture, `"index":0`, `"index":2`, 1), 200, 1, false},
		{"unsafe error code", strings.Replace(failedFillFixture, "element_not_found", "credential-sentinel", 1), 200, 1, false},
		{"invalid completion", strings.Replace(failedFillFixture, `"status":"failed"`, `"status":"completed"`, 1), 200, 1, false},
		{"SDK upgrade", `{"code":"sdk_upgrade_required","message":"credential-sentinel"}`, 400, 1, false},
		{"dropped connection", "", 0, 1, false},
		{"truncated response", "", -1, 1, false},
	}
	for _, status := range []int{400, 401, 403, 404, 409, 429, 500, 503} {
		for _, get := range []bool{false, true} {
			tests = append(tests, testCase{fmt.Sprintf("HTTP %d get=%t", status, get), `{"code":"credential-sentinel","message":"credential-sentinel #secret-selector"}`, status, 1, get})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, "test-project", r.Header.Get("X-Kernel-Project"))
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet && !tt.getFailure {
					_, _ = io.WriteString(w, readyFillCardFixture)
					return
				}
				if tt.status <= 0 {
					conn, buf, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					if tt.status == -1 {
						_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{\"secret\":")
						_ = buf.Flush()
					}
					_ = conn.Close()
					return
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()
			out, stderr, exit := runVaultFillCLI(t, server.URL, "fill", "--params", fillParamsFixture, "-o", "json")
			assert.Equal(t, tt.exit, exit, "stderr=%s", stderr)
			wantCalls := int32(2)
			if tt.getFailure {
				wantCalls = 1
			}
			assert.Equal(t, wantCalls, calls.Load(), "must not retry or perform other actions")
			assert.NotContains(t, out+stderr, "credential-sentinel")
			assert.NotContains(t, out+stderr, "#secret-selector")
			switch tt.name {
			case "completed", "failed", "unknown":
				assert.JSONEq(t, tt.response, out)
				assert.Empty(t, stderr)
			default:
				assert.Empty(t, out)
				assert.NotEmpty(t, stderr)
				if !tt.getFailure {
					assert.Contains(t, stderr, "may have been written")
				}
			}
		})
	}
}

func TestVaultFillCLIValidationAndAuthorizeCompatibility(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.JSONEq(t, `{"type":"authorize"}`, string(body))
		}
		_, _ = io.WriteString(w, requestedCardFixture)
	}))
	defer server.Close()
	for _, raw := range []string{`{"credential-sentinel":`, `{"type":"credential-sentinel"}`, `[]`, ``} {
		out, stderr, exit := runVaultFillCLI(t, server.URL, "fill", "--params", raw, "-o", "json")
		assert.Equal(t, 1, exit)
		assert.Empty(t, out)
		assert.NotContains(t, stderr, "credential-sentinel")
		assert.NotEmpty(t, stderr)
	}
	assert.Zero(t, calls.Load())
	out, stderr, exit := runVaultFillCLI(t, server.URL, "authorize", "--open", "-o", "json")
	assert.Zero(t, exit)
	assert.Empty(t, stderr)
	assert.JSONEq(t, requestedCardFixture, out)
	assert.Equal(t, int32(2), calls.Load())
}

func TestVaultFillSingleFieldAndHint(t *testing.T) {
	const params = `{"browser_id":"session-id","page_url":"https://shop.example/","fields":[{"field":"billing_country","selector":"select[name=\"country\"]"}]}`
	const result = `{"type":"fill","status":"completed","fields":[{"index":0,"status":"filled"}]}`
	client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"fill",`+params[1:], string(body))
			_, _ = io.WriteString(w, result)
			return
		}
		_, _ = io.WriteString(w, readyFillCardFixture)
	})
	out, _, err := executeVaultCommand(t, client, "vaults", "items", "invoke", "checkout", "order-1", "fill", "--params", params, "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, result, out)
	_, human, err := executeVaultCommand(t, client, "vaults", "items", "get", "checkout", "order-1", "--project", "chosen-project")
	require.NoError(t, err)
	assert.Contains(t, human, "Invoke: kernel vaults items invoke --project=chosen-project --params '<json>' -- checkout order-1 fill")
}

func TestVaultAuthorizeRejectsFillResponse(t *testing.T) {
	for _, response := range []string{completedFillFixture, "null"} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, requestedCardFixture)
					return
				}
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()
			out, stderr, exit := runVaultFillCLI(t, server.URL, "authorize", "-o", "json")
			assert.Equal(t, 1, exit)
			assert.Empty(t, out)
			assert.Contains(t, strings.ToLower(stderr), "unexpected vault operation response")
		})
	}
}
