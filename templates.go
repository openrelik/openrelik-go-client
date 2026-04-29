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

// WorkflowTemplatesService handles communication with workflow template methods of the OpenRelik API.
type WorkflowTemplatesService struct {
	client *Client
}

// List retrieves all workflow templates available in the system.
func (s *WorkflowTemplatesService) List(ctx context.Context) ([]WorkflowTemplate, *http.Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "workflows/templates/", nil)
	if err != nil {
		return nil, nil, err
	}

	var templates []WorkflowTemplate
	resp, err := s.client.Do(req, &templates)
	if err != nil {
		return nil, resp, err
	}

	return templates, resp, nil
}
