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

		if req.TemplateParams["test"] != "param" {
			t.Errorf("Expected TemplateParams['test'] 'param', got %v", req.TemplateParams["test"])
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
	params := map[string]any{"test": "param"}
	// Test with explicit folderID
	workflow, resp, err := client.Workflows().Create(ctx, folderID, fileIDs, nil, params)

	if err != nil {
		t.Fatalf("Workflows.Create (explicit folderID) returned error: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	// Test with folderID resolution (0)
	workflow, _, err = client.Workflows().Create(ctx, 0, fileIDs, nil, params)
	if err != nil {
		t.Fatalf("Workflows.Create (resolved folderID) returned error: %v", err)
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
	_, resp, err := client.Workflows().Create(ctx, 0, fileIDs, nil, nil)

	if err == nil {
		t.Fatal("Expected error when file info fails, got nil")
	}

	if resp == nil {
		t.Fatal("Expected response when file info fails, got nil")
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, resp.StatusCode)
	}
}

func TestWorkflowsService_Create_FolderIDZero(t *testing.T) {
	mux, server, client := setupWorkflowsTestServer(t)
	defer server.Close()

	fileID := 345

	// Mock Files.Info to return a file with no folder ID
	mux.HandleFunc(fmt.Sprintf("/api/v1/files/%d", fileID), func(w http.ResponseWriter, r *http.Request) {
		file := &File{
			ID: fileID,
			Folder: Folder{
				ID: 0,
			},
		}
		json.NewEncoder(w).Encode(file)
	})

	ctx := context.Background()
	fileIDs := []int{fileID}
	_, resp, err := client.Workflows().Create(ctx, 0, fileIDs, nil, nil)

	if err == nil {
		t.Fatal("Expected error when folder ID resolution results in 0, got nil")
	}

	if resp == nil {
		t.Fatal("Expected response when folder ID resolution results in 0, got nil")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	expectedError := fmt.Sprintf("openrelik: could not resolve folder ID for file ID %d", fileID)
	if err.Error() != expectedError {
		t.Errorf("Expected error %q, got %q", expectedError, err.Error())
	}
}

func TestWorkflowsService_Run(t *testing.T) {
	mux, server, client := setupWorkflowsTestServer(t)
	defer server.Close()

	folderID := 111
	workflowID := 95
	specJSON := `{"workflow":{"type":"chain"}}`

	// Mock Workflows.Run
	mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/workflows/%d/run/", folderID, workflowID), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		var reqBody struct {
			WorkflowSpec json.RawMessage `json:"workflow_spec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if string(reqBody.WorkflowSpec) != specJSON {
			t.Errorf("Expected WorkflowSpec %s, got %s", specJSON, string(reqBody.WorkflowSpec))
		}

		workflow := &Workflow{
			ID:          workflowID,
			DisplayName: "Simple Strings Workflow",
			Tasks: []Task{
				{
					ID:          241,
					DisplayName: "Strings",
				},
			},
		}
		workflow.Folder.ID = folderID
		json.NewEncoder(w).Encode(workflow)
	})

	ctx := context.Background()
	updatedWorkflow, resp, err := client.Workflows().Run(ctx, folderID, workflowID, &specJSON)

	if err != nil {
		t.Fatalf("Workflows.Run returned error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if updatedWorkflow.ID != workflowID {
		t.Errorf("Expected workflow ID %d, got %d", workflowID, updatedWorkflow.ID)
	}

	if len(updatedWorkflow.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(updatedWorkflow.Tasks))
	}
}

func TestWorkflowsService_Status(t *testing.T) {
	mux, server, client := setupWorkflowsTestServer(t)
	defer server.Close()

	folderID := 113
	workflowID := 97

	// Mock Workflows.Status
	mux.HandleFunc(fmt.Sprintf("/api/v1/folders/%d/workflows/%d/status/", folderID, workflowID), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected method GET, got %s", r.Method)
		}

		status := &WorkflowStatus{
			Status: "COMPLETE",
			Tasks: []Task{
				{
					ID:          243,
					DisplayName: "Strings",
					StatusShort: stringPtr("SUCCESS"),
				},
			},
		}
		json.NewEncoder(w).Encode(status)
	})

	ctx := context.Background()
	status, resp, err := client.Workflows().Status(ctx, folderID, workflowID)

	if err != nil {
		t.Fatalf("Workflows.Status returned error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if status.Status != "COMPLETE" {
		t.Errorf("Expected status 'COMPLETE', got %q", status.Status)
	}

	if len(status.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(status.Tasks))
	}

	if *status.Tasks[0].StatusShort != "SUCCESS" {
		t.Errorf("Expected task status 'SUCCESS', got %q", *status.Tasks[0].StatusShort)
	}
}

func TestWorkflowsService_Get(t *testing.T) {
	mux, server, client := setupWorkflowsTestServer(t)
	defer server.Close()

	workflowID := 98

	// Mock Workflows.Get
	mux.HandleFunc(fmt.Sprintf("/api/v1/workflows/%d", workflowID), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected method GET, got %s", r.Method)
		}

		workflow := &Workflow{
			ID:          workflowID,
			DisplayName: "Test Workflow",
		}
		json.NewEncoder(w).Encode(workflow)
	})

	ctx := context.Background()
	workflow, resp, err := client.Workflows().Get(ctx, workflowID)

	if err != nil {
		t.Fatalf("Workflows.Get returned error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if workflow.ID != workflowID {
		t.Errorf("Expected workflow ID %d, got %d", workflowID, workflow.ID)
	}
}

func stringPtr(s string) *string {
	return &s
}
