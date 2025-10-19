// Copyright (c) HashiCorp, Inc.

package sdk

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config captures the inputs required to construct a Forward Networks API client.
type Config struct {
	BaseURL   string
	APIKey    string
	Insecure  bool
	UserAgent string

	HTTPClient *http.Client
	MaxRetries int
	RetryDelay time.Duration
}

// Client is a thin wrapper around http.Client that ensures each request targets
// the configured Forward Networks appliance and carries the correct headers.
type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
	apiKey     string
	userAgent  string
	maxRetries int
	retryDelay time.Duration
}

// NewClient validates the configuration and instantiates a new Client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	_ = ctx // reserved for future use when requests require context during initialization.
	if cfg.BaseURL == "" {
		return nil, errors.New("base URL must be provided")
	}

	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse base URL: %w", err)
	}

	if parsed.Scheme == "" {
		return nil, errors.New("base URL must include an HTTP or HTTPS scheme")
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")

	if cfg.APIKey == "" {
		return nil, errors.New("API key must be provided")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	if cfg.Insecure {
		transport := httpClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}

		if t, ok := transport.(*http.Transport); ok {
			clone := t.Clone()
			if clone.TLSClientConfig == nil {
				clone.TLSClientConfig = &tls.Config{}
			}
			clone.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- controlled via provider config for testing only.
			httpClient.Transport = clone
		}
	}

	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = "terraform-provider-forward/dev"
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries == 0 {
		maxRetries = 3
	}

	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 500 * time.Millisecond
	}

	client := &Client{
		httpClient: httpClient,
		baseURL:    parsed,
		apiKey:     cfg.APIKey,
		userAgent:  userAgent,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}

	return client, nil
}

// NewRequest creates an HTTP request that points at the configured Forward Networks base URL.
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if c == nil {
		return nil, errors.New("client is nil")
	}

	rel, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("unable to parse request path: %w", err)
	}

	target := c.baseURL.ResolveReference(rel)

	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// Do executes the provided HTTP request using the underlying client.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("client is nil")
	}

	attempt := 0
	var lastErr error

	for {
		if attempt > 0 && req.Body != nil && req.GetBody != nil {
			rc, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("reset request body: %w", err)
			}
			req.Body = rc
		}

		resp, err := c.httpClient.Do(req)
		if err == nil && !shouldRetryStatus(resp.StatusCode) {
			return resp, nil
		}

		if err != nil {
			lastErr = err
		} else {
			// Consume and close before retrying.
			io.Copy(io.Discard, resp.Body) // best effort
			resp.Body.Close()
			lastErr = fmt.Errorf("received status %d", resp.StatusCode)
		}

		if attempt >= c.maxRetries {
			return nil, lastErr
		}

		attempt++
		backoff := c.retryDelay * time.Duration(1<<uint(attempt-1))

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(backoff):
		}
	}
}

func shouldRetryStatus(status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status >= 500 && status != http.StatusNotImplemented {
		return true
	}
	return false
}
