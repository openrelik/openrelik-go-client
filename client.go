// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openrelik

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"
)

const (
	defaultAPIVersion      = "v1"
	defaultUserAgent       = "openrelik-go-client/1.0"
	tokenRefreshTimeout    = 10 * time.Second
	defaultMaxResponseSize = 10 * 1024 * 1024 // 10MB

	headerRefreshToken = "x-openrelik-refresh-token"
	headerAccessToken  = "x-openrelik-access-token"
)

// A Client is a reusable API client for OpenRelik.
type Client struct {
	// baseURL is the versioned root endpoint for the API.
	baseURL string

	// httpClient is the underlying client used for all network I/O.
	httpClient *http.Client

	// userAgent is the string sent in the User-Agent header.
	userAgent string

	// maxResponseSize is the maximum size in bytes that the response body can be.
	// If 0, no limit is applied. Defaults to 10MB.
	maxResponseSize int64

	// Services used for communicating with different parts of the OpenRelik API.
	users *UsersService
}

// Users returns the service for communicating with user-related methods of the OpenRelik API.
func (c *Client) Users() *UsersService {
	return c.users
}

// Option defines a functional option for configuring the Client.
type Option func(*Client) error

// WithHTTPClient allows the user to provide their own pre-configured http.Client.
// The client's Transport will be wrapped by the OpenRelik TokenRefreshTransport.
// A shallow copy of the provided client is made to avoid side effects on the original.
// If WithBaseTransport is also provided, it will override the Transport of this client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) error {
		if httpClient == nil {
			return nil
		}
		// Create a shallow copy of the client to avoid side effects on the original.
		cl := *httpClient
		c.httpClient = &cl
		return nil
	}
}

// WithBaseTransport allows the user to provide a custom base RoundTripper
// (e.g., for proxies or custom TLS settings) while keeping automatic auth.
// This overrides the Transport of the http.Client (including one provided by WithHTTPClient).
func WithBaseTransport(base http.RoundTripper) Option {
	return func(c *Client) error {
		if base == nil {
			return nil
		}
		if c.httpClient == nil {
			c.httpClient = &http.Client{}
		}
		c.httpClient.Transport = base
		return nil
	}
}

// WithUserAgent sets the User-Agent header for the client.
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		c.userAgent = ua
		return nil
	}
}

// WithMaxResponseSize sets the maximum size in bytes that the response body can be.
// If 0, no limit is applied.
func WithMaxResponseSize(size int64) Option {
	return func(c *Client) error {
		c.maxResponseSize = size
		return nil
	}
}

// WithVersion sets the API version to use.
func WithVersion(version string) Option {
	return func(c *Client) error {
		u, err := url.Parse(c.baseURL)
		if err != nil {
			return fmt.Errorf("openrelik: failed to parse baseURL in WithVersion: %w", err)
		}
		// Replace the last element of the path with the new version.
		// e.g. /api/v1 -> /api/v2
		u.Path = path.Join(path.Dir(u.Path), version)
		c.baseURL = u.String()
		return nil
	}
}

