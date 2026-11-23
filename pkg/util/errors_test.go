package util

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/stretchr/testify/require"
)

func TestCleanedUpSdkErrorIncludesUnsupportedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":"unsupported_configuration","message":"The requested browser platform does not support this configuration.","unsupported_fields":["headless","memory","region"]}`)
	}))
	defer server.Close()

	client := kernel.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test"),
		option.WithMaxRetries(0),
	)
	_, err := client.Browsers.New(context.Background(), kernel.BrowserNewParams{})
	require.Error(t, err)
	require.EqualError(t, CleanedUpSdkError{Err: err}, "unsupported_configuration: The requested browser platform does not support this configuration. (unsupported fields: headless, memory, region)")
}
