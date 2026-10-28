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
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestFoldersService_ListRootFolders(t *testing.T) {
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

		folders, resp, err := client.Folders().ListRootFolders(ctx)
		if err != nil {
			t.Fatalf("ListRootFolders returned error: %v", err)
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

		_, _, err := client.Folders().ListRootFolders(ctx)
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}

func TestFoldersService_ListRootFoldersPage(t *testing.T) {
	ctx := context.Background()

	t.Run("Sends pagination params", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("page"); got != "3" {
				t.Errorf("Expected page=3, got %q", got)
			}
			if got := r.URL.Query().Get("page_size"); got != "25" {
				t.Errorf("Expected page_size=25, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [{"id": 7, "display_name": "Seven"}], "page": 3, "page_size": 25, "total_count": 51}`)
		})

		page, _, err := client.Folders().ListRootFoldersPage(ctx, &PageOptions{Page: 3, PageSize: 25})
		if err != nil {
			t.Fatalf("ListRootFoldersPage returned error: %v", err)
		}
		if len(page.Folders) != 1 || page.Folders[0].ID != 7 {
			t.Errorf("Unexpected folders: %+v", page.Folders)
		}
		if page.Page != 3 || page.PageSize != 25 || page.TotalCount != 51 {
			t.Errorf("Unexpected pagination metadata: %+v", page)
		}
	})

	t.Run("Older server ignores sort and cursor", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [{"id": 7}], "page": 1, "page_size": 25, "total_count": 51}`)
		})

		page, _, err := client.Folders().ListRootFoldersPage(ctx, &PageOptions{PageSize: 25, Sort: "updated_at", Order: "asc"})
		if err != nil {
			t.Fatalf("ListRootFoldersPage returned error: %v", err)
		}
		if page.Sort != "" || page.Order != "" || page.NextCursor != "" || page.TotalCount != 51 {
			t.Errorf("Unexpected pagination metadata: %+v", page)
		}
	})

	t.Run("Sends sort, order and cursor", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("sort") != "updated_at" || q.Get("order") != "asc" || q.Get("after") != "abc=" || q.Get("page_size") != "1000" {
				t.Errorf("Unexpected query: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [{"id": 7}], "page": null, "page_size": 1000, "total_count": null, "sort": "updated_at", "order": "asc", "next_cursor": "def="}`)
		})

		page, _, err := client.Folders().ListRootFoldersPage(ctx, &PageOptions{PageSize: 1000, Sort: "updated_at", Order: "asc", After: "abc="})
		if err != nil {
			t.Fatalf("ListRootFoldersPage returned error: %v", err)
		}
		if page.Sort != "updated_at" || page.Order != "asc" || page.NextCursor != "def=" || page.TotalCount != 0 || page.Page != 0 {
			t.Errorf("Unexpected pagination metadata: %+v", page)
		}
	})

	t.Run("Cursor leaves out the page number", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Has("page") {
				t.Errorf("Expected no page with a cursor, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [], "page": null, "page_size": 40, "total_count": null, "sort": "created_at", "order": "desc", "next_cursor": null}`)
		})

		if _, _, err := client.Folders().ListRootFoldersPage(ctx, &PageOptions{Page: 3, After: "abc="}); err != nil {
			t.Fatalf("ListRootFoldersPage returned error: %v", err)
		}
	})

	t.Run("Cursor to an older server is an error", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		// Older servers ignore after and return the first page.
		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [{"id": 7}], "page": 1, "page_size": 40, "total_count": 51}`)
		})

		page, resp, err := client.Folders().ListRootFoldersPage(ctx, &PageOptions{After: "abc="})
		if !errors.Is(err, ErrCursorsUnsupported) {
			t.Errorf("Expected ErrCursorsUnsupported, got %v", err)
		}
		if page != nil {
			t.Errorf("Expected no page, got %+v", page)
		}
		// Like other errors after a response, the response is still returned.
		if resp == nil || resp.StatusCode != http.StatusOK {
			t.Errorf("Expected the server's 200 response, got %v", resp)
		}
	})

	t.Run("Last page has no cursor", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [], "page": 1, "page_size": 40, "total_count": 3, "sort": "created_at", "order": "desc", "next_cursor": null}`)
		})

		page, _, err := client.Folders().ListRootFoldersPage(ctx, nil)
		if err != nil {
			t.Fatalf("ListRootFoldersPage returned error: %v", err)
		}
		if page.NextCursor != "" || page.TotalCount != 3 || page.Sort != "created_at" {
			t.Errorf("Unexpected pagination metadata: %+v", page)
		}
	})

	t.Run("Nil options omits params", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "" {
				t.Errorf("Expected no query string, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [], "page": 1, "page_size": 40, "total_count": 0}`)
		})

		if _, _, err := client.Folders().ListRootFoldersPage(ctx, nil); err != nil {
			t.Fatalf("ListRootFoldersPage returned error: %v", err)
		}
	})
}

