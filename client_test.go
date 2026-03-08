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
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	t.Run("Default Client", func(t *testing.T) {
		client := NewClient("http://localhost:8080", "test-key")
		expectedBase := "http://localhost:8080/api/v1"
		if client.BaseURL != expectedBase {
			t.Errorf("Expected %s, got %s", expectedBase, client.BaseURL)
		}
	})

	t.Run("WithVersion", func(t *testing.T) {
		client := NewClient("http://localhost:8080", "test-key", WithVersion("v2"))
		expectedBase := "http://localhost:8080/api/v2"
		if client.BaseURL != expectedBase {
			t.Errorf("Expected %s, got %s", expectedBase, client.BaseURL)
		}
	})

	t.Run("WithHTTPClient", func(t *testing.T) {
		custom := &http.Client{Timeout: 42 * time.Second}
		client := NewClient("http://localhost", "key", WithHTTPClient(custom))
		if client.HTTPClient.Timeout != 42*time.Second {
			t.Errorf("Expected 42s timeout, got %v", client.HTTPClient.Timeout)
		}
		// Ensure transport is wrapped
		if _, ok := client.HTTPClient.Transport.(*TokenRefreshTransport); !ok {
			t.Error("Expected Transport to be TokenRefreshTransport")
		}
	})

	t.Run("WithBaseTransport", func(t *testing.T) {
		recorder := &requestRecorder{base: http.DefaultTransport}
		client := NewClient("http://localhost", "key", WithBaseTransport(recorder))
		
		transport, ok := client.HTTPClient.Transport.(*TokenRefreshTransport)
		if !ok {
			t.Fatal("Expected TokenRefreshTransport")
		}
		if transport.base != recorder {
			t.Error("Expected base transport to be our recorder")
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

	client := NewClient("http://openrelik.local", "test-key", WithHTTPClient(custom))

	if custom.Transport != originalTransport {
		t.Error("Original client's Transport was modified!")
	}
	if client.HTTPClient.Timeout != 42*time.Second {
		t.Errorf("Expected timeout 42s, got %v", client.HTTPClient.Timeout)
	}

	// 2. Auth headers should only be added to OpenRelik host
	var lastReq *http.Request
	recorder := RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		lastReq = req
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
	})

	client.HTTPClient.Transport.(*TokenRefreshTransport).base = recorder

	t.Run("OpenRelik Host", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://openrelik.local/api/v1/test", nil)
		client.HTTPClient.Do(req)

		if lastReq.Header.Get("x-openrelik-refresh-token") != "test-key" {
			t.Error("Missing auth header for OpenRelik host")
		}
	})

	t.Run("Other Host", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://google.com/search", nil)
		client.HTTPClient.Do(req)

		if lastReq.Header.Get("x-openrelik-refresh-token") != "" {
			t.Error("Auth header leaked to unrelated host!")
		}
	})
}

func TestRoundTrip_RedirectLeakage(t *testing.T) {
	// Start two servers: one representing OpenRelik, one representing an external host.
	externalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-openrelik-refresh-token") != "" {
			t.Error("Auth header leaked to external host!")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer externalServer.Close()

	relikServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header is present on the initial request.
		if r.Header.Get("x-openrelik-refresh-token") != "test-key" {
			t.Error("Missing auth header on initial request")
		}
		// Redirect to external host.
		http.Redirect(w, r, externalServer.URL, http.StatusFound)
	}))
	defer relikServer.Close()

	client := NewClient(relikServer.URL, "test-key")
	
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

type requestRecorder struct {
	base    http.RoundTripper
	sampled bool
}

func (r *requestRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.sampled = true
	return r.base.RoundTrip(req)
}

func TestNewRequest(t *testing.T) {
	c := NewClient("http://localhost", "key")
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

	client := NewClient(server.URL, "key")
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
		})

		req, _ := client.NewRequest(ctx, http.MethodGet, "/error", nil)
		_, err := client.Do(req, nil)
		if err == nil {
			t.Error("Expected error for 400 status code")
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
		if r.Header.Get("x-openrelik-refresh-token") != "valid-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"new_access_token": "new-token"}`)
	})

	callCount := 0
	mux.HandleFunc("/api/v1/resource", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("x-openrelik-access-token") != "new-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	client := NewClient(server.URL, "valid-key")
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

	client := NewClient(server.URL, "key")
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

	client := NewClient(server.URL, "key")
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
		client := NewClient("http://localhost", "key")
		client.HTTPClient.Transport.(*TokenRefreshTransport).base = &errorTransport{}
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
