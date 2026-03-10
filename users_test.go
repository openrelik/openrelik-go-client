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

func setupUsersTestServer() (mux *http.ServeMux, server *httptest.Server, client *Client) {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)
	client, _ = NewClient(server.URL, "test-key")
	return
}

func TestUsersService_GetMe(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupUsersTestServer()
		defer server.Close()

		mux.HandleFunc("/api/v1/users/me/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"id": 1,
				"created_at": "2026-01-07T13:48:36.414353Z",
				"updated_at": "2026-01-07T13:48:36.414353Z",
				"deleted_at": null,
				"is_deleted": false,
				"display_name": "jbn",
				"username": "jbn",
				"email": null,
				"auth_method": "local",
				"profile_picture_url": null,
				"uuid": "d9209534cd9b4d2b8330d90563760477",
				"is_admin": false
			}`)
		})

		user, resp, err := client.users.GetMe(ctx)
		if err != nil {
			t.Fatalf("GetMe returned error: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if user.ID != 1 {
			t.Errorf("Expected user ID 1, got %d", user.ID)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupUsersTestServer()
		defer server.Close()

		mux.HandleFunc("/api/v1/users/me/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		_, _, err := client.users.GetMe(ctx)
		if err == nil {
			t.Error("Expected error for 500 status code")
		}
	})
}