func TestFoldersService_SearchFolders(t *testing.T) {
	ctx := context.Background()

	t.Run("Sends query and pagination params", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("q") != "plaso run" || q.Get("page") != "2" || q.Get("page_size") != "50" {
				t.Errorf("Unexpected query: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [{"id": 9, "display_name": "plaso run"}], "page": 2, "page_size": 50, "total_count": 51}`)
		})

		page, _, err := client.Folders().SearchFolders(ctx, "plaso run", &PageOptions{Page: 2, PageSize: 50})
		if err != nil {
			t.Fatalf("SearchFolders returned error: %v", err)
		}
		if len(page.Folders) != 1 || page.Folders[0].ID != 9 || page.TotalCount != 51 {
			t.Errorf("Unexpected page: %+v", page)
		}
	})

	t.Run("Sends sort, order and cursor", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("q") != "plaso" || q.Get("sort") != "display_name" || q.Get("order") != "asc" || q.Get("after") != "abc=" {
				t.Errorf("Unexpected query: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [{"id": 9}], "page": null, "page_size": 40, "total_count": null, "sort": "display_name", "order": "asc", "next_cursor": "def="}`)
		})

		page, _, err := client.Folders().SearchFolders(ctx, "plaso", &PageOptions{Sort: "display_name", Order: "asc", After: "abc="})
		if err != nil {
			t.Fatalf("SearchFolders returned error: %v", err)
		}
		if page.Sort != "display_name" || page.NextCursor != "def=" {
			t.Errorf("Unexpected page: %+v", page)
		}
	})

	t.Run("Nil options sends only trimmed query", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/all/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "q=plaso" {
				t.Errorf("Expected query %q, got %q", "q=plaso", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"folders": [], "page": 1, "page_size": 40, "total_count": 0}`)
		})

		if _, _, err := client.Folders().SearchFolders(ctx, "  plaso ", nil); err != nil {
			t.Fatalf("SearchFolders returned error: %v", err)
		}
	})

	t.Run("Empty query", func(t *testing.T) {
		_, server, client := setupFoldersTestServer(t)
		defer server.Close()

		for _, query := range []string{"", "   "} {
			if _, _, err := client.Folders().SearchFolders(ctx, query, nil); err == nil {
				t.Errorf("Expected error for query %q", query)
			}
		}
	})
}

func TestFoldersService_Get(t *testing.T) {
	ctx := context.Background()
	mux, server, client := setupFoldersTestServer(t)
	defer server.Close()

	mux.HandleFunc("/api/v1/folders/3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id": 3, "display_name": "extra", "parent": {"id": 2, "display_name": "logs", "parent": {"id": 1, "display_name": "case", "parent": null}}}`)
	})

	folder, _, err := client.Folders().Get(ctx, 3)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	var names []string
	for f := folder; f != nil; f = f.Parent {
		names = append(names, f.DisplayName)
	}
	if got := strings.Join(names, " < "); got != "extra < logs < case" {
		t.Errorf("Unexpected parent chain: %s", got)
	}
}

func TestFoldersService_ListSubFolders(t *testing.T) {
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

		folders, resp, err := client.Folders().ListSubFolders(ctx, folderID)
		if err != nil {
			t.Fatalf("ListSubFolders returned error: %v", err)
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

		_, _, err := client.Folders().ListSubFolders(ctx, folderID)
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}

func TestFoldersService_ListFiles(t *testing.T) {
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

		files, resp, err := client.Folders().ListFiles(ctx, folderID)
		if err != nil {
			t.Fatalf("ListFiles returned error: %v", err)
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

		_, _, err := client.Folders().ListFiles(ctx, folderID)
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
