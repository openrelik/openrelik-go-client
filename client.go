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
	"sync"
	"time"
)

const (
	defaultAPIVersion      = "v1"
	defaultUserAgent       = "openrelik-go-client/1.0"
	defaultMaxResponseSize = 10 * 1024 * 1024 // 10MB
	tokenRefreshTimeout    = 10 * time.Second

	headerRefreshToken = "x-openrelik-refresh-token"
	headerAccessToken  = "x-openrelik-access-token"
)

// A Client is a reusable API client for OpenRelik.
type Client struct {
	// serverURL is the root endpoint for the OpenRelik server.
	serverURL *url.URL

	// apiVersion is the version of the API to use (e.g. "v1").
	apiVersion string

	// httpClient is the underlying client used for all network I/O.
	httpClient *http.Client

	// baseTransport is a custom RoundTripper provided via WithBaseTransport.
	baseTransport http.RoundTripper

	// userAgent is the string sent in the User-Agent header.
	userAgent string

	// maxResponseSize is the maximum size in bytes that the response body can be.
	// If 0, no limit is applied. Defaults to 10MB.
	maxResponseSize int64

	// Services used for communicating with different parts of the OpenRelik API.
	users     *UsersService
	folders   *FoldersService
	files     *FilesService
	workflows *WorkflowsService
	workers   *WorkersService
}

// Users returns the service for communicating with user-related methods of the OpenRelik API.
func (c *Client) Users() *UsersService {
	return c.users
}

// Folders returns the service for communicating with folder-related methods of the OpenRelik API.
func (c *Client) Folders() *FoldersService {
	return c.folders
}

// Files returns the service for communicating with file-related methods of the OpenRelik API.
func (c *Client) Files() *FilesService {
	return c.files
}

// Workflows returns the service for communicating with workflow-related methods of the OpenRelik API.
func (c *Client) Workflows() *WorkflowsService {
	return c.workflows

// Workers returns the service for communicating with worker-related methods of the OpenRelik API.
func (c *Client) Workers() *WorkersService {
	return c.workers
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
		c.baseTransport = base
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
		c.apiVersion = version
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
		serverURL:  u,
		apiVersion: defaultAPIVersion,
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

	// Apply custom base transport if provided
	if c.baseTransport != nil {
		c.httpClient.Transport = c.baseTransport
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
	c.folders = &FoldersService{client: c}
	c.files = &FilesService{client: c}
	c.workflows = &WorkflowsService{client: c}
	c.workers = &WorkersService{client: c}

	return c, nil
}

// Get performs an authenticated GET request to the given endpoint path,
// relative to the versioned API root (e.g. /api/v1/). It is an escape hatch for
// endpoints not yet covered by a service — prefer the typed service methods
// (e.g. Users()) when available. v, if non-nil, is populated by JSON-decoding
// the response body.
func (c *Client) Get(ctx context.Context, endpoint string, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

// Post performs an authenticated POST request. See Get for usage notes.
func (c *Client) Post(ctx context.Context, endpoint string, body any, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

// Put performs an authenticated PUT request. See Get for usage notes.
func (c *Client) Put(ctx context.Context, endpoint string, body any, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPut, endpoint, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

// Patch performs an authenticated PATCH request. See Get for usage notes.
func (c *Client) Patch(ctx context.Context, endpoint string, body any, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPatch, endpoint, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

// Delete performs an authenticated DELETE request. See Get for usage notes.
func (c *Client) Delete(ctx context.Context, endpoint string, v any) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req, v)
}

// NewRequest handles JSON marshaling, context attachment, and header setup.
func (c *Client) NewRequest(ctx context.Context, method, endpoint string, body any) (*http.Request, error) {
	u := c.serverURL.JoinPath("api", c.apiVersion, endpoint)

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

	// Message is a human-readable error message from the API, if any.
	Message string

	// Cause is the underlying error, if any.
	Cause error
}

func (e *Error) Error() string {
	var msg string
	if e.Message != "" {
		msg = fmt.Sprintf(": %s", e.Message)
	}

	var cause string
	if e.Cause != nil {
		cause = fmt.Sprintf(" (cause: %v)", e.Cause)
	}

	if e.Response != nil && e.Response.Request != nil {
		return fmt.Sprintf("openrelik: %s %s: %s%s%s",
			e.Response.Request.Method,
			e.Response.Request.URL,
			e.Response.Status,
			msg,
			cause)
	}
	return fmt.Sprintf("openrelik: api error: %d%s%s", e.StatusCode, msg, cause)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// Do executes the HTTP request and decodes the response into v if provided.
// If an error occurs during the request or while reading the body, a non-nil
// *http.Response may still be returned alongside the error if one was received
// from the server, allowing callers to inspect status codes or headers.
func (c *Client) Do(req *http.Request, v any) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	// We defer closing the original response body. It is important to note that
	// we replace resp.Body with a new NopCloser wrapping the buffered data
	// before returning, so the caller can still read the response body.
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
		return resp, c.newError(resp, data)
	}

	if v != nil {
		if err := json.Unmarshal(data, v); err != nil {
			return resp, &Error{
				Response:   resp,
				StatusCode: resp.StatusCode,
				Body:       data,
				Cause:      err,
			}
		}
	}

	return resp, nil
}

// newError creates a structured Error from the provided response and body data.
func (c *Client) newError(resp *http.Response, data []byte) *Error {
	apiErr := &Error{
		Response:   resp,
		StatusCode: resp.StatusCode,
		Body:       data,
	}

	// Attempt to decode structured error message.
	var errorResponse struct {
		Detail  any `json:"detail"`
		Message any `json:"message"`
	}
	if err := json.Unmarshal(data, &errorResponse); err == nil {
		if errorResponse.Detail != nil {
			apiErr.Message = fmt.Sprint(errorResponse.Detail)
		} else if errorResponse.Message != nil {
			apiErr.Message = fmt.Sprint(errorResponse.Message)
		}
	}

	return apiErr
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

func (t *tokenRefreshTransport) isOwnHost(u *url.URL) bool {
	return t.host != "" && u.Host == t.host && u.Scheme == t.scheme
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
	if t.isOwnHost(req.URL) {
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

	if resp.StatusCode == http.StatusUnauthorized && t.isOwnHost(req.URL) {
		if req.URL.String() == t.refreshURL {
			return resp, nil
		}

		resp.Body.Close()

		// Perform refresh with a write lock
		newAccessToken, err := t.refreshIfStale(req.Context(), accessToken)
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
func (t *tokenRefreshTransport) refreshIfStale(ctx context.Context, failedToken string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If the token changed while we were waiting for the lock, someone else refreshed it.
	if t.accessToken != failedToken && t.accessToken != "" {
		return t.accessToken, nil
	}

	ctx, cancel := context.WithTimeout(ctx, tokenRefreshTimeout)
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
