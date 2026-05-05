package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/airbytehq/airbyte-cli/internal/auth"
)

const (
	maxRetries     = 3
	baseRetryDelay = 1 * time.Second
	requestTimeout = 30 * time.Second
)

type Client struct {
	apiHost        string
	organizationID string
	userAgent      string
	tokenManager   *auth.TokenManager
	httpClient     *http.Client
	debug          bool
}

type Option func(*Client)

func WithDebug(debug bool) Option {
	return func(c *Client) {
		c.debug = debug
	}
}

func New(apiHost, organizationID, version string, tm *auth.TokenManager, opts ...Option) *Client {
	c := &Client{
		apiHost:        apiHost,
		organizationID: organizationID,
		userAgent:      "airbyte-cli/" + version,
		tokenManager:   tm,
		httpClient:     &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	u, err := url.Parse(c.apiHost + path)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	if len(params) > 0 {
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	return c.do(ctx, http.MethodGet, u.String(), nil)
}

func (c *Client) Post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.doWithBody(ctx, http.MethodPost, path, body)
}

func (c *Client) Patch(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.doWithBody(ctx, http.MethodPatch, path, body)
}

func (c *Client) Delete(ctx context.Context, path string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodDelete, c.apiHost+path, nil)
}

func (c *Client) GetURL(ctx context.Context, rawURL string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, rawURL, nil)
}

func (c *Client) doWithBody(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
	}
	return c.do(ctx, method, c.apiHost+path, &buf)
}

func (c *Client) do(ctx context.Context, method, rawURL string, body io.Reader) (json.RawMessage, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		result, retryable, err := c.doOnce(ctx, method, rawURL, bodyReader)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}

		if c.debug {
			log.Printf("[DEBUG] request %s %s attempt %d failed: %v", method, rawURL, attempt+1, err)
		}
	}

	return nil, lastErr
}

func (c *Client) doOnce(ctx context.Context, method, rawURL string, body io.Reader) (json.RawMessage, bool, error) {
	token, err := c.tokenManager.GetToken()
	if err != nil {
		return nil, false, fmt.Errorf("getting auth token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, false, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.organizationID != "" {
		req.Header.Set("X-Organization-Id", c.organizationID)
	}

	if c.debug {
		log.Printf("[DEBUG] %s %s", method, rawURL)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("reading response body: %w", err)
	}

	if c.debug {
		log.Printf("[DEBUG] response %d: %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode >= 400 {
		message := extractErrorMessage(respBody, resp.StatusCode)
		apiErr := newAPIError(resp.StatusCode, message, respBody)
		return nil, apiErr.Retryable, apiErr
	}

	if len(respBody) == 0 {
		return json.RawMessage("null"), false, nil
	}

	return json.RawMessage(respBody), false, nil
}

func extractErrorMessage(body []byte, statusCode int) string {
	var parsed struct {
		Detail  string `json:"detail"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Detail != "" {
			return parsed.Detail
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	if len(body) > 0 {
		return string(body)
	}
	return fmt.Sprintf("HTTP %d", statusCode)
}
