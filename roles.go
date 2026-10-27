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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Roles a user or group can have on a folder. The server expects these exact
// values when sharing.
const (
	RoleOwner    = "Owner"
	RoleEditor   = "Editor"
	RoleViewer   = "Viewer"
	RoleNoAccess = "No Access"
)

// UserRole is a role a user has on a folder.
type UserRole struct {
	ID        int        `json:"id,omitzero"`
	CreatedAt time.Time  `json:"created_at,omitzero"`
	UpdatedAt time.Time  `json:"updated_at,omitzero"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	IsDeleted *bool      `json:"is_deleted,omitempty"` // nil if the server didn't send it
	Role      string     `json:"role,omitempty"`
	User      User       `json:"user,omitzero"`
}

// GroupRole is a role a group has on a folder.
type GroupRole struct {
	ID        int        `json:"id,omitzero"`
	CreatedAt time.Time  `json:"created_at,omitzero"`
	UpdatedAt time.Time  `json:"updated_at,omitzero"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	IsDeleted *bool      `json:"is_deleted,omitempty"` // nil if the server didn't send it
	Role      string     `json:"role,omitempty"`
	Group     Group      `json:"group,omitzero"`
}

// MyRole is the role that gives the current user access to a folder. It can
// be set on the folder itself or inherited from a parent folder (FolderID),
// and it can belong to the user or to one of their groups (GroupID).
type MyRole struct {
	ID       int    `json:"id,omitzero"`
	Role     string `json:"role,omitempty"`
	UserID   *int   `json:"user_id,omitempty"`
	GroupID  *int   `json:"group_id,omitempty"`
	FolderID *int   `json:"folder_id,omitempty"`
}

// FolderShareRequest describes who to share a folder with. UserRole applies
// to the users and GroupRole to the groups, and both take one of the Role
// constants.
type FolderShareRequest struct {
	UserIDs    []int    `json:"user_ids,omitempty"`
	UserNames  []string `json:"user_names,omitempty"`
	GroupIDs   []int    `json:"group_ids,omitempty"`
	GroupNames []string `json:"group_names,omitempty"`
	UserRole   string   `json:"user_role,omitempty"`
	GroupRole  string   `json:"group_role,omitempty"`
}

// Share gives users and groups a role on a folder, and returns the folder
// with its updated roles. The server adds the roles one at a time, so if it
// fails (for example on an unknown user name) the roles before the failure
// have already been added. It returns 409 if a user or group already has
// that role on the folder.
func (s *FoldersService) Share(ctx context.Context, folderID int, share *FolderShareRequest) (*Folder, *http.Response, error) {
	if share == nil {
		return nil, nil, fmt.Errorf("openrelik: share request must not be nil")
	}

	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "roles")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodPost, endpoint, share)
	if err != nil {
		return nil, nil, err
	}

	folder := new(Folder)
	resp, err := s.client.Do(req, folder)
	if err != nil {
		return nil, resp, err
	}

	return folder, resp, nil
}

// DeleteUserRole removes a user's role from a folder. roleID is the ID of
// the role (UserRole.ID), not of the user.
func (s *FoldersService) DeleteUserRole(ctx context.Context, folderID, roleID int) (*http.Response, error) {
	return s.deleteRole(ctx, folderID, "users", roleID)
}

// DeleteGroupRole removes a group's role from a folder. roleID is the ID of
// the role (GroupRole.ID), not of the group.
func (s *FoldersService) DeleteGroupRole(ctx context.Context, folderID, roleID int) (*http.Response, error) {
	return s.deleteRole(ctx, folderID, "groups", roleID)
}

func (s *FoldersService) deleteRole(ctx context.Context, folderID int, kind string, roleID int) (*http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "roles", kind, strconv.Itoa(roleID))
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return nil, err
	}

	return s.client.Do(req, nil)
}

// GetMyRole retrieves the role that gives the current user access to a
// folder.
func (s *FoldersService) GetMyRole(ctx context.Context, folderID int) (*MyRole, *http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "roles", "me")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	var raw json.RawMessage
	resp, err := s.client.Do(req, &raw)
	if err != nil {
		return nil, resp, err
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("false")) || bytes.Equal(trimmed, []byte("null")) {
		return nil, resp, fmt.Errorf("openrelik: server returned no role for folder %d", folderID)
	}

	role := new(MyRole)
	if err := json.Unmarshal(raw, role); err != nil {
		return nil, resp, &Error{
			Response:   resp,
			StatusCode: resp.StatusCode,
			Body:       raw,
			Cause:      err,
		}
	}
	if role.Role == "" {
		return nil, resp, fmt.Errorf("openrelik: server returned no role for folder %d", folderID)
	}

	return role, resp, nil
}

// ListSharedFolders retrieves the root folders other users have shared with
// the current user, directly or through a group.
func (s *FoldersService) ListSharedFolders(ctx context.Context) ([]Folder, *http.Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "/folders/shared/", nil)
	if err != nil {
		return nil, nil, err
	}

	var folders []Folder
	resp, err := s.client.Do(req, &folders)
	if err != nil {
		return nil, resp, err
	}

	return folders, resp, nil
}