// NewClient initializes a new OpenRelik client with functional options.
// apiServerURL: The root URL of the OpenRelik server (e.g., http://localhost:8710).
// apiKey: The long-lived refresh token used for authentication.
func NewClient(apiServerURL, apiKey string, opts ...Option) (*Client, error) {
	u, err := url.Parse(apiServerURL)
	if err != nil {
		return nil, fmt.Errorf("openrelik: failed to parse API server URL: %w", err)
	}

	c := &Client{
		baseURL: u.JoinPath("api", defaultAPIVersion).String(),
		httpClient: &http.Client{
			Transport: http.DefaultTransport,
		},
		userAgent:       defaultUserAgent,
		maxResponseSize: defaultMaxResponseSize,
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	// Wrap the transport with token refresh logic
	base := c.httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	refreshURL, err := url.JoinPath(apiServerURL, "auth/refresh")
	if err != nil {
		return nil, fmt.Errorf("openrelik: could not construct refresh URL: %w", err)
	}

	c.httpClient.Transport = &tokenRefreshTransport{
		refreshURL: refreshURL,
		host:       u.Host,
		scheme:     u.Scheme,
		apiKey:     apiKey,
		base:       base,
	}

	// Initialize services
	c.users = &UsersService{client: c}

	return c, nil
}

// --- Low-Level HTTP Methods ---

func (c *Client) Get(ctx context.Context, endpoint string, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

func (c *Client) Post(ctx context.Context, endpoint string, body any, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

func (c *Client) Put(ctx context.Context, endpoint string, body any, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPut, endpoint, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

func (c *Client) Patch(ctx context.Context, endpoint string, body any, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPatch, endpoint, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

func (c *Client) Delete(ctx context.Context, endpoint string, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

// NewRequest handles JSON marshaling, context attachment, and header setup.
func (c *Client) NewRequest(ctx context.Context, method, endpoint string, body any) (*http.Request, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}

	// Ensure endpoint doesn't escape the baseURL path.
	// JoinPath cleans the path.
	fullPath, err := url.JoinPath(u.Path, endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = fullPath

	var buf io.ReadSeeker
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.GetBody = func() (io.ReadCloser, error) {
			_, _ = buf.Seek(0, io.SeekStart)
			return io.NopCloser(buf), nil
		}
	}

	return req, nil
}

// Error represents an error returned by the OpenRelik API.
type Error struct {
	// Response is the HTTP response that caused the error.
	Response *http.Response

	// StatusCode is the HTTP status code of the response.
	StatusCode int

	// Body is the raw response body.
	Body []byte
}

func (e *Error) Error() string {
	if e.Response != nil && e.Response.Request != nil {
		return fmt.Sprintf("openrelik: %s %s: %d %s",
			e.Response.Request.Method,
			e.Response.Request.URL,
			e.StatusCode,
			e.Response.Status)
	}
	return fmt.Sprintf("openrelik: api error: %d", e.StatusCode)
}

// Do executes the HTTP request and decodes the response into v if provided.
func (c *Client) Do(req *http.Request, v any) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read body into memory to allow inspection if decoding fails.
	// We use a LimitedReader to prevent OOM if the response is too large.
	var reader io.Reader = resp.Body
	if c.maxResponseSize > 0 {
		reader = io.LimitReader(resp.Body, c.maxResponseSize+1)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return resp, err
	}

	if c.maxResponseSize > 0 && int64(len(data)) > c.maxResponseSize {
		return resp, fmt.Errorf("openrelik: response body too large (limit %d bytes)", c.maxResponseSize)
	}

	// Re-populate the body so the caller can still read it
	resp.Body = io.NopCloser(bytes.NewBuffer(data))

	if resp.StatusCode >= 400 {
		return resp, &Error{
			Response:   resp,
			StatusCode: resp.StatusCode,
			Body:       data,
		}
	}

	if v != nil {
		if err := json.Unmarshal(data, v); err != nil {
			return resp, err
		}
	}

	return resp, nil
}

// tokenRefreshTransport handles automatic auth and concurrent token refresh.
type tokenRefreshTransport struct {
	refreshURL  string
	host        string
	scheme      string
	apiKey      string
	accessToken string
	mu          sync.RWMutex
	base        http.RoundTripper
}

// RoundTrip adds authentication headers and handles token refresh on 401 responses.
func (t *tokenRefreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.RLock()
	accessToken := t.accessToken
	t.mu.RUnlock()

	// Only add auth headers if the request host and scheme match the API server.
	// We clone the request before adding headers to avoid modifying the original
	// request and to prevent credentials from being leaked during redirects
	// (Go's http.Client copies headers from the original request, not the clone).
	if t.host != "" && req.URL.Host == t.host && req.URL.Scheme == t.scheme {
		req = req.Clone(req.Context())
		if t.apiKey != "" {
			req.Header.Set(headerRefreshToken, t.apiKey)
		}
		if accessToken != "" {
			req.Header.Set(headerAccessToken, accessToken)
		}
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && t.host != "" && req.URL.Host == t.host && req.URL.Scheme == t.scheme {
		if req.URL.String() == t.refreshURL {
			return resp, nil
		}

		resp.Body.Close()

		// Perform refresh with a write lock
		newAccessToken, err := t.refreshIfStale(accessToken)
		if err != nil {
			return nil, err
		}

		newReq := req.Clone(req.Context())
		newReq.Header.Set(headerAccessToken, newAccessToken)

		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			newReq.Body = body
		}

		return t.base.RoundTrip(newReq)
	}

	return resp, nil
}

// refreshIfStale ensures only one concurrent refresh happens at a time.
func (t *tokenRefreshTransport) refreshIfStale(failedToken string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If the token changed while we were waiting for the lock, someone else refreshed it.
	if t.accessToken != failedToken && t.accessToken != "" {
		return t.accessToken, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), tokenRefreshTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.refreshURL, nil)
	if err != nil {
		return "", err
	}
	if t.apiKey != "" {
		req.Header.Set(headerRefreshToken, t.apiKey)
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrelik: failed to refresh token: %s", resp.Status)
	}

	var result struct {
		NewAccessToken string `json:"new_access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	t.accessToken = result.NewAccessToken
	return t.accessToken, nil
}
