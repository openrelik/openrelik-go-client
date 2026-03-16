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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupFilesTestServer(t *testing.T) (mux *http.ServeMux, server *httptest.Server, client *Client) {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)
	var err error
	client, err = NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return
}

func TestFilesService_Info(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		fileID := 1
		mux.HandleFunc(fmt.Sprintf("/api/v1/files/%d", fileID), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"id": 1,
				"created_at": "2026-03-10T12:59:19.516455Z",
				"updated_at": "2026-03-10T12:59:19.667064Z",
				"deleted_at": null,
				"is_deleted": false,
				"display_name": "test-file.dd",
				"description": null,
				"uuid": "test-file-uuid",
				"filename": "test-file.dd",
				"filesize": 20971520,
				"extension": "dd",
				"original_path": null,
				"magic_text": "DOS/MBR boot sector",
				"magic_mime": "application/octet-stream",
				"data_type": "file:generic",
				"hash_md5": "md5-hash",
				"hash_sha1": "sha1-hash",
				"hash_sha256": "sha256-hash",
				"hash_ssdeep": null,
				"storage_provider": null,
				"storage_key": null,
				"user_id": 1,
				"user": {
					"id": 1,
					"display_name": "testuser",
					"username": "testuser",
					"profile_picture_url": null,
					"uuid": "test-user-uuid"
				}
			}`)
		})

		file, resp, err := client.Files().Info(ctx, fileID)
		if err != nil {
			t.Fatalf("Info returned error: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if file.ID != 1 {
			t.Errorf("Expected file ID 1, got %d", file.ID)
		}

		if file.DisplayName != "test-file.dd" {
			t.Errorf("Expected display name 'test-file.dd', got %q", file.DisplayName)
		}

		if file.HashSHA256 != "sha256-hash" {
			t.Errorf("Expected SHA256 hash 'sha256-hash', got %q", file.HashSHA256)
		}

		if file.User.Username != "testuser" {
			t.Errorf("Expected username 'testuser', got %q", file.User.Username)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		fileID := 1
		mux.HandleFunc(fmt.Sprintf("/api/v1/files/%d", fileID), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		_, _, err := client.Files().Info(ctx, fileID)
		if err == nil {
			t.Error("Expected error for 404 status code")
		}
	})
}

func TestFilesService_Download(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		fileID := 1
		expectedContent := "this is a test file content"
		mux.HandleFunc(fmt.Sprintf("/api/v1/files/%d/download", fileID), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			w.Header().Set("Content-Disposition", "attachment; filename=\"test.txt\"")
			w.Header().Set("Content-Length", fmt.Sprint(len(expectedContent)))
			w.Header().Set("Content-Type", "application/octet-stream")
			fmt.Fprint(w, expectedContent)
		})

		body, resp, err := client.Files().Download(ctx, fileID)
		if err != nil {
			t.Fatalf("Download returned error: %v", err)
		}
		defer body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if resp.Header.Get("Content-Length") != fmt.Sprint(len(expectedContent)) {
			t.Errorf("Expected Content-Length %d, got %s", len(expectedContent), resp.Header.Get("Content-Length"))
		}

		content, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("Failed to read body: %v", err)
		}

		if string(content) != expectedContent {
			t.Errorf("Expected content %q, got %q", expectedContent, string(content))
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		fileID := 999
		mux.HandleFunc(fmt.Sprintf("/api/v1/files/%d/download", fileID), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail": "File not found"}`)
		})

		_, resp, err := client.Files().Download(ctx, fileID)
		if err == nil {
			t.Fatal("Expected error for 404 status code")
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}

		apiErr, ok := err.(*Error)
		if !ok {
			t.Fatalf("Expected *openrelik.Error, got %T", err)
		}

		if apiErr.Message != "File not found" {
			t.Errorf("Expected error message 'File not found', got %q", apiErr.Message)
		}
	})
}

