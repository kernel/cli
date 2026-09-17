package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOAuthCallback(t *testing.T) {
	state := base64.StdEncoding.EncodeToString([]byte(`{"csrf":"test-csrf"}`))
	scopedState := base64.StdEncoding.EncodeToString([]byte(`{"csrf":"test-csrf","org_id":"org_test","access_scope":"project","project_id":"proj_test"}`))

	tests := []struct {
		name, state, code, oauthError string
		status                        int
		wantError                     string
		denied                        bool
		wantResult                    AuthResult
	}{
		{
			name: "success", state: state, code: "test-code", status: http.StatusOK,
			wantResult: AuthResult{Code: "test-code"},
		},
		{
			name: "legacy scoped state", state: scopedState, code: "test-code", status: http.StatusOK,
			wantResult: AuthResult{Code: "test-code", OrgID: "org_test", AccessScope: "project", ProjectID: "proj_test"},
		},
		{
			name: "legacy raw csrf", state: "test-csrf", code: "test-code", status: http.StatusOK,
			wantResult: AuthResult{Code: "test-code"},
		},
		{
			name: "denied", state: state, oauthError: "access_denied", status: http.StatusOK,
			wantError: ErrAuthorizationDenied.Error(), denied: true,
		},
		{
			name: "error takes precedence over code", state: state, code: "test-code", oauthError: "access_denied", status: http.StatusOK,
			wantError: ErrAuthorizationDenied.Error(), denied: true,
		},
		{
			name: "server error", state: state, code: "test-code", oauthError: "server_error", status: http.StatusBadRequest,
			wantError: "authorization server rejected the request; please try again",
		},
		{
			name: "untrusted error", state: state, oauthError: "<script>untrusted-error</script>", status: http.StatusBadRequest,
			wantError: "authorization server rejected the request; please try again",
		},
		{
			name: "wrong state before denial", state: "wrong-state", oauthError: "access_denied", status: http.StatusBadRequest,
			wantError: "invalid state parameter",
		},
		{
			name: "missing state before denial", oauthError: "access_denied", status: http.StatusBadRequest,
			wantError: "invalid state parameter",
		},
		{
			name: "missing code without error", state: state, status: http.StatusBadRequest,
			wantError: "missing authorization code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &OAuthConfig{State: state}
			codeChan := make(chan string, 1)
			errChan := make(chan error, 1)
			query := url.Values{
				"state": {tt.state}, "code": {tt.code}, "error": {tt.oauthError},
				"error_description": {"untrusted-description"},
			}
			response := httptest.NewRecorder()
			config.callbackHandler(codeChan, errChan).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?"+query.Encode(), nil))

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d", response.Code, tt.status)
			}
			body := response.Body.String()
			for _, untrusted := range []string{"untrusted-description", "untrusted-error"} {
				if strings.Contains(body, untrusted) {
					t.Fatalf("callback reflected untrusted input: %s", untrusted)
				}
			}

			if tt.wantError != "" {
				select {
				case err := <-errChan:
					if err.Error() != tt.wantError || errors.Is(err, ErrAuthorizationDenied) != tt.denied {
						t.Fatalf("error = %v, want %q (denied=%v)", err, tt.wantError, tt.denied)
					}
				default:
					t.Fatal("callback did not return an error")
				}
				if len(codeChan) != 0 {
					t.Fatal("error callback supplied a code for token exchange")
				}
				if tt.denied {
					if response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
						t.Fatal("denial page is not HTML")
					}
					if !strings.Contains(body, "authorization denied") || !strings.Contains(body, "no new access was granted") {
						t.Fatal("denial page is missing its explanation")
					}
					if strings.Contains(body, "authentication successful") || strings.Contains(body, `<div class="check-container">`) {
						t.Fatal("denial page contains success content")
					}
				}
				return
			}

			if len(errChan) != 0 {
				t.Fatalf("successful callback returned an error: %v", <-errChan)
			}
			select {
			case resultJSON := <-codeChan:
				var result AuthResult
				if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
					t.Fatal(err)
				}
				if result != tt.wantResult {
					t.Fatalf("result = %+v, want %+v", result, tt.wantResult)
				}
			default:
				t.Fatal("successful callback did not supply a code")
			}
			if !strings.Contains(body, "authentication successful") || !strings.Contains(body, `<div class="check-container">`) {
				t.Fatal("success page is missing its existing content")
			}
		})
	}
}

func TestCallbackPagesEmbedFavicon(t *testing.T) {
	for _, page := range []string{successHTML, deniedHTML} {
		if !strings.Contains(html.UnescapeString(page), `href="data:image/svg+xml;base64,`) || strings.Contains(page, "ZgotmplZ") {
			t.Fatal("callback page must inline the trusted favicon")
		}
	}
}
