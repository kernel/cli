package browserimport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL   string
	token     string
	projectID string
	http      *http.Client
}

type responseError struct {
	status  int
	message string
}

func (e *responseError) Error() string {
	if e.message != "" {
		return fmt.Sprintf("Kernel API returned %d: %s", e.status, e.message)
	}
	return fmt.Sprintf("Kernel API returned %d", e.status)
}

func NewClient(baseURL, token, projectID string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Kernel API URL")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("Kernel API URL must not contain credentials, a path, query, or fragment")
	}
	local := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1")
	official := parsed.Scheme == "https" && (parsed.Hostname() == "api.onkernel.com" || strings.HasSuffix(parsed.Hostname(), ".onkernel.com"))
	if !local && !official {
		return nil, fmt.Errorf("Kernel API URL must use HTTPS or local development")
	}
	parsed.Path = ""
	return &Client{baseURL: strings.TrimRight(parsed.String(), "/"), token: token, projectID: projectID, http: &http.Client{
		Timeout:       15 * time.Minute,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) Create(ctx context.Context) (CreateResponse, error) {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return CreateResponse{}, fmt.Errorf("create browser import idempotency key: %w", err)
	}
	idempotencyKey := hex.EncodeToString(keyBytes)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/browser-imports", nil)
		if err != nil {
			return CreateResponse{}, err
		}
		c.setHeaders(request, c.token)
		request.Header.Set("Idempotency-Key", idempotencyKey)
		response, err := c.http.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		var result CreateResponse
		err = decodeResponse(response, &result)
		response.Body.Close()
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return CreateResponse{}, lastErr
}

func (c *Client) SubmitInventory(ctx context.Context, id, helperToken string, inventory Inventory) (Status, error) {
	var result Status
	err := c.doJSON(ctx, http.MethodPost, "/browser-imports/"+url.PathEscape(id)+"/inventory", helperToken, inventory, &result)
	if err != nil {
		return c.reconcile(ctx, id, err, "awaiting_selection", "awaiting_bundle", "staged", "applying", "completed")
	}
	return result, err
}

func (c *Client) SubmitSelection(ctx context.Context, id string, selection Selection) (Status, error) {
	var result Status
	err := c.doJSON(ctx, http.MethodPost, "/browser-imports/"+url.PathEscape(id)+"/selection", c.token, selection, &result)
	if err != nil {
		return c.reconcile(ctx, id, err, "awaiting_bundle", "staged", "applying", "completed")
	}
	return result, err
}

func (c *Client) Upload(ctx context.Context, id, helperToken string, bundle []byte) (Status, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/browser-imports/"+url.PathEscape(id)+"/bundle", bytes.NewReader(bundle))
	if err != nil {
		return Status{}, err
	}
	c.setHeaders(request, helperToken)
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := c.http.Do(request)
	if err != nil {
		return c.reconcile(ctx, id, err, "staged", "applying", "completed", "failed")
	}
	defer response.Body.Close()
	var result Status
	if err := decodeResponse(response, &result); err != nil {
		return c.reconcile(ctx, id, err, "staged", "applying", "completed", "failed")
	}
	return result, nil
}

func (c *Client) reconcile(ctx context.Context, id string, requestErr error, advancedPhases ...string) (Status, error) {
	status, statusErr := c.Status(ctx, id)
	if statusErr != nil {
		return Status{}, errors.Join(requestErr, fmt.Errorf("check browser import %s after request failure: %w", id, statusErr))
	}
	for _, phase := range advancedPhases {
		if status.Phase == phase {
			return status, nil
		}
	}
	return status, requestErr
}

func (c *Client) Status(ctx context.Context, id string) (Status, error) {
	var result Status
	err := c.doJSON(ctx, http.MethodGet, "/browser-imports/"+url.PathEscape(id), c.token, nil, &result)
	return result, err
}

func (c *Client) Wait(ctx context.Context, id string, interval time.Duration) (Status, error) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	consecutiveErrors := 0
	for {
		status, err := c.Status(ctx, id)
		if err != nil {
			consecutiveErrors++
			if !transientStatusError(err) || consecutiveErrors >= 3 {
				return Status{}, err
			}
		} else {
			consecutiveErrors = 0
			switch status.Phase {
			case "completed":
				return status, nil
			case "failed":
				if status.Applied != nil && status.Applied.Failure != nil {
					return status, fmt.Errorf("browser import failed during %s: %s", status.Applied.Failure.Stage, status.Applied.Failure.Message)
				}
				return status, fmt.Errorf("browser import failed")
			}
		}
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func transientStatusError(err error) bool {
	var responseErr *responseError
	if !errors.As(err, &responseErr) {
		return true
	}
	return responseErr.status == http.StatusRequestTimeout || responseErr.status == http.StatusTooManyRequests || responseErr.status >= 500
}

func (c *Client) doJSON(ctx context.Context, method, path, token string, body any, output any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	c.setHeaders(request, token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return decodeResponse(response, output)
}

func (c *Client) setHeaders(request *http.Request, token string) {
	request.Header.Set("Authorization", "Bearer "+token)
	if c.projectID != "" {
		request.Header.Set("X-Kernel-Project-Id", c.projectID)
	}
}

func decodeResponse(response *http.Response, output any) error {
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var apiError struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &apiError) == nil && apiError.Message != "" {
			return &responseError{status: response.StatusCode, message: apiError.Message}
		}
		return &responseError{status: response.StatusCode}
	}
	if output == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode Kernel API response: %w", err)
	}
	return nil
}
