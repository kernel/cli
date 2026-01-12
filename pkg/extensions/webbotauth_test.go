package extensions

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebBotAuthDownloadable verifies that the web-bot-auth package can be downloaded from GitHub
func TestWebBotAuthDownloadable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, webBotAuthDownloadURL, nil)
	require.NoError(t, err, "Failed to create request")

	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to download web-bot-auth")
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

// TestDownloadAndExtractWebBotAuth tests the full download and extraction process
func TestDownloadAndExtractWebBotAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	browserExtDir, cleanup, err := downloadAndExtractWebBotAuth(ctx)
	defer cleanup()

	require.NoError(t, err, "Failed to download and extract web-bot-auth")
	require.NotEmpty(t, browserExtDir, "Expected non-empty browser extension directory path")

	t.Logf("Successfully downloaded and extracted to: %s", browserExtDir)
}
