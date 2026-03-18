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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// FilesService handles communication with file-related methods of the OpenRelik API.
type FilesService struct {
	client *Client
}

// File represents a file within the OpenRelik system.
type File struct {
	ID              int        `json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at"`
	IsDeleted       bool       `json:"is_deleted"`
	DisplayName     string     `json:"display_name"`
	Description     *string    `json:"description"`
	UUID            string     `json:"uuid"`
	Filename        string     `json:"filename"`
	Filesize        int64      `json:"filesize"`
	Extension       string     `json:"extension"`
	OriginalPath    *string    `json:"original_path"`
	MagicText       string     `json:"magic_text"`
	MagicMime       string     `json:"magic_mime"`
	DataType        string     `json:"data_type"`
	HashMD5         string     `json:"hash_md5"`
	HashSHA1        string     `json:"hash_sha1"`
	HashSHA256      string     `json:"hash_sha256"`
	HashSSDeep      *string    `json:"hash_ssdeep"`
	StorageProvider *string    `json:"storage_provider"`
	StorageKey      *string    `json:"storage_key"`
	UserID          int        `json:"user_id"`
	User            User       `json:"user"`
	Folder          Folder     `json:"folder"`
}

// Info retrieves the metadata for a single file by ID.
func (s *FilesService) Info(ctx context.Context, fileID int) (*File, *http.Response, error) {
	endpoint, err := url.JoinPath("files", strconv.Itoa(fileID))
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	file := new(File)
	resp, err := s.client.Do(req, file)
	if err != nil {
		return nil, resp, err
	}

	return file, resp, nil
}

// Download downloads a file by ID. It returns an io.ReadCloser for streaming
// the file content, the HTTP response, and any error encountered.
// The caller is responsible for closing the returned io.ReadCloser.
func (s *FilesService) Download(ctx context.Context, fileID int) (io.ReadCloser, *http.Response, error) {
	endpoint, err := url.JoinPath("files", strconv.Itoa(fileID), "download")
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	// We use the client's httpClient directly to support streaming.
	// client.Do() buffers the entire response body into memory.
	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()

		// Read body into memory to allow inspection.
		reader := io.Reader(resp.Body)
		if s.client.maxResponseSize > 0 {
			reader = io.LimitReader(resp.Body, s.client.maxResponseSize+1)
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, resp, err
		}

		if s.client.maxResponseSize > 0 && int64(len(data)) > s.client.maxResponseSize {
			return nil, resp, fmt.Errorf("openrelik: response body too large (limit %d bytes)", s.client.maxResponseSize)
		}

		// Repopulate body so it can be read again if needed, matching Client.Do behavior.
		resp.Body = io.NopCloser(bytes.NewBuffer(data))
		return nil, resp, s.client.newError(resp, data)
	}

	return resp.Body, resp, nil
}
