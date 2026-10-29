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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFoldersService_CreateCaseFolder(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected method POST, got %s", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			if body["display_name"] != "ACME" || body["case_ref"] != "CASE 123" {
				t.Errorf("Unexpected body: %v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id": 10, "display_name": "ACME", "case_ref": "case-123"}`)
		})

		folder, _, err := client.Folders().CreateCaseFolder(ctx, "CASE 123", "ACME")
		if err != nil {
			t.Fatalf("CreateCaseFolder returned error: %v", err)
		}
		if folder.ID != 10 || folder.CaseRef != "case-123" {
			t.Errorf("Unexpected folder: %+v", folder)
		}
	})

	t.Run("Older server creates an ordinary folder", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id": 10, "display_name": "ACME"}`)
		})

		folder, _, err := client.Folders().CreateCaseFolder(ctx, "case-123", "ACME")
		if !errors.Is(err, ErrCaseFoldersUnsupported) {
			t.Fatalf("Expected ErrCaseFoldersUnsupported, got %v", err)
		}
		if folder == nil || folder.ID != 10 {
			t.Errorf("Expected the created folder, got %+v", folder)
		}
	})

	t.Run("Taken reference", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"detail": "Case case-123 already exists. Ask your OpenRelik admin for access."}`)
		})

		_, _, err := client.Folders().CreateCaseFolder(ctx, "case-123", "ACME")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
			t.Fatalf("Expected a 409 *Error, got %v", err)
		}
		if errors.Is(err, ErrCaseFoldersUnsupported) {
			t.Error("A conflict must not read as an unsupported server")
		}
	})
}

func TestFoldersService_GetCaseFolder(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/case/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			if got := r.URL.EscapedPath(); got != "/api/v1/folders/case/IR%202024/007" {
				t.Errorf("Unexpected path: %s", got)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": 42, "display_name": "ACME", "case_ref": "ir-2024/007"}`)
		})

		folder, _, err := client.Folders().GetCaseFolder(ctx, "IR 2024/007")
		if err != nil {
			t.Fatalf("GetCaseFolder returned error: %v", err)
		}
		if folder.ID != 42 || folder.CaseRef != "ir-2024/007" {
			t.Errorf("Unexpected folder: %+v", folder)
		}
	})

	tests := []struct {
		name        string
		status      int
		body        string
		unsupported bool
	}{
		{"Case not found", http.StatusNotFound, `{"detail": "Case not found."}`, false},
		{"Older server", http.StatusNotFound, `{"detail": "Not Found"}`, true},
		{"Older server behind a proxy", http.StatusNotFound, `<html>404</html>`, true},
		{"Invalid reference", http.StatusUnprocessableEntity, `{"detail": "bad"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, server, client := setupFoldersTestServer(t)
			defer server.Close()

			mux.HandleFunc("/api/v1/folders/case/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})

			_, _, err := client.Folders().GetCaseFolder(ctx, "case-123")
			if errors.Is(err, ErrCaseFoldersUnsupported) != tt.unsupported {
				t.Errorf("Expected unsupported %v, got %v", tt.unsupported, err)
			}
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
				t.Errorf("Expected a %d *Error, got %v", tt.status, err)
			}
		})
	}

	t.Run("Escapes the reference", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		var paths []string
		mux.HandleFunc("/api/v1/folders/case/", func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.EscapedPath())
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"detail": "bad"}`)
		})

		for _, ref := range []string{"50%off", "case?1", "case#1", "a%2Fb"} {
			if _, _, err := client.Folders().GetCaseFolder(ctx, ref); errors.Is(err, ErrCaseFoldersUnsupported) {
				t.Errorf("%q: unexpected %v", ref, err)
			}
		}
		want := []string{
			"/api/v1/folders/case/50%25off",
			"/api/v1/folders/case/case%3F1",
			"/api/v1/folders/case/case%231",
			"/api/v1/folders/case/a%252Fb",
		}
		if fmt.Sprint(paths) != fmt.Sprint(want) {
			t.Errorf("Unexpected paths:\n got %v\nwant %v", paths, want)
		}
	})

	t.Run("Escapes the reference under a base path", func(t *testing.T) {
		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		defer server.Close()
		client, err := NewClient(server.URL+"/openrelik", "test-key")
		if err != nil {
			t.Fatalf("failed to create test client: %v", err)
		}

		var paths []string
		mux.HandleFunc("/openrelik/api/v1/folders/case/", func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.EscapedPath())
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": 42, "case_ref": "x"}`)
		})

		for _, ref := range []string{"IR 2024/007", "50%off"} {
			if _, _, err := client.Folders().GetCaseFolder(ctx, ref); err != nil {
				t.Errorf("%q: %v", ref, err)
			}
		}
		want := []string{
			"/openrelik/api/v1/folders/case/IR%202024/007",
			"/openrelik/api/v1/folders/case/50%25off",
		}
		if fmt.Sprint(paths) != fmt.Sprint(want) {
			t.Errorf("Unexpected paths:\n got %v\nwant %v", paths, want)
		}
	})

	t.Run("Older server redirects", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		// Older servers add a trailing slash, which can match another route.
		mux.HandleFunc("/api/v1/folders/case/files", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusTemporaryRedirect)
		})
		mux.HandleFunc("/api/v1/folders/case/files/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"detail": "folder_id must be an integer"}`)
		})

		if _, _, err := client.Folders().GetCaseFolder(ctx, "files"); !errors.Is(err, ErrCaseFoldersUnsupported) {
			t.Errorf("Expected ErrCaseFoldersUnsupported, got %v", err)
		}
	})

	t.Run("Folder without a case reference", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/case/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": 42, "display_name": "ACME"}`)
		})

		folder, _, err := client.Folders().GetCaseFolder(ctx, "case-123")
		if !errors.Is(err, ErrCaseFoldersUnsupported) || folder != nil {
			t.Errorf("Expected ErrCaseFoldersUnsupported and no folder, got %v, %+v", err, folder)
		}
	})

	t.Run("Invalid references are refused locally", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("Unexpected request: %s", r.URL)
		})

		for _, ref := range []string{"", "  ", "/case-1", "case-1/", "ir//007", "ir/./007", "ir/../007"} {
			if _, _, err := client.Folders().GetCaseFolder(ctx, ref); err == nil {
				t.Errorf("Expected an error for %q", ref)
			}
		}
	})
}

