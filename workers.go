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
	"net/http"
)

// WorkersService handles communication with worker-related methods of the OpenRelik API.
type WorkersService struct {
	client *Client
}

// TaskConfig represents the configuration for a worker task.
type TaskConfig struct {
	Name        string      `json:"name"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Type        string      `json:"type"`
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// Worker represents a registered worker in the OpenRelik system.
type Worker struct {
	TaskName    string       `json:"task_name"`
	QueueName   string       `json:"queue_name"`
	DisplayName string       `json:"display_name"`
	Description string       `json:"description"`
	TaskConfig  []TaskConfig `json:"task_config,omitempty"`
}

// Registered retrieves the list of currently registered workers in the backend system.
func (s *WorkersService) Registered(ctx context.Context) ([]Worker, *http.Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "/taskqueue/tasks/registered", nil)
	if err != nil {
		return nil, nil, err
	}

	var workers []Worker
	resp, err := s.client.Do(req, &workers)
	if err != nil {
		return nil, resp, err
	}

	return workers, resp, nil
}
