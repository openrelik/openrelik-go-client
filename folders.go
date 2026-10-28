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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// FoldersService handles communication with folder-related methods of the OpenRelik API.
type FoldersService struct {
	client *Client
}

// Folder represents a folder within the OpenRelik system.
type Folder struct {
	ID          int        `json:"id,omitzero"`
	CreatedAt   time.Time  `json:"created_at,omitzero"`
	UpdatedAt   time.Time  `json:"updated_at,omitzero"`
	DeletedAt   *time.Time `json:"deleted_at"`
	IsDeleted   *bool      `json:"is_deleted,omitempty"` // nil if the server didn't send it
	DisplayName string     `json:"display_name,omitempty"`
	Description *string    `json:"description"`
	UUID        string     `json:"uuid,omitempty"`
	User        User       `json:"user,omitzero"`
	Workflows   []any      `json:"workflows"`
	// Parent is the folder's parent, with its own Parent set recursively up
	// to the root. It is typically only populated by Get; list endpoints
	// return compact folders without a parent.
	Parent *Folder `json:"parent,omitempty"`
	// UserRoles and GroupRoles are the roles set on this folder itself, not
	// those inherited from parent folders. Only Get and Share populate them.
	UserRoles  []UserRole  `json:"user_roles,omitempty"`
	GroupRoles []GroupRole `json:"group_roles,omitempty"`
}

// FolderFile represents a compact file entry within a folder.
type FolderFile struct {
	ID          int       `json:"id,omitzero"`
	DisplayName string    `json:"display_name,omitempty"`
	Filesize    int64     `json:"filesize"`
	DataType    string    `json:"data_type,omitempty"`
	MagicMime   string    `json:"magic_mime,omitempty"`
	User        User      `json:"user,omitzero"`
	CreatedAt   time.Time `json:"created_at,omitzero"`
	UpdatedAt   time.Time `json:"updated_at,omitzero"`
	IsDeleted   *bool     `json:"is_deleted,omitempty"` // nil if the server didn't send it
}

