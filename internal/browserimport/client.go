package browserimport

import (
	"bytes"
	"context"
	"encoding/json"
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

func NewClient(baseURL, token, projectID string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Kernel API URL")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1")) {
		return nil, fmt.Errorf("Kernel API URL must use HTTPS or local development")
	}
	return &Client{baseURL: strings.TrimRight(parsed.String(), "/"), token: token, projectID: projectID, http: &http.Client{Timeout: 15 * time.Minute}}, nil
}

func (c *Client) Create(ctx context.Context) (CreateResponse, error) {
	var result CreateResponse
	err := c.doJSON(ctx, http.MethodPost, "/browser-imports", c.token, nil, &result)
	return result, err
}

func (c *Client) SubmitInventory(ctx context.Context, id, helperToken string, inventory Inventory) (Status, error) {
	var result Status
	err := c.doJSON(ctx, http.MethodPost, "/browser-imports/"+url.PathEscape(id)+"/inventory", helperToken, inventory, &result)
	return result, err
}

func (c *Client) SubmitSelection(ctx context.Context, id string, selection Selection) (Status, error) {
	var result Status
	err := c.doJSON(ctx, http.MethodPost, "/browser-imports/"+url.PathEscape(id)+"/selection", c.token, selection, &result)
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
		return Status{}, err
	}
	defer response.Body.Close()
	var result Status
	if err := decodeResponse(response, &result); err != nil {
		return Status{}, err
	}
	return result, nil
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
	for {
		status, err := c.Status(ctx, id)
		if err != nil {
			return Status{}, err
		}
		switch status.Phase {
		case "completed":
			return status, nil
		case "failed":
			if status.Applied != nil && status.Applied.Failure != nil {
				return status, fmt.Errorf("browser import failed during %s: %s", status.Applied.Failure.Stage, status.Applied.Failure.Message)
			}
			return status, fmt.Errorf("browser import failed")
		}
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-ticker.C:
		}
	}
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
			return fmt.Errorf("Kernel API returned %d: %s", response.StatusCode, apiError.Message)
		}
		return fmt.Errorf("Kernel API returned %d", response.StatusCode)
	}
	if output == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode Kernel API response: %w", err)
	}
	return nil
}
