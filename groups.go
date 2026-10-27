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
	"time"
)

// GroupsService handles communication with group-related methods of the OpenRelik API.
type GroupsService struct {
	client *Client
}

// Group represents a group of users within the OpenRelik system.
type Group struct {
	ID          int        `json:"id,omitzero"`
	CreatedAt   time.Time  `json:"created_at,omitzero"`
	UpdatedAt   time.Time  `json:"updated_at,omitzero"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	IsDeleted   *bool      `json:"is_deleted,omitempty"` // nil if the server didn't send it
	Name        string     `json:"name,omitempty"`
	Description *string    `json:"description,omitempty"`
}

// List retrieves all groups.
func (s *GroupsService) List(ctx context.Context) ([]Group, *http.Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "/groups/", nil)
	if err != nil {
		return nil, nil, err
	}

	var groups []Group
	resp, err := s.client.Do(req, &groups)
	if err != nil {
		return nil, resp, err
	}

	return groups, resp, nil
}
