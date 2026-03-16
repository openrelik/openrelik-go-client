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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupWorkflowsTestServer(t *testing.T) (mux *http.ServeMux, server *httptest.Server, client *Client) {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)
	client, err := NewClient(server.URL, "fake-api-key")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	return mux, server, client
}

func TestWorkflowsService_Create(t *testing.T) {
	mux, server, client := setupWorkflowsTestServer(t)
	defer server.Close()

	fileID := 345
	folderID := 104
	workflowID := 90

	// Mock Files.Info
	mux.HandleFunc(fmt.Sprintf("/api/v1/files/%d", fileID), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected method GET, got %s", r.Method)
		}
		file := &File{
			ID: fileID,
			Folder: Folder{
				ID: folderID,
			},
		}
		json.NewEncoder(w).Encode(file)
	})

	// Mock Workflows.Create
	mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/workflows/", folderID), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		var req WorkflowCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if req.FolderID != folderID {
			t.Errorf("Expected folder ID %d in body, got %d", folderID, req.FolderID)
		}

		if len(req.FileIDs) != 1 || req.FileIDs[0] != fileID {
			t.Errorf("Expected file IDs [%d], got %v", fileID, req.FileIDs)
		}

		workflow := &Workflow{
			ID:          workflowID,
			DisplayName: "Untitled workflow",
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(workflow)
	})

	ctx := context.Background()
	fileIDs := []int{fileID}
	workflow, resp, err := client.Workflows().Create(ctx, fileIDs, nil)

	if err != nil {
		t.Fatalf("Workflows.Create returned error: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	if workflow.ID != workflowID {
		t.Errorf("Expected workflow ID %d, got %d", workflowID, workflow.ID)
	}

	if workflow.DisplayName != "Untitled workflow" {
		t.Errorf("Expected display name 'Untitled workflow', got %q", workflow.DisplayName)
	}
}

func TestWorkflowsService_Create_Error(t *testing.T) {
	mux, server, client := setupWorkflowsTestServer(t)
	defer server.Close()

	fileID := 345

	// Mock Files.Info to return error
	mux.HandleFunc(fmt.Sprintf("/api/v1/files/%d", fileID), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail": "File not found"}`))
	})

	ctx := context.Background()
	fileIDs := []int{fileID}
	_, _, err := client.Workflows().Create(ctx, fileIDs, nil)

	if err == nil {
		t.Fatal("Expected error when file info fails, got nil")
	}
}
