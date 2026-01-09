package extensions

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestWebBotAuthDownloadable verifies that the web-bot-auth package can be downloaded from GitHub
func TestWebBotAuthDownloadable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, webBotAuthDownloadURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to download web-bot-auth: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify Content-Type indicates a zip file
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/zip" && contentType != "application/x-zip-compressed" {
		t.Logf("Warning: unexpected Content-Type: %s (expected application/zip)", contentType)
	}

	// Verify Content-Length is reasonable (should be at least 1KB)
	contentLength := resp.ContentLength
	if contentLength > 0 && contentLength < 1024 {
		t.Fatalf("Content-Length too small: %d bytes (expected at least 1KB)", contentLength)
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

	if err != nil {
		t.Fatalf("Failed to download and extract web-bot-auth: %v", err)
	}

	if browserExtDir == "" {
		t.Fatal("Expected non-empty browser extension directory path")
	}

	t.Logf("Successfully downloaded and extracted to: %s", browserExtDir)
}
