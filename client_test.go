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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	t.Run("Default Client", func(t *testing.T) {
		client, err := NewClient("http://localhost:8080", "test-key")
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		expectedServer := "http://localhost:8080"
		if client.serverURL.String() != expectedServer {
			t.Errorf("Expected %s, got %s", expectedServer, client.serverURL.String())
		}
		if client.apiVersion != defaultAPIVersion {
			t.Errorf("Expected version %s, got %s", defaultAPIVersion, client.apiVersion)
		}
		if client.maxResponseSize != defaultMaxResponseSize {
			t.Errorf("Expected default MaxResponseSize %d, got %d", defaultMaxResponseSize, client.maxResponseSize)
		}
	})

	t.Run("WithVersion", func(t *testing.T) {
		client, err := NewClient("http://localhost:8080", "test-key", WithVersion("v2"))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if client.apiVersion != "v2" {
			t.Errorf("Expected version v2, got %s", client.apiVersion)
		}
	})

	t.Run("WithHTTPClient", func(t *testing.T) {
		custom := &http.Client{Timeout: 42 * time.Second}
		client, err := NewClient("http://localhost", "key", WithHTTPClient(custom))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if client.httpClient.Timeout != 42*time.Second {
			t.Errorf("Expected 42s timeout, got %v", client.httpClient.Timeout)
		}
		// Ensure transport is wrapped
		if _, ok := client.httpClient.Transport.(*tokenRefreshTransport); !ok {
			t.Error("Expected Transport to be tokenRefreshTransport")
		}
	})

	t.Run("WithBaseTransport", func(t *testing.T) {
		recorder := &requestRecorder{base: http.DefaultTransport}
		client, err := NewClient("http://localhost", "key", WithBaseTransport(recorder))
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		transport, ok := client.httpClient.Transport.(*tokenRefreshTransport)
		if !ok {
			t.Fatal("Expected tokenRefreshTransport")
		}
		if transport.base != recorder {
			t.Error("Expected base transport to be our recorder")
		}
	})

	t.Run("WithUserAgent", func(t *testing.T) {
		client, _ := NewClient("http://localhost", "key", WithUserAgent("custom-ua"))
		if client.userAgent != "custom-ua" {
			t.Errorf("Expected custom-ua, got %s", client.userAgent)
		}
	})

	t.Run("WithMaxResponseSize", func(t *testing.T) {
		client, _ := NewClient("http://localhost", "key", WithMaxResponseSize(1234))
		if client.maxResponseSize != 1234 {
			t.Errorf("Expected 1234, got %d", client.maxResponseSize)
		}
	})

	t.Run("Invalid URL", func(t *testing.T) {
		_, err := NewClient(":", "key")
		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
	})
}

func TestMaxResponseSize(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(server.URL, "key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	// Set a very small limit for testing
	client.maxResponseSize = 5
	ctx := context.Background()

	t.Run("Response too large", func(t *testing.T) {
		mux.HandleFunc("/api/v1/too-large", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "this is more than 5 bytes")
		})

		req, _ := client.NewRequest(ctx, http.MethodGet, "/too-large", nil)
		_, err := client.Do(req, nil)
		if err == nil {
			t.Fatal("Expected error for response exceeding MaxResponseSize")
		}
		expectedErr := "openrelik: response body too large (limit 5 bytes)"
		if err.Error() != expectedErr {
			t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("Response within limit", func(t *testing.T) {
		mux.HandleFunc("/api/v1/within-limit", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "12345")
		})

		req, _ := client.NewRequest(ctx, http.MethodGet, "/within-limit", nil)
		resp, err := client.Do(req, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "12345" {
			t.Errorf("Expected '12345', got %q", string(body))
		}
	})

	t.Run("Unlimited (MaxResponseSize = 0)", func(t *testing.T) {
		client.maxResponseSize = 0
		mux.HandleFunc("/api/v1/unlimited", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "this is now allowed")
		})

		req, _ := client.NewRequest(ctx, http.MethodGet, "/unlimited", nil)
		resp, err := client.Do(req, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "this is now allowed" {
			t.Errorf("Expected 'this is now allowed', got %q", string(body))
		}
	})
}