func TestFoldersService_BlankCaseRef(t *testing.T) {
	ctx := context.Background()
	mux, server, client := setupFoldersTestServer(t)
	defer server.Close()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Unexpected request: %s", r.URL)
	})

	if _, _, err := client.Folders().CreateCaseFolder(ctx, " ", "ACME"); err == nil {
		t.Error("CreateCaseFolder: expected an error for a blank reference")
	}
	if _, _, err := client.Folders().SetCaseRef(ctx, 42, ""); err == nil {
		t.Error("SetCaseRef: expected an error for a blank reference")
	}
}

func TestFoldersService_SetCaseRef(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/42/case", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Errorf("Expected method PUT, got %s", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			if body["case_ref"] != "Case-123" {
				t.Errorf("Unexpected body: %v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": 42, "display_name": "ACME", "case_ref": "case-123"}`)
		})

		folder, _, err := client.Folders().SetCaseRef(ctx, 42, "Case-123")
		if err != nil {
			t.Fatalf("SetCaseRef returned error: %v", err)
		}
		if folder.CaseRef != "case-123" {
			t.Errorf("Unexpected folder: %+v", folder)
		}
	})

	tests := []struct {
		name        string
		status      int
		body        string
		unsupported bool
	}{
		{"Folder not found", http.StatusNotFound, `{"detail": "Folder not found."}`, false},
		{"Older server", http.StatusNotFound, `{"detail": "Not Found"}`, true},
		{"Older server, other methods only", http.StatusMethodNotAllowed, `{"detail": "Method Not Allowed"}`, true},
		{"Forbidden", http.StatusForbidden, `{"detail": "No access to folder"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, server, client := setupFoldersTestServer(t)
			defer server.Close()

			mux.HandleFunc("/api/v1/folders/42/case", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})

			_, _, err := client.Folders().SetCaseRef(ctx, 42, "case-123")
			if errors.Is(err, ErrCaseFoldersUnsupported) != tt.unsupported {
				t.Errorf("Expected unsupported %v, got %v", tt.unsupported, err)
			}
		})
	}
}

func TestFoldersService_SetCaseRef_NoCaseRefInResponse(t *testing.T) {
	ctx := context.Background()
	mux, server, client := setupFoldersTestServer(t)
	defer server.Close()

	mux.HandleFunc("/api/v1/folders/42/case", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id": 42, "display_name": "ACME"}`)
	})

	folder, _, err := client.Folders().SetCaseRef(ctx, 42, "case-123")
	if !errors.Is(err, ErrCaseFoldersUnsupported) || folder != nil {
		t.Errorf("Expected ErrCaseFoldersUnsupported and no folder, got %v, %+v", err, folder)
	}
}

func TestFoldersService_ClearCaseRef(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		called := false
		mux.HandleFunc("/api/v1/folders/42/case", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("Expected method DELETE, got %s", r.Method)
			}
			called = true
			w.WriteHeader(http.StatusNoContent)
		})

		if _, err := client.Folders().ClearCaseRef(ctx, 42); err != nil {
			t.Fatalf("ClearCaseRef returned error: %v", err)
		}
		if !called {
			t.Error("Expected a request")
		}
	})

	t.Run("Older server", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/42/case", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail": "Not Found"}`)
		})

		if _, err := client.Folders().ClearCaseRef(ctx, 42); !errors.Is(err, ErrCaseFoldersUnsupported) {
			t.Errorf("Expected ErrCaseFoldersUnsupported, got %v", err)
		}
	})
}
