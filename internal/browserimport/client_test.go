package browserimport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientRunsBrowserImportLifecycleWithScopedTokens(t *testing.T) {
	var statusCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "proj_test", request.Header.Get("X-Kernel-Project-Id"))
		switch request.URL.Path {
		case "/browser-imports":
			assert.Equal(t, "Bearer user-token", request.Header.Get("Authorization"))
			response.WriteHeader(http.StatusCreated)
			fmt.Fprint(response, `{"id":"imp_1","helper_token":"helper-token","helper_token_expires_at":"2030-01-01T00:00:00Z"}`)
		case "/browser-imports/imp_1/inventory":
			assert.Equal(t, "Bearer helper-token", request.Header.Get("Authorization"))
			response.WriteHeader(http.StatusAccepted)
			fmt.Fprint(response, `{"id":"imp_1","phase":"awaiting_selection"}`)
		case "/browser-imports/imp_1/selection":
			assert.Equal(t, "Bearer user-token", request.Header.Get("Authorization"))
			response.WriteHeader(http.StatusAccepted)
			fmt.Fprint(response, `{"id":"imp_1","phase":"awaiting_bundle"}`)
		case "/browser-imports/imp_1/bundle":
			assert.Equal(t, "Bearer helper-token", request.Header.Get("Authorization"))
			assert.Equal(t, "application/octet-stream", request.Header.Get("Content-Type"))
			response.WriteHeader(http.StatusAccepted)
			fmt.Fprint(response, `{"id":"imp_1","phase":"staged"}`)
		case "/browser-imports/imp_1":
			assert.Equal(t, "Bearer user-token", request.Header.Get("Authorization"))
			phase := "applying"
			if statusCalls.Add(1) > 1 {
				phase = "completed"
			}
			fmt.Fprintf(response, `{"id":"imp_1","phase":%q}`, phase)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "user-token", "proj_test")
	require.NoError(t, err)
	created, err := client.Create(context.Background())
	require.NoError(t, err)
	_, err = client.SubmitInventory(context.Background(), created.ID, created.HelperToken, Inventory{Sources: []Source{{ID: "chrome", Kind: "browser", Name: "Chrome", DataTypes: []string{"cookies"}}}})
	require.NoError(t, err)
	_, err = client.SubmitSelection(context.Background(), created.ID, Selection{Profiles: []ProfileSelection{}, CredentialSources: []string{}})
	require.NoError(t, err)
	_, err = client.Upload(context.Background(), created.ID, created.HelperToken, []byte("bundle"))
	require.NoError(t, err)
	status, err := client.Wait(context.Background(), created.ID, time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, "completed", status.Phase)
}

func TestClientRejectsUntrustedPlaintextAPI(t *testing.T) {
	_, err := NewClient("http://api.example.com", "token", "")
	assert.EqualError(t, err, "Kernel API URL must use HTTPS or local development")
}

func TestClientRejectsUntrustedHTTPSAPI(t *testing.T) {
	_, err := NewClient("https://evil.example", "token", "")
	assert.EqualError(t, err, "Kernel API URL must use HTTPS or local development")
}

func TestClientRejectsAPIURLWithPath(t *testing.T) {
	_, err := NewClient("https://api.onkernel.com/steal", "token", "")
	assert.EqualError(t, err, "Kernel API URL must not contain credentials, a path, query, or fragment")
}

func TestClientAcceptsOfficialAPIURLWithTrailingSlash(t *testing.T) {
	_, err := NewClient("https://api.onkernel.com/", "token", "")
	require.NoError(t, err)
}

func TestSubmitInventoryReconcilesAcceptedResponseLoss(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "POST /browser-imports/imp_1/inventory":
			requests.Add(1)
			hijacker := response.(http.Hijacker)
			connection, _, err := hijacker.Hijack()
			require.NoError(t, err)
			connection.Close()
		case "GET /browser-imports/imp_1":
			fmt.Fprint(response, `{"id":"imp_1","phase":"awaiting_selection"}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "user-token", "")
	require.NoError(t, err)
	status, err := client.SubmitInventory(context.Background(), "imp_1", "helper-token", Inventory{Sources: []Source{{ID: "chrome"}}})
	require.NoError(t, err)
	assert.Equal(t, "awaiting_selection", status.Phase)
	assert.EqualValues(t, 1, requests.Load())
}