func TestWithHTTPClient_SideEffects(t *testing.T) {
	// 1. Original client should not be modified
	originalTransport := &http.Transport{}
	custom := &http.Client{
		Timeout:   42 * time.Second,
		Transport: originalTransport,
	}

	client, err := NewClient("http://openrelik.local", "test-key", WithHTTPClient(custom))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if custom.Transport != originalTransport {
		t.Error("Original client's Transport was modified!")
	}
	if client.httpClient.Timeout != 42*time.Second {
		t.Errorf("Expected timeout 42s, got %v", client.httpClient.Timeout)
	}

	// 2. Auth headers should only be added to OpenRelik host
	var lastReq *http.Request
	recorder := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		lastReq = req
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
	})

	client.httpClient.Transport.(*tokenRefreshTransport).base = recorder

	t.Run("OpenRelik Host", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://openrelik.local/api/v1/test", nil)
		client.httpClient.Do(req)

		if lastReq.Header.Get(headerRefreshToken) != "test-key" {
			t.Error("Missing auth header for OpenRelik host")
		}
	})

	t.Run("Other Host", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://google.com/search", nil)
		client.httpClient.Do(req)

		if lastReq.Header.Get(headerRefreshToken) != "" {
			t.Error("Auth header leaked to unrelated host!")
		}
	})

	t.Run("Scheme Mismatch (HTTPS -> HTTP)", func(t *testing.T) {
		// Configure client for HTTPS
		client, _ := NewClient("https://openrelik.local", "test-key")

		var lastReq *http.Request
		recorder := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			lastReq = req
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
		})
		client.httpClient.Transport.(*tokenRefreshTransport).base = recorder

		// Request to same host but using HTTP
		req, _ := http.NewRequest(http.MethodGet, "http://openrelik.local/api/v1/test", nil)
		client.httpClient.Do(req)

		if lastReq.Header.Get(headerRefreshToken) != "" {
			t.Error("Auth header leaked to insecure HTTP connection on same host!")
		}
	})
}

func TestRoundTrip_RedirectLeakage(t *testing.T) {
	// Start two servers: one representing OpenRelik, one representing an external host.
	externalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(headerRefreshToken) != "" {
			t.Error("Auth header leaked to external host!")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer externalServer.Close()

	relikServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header is present on the initial request.
		if r.Header.Get(headerRefreshToken) != "test-key" {
			t.Error("Missing auth header on initial request")
		}
		// Redirect to external host.
		http.Redirect(w, r, externalServer.URL, http.StatusFound)
	}))
	defer relikServer.Close()

	client, err := NewClient(relikServer.URL, "test-key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx := context.Background()
	req, _ := client.NewRequest(ctx, http.MethodGet, "/redirect", nil)
	resp, err := client.Do(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK from final destination, got %d", resp.StatusCode)
	}
}

