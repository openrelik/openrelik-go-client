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

const defaultAPIVersion = "v1"
const userAgent = "openrelik-go-client/1.0"
const tokenRefreshTimeout = 10 * time.Second

// A Client is a reusable API client for OpenRelik.
type Client struct {
	// BaseURL is the versioned root endpoint for the API.
	BaseURL string

	// HTTPClient is the underlying client used for all network I/O.
	HTTPClient *http.Client

	// UserAgent is the string sent in the User-Agent header.
	UserAgent string

	// Services used for communicating with different parts of the OpenRelik API.
	Users *UsersService
}

// Option defines a functional option for configuring the Client.
type Option func(*Client)

// WithHTTPClient allows the user to provide their own pre-configured http.Client.
// The client's Transport will be wrapped by the OpenRelik TokenRefreshTransport.
// A shallow copy of the provided client is made to avoid side effects on the original.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient == nil {
			return
		}

		// Create a shallow copy of the client to avoid side effects on the original.
		cl := *httpClient

		base := cl.Transport
		if base == nil {
			base = http.DefaultTransport
		}

		// Preserve auth configuration from the default transport
		if t, ok := c.HTTPClient.Transport.(*TokenRefreshTransport); ok {
			cl.Transport = &TokenRefreshTransport{
				apiServerURL: t.apiServerURL,
				apiHost:      t.apiHost,
				apiKey:       t.apiKey,
				base:         base,
			}
		}

		c.HTTPClient = &cl
	}
}

// WithBaseTransport allows the user to provide a custom base RoundTripper
// (e.g., for proxies or custom TLS settings) while keeping automatic auth.
func WithBaseTransport(base http.RoundTripper) Option {
	return func(c *Client) {
		if base == nil {
			return
		}
		if t, ok := c.HTTPClient.Transport.(*TokenRefreshTransport); ok {
			t.base = base
		}
	}
}

// WithVersion sets the API version to use.
func WithVersion(version string) Option {
	return func(c *Client) {
		u, err := url.Parse(c.BaseURL)
		if err != nil {
			panic(fmt.Sprintf("openrelik: failed to parse BaseURL in WithVersion: %v", err))
		}
		// Replace the last element of the path with the new version.
		// e.g. /api/v1 -> /api/v2
		u.Path = path.Join(path.Dir(u.Path), version)
		c.BaseURL = u.String()
	}
}

// NewClient initializes a new OpenRelik client with functional options.
// apiServerURL: The root URL of the OpenRelik server (e.g., http://localhost:8710).
// apiKey: The long-lived refresh token used for authentication.
func NewClient(apiServerURL, apiKey string, opts ...Option) (*Client, error) {
	baseURL, err := url.JoinPath(apiServerURL, "api", defaultAPIVersion)
	if err != nil {
		return nil, fmt.Errorf("openrelik: invalid API server URL: %w", err)
	}

	u, err := url.Parse(apiServerURL)
	if err != nil {
		return nil, fmt.Errorf("openrelik: failed to parse API server URL: %w", err)
	}
	var apiHost string
	if u != nil {
		apiHost = u.Host
	}

	transport := &TokenRefreshTransport{
		apiServerURL: apiServerURL,
		apiHost:      apiHost,
		apiKey:       apiKey,
		base:         http.DefaultTransport,
	}

	c := &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Transport: transport,
		},
		UserAgent: userAgent,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Initialize services
	c.Users = &UsersService{client: c}

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
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	u.Path, err = url.JoinPath(u.Path, endpoint)
	if err != nil {
		return nil, err
	}

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

	req.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.GetBody = func() (io.ReadCloser, error) {
			_, _ = buf.Seek(0, io.SeekStart)
			return io.NopCloser(buf), nil
		}
	}

	return req, nil
}

// Do executes the HTTP request and decodes the response into v if provided.
func (c *Client) Do(req *http.Request, v any) (*http.Response, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Read body into memory to allow inspection if decoding fails
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return resp, err
	}
	// Re-populate the body so the caller can still read it
	resp.Body = io.NopCloser(bytes.NewBuffer(data))

	if resp.StatusCode >= 400 {
		return resp, fmt.Errorf("openrelik: api error: %s", resp.Status)
	}

	if v != nil {
		if err := json.Unmarshal(data, v); err != nil {
			return resp, err
		}
	}

	return resp, nil
}

// TokenRefreshTransport handles automatic auth and concurrent token refresh.
type TokenRefreshTransport struct {
	apiServerURL string
	apiHost      string
	apiKey       string
	accessToken  string
	mu           sync.RWMutex
	base         http.RoundTripper
}

// RoundTrip adds authentication headers and handles token refresh on 401 responses.
func (t *TokenRefreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.RLock()
	accessToken := t.accessToken
	t.mu.RUnlock()

	// Only add auth headers if the request host matches the API server host.
	// We clone the request before adding headers to avoid modifying the original
	// request and to prevent credentials from being leaked during redirects
	// (Go's http.Client copies headers from the original request, not the clone).
	if t.apiHost != "" && req.URL.Host == t.apiHost {
		req = req.Clone(req.Context())
		if t.apiKey != "" {
			req.Header.Set("x-openrelik-refresh-token", t.apiKey)
		}
		if accessToken != "" {
			req.Header.Set("x-openrelik-access-token", accessToken)
		}
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		refreshURL, err := url.JoinPath(t.apiServerURL, "auth/refresh")
		if err != nil {
			return nil, fmt.Errorf("openrelik: could not construct refresh URL: %w", err)
		}
		if req.URL.String() == refreshURL {
			return resp, nil
		}

		resp.Body.Close()

		// Perform refresh with a write lock
		newAccessToken, err := t.refreshIfStale(accessToken)
		if err != nil {
			return nil, err
		}

		newReq := req.Clone(req.Context())
		newReq.Header.Set("x-openrelik-access-token", newAccessToken)

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
func (t *TokenRefreshTransport) refreshIfStale(failedToken string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If the token changed while we were waiting for the lock, someone else refreshed it.
	if t.accessToken != failedToken && t.accessToken != "" {
		return t.accessToken, nil
	}

	refreshURL, err := url.JoinPath(t.apiServerURL, "auth/refresh")
	if err != nil {
		return "", fmt.Errorf("openrelik: invalid refresh URL: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), tokenRefreshTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, refreshURL, nil)
	if err != nil {
		return "", err
	}
	if t.apiKey != "" {
		req.Header.Set("x-openrelik-refresh-token", t.apiKey)
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
