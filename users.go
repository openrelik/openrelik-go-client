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

// UsersService handles communication with user-related methods of the OpenRelik API.
type UsersService struct {
	client *Client
}

// User represents a user within the OpenRelik system.
type User struct {
	ID                int        `json:"id"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at"`
	IsDeleted         bool       `json:"is_deleted"`
	DisplayName       string     `json:"display_name"`
	Username          string     `json:"username"`
	Email             *string    `json:"email"`
	AuthMethod        string     `json:"auth_method"`
	ProfilePictureURL *string    `json:"profile_picture_url"`
	UUID              string     `json:"uuid"`
	IsAdmin           bool       `json:"is_admin"`
}

// GetMe retrieves the profile of the currently authenticated user.
func (s *UsersService) GetMe(ctx context.Context) (*User, *http.Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "/users/me/", nil)
	if err != nil {
		return nil, nil, err
	}

	user := new(User)
	resp, err := s.client.Do(req, user)
	if err != nil {
		return nil, resp, err
	}

	return user, resp, nil
}