func TestRoundTrip_TokenLeakage(t *testing.T) {
	var retryCount int
	var leakedToken string

	// Malicious external server that returns 401 to trigger refresh/retry
	maliciousServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount++
		leakedToken = r.Header.Get(headerAccessToken)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer maliciousServer.Close()

	// OpenRelik server for token refresh
	relikServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/refresh" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"new_access_token": "secret-token"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer relikServer.Close()

	client, err := NewClient(relikServer.URL, "test-key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Use the client to make a request to the MALICIOUS server
	req, _ := http.NewRequest(http.MethodGet, maliciousServer.URL, nil)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if leakedToken != "" {
		t.Errorf("Token was leaked to malicious server: %s", leakedToken)
	}
}

type requestRecorder struct {
	base    http.RoundTripper
	sampled bool
}

func (r *requestRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.sampled = true
	return r.base.RoundTrip(req)
}

func TestNewRequest(t *testing.T) {
	c, err := NewClient("http://localhost", "key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	ctx := context.Background()

	t.Run("With Body", func(t *testing.T) {
		type testBody struct {
			Name string `json:"name"`
		}
		body := &testBody{Name: "test"}

		req, err := c.NewRequest(ctx, http.MethodPost, "/test", body)
		if err != nil {
			t.Fatal(err)
		}

		if req.URL.String() != "http://localhost/api/v1/test" {
			t.Errorf("URL mismatch, got %s", req.URL.String())
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Error("Content-Type header not set")
		}
		if req.GetBody == nil {
			t.Error("Expected GetBody to be set for retries")
		}
	})

	t.Run("Without Body", func(t *testing.T) {
		req, err := c.NewRequest(ctx, http.MethodGet, "/test", nil)
		if err != nil {
			t.Fatal(err)
		}
		if req.Body != nil {
			t.Error("Expected nil body")
		}
	})

	t.Run("Marshal Error", func(t *testing.T) {
		// Unsupported type for JSON marshaling (channels)
		_, err := c.NewRequest(ctx, http.MethodPost, "/test", make(chan int))
		if err == nil {
			t.Error("Expected error for invalid JSON body type")
		}
	})
}

func TestDo(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(server.URL, "key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	ctx := context.Background()

	t.Run("Success with Decode", func(t *testing.T) {
		mux.HandleFunc("/api/v1/success", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": 1, "username": "testuser"}`)
		})

		type user struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
		}
		var u user
		req, _ := client.NewRequest(ctx, http.MethodGet, "/success", nil)
		_, err := client.Do(req, &u)
		if err != nil {
			t.Fatal(err)
		}
		if u.Username != "testuser" {
			t.Errorf("Expected testuser, got %s", u.Username)
		}
	})

	t.Run("Success without Decode (Raw)", func(t *testing.T) {
		mux.HandleFunc("/api/v1/raw", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "raw data")
		})

		req, _ := client.NewRequest(ctx, http.MethodGet, "/raw", nil)
		resp, err := client.Do(req, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "raw data" {
			t.Errorf("Expected 'raw data', got %s", string(body))
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux.HandleFunc("/api/v1/error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "error detail")
		})

		req, _ := client.NewRequest(ctx, http.MethodGet, "/error", nil)
		_, err := client.Do(req, nil)
		if err == nil {
			t.Error("Expected error for 400 status code")
		}

		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("Expected error to be of type *Error, got %T", err)
		}
		if apiErr.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected StatusCode 400, got %d", apiErr.StatusCode)
		}
		if string(apiErr.Body) != "error detail" {
			t.Errorf("Expected Body 'error detail', got %q", string(apiErr.Body))
		}

		expectedMsgPrefix := "openrelik: GET http://"
		if !strings.HasPrefix(apiErr.Error(), expectedMsgPrefix) {
			t.Errorf("Expected error message to start with %q, got %q", expectedMsgPrefix, apiErr.Error())
		}
		if !strings.Contains(apiErr.Error(), "400 Bad Request") {
			t.Errorf("Expected error message to contain '400 Bad Request', got %q", apiErr.Error())
		}
	})

	t.Run("Structured API Error (detail)", func(t *testing.T) {
		mux.HandleFunc("/api/v1/structured-error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail": "object not found"}`)
		})

		req, _ := client.NewRequest(ctx, http.MethodGet, "/structured-error", nil)
		_, err := client.Do(req, nil)

		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("Expected *Error, got %T", err)
		}
		if apiErr.Message != "object not found" {
			t.Errorf("Expected message 'object not found', got %q", apiErr.Message)
		}
		if !strings.Contains(apiErr.Error(), "object not found") {
			t.Errorf("Expected Error() to contain message, got %q", apiErr.Error())
		}
	})

	t.Run("Readable API Error (Stripped Query)", func(t *testing.T) {
		mux.HandleFunc("/api/v1/query-error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"detail": "invalid input"}`)
		})

		// Use a URL with query parameters
		req, _ := client.NewRequest(ctx, http.MethodGet, "/query-error?long_param=very_long_value&other=123", nil)
		_, err := client.Do(req, nil)

		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("Expected *Error, got %T", err)
		}

		errStr := apiErr.Error()
		if strings.Contains(errStr, "very_long_value") {
			t.Errorf("Error message should NOT contain query parameters: %q", errStr)
		}
		if !strings.Contains(errStr, "/api/v1/query-error") {
			t.Errorf("Error message should contain the path: %q", errStr)
		}
		if !strings.Contains(errStr, "invalid input") {
			t.Errorf("Error message should contain API error message: %q", errStr)
		}
	})

	t.Run("Decode Error with Unwrap", func(t *testing.T) {
		mux.HandleFunc("/api/v1/bad-json", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": "not-a-number"}`)
		})

		type data struct {
			ID int `json:"id"`
		}
		var d data
		req, _ := client.NewRequest(ctx, http.MethodGet, "/bad-json", nil)
		resp, err := client.Do(req, &d)
		if err == nil {
			t.Fatal("Expected error for invalid JSON, got nil")
		}

		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("Expected *Error wrapping the decode error, got %T", err)
		}

		if apiErr.Cause == nil {
			t.Fatal("Expected Cause to be set on decode error")
		}

		if !errors.Is(err, apiErr.Cause) {
			t.Error("errors.Is failed to identify the Cause")
		}

		if resp == nil {
			t.Fatal("Expected response to be returned even on decode error")
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body after decode error: %v", err)
		}
		if string(body) != `{"id": "not-a-number"}` {
			t.Errorf("Unexpected body content: %s", string(body))
		}
	})
}

