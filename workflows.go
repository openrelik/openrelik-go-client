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
	ID          int        `json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
	IsDeleted   bool       `json:"is_deleted"`
	DisplayName string     `json:"display_name"`
	Description *string    `json:"description"`
	SpecJSON    *string    `json:"spec_json"`
	UUID        string     `json:"uuid"`
	User        User       `json:"user"`
	Files       []struct {
		ID          int    `json:"id"`
		DisplayName string `json:"display_name"`
		DataType    string `json:"data_type"`
	} `json:"files"`
	Tasks  []any `json:"tasks"`
	Folder struct {
		ID int `json:"id"`
	} `json:"folder"`
	Template any `json:"template"`
}

// WorkflowCreateRequest represents the request body to create a new workflow.
type WorkflowCreateRequest struct {
	FolderID       int            `json:"folder_id"`
	FileIDs        []int          `json:"file_ids"`
	TemplateID     *int           `json:"template_id,omitempty"`
	TemplateParams map[string]any `json:"template_params,omitempty"`
}

// Create creates a new workflow on the server.
// It fetches the folder ID from the first input file using the files.Info() method.
func (s *WorkflowsService) Create(ctx context.Context, fileIDs []int, templateID *int) (*Workflow, *http.Response, error) {
	if len(fileIDs) == 0 {
		return nil, nil, fmt.Errorf("openrelik: at least one file ID is required to create a workflow")
	}

	// Fetch file info to get the folder ID.
	file, _, err := s.client.Files().Info(ctx, fileIDs[0])
	if err != nil {
		return nil, nil, fmt.Errorf("openrelik: failed to fetch file info for file ID %d: %w", fileIDs[0], err)
	}

	folderID := file.Folder.ID

	requestBody := &WorkflowCreateRequest{
		FolderID:   folderID,
		FileIDs:    fileIDs,
		TemplateID: templateID,
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