func TestFilesService_Upload(t *testing.T) {
	ctx := context.Background()

	t.Run("SingleChunkSuccess", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		filename := "test.txt"
		content := []byte("hello world")
		folderID := 123

		mux.HandleFunc("/api/v1/files/upload", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected method POST, got %s", r.Method)
			}

			// Check query params
			q := r.URL.Query()
			if q.Get("resumableFilename") != filename {
				t.Errorf("Expected filename %s, got %s", filename, q.Get("resumableFilename"))
			}
			if q.Get("resumableChunkNumber") != "1" {
				t.Errorf("Expected chunk number 1, got %s", q.Get("resumableChunkNumber"))
			}
			if q.Get("resumableTotalChunks") != "1" {
				t.Errorf("Expected total chunks 1, got %s", q.Get("resumableTotalChunks"))
			}
			if q.Get("folder_id") != "123" {
				t.Errorf("Expected folder_id 123, got %s", q.Get("folder_id"))
			}

			// Check multipart body
			err := r.ParseMultipartForm(1024)
			if err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("Failed to get form file: %v", err)
			}
			defer file.Close()
			uploadedContent, _ := io.ReadAll(file)
			if string(uploadedContent) != string(content) {
				t.Errorf("Expected content %s, got %s", content, uploadedContent)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id": 1, "filename": "test.txt"}`)
		})

		file, resp, err := client.Files().Upload(ctx, folderID, filename, bytes.NewReader(content))
		if err != nil {
			t.Fatalf("Upload returned error: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		if file.Filename != filename {
			t.Errorf("Expected filename %s, got %s", filename, file.Filename)
		}
	})

	t.Run("MultiChunkWithProgress", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		filename := "large.dat"
		content := make([]byte, 100) // 100 bytes
		chunkSize := 40              // Will result in 3 chunks: 40, 40, 20
		folderID := 456

		chunkCount := 0
		progressCalls := 0
		var lastBytesSent int64

		mux.HandleFunc("/api/v1/files/upload", func(w http.ResponseWriter, r *http.Request) {
			chunkCount++
			q := r.URL.Query()
			if q.Get("resumableChunkNumber") != fmt.Sprint(chunkCount) {
				t.Errorf("Expected chunk number %d, got %s", chunkCount, q.Get("resumableChunkNumber"))
			}
			if q.Get("resumableTotalChunks") != "3" {
				t.Errorf("Expected total chunks 3, got %s", q.Get("resumableTotalChunks"))
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id": 1, "filename": "large.dat"}`)
		})

		file, _, err := client.Files().Upload(ctx, folderID, filename, bytes.NewReader(content),
			WithChunkSize(chunkSize),
			WithUploadProgress(func(bytesSent, totalBytes int64) {
				progressCalls++
				if bytesSent <= lastBytesSent {
					t.Errorf("Expected bytesSent to increase, got %d <= %d", bytesSent, lastBytesSent)
				}
				lastBytesSent = bytesSent
				if totalBytes != 100 {
					t.Errorf("Expected totalBytes 100, got %d", totalBytes)
				}
			}),
		)

		if err != nil {
			t.Fatalf("Upload returned error: %v", err)
		}

		if chunkCount != 3 {
			t.Errorf("Expected 3 chunks uploaded, got %d", chunkCount)
		}
		if progressCalls != 3 {
			t.Errorf("Expected 3 progress calls, got %d", progressCalls)
		}
		if file.ID != 1 {
			t.Errorf("Expected file ID 1, got %d", file.ID)
		}
	})

	t.Run("RetryOn503", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		attempts := 0
		mux.HandleFunc("/api/v1/files/upload", func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id": 1}`)
		})

		file, resp, err := client.Files().Upload(ctx, 1, "test.txt", bytes.NewReader([]byte("data")))
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}
		if file.ID != 1 {
			t.Errorf("Expected file ID 1, got %d", file.ID)
		}
	})

	t.Run("AbortOn429", func(t *testing.T) {
		mux, server, client := setupFilesTestServer(t)
		defer server.Close()

		attempts := 0
		mux.HandleFunc("/api/v1/files/upload", func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.WriteHeader(http.StatusTooManyRequests)
		})

		_, _, err := client.Files().Upload(ctx, 1, "test.txt", bytes.NewReader([]byte("data")))
		if err == nil {
			t.Fatal("Expected error on 429 status code")
		}

		// Should not retry on 429
		if attempts != 1 {
			t.Errorf("Expected 1 attempt for 429, got %d", attempts)
		}
	})
}
