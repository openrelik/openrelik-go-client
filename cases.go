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
)

// A case folder is the one root folder of a case, found by its case
// reference (Folder.CaseRef). The server allows at most one folder per
// reference. It normalizes references, so "CASE 123" and "case-123" are the
// same case, and returns the normalized form.

// ErrCaseFoldersUnsupported is returned by the case folder methods when the
// server doesn't support case folders (older servers).
var ErrCaseFoldersUnsupported = errors.New("openrelik: the server doesn't support case folders")

// caseFoldersUnsupported reports whether err is the response of a server
// without the case folder endpoints: a 404 with the server's message for an
// unknown route ("Not Found"), or a 405 when the route exists for other
// methods. Servers that support case folders say what wasn't found ("Case not
// found.", "Folder not found."). Other 404s, such as a proxy's HTML page for
// a wrong server URL, don't come from the server and stay plain 404s.
func caseFoldersUnsupported(err error) bool {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusMethodNotAllowed:
		return true
	case http.StatusNotFound:
		return apiErr.Message == "Not Found"
	}
	return false
}

// caseError returns err, marked with ErrCaseFoldersUnsupported if the server
// doesn't support case folders.
func caseError(err error) error {
	if caseFoldersUnsupported(err) {
		return fmt.Errorf("%w: %w", ErrCaseFoldersUnsupported, err)
	}
	return err
}

// checkCaseRef returns an error if caseRef is blank. The server refuses blank
// references too, but checking first gives a clearer error.
func checkCaseRef(caseRef string) error {
	if strings.TrimSpace(caseRef) == "" {
		return fmt.Errorf("openrelik: case reference must not be empty")
	}
	return nil
}

// CreateCaseFolder creates a root folder that is the case folder of caseRef.
// If another folder has the reference, the server answers 409 Conflict,
// naming that folder if the user can access it.
//
// Older servers ignore the reference and create an ordinary root folder. Then
// the folder is returned with ErrCaseFoldersUnsupported, so the caller can
// report or remove it.
func (s *FoldersService) CreateCaseFolder(ctx context.Context, caseRef, displayName string) (*Folder, *http.Response, error) {
	if err := checkCaseRef(caseRef); err != nil {
		return nil, nil, err
	}
	folder, resp, err := s.createRootFolder(ctx, displayName, caseRef)
	if err != nil {
		return nil, resp, err
	}
	if folder.CaseRef == "" {
		return folder, resp, ErrCaseFoldersUnsupported
	}

	return folder, resp, nil
}

// GetCaseFolder retrieves the case folder of caseRef. The server answers 404
// both when there is no such case and when the user can't access it.
func (s *FoldersService) GetCaseFolder(ctx context.Context, caseRef string) (*Folder, *http.Response, error) {
	if err := checkCaseRef(caseRef); err != nil {
		return nil, nil, err
	}
	// The reference goes into the path with its "/" kept, which the server
	// reads as part of the reference. Every other character is escaped. Empty,
	// "." and ".." parts would be resolved away, so they are refused, as the
	// server does.
	parts := strings.Split(caseRef, "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, nil, fmt.Errorf("openrelik: invalid case reference %q", caseRef)
		}
		parts[i] = url.PathEscape(part)
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, "folders/case/"+strings.Join(parts, "/"), nil)
	if err != nil {
		return nil, nil, err
	}

	folder := new(Folder)
	resp, err := s.client.Do(req, folder)
	// Older servers can redirect some references to other folder routes, by
	// adding a trailing slash. Servers with case folders don't redirect.
	if resp != nil && resp.Request != nil && resp.Request.URL.Path != req.URL.Path {
		if err == nil {
			err = fmt.Errorf("redirected to %s", resp.Request.URL.Path)
		}
		return nil, resp, fmt.Errorf("%w: %w", ErrCaseFoldersUnsupported, err)
	}
	if err != nil {
		return nil, resp, caseError(err)
	}
	if folder.CaseRef == "" {
		return nil, resp, ErrCaseFoldersUnsupported
	}

	return folder, resp, nil
}

// SetCaseRef makes the root folder folderID the case folder of caseRef, or
// changes its reference. Editors and owners of the folder, and admins, can
// set it. If another folder has the reference, the server answers 409
// Conflict.
func (s *FoldersService) SetCaseRef(ctx context.Context, folderID int, caseRef string) (*Folder, *http.Response, error) {
	if err := checkCaseRef(caseRef); err != nil {
		return nil, nil, err
	}
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "case")
	if err != nil {
		return nil, nil, err
	}

	body := struct {
		CaseRef string `json:"case_ref"`
	}{
		CaseRef: caseRef,
	}

	req, err := s.client.NewRequest(ctx, http.MethodPut, endpoint, body)
	if err != nil {
		return nil, nil, err
	}

	folder := new(Folder)
	resp, err := s.client.Do(req, folder)
	if err != nil {
		return nil, resp, caseError(err)
	}
	if folder.CaseRef == "" {
		return nil, resp, ErrCaseFoldersUnsupported
	}

	return folder, resp, nil
}

// ClearCaseRef removes the case reference of folderID, which becomes an
// ordinary root folder. The reference is free for other folders again.
func (s *FoldersService) ClearCaseRef(ctx context.Context, folderID int) (*http.Response, error) {
	endpoint, err := url.JoinPath("folders", strconv.Itoa(folderID), "case")
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, caseError(err)
	}

	return resp, nil
}
