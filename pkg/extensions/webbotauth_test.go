package extensions

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// GitHub builds archives on demand and is sometimes slow to start responding,
	// so allow a generous budget per attempt and retry before giving up
	downloadAttemptTimeout = 60 * time.Second
	downloadAttempts       = 3
)

// skipIfNetworkUnavailable skips the test when err comes from transient network trouble
// rather than a problem with web-bot-auth itself. These tests reach github.com, which is
// not always reachable from CI.
func skipIfNetworkUnavailable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) {
		t.Skipf("Skipping: github.com is unreachable: %v", err)
	}
}

// TestWebBotAuthDownloadable verifies that the web-bot-auth package can be downloaded from GitHub
func TestWebBotAuthDownloadable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	resp := getWebBotAuthArchive(t)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status 200")

	// Verify Content-Type indicates a zip file
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/zip" && contentType != "application/x-zip-compressed" {
		t.Logf("Warning: unexpected Content-Type: %s (expected application/zip)", contentType)
	}

	// Verify Content-Length is reasonable (should be at least 1KB)
	contentLength := resp.ContentLength
	if contentLength > 0 {
		assert.GreaterOrEqual(t, contentLength, int64(1024), "Content-Length should be at least 1KB")
	}

	t.Logf("Successfully verified web-bot-auth is downloadable")
	t.Logf("Content-Type: %s", contentType)
	t.Logf("Content-Length: %d bytes", contentLength)
}

// getWebBotAuthArchive requests the archive, retrying while github.com fails to respond
func getWebBotAuthArchive(t *testing.T) *http.Response {
	t.Helper()

	client := &http.Client{Timeout: downloadAttemptTimeout}

	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), downloadAttemptTimeout)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, webBotAuthDownloadURL, nil)
		require.NoError(t, err, "Failed to create request")

		resp, err := client.Do(req)
		if err == nil {
			// The context has to stay alive while the caller reads the response
			t.Cleanup(cancel)
			return resp
		}

		cancel()
		lastErr = err
		t.Logf("Attempt %d of %d failed to download web-bot-auth: %v", attempt, downloadAttempts, err)
	}

	skipIfNetworkUnavailable(t, lastErr)
	require.NoError(t, lastErr, "Failed to download web-bot-auth")
	return nil
}

// TestDownloadAndExtractWebBotAuth tests the full download and extraction process
func TestDownloadAndExtractWebBotAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	browserExtDir, cleanup, err := downloadAndExtractWebBotAuth(ctx)
	defer cleanup()

	skipIfNetworkUnavailable(t, err)
	require.NoError(t, err, "Failed to download and extract web-bot-auth")
	require.NotEmpty(t, browserExtDir, "Expected non-empty browser extension directory path")

	t.Logf("Successfully downloaded and extracted to: %s", browserExtDir)
}
