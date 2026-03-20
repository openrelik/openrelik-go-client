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
	"net/url"
	"strconv"
	"time"
)

// WorkflowsService handles communication with workflow-related methods of the OpenRelik API.
type WorkflowsService struct {
	client *Client
}

// Workflow represents a workflow within the OpenRelik system.
type Workflow struct {
	ID          int               `json:"id"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	DeletedAt   *time.Time        `json:"deleted_at"`
	IsDeleted   bool              `json:"is_deleted"`
	DisplayName string            `json:"display_name"`
	Description *string           `json:"description"`
	SpecJSON    *string           `json:"spec_json"`
	UUID        string            `json:"uuid"`
	User        User              `json:"user"`
	Files       []FolderFile      `json:"files"`
	Tasks       []Task            `json:"tasks"`
	Folder      Folder            `json:"folder"`
	Template    *WorkflowTemplate `json:"template"`
}

// WorkflowTemplate represents a template used to create a workflow.
type WorkflowTemplate struct {
	ID          int    `json:"id"`
	DisplayName string `json:"display_name"`
}

// Task represents a task within a workflow.
type Task struct {
	ID             int              `json:"id"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	DeletedAt      *time.Time       `json:"deleted_at"`
	IsDeleted      bool             `json:"is_deleted"`
	DisplayName    string           `json:"display_name"`
	Description    string           `json:"description"`
	UUID           string           `json:"uuid"`
	StatusShort    *string          `json:"status_short"`
	StatusDetail   *string          `json:"status_detail"`
	StatusProgress *string          `json:"status_progress"`
	Result         *string          `json:"result"`
	Runtime        *float64         `json:"runtime"`
	ErrorException *string          `json:"error_exception"`
	ErrorTraceback *string          `json:"error_traceback"`
	User           User             `json:"user"`
	OutputFiles    []TaskOutputFile `json:"output_files"`
	FileReports    []any            `json:"file_reports"`
	TaskReport     any              `json:"task_report"`
}

// TaskOutputFile represents a file produced by a task.
type TaskOutputFile struct {
	ID          int    `json:"id"`
	DisplayName string `json:"display_name"`
	Filesize    int64  `json:"filesize"`
	UUID        string `json:"uuid"`
	FolderID    int    `json:"folder_id"`
	IsDeleted   bool   `json:"is_deleted"`
}

// WorkflowStatus represents the status of a workflow and its tasks.
type WorkflowStatus struct {
	Status string `json:"status"`
	Tasks  []Task `json:"tasks"`
}

// WorkflowCreateRequest represents the request body to create a new workflow.
type WorkflowCreateRequest struct {
	FolderID       int            `json:"folder_id"`
	FileIDs        []int          `json:"file_ids"`
	TemplateID     *int           `json:"template_id,omitempty"`
	TemplateParams map[string]any `json:"template_params,omitempty"`
}

// Create creates a new workflow on the server.
// If folderID is 0, it fetches the folder ID from the first input file using the files.Info() method.
func (s *WorkflowsService) Create(ctx context.Context, folderID int, fileIDs []int, templateID *int, templateParams map[string]any) (*Workflow, *http.Response, error) {
	if len(fileIDs) == 0 {
		return nil, nil, fmt.Errorf("openrelik: at least one file ID is required to create a workflow")
	}

	// Resolve folder ID if not provided.
	if folderID == 0 {
		file, resp, err := s.client.Files().Info(ctx, fileIDs[0])
		if err != nil {
			return nil, resp, fmt.Errorf("openrelik: failed to fetch file info for file ID %d: %w", fileIDs[0], err)
		}
		folderID = file.Folder.ID
		if folderID == 0 {
			return nil, resp, fmt.Errorf("openrelik: could not resolve folder ID for file ID %d", fileIDs[0])
		}
	}

	requestBody := &WorkflowCreateRequest{
		FolderID:       folderID,
		FileIDs:        fileIDs,
		TemplateID:     templateID,
		TemplateParams: templateParams,
	}

	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "workflows/")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodPost, endpoint, requestBody)
	if err != nil {
		return nil, nil, err
	}

	workflow := new(Workflow)
	resp, err := s.client.Do(req, workflow)
	if err != nil {
		return nil, resp, err
	}

	return workflow, resp, nil
}

// Run executes a workflow on the server by its ID and provided workflow specification.
func (s *WorkflowsService) Run(ctx context.Context, folderID, workflowID int, specJSON *string) (*Workflow, *http.Response, error) {
	spec := json.RawMessage("{}")
	if specJSON != nil && *specJSON != "" {
		spec = json.RawMessage(*specJSON)
	}

	body := struct {
		WorkflowSpec json.RawMessage `json:"workflow_spec"`
	}{
		WorkflowSpec: spec,
	}

	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "workflows", strconv.Itoa(workflowID), "run/")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, nil, err
	}

	updatedWorkflow := new(Workflow)
	resp, err := s.client.Do(req, updatedWorkflow)
	if err != nil {
		return nil, resp, err
	}

	return updatedWorkflow, resp, nil
}

// Status retrieves the current status of a workflow by its ID.
func (s *WorkflowsService) Status(ctx context.Context, folderID, workflowID int) (*WorkflowStatus, *http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "workflows", strconv.Itoa(workflowID), "status/")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	status := new(WorkflowStatus)
	resp, err := s.client.Do(req, status)
	if err != nil {
		return nil, resp, err
	}

	return status, resp, nil
}

// Get retrieves a single workflow by workflow ID.
func (s *WorkflowsService) Get(ctx context.Context, workflowID int) (*Workflow, *http.Response, error) {
	endpoint, err := url.JoinPath("workflows", strconv.Itoa(workflowID))
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	workflow := new(Workflow)
	resp, err := s.client.Do(req, workflow)
	if err != nil {
		return nil, resp, err
	}

	return workflow, resp, nil
}