func TestTokenRefresh(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	refreshCount := 0
	mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		refreshCount++
		if r.Header.Get(headerRefreshToken) != "valid-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"new_access_token": "new-token"}`)
	})

	callCount := 0
	mux.HandleFunc("/api/v1/resource", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get(headerAccessToken) != "new-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	client, err := NewClient(server.URL, "valid-key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	ctx := context.Background()

	// First call triggers 401 -> refresh -> retry
	resp, err := client.Get(ctx, "/resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if refreshCount != 1 {
		t.Errorf("Expected 1 refresh call, got %d", refreshCount)
	}
	if callCount != 2 {
		t.Errorf("Expected 2 resource calls (original + retry), got %d", callCount)
	}
}

func TestClient_LowLevelMethods(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(server.URL, "key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	ctx := context.Background()

	type testResource struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	mux.HandleFunc("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id": 1, "name": "created"}`)
		case http.MethodPut:
			fmt.Fprint(w, `{"id": 1, "name": "updated"}`)
		case http.MethodPatch:
			fmt.Fprint(w, `{"id": 1, "name": "patched"}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	t.Run("Post", func(t *testing.T) {
		var res testResource
		resp, err := client.Post(ctx, "/test", map[string]string{"name": "new"}, &res)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected 201, got %d", resp.StatusCode)
		}
		if res.Name != "created" {
			t.Errorf("Expected name created, got %s", res.Name)
		}
	})

	t.Run("Put", func(t *testing.T) {
		var res testResource
		_, err := client.Put(ctx, "/test", map[string]string{"name": "update"}, &res)
		if err != nil {
			t.Fatal(err)
		}
		if res.Name != "updated" {
			t.Errorf("Expected name updated, got %s", res.Name)
		}
	})

	t.Run("Patch", func(t *testing.T) {
		var res testResource
		_, err := client.Patch(ctx, "/test", map[string]string{"name": "patch"}, &res)
		if err != nil {
			t.Fatal(err)
		}
		if res.Name != "patched" {
			t.Errorf("Expected name patched, got %s", res.Name)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		resp, err := client.Delete(ctx, "/test", nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected 204, got %d", resp.StatusCode)
		}
	})
}

func TestClient_Errors(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(server.URL, "key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	ctx := context.Background()

	t.Run("Invalid JSON Response", func(t *testing.T) {
		mux.HandleFunc("/api/v1/invalid-json", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": "not-an-int"}`)
		})

		type data struct {
			ID int `json:"id"`
		}
		var d data
		_, err := client.Get(ctx, "/invalid-json", &d)
		if err == nil {
			t.Error("Expected error for invalid JSON mapping")
		}
	})

	t.Run("Request Creation Error", func(t *testing.T) {
		_, err := client.NewRequest(ctx, " INVALID METHOD ", "/test", nil)
		if err == nil {
			t.Error("Expected error for invalid HTTP method")
		}
	})

	t.Run("Network Error", func(t *testing.T) {
		client, _ := NewClient("http://localhost", "key")
		client.httpClient.Transport.(*tokenRefreshTransport).base = &errorTransport{}
		_, err := client.Get(ctx, "/test", nil)
		if err == nil {
			t.Error("Expected error for network failure")
		}
	})
}

type errorTransport struct{}

func (e *errorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("forced network error")
}

type RoundTripFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
