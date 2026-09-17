package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrAuthorizationDenied indicates that consent was declined, without issuing new credentials.
var ErrAuthorizationDenied = errors.New("authorization denied; no new credentials were saved")

func (oc *OAuthConfig) callbackHandler(codeChan chan<- string, errChan chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		encodedState := query.Get("state")
		var csrfToken, orgID, accessScope, projectID string

		if encodedState != "" {
			if decodedBytes, err := base64.StdEncoding.DecodeString(encodedState); err == nil {
				var stateData map[string]string
				if json.Unmarshal(decodedBytes, &stateData) == nil {
					csrfToken = stateData["csrf"]
					orgID = stateData["org_id"]
					accessScope = stateData["access_scope"]
					projectID = stateData["project_id"]
				}
			}

			// Older callbacks can carry the raw CSRF token instead of encoded state.
			if csrfToken == "" {
				csrfToken = encodedState
			}
		}

		var expectedCSRF string
		if decodedBytes, err := base64.StdEncoding.DecodeString(oc.State); err == nil {
			var stateData map[string]string
			if json.Unmarshal(decodedBytes, &stateData) == nil {
				expectedCSRF = stateData["csrf"]
			}
		}

		if csrfToken != expectedCSRF || expectedCSRF == "" {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			errChan <- fmt.Errorf("invalid state parameter")
			return
		}

		if oauthError := query.Get("error"); oauthError != "" {
			if oauthError == "access_denied" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte(deniedHTML))
				errChan <- ErrAuthorizationDenied
			} else {
				http.Error(w, "Authorization failed. Return to your terminal and try again.", http.StatusBadRequest)
				errChan <- fmt.Errorf("authorization server rejected the request; please try again")
			}
			return
		}

		code := query.Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			errChan <- fmt.Errorf("missing authorization code")
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(successHTML))

		result := AuthResult{
			Code:        code,
			OrgID:       orgID,
			AccessScope: accessScope,
			ProjectID:   projectID,
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			errChan <- fmt.Errorf("failed to encode auth result: %w", err)
			return
		}
		codeChan <- string(resultJSON)
	}
}
