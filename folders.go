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
	"net/url"
	"strconv"
	"time"
)

// FoldersService handles communication with folder-related methods of the OpenRelik API.
type FoldersService struct {
	client *Client
}

// Folder represents a folder within the OpenRelik system.
type Folder struct {
	ID          int        `json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
	IsDeleted   bool       `json:"is_deleted"`
	DisplayName string     `json:"display_name"`
	User        User       `json:"user"`
	Workflows   []any      `json:"workflows"`
}

// FolderFile represents a compact file entry within a folder.
type FolderFile struct {
	ID          int       `json:"id"`
	DisplayName string    `json:"display_name"`
	Filesize    int64     `json:"filesize"`
	DataType    string    `json:"data_type"`
	MagicMime   string    `json:"magic_mime"`
	User        User      `json:"user"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	IsDeleted   bool      `json:"is_deleted"`
}

type rootFoldersResponse struct {
	Folders    []Folder `json:"folders"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	TotalCount int      `json:"total_count"`
}

// GetRootFolders retrieves all root folders.
func (s *FoldersService) GetRootFolders(ctx context.Context) ([]Folder, *http.Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "/folders/all/", nil)
	if err != nil {
		return nil, nil, err
	}

	rootResp := new(rootFoldersResponse)
	resp, err := s.client.Do(req, rootResp)
	if err != nil {
		return nil, resp, err
	}

	return rootResp.Folders, resp, nil
}

// GetSubFolders retrieves subfolders for a given folder ID.
func (s *FoldersService) GetSubFolders(ctx context.Context, folderID int) ([]Folder, *http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "folders/")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
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

// GetFiles retrieves files for a given folder ID.
func (s *FoldersService) GetFiles(ctx context.Context, folderID int) ([]FolderFile, *http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "files/")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	var files []FolderFile
	resp, err := s.client.Do(req, &files)
	if err != nil {
		return nil, resp, err
	}

	return files, resp, nil
}

// CreateRootFolder creates a new root folder.
func (s *FoldersService) CreateRootFolder(ctx context.Context, displayName string) (*Folder, *http.Response, error) {
	body := struct {
		DisplayName string `json:"display_name"`
	}{
		DisplayName: displayName,
	}

	req, err := s.client.NewRequest(ctx, http.MethodPost, "/folders/", body)
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

// CreateSubFolder creates a new subfolder within a parent folder.
func (s *FoldersService) CreateSubFolder(ctx context.Context, parentID int, displayName string) (*Folder, *http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(parentID), "folders/")
	if err != nil {
		return nil, nil, err
	}

	body := struct {
		DisplayName string `json:"display_name"`
	}{
		DisplayName: displayName,
	}

	req, err := s.client.NewRequest(ctx, http.MethodPost, endpoint, body)
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
