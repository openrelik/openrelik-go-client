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

func setupFoldersTestServer(t *testing.T) (mux *http.ServeMux, server *httptest.Server, client *Client) {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)
	var err error
	client, err = NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return
}

func TestFoldersService_GetRootFolders(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"folders": [
					{
						"id": 1,
						"created_at": "2026-01-12T07:46:14.505327Z",
						"updated_at": "2026-01-12T07:46:14.505327Z",
						"deleted_at": null,
						"is_deleted": false,
						"display_name": "Test",
						"user": {
							"id": 1,
							"display_name": "testuser",
							"username": "testuser",
							"profile_picture_url": null,
							"uuid": "test-user-uuid"
						},
						"workflows": []
					}
				],
				"page": 1,
				"page_size": 40,
				"total_count": 1
			}`)
		})

		folders, resp, err := client.Folders().GetRootFolders(ctx)
		if err != nil {
			t.Fatalf("GetRootFolders returned error: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if len(folders) != 1 {
			t.Errorf("Expected 1 folder, got %d", len(folders))
		}

		if folders[0].ID != 1 {
			t.Errorf("Expected folder ID 1, got %d", folders[0].ID)
		}

		if folders[0].DisplayName != "Test" {
			t.Errorf("Expected display name 'Test', got %q", folders[0].DisplayName)
		}

		if folders[0].User.Username != "testuser" {
			t.Errorf("Expected username 'testuser', got %q", folders[0].User.Username)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		_, _, err := client.Folders().GetRootFolders(ctx)
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}

func TestFoldersService_GetSubFolders(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		folderID := 1
		mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/folders/", folderID), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[
				{
					"id": 2,
					"created_at": "2026-03-10T13:04:22.069383Z",
					"updated_at": "2026-03-10T13:04:22.069383Z",
					"deleted_at": null,
					"is_deleted": false,
					"display_name": "subfolder two",
					"user": {
						"id": 1,
						"display_name": "testuser",
						"username": "testuser",
						"profile_picture_url": null,
						"uuid": "test-user-uuid"
					},
					"workflows": []
				}
			]`)
		})

		folders, resp, err := client.Folders().GetSubFolders(ctx, folderID)
		if err != nil {
			t.Fatalf("GetSubFolders returned error: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if len(folders) != 1 {
			t.Errorf("Expected 1 folder, got %d", len(folders))
		}

		if folders[0].ID != 2 {
			t.Errorf("Expected folder ID 2, got %d", folders[0].ID)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		folderID := 1
		mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/folders/", folderID), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		_, _, err := client.Folders().GetSubFolders(ctx, folderID)
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}

func TestFoldersService_GetFiles(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		folderID := 2
		mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/files/", folderID), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[
				{
					"id": 3,
					"display_name": "artifact_disk.dd",
					"filesize": 20971520,
					"data_type": "file:generic",
					"magic_mime": "application/octet-stream",
					"user": {
						"id": 1,
						"display_name": "testuser",
						"username": "testuser",
						"profile_picture_url": null,
						"uuid": "test-user-uuid"
					},
					"created_at": "2026-03-10T12:59:19.516455Z",
					"updated_at": "2026-03-10T12:59:19.667064Z",
					"is_deleted": false
				}
			]`)
		})

		files, resp, err := client.Folders().GetFiles(ctx, folderID)
		if err != nil {
			t.Fatalf("GetFiles returned error: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if len(files) != 1 {
			t.Errorf("Expected 1 file, got %d", len(files))
		}

		if files[0].ID != 3 {
			t.Errorf("Expected file ID 3, got %d", files[0].ID)
		}

		if files[0].DisplayName != "artifact_disk.dd" {
			t.Errorf("Expected display name 'artifact_disk.dd', got %q", files[0].DisplayName)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		folderID := 2
		mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/files/", folderID), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		_, _, err := client.Folders().GetFiles(ctx, folderID)
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}

func TestFoldersService_CreateRootFolder(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		displayName := "New Root Folder"
		mux.HandleFunc("/api/v1/folders/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected method POST, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, fmt.Sprintf(`{
				"id": 10,
				"display_name": "%s",
				"created_at": "2026-03-10T12:59:19.516455Z",
				"updated_at": "2026-03-10T12:59:19.516455Z",
				"user": {
					"id": 1,
					"display_name": "testuser",
					"username": "testuser",
					"uuid": "test-user-uuid"
				}
			}`, displayName))
		})

		folder, resp, err := client.Folders().CreateRootFolder(ctx, displayName)
		if err != nil {
			t.Fatalf("CreateRootFolder returned error: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		if folder.DisplayName != displayName {
			t.Errorf("Expected display name %q, got %q", displayName, folder.DisplayName)
		}

		if folder.ID != 10 {
			t.Errorf("Expected folder ID 10, got %d", folder.ID)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		_, _, err := client.Folders().CreateRootFolder(ctx, "Fail")
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}

func TestFoldersService_CreateSubFolder(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		parentID := 101
		displayName := "New Subfolder"
		mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/folders/", parentID), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected method POST, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, fmt.Sprintf(`{
				"id": 102,
				"display_name": "%s",
				"created_at": "2026-03-10T12:59:19.516455Z",
				"updated_at": "2026-03-10T12:59:19.516455Z",
				"user": {
					"id": 1,
					"display_name": "testuser",
					"username": "testuser",
					"uuid": "test-user-uuid"
				}
			}`, displayName))
		})

		folder, resp, err := client.Folders().CreateSubFolder(ctx, parentID, displayName)
		if err != nil {
			t.Fatalf("CreateSubFolder returned error: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		if folder.DisplayName != displayName {
			t.Errorf("Expected display name %q, got %q", displayName, folder.DisplayName)
		}

		if folder.ID != 102 {
			t.Errorf("Expected folder ID 102, got %d", folder.ID)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		parentID := 101
		mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/folders/", parentID), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		_, _, err := client.Folders().CreateSubFolder(ctx, parentID, "Fail")
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}