type rootFoldersResponse struct {
	Folders    []Folder `json:"folders"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	TotalCount int      `json:"total_count"`
	Sort       string   `json:"sort"`
	Order      string   `json:"order"`
	NextCursor string   `json:"next_cursor"`
}

// ErrCursorsUnsupported is returned when PageOptions.After is set but the
// server doesn't support cursors (older servers). Such servers ignore the
// cursor and return the first page, which would repeat folders.
var ErrCursorsUnsupported = errors.New("openrelik: the server doesn't support cursors")

// PageOptions specifies pagination parameters for list methods that support it.
type PageOptions struct {
	// Page is the 1-based page number. Zero uses the server default. Not
	// sent when After is set.
	Page int
	// PageSize is the number of items per page. Zero uses the server default.
	PageSize int
	// Sort is the field to sort folders by, in lowercase: "created_at",
	// "updated_at" or "display_name" (names are compared ignoring case).
	// Empty uses the server default, created_at. Older servers ignore it;
	// check FolderPage.Sort.
	Sort string
	// Order is "asc" or "desc". Empty uses the server default, desc.
	Order string
	// After continues a listing from the NextCursor of a previous page,
	// requested with the same Sort and Order (and search query). Unlike page
	// numbers, cursors don't skip or repeat folders created or deleted while
	// paging. Servers that don't support cursors give ErrCursorsUnsupported.
	After string
}

// FolderPage is a single page of folders along with pagination metadata.
type FolderPage struct {
	Folders []Folder
	// Page is the page number, zero for pages requested with
	// PageOptions.After.
	Page     int
	PageSize int
	// TotalCount is the number of folders in the listing. The server only
	// counts them for requests without a cursor, so it is zero for pages
	// requested with PageOptions.After.
	TotalCount int
	// Sort and Order are the order the server applied. They are empty when
	// the server doesn't support sorting and cursors (older servers), in
	// which case it ignored PageOptions.Sort, Order and After, and the
	// folders are newest created first.
	Sort  string
	Order string
	// NextCursor is the PageOptions.After for the next page, or empty on the
	// last page and on servers that don't support cursors.
	NextCursor string
}

// ListRootFolders retrieves the first page of root folders using the server's
// default page size. Use ListRootFoldersPage to control pagination.
//
// Root folders are the user's own top-level folders plus any folder shared
// directly with the user, which may be a subfolder.
func (s *FoldersService) ListRootFolders(ctx context.Context) ([]Folder, *http.Response, error) {
	page, resp, err := s.ListRootFoldersPage(ctx, nil)
	if err != nil {
		return nil, resp, err
	}
	return page.Folders, resp, nil
}

// ListRootFoldersPage retrieves a single page of root folders (see
// ListRootFolders for what counts as a root folder). A nil opts uses the
// server defaults.
func (s *FoldersService) ListRootFoldersPage(ctx context.Context, opts *PageOptions) (*FolderPage, *http.Response, error) {
	return s.listAllFolders(ctx, "", opts)
}

// SearchFolders retrieves a single page of folders whose names contain query
// (case-insensitive). Leading and trailing whitespace in query is ignored.
// Unlike ListRootFoldersPage, it searches every folder the user can access,
// including subfolders. A nil opts uses the server defaults. Sort, Order and
// After work as for ListRootFoldersPage; a cursor only continues the search
// it came from.
func (s *FoldersService) SearchFolders(ctx context.Context, query string, opts *PageOptions) (*FolderPage, *http.Response, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil, fmt.Errorf("openrelik: search query must not be empty")
	}
	return s.listAllFolders(ctx, query, opts)
}

// listAllFolders calls the /folders/all/ endpoint, which lists root folders,
// or searches all folders when query is set.
func (s *FoldersService) listAllFolders(ctx context.Context, query string, opts *PageOptions) (*FolderPage, *http.Response, error) {
	endpoint := "/folders/all/"
	q := url.Values{}
	if query != "" {
		q.Set("q", query)
	}
	if opts != nil {
		// The cursor gives the position; a page number would contradict it.
		if opts.Page > 0 && opts.After == "" {
			q.Set("page", strconv.Itoa(opts.Page))
		}
		if opts.PageSize > 0 {
			q.Set("page_size", strconv.Itoa(opts.PageSize))
		}
		if opts.Sort != "" {
			q.Set("sort", opts.Sort)
		}
		if opts.Order != "" {
			q.Set("order", opts.Order)
		}
		if opts.After != "" {
			q.Set("after", opts.After)
		}
	}
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	rootResp := new(rootFoldersResponse)
	resp, err := s.client.Do(req, rootResp)
	if err != nil {
		return nil, resp, err
	}
	// Servers that support cursors echo the order they applied.
	if opts != nil && opts.After != "" && rootResp.Sort == "" {
		return nil, resp, ErrCursorsUnsupported
	}

	// JSON null (page and total_count with a cursor, next_cursor on the last
	// page) leaves the zero value.
	return &FolderPage{
		Folders:    rootResp.Folders,
		Page:       rootResp.Page,
		PageSize:   rootResp.PageSize,
		TotalCount: rootResp.TotalCount,
		Sort:       rootResp.Sort,
		Order:      rootResp.Order,
		NextCursor: rootResp.NextCursor,
	}, resp, nil
}

// Get retrieves a single folder, including its chain of parent folders.
func (s *FoldersService) Get(ctx context.Context, folderID int) (*Folder, *http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID))
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
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

// ListSubFolders retrieves subfolders for a given folder ID.
func (s *FoldersService) ListSubFolders(ctx context.Context, folderID int) ([]Folder, *http.Response, error) {
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

// ListFiles retrieves files for a given folder ID.
func (s *FoldersService) ListFiles(ctx context.Context, folderID int) ([]FolderFile, *http.Response, error) {
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
