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
	"reflect"
	"strings"
	"testing"
)

func TestFoldersService_Share(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/42/roles", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected method POST, got %s", r.Method)
			}
			var got map[string]any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			want := map[string]any{"user_names": []any{"alice"}, "user_role": "Viewer"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("body = %v, want %v", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id": 42, "display_name": "Case",
				"user_roles": [{"id": 9, "role": "Viewer", "user": {"id": 17, "username": "alice"}}],
				"group_roles": [{"id": 3, "role": "Editor", "group": {"id": 2, "name": "ir"}}]}`)
		})

		folder, _, err := client.Folders().Share(ctx, 42, &FolderShareRequest{UserNames: []string{"alice"}, UserRole: RoleViewer})
		if err != nil {
			t.Fatalf("Share returned error: %v", err)
		}
		if len(folder.UserRoles) != 1 || folder.UserRoles[0].ID != 9 || folder.UserRoles[0].Role != "Viewer" || folder.UserRoles[0].User.Username != "alice" {
			t.Errorf("UserRoles = %+v", folder.UserRoles)
		}
		if len(folder.GroupRoles) != 1 || folder.GroupRoles[0].ID != 3 || folder.GroupRoles[0].Group.Name != "ir" {
			t.Errorf("GroupRoles = %+v", folder.GroupRoles)
		}

		// Verify compact nested roles/groups don't emit zero/placeholder fields when re-marshalled.
		out, err := json.Marshal(folder.GroupRoles)
		if err != nil {
			t.Fatalf("Marshal group roles: %v", err)
		}
		if strings.Contains(string(out), `"deleted_at":null`) || strings.Contains(string(out), `"0001-01-01T00:00:00Z"`) {
			t.Errorf("marshalled group roles contain placeholders: %s", out)
		}
	})

	t.Run("NilShareRequest", func(t *testing.T) {
		_, server, client := setupFoldersTestServer(t)
		defer server.Close()

		if _, _, err := client.Folders().Share(ctx, 42, nil); err == nil {
			t.Fatal("Expected error for nil FolderShareRequest, got nil")
		}
	})

	t.Run("Conflict", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/42/roles", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"detail": "already has the role"}`)
		})

		_, _, err := client.Folders().Share(ctx, 42, &FolderShareRequest{UserNames: []string{"alice"}, UserRole: RoleViewer})
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
			t.Fatalf("err = %v, want a 409 *Error", err)
		}
	})
}

func TestFoldersService_DeleteRoles(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		var deleted []string
		mux.HandleFunc("/api/v1/folders/42/roles/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("Expected method DELETE, got %s", r.Method)
			}
			deleted = append(deleted, r.URL.Path)
			fmt.Fprint(w, "null")
		})

		if _, err := client.Folders().DeleteUserRole(ctx, 42, 9); err != nil {
			t.Fatalf("DeleteUserRole returned error: %v", err)
		}
		if _, err := client.Folders().DeleteGroupRole(ctx, 42, 3); err != nil {
			t.Fatalf("DeleteGroupRole returned error: %v", err)
		}
		want := []string{"/api/v1/folders/42/roles/users/9", "/api/v1/folders/42/roles/groups/3"}
		if !reflect.DeepEqual(deleted, want) {
			t.Errorf("deleted = %v, want %v", deleted, want)
		}
	})

	t.Run("APIError", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/42/roles/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail": "role not found"}`)
		})

		if _, err := client.Folders().DeleteUserRole(ctx, 42, 9); err == nil {
			t.Error("Expected error for DeleteUserRole 404")
		}
		if _, err := client.Folders().DeleteGroupRole(ctx, 42, 3); err == nil {
			t.Error("Expected error for DeleteGroupRole 404")
		}
	})
}

func TestFoldersService_GetMyRole(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/42/roles/me", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			fmt.Fprint(w, `{"id": 9, "role": "Owner", "user_id": 4, "folder_id": 40, "file_id": null}`)
		})

		role, _, err := client.Folders().GetMyRole(ctx, 42)
		if err != nil {
			t.Fatalf("GetMyRole returned error: %v", err)
		}
		if role.Role != RoleOwner || role.FolderID == nil || *role.FolderID != 40 || role.GroupID != nil {
			t.Errorf("role = %+v", role)
		}
	})

	t.Run("NoRole", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/42/roles/me", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `false`)
		})

		_, _, err := client.Folders().GetMyRole(ctx, 42)
		if err == nil || !strings.Contains(err.Error(), "server returned no role for folder 42") {
			t.Fatalf("GetMyRole err = %v, want 'server returned no role for folder 42'", err)
		}
	})
}

func TestFoldersService_ListSharedFolders(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/shared/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			fmt.Fprint(w, `[{"id": 5, "display_name": "Shared", "user": {"username": "bob"}}]`)
		})

		folders, _, err := client.Folders().ListSharedFolders(ctx)
		if err != nil {
			t.Fatalf("ListSharedFolders returned error: %v", err)
		}
		if len(folders) != 1 || folders[0].ID != 5 || folders[0].User.Username != "bob" {
			t.Errorf("folders = %+v", folders)
		}
	})

	t.Run("APIError", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/folders/shared/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		if _, _, err := client.Folders().ListSharedFolders(ctx); err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}

func TestGroupsService_List(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/groups/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			fmt.Fprint(w, `[{"id": 1, "name": "Everyone", "description": null}]`)
		})

		groups, _, err := client.Groups().List(ctx)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(groups) != 1 || groups[0].Name != "Everyone" {
			t.Errorf("groups = %+v", groups)
		}
	})

	t.Run("APIError", func(t *testing.T) {
		mux, server, client := setupFoldersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/groups/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		if _, _, err := client.Groups().List(ctx); err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}
