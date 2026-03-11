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
	"fmt"
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

func TestFilesService_GetMetadata(t *testing.T) {
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

		file, resp, err := client.Files().GetMetadata(ctx, fileID)
		if err != nil {
			t.Fatalf("GetMetadata returned error: %v", err)
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

		_, _, err := client.Files().GetMetadata(ctx, fileID)
		if err == nil {
			t.Error("Expected error for 404 status code")
		}
	})
}
