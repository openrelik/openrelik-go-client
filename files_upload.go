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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultChunkSize = 4 * 1024 * 1024 // 4MB
	maxRetries       = 3
	retryDelay       = 1 * time.Second
)

// ProgressCallback is called during upload to report progress.
// bytesSent is the cumulative number of bytes successfully uploaded so far.
// totalBytes is the total size of the file being uploaded, or -1 if the size is unknown.
type ProgressCallback func(bytesSent, totalBytes int64)

// RetryCallback is called when a chunk upload fails and is about to be retried.
// chunkNum is the number of the chunk being uploaded.
// attempt is the retry attempt number (1, 2, ...).
// err is the error that caused the retry.
type RetryCallback func(chunkNum, attempt int, err error)

type uploadConfig struct {
	chunkSize    int
	progressFunc ProgressCallback
	retryFunc    RetryCallback
	totalSize    int64
}

// UploadOption defines a functional option for UploadFile.
type UploadOption func(*uploadConfig)

// WithChunkSize sets the size of each chunk uploaded. Default is 4MB.
func WithChunkSize(size int) UploadOption {
	return func(c *uploadConfig) {
		c.chunkSize = size
	}
}

// WithUploadProgress sets a callback to track the upload progress.
func WithUploadProgress(fn ProgressCallback) UploadOption {
	return func(c *uploadConfig) {
		c.progressFunc = fn
	}
}

// WithUploadRetry sets a callback to be notified when a chunk upload is retried.
func WithUploadRetry(fn RetryCallback) UploadOption {
	return func(c *uploadConfig) {
		c.retryFunc = fn
	}
}

// WithTotalSize sets the total size of the file. If not provided,
// UploadFile will try to determine it if the reader is an io.Seeker.
// Use -1 to explicitly signal an unknown size for streaming readers.
func WithTotalSize(size int64) UploadOption {
	return func(c *uploadConfig) {
		c.totalSize = size
	}
}

// Upload uploads a file to the specified folder using chunked, resumable uploads.
// The backend API expects multipart/form-data for each chunk.
func (s *FilesService) Upload(ctx context.Context, folderID int, filename string, r io.Reader, opts ...UploadOption) (*File, *http.Response, error) {
	config := &uploadConfig{
		chunkSize: defaultChunkSize,
		totalSize: -1, // Default to unknown
	}
	for _, opt := range opts {
		opt(config)
	}

	// Try to determine total size if not provided and not explicitly unknown
	if config.totalSize < 0 {
		if s, ok := r.(io.Seeker); ok {
			size, err := s.Seek(0, io.SeekEnd)
			if err == nil {
				config.totalSize = size
				_, _ = s.Seek(0, io.SeekStart)
			}
		}
	}

	// Prevent "Stream Footgun": If we still don't know the size and can't seek,
	// we cannot reliably tell the backend how many chunks to expect.
	if config.totalSize < 0 {
		return nil, nil, fmt.Errorf("openrelik: total file size must be provided via WithTotalSize for non-seeking readers (e.g. pipes or stdin)")
	}

	// Generate a unique identifier for this upload session
	resumableIdentifier, err := generateRandomID(16)
	if err != nil {
		return nil, nil, fmt.Errorf("openrelik: failed to generate upload ID: %w", err)
	}

	var lastResp *http.Response
	var lastFile *File
	var bytesSent int64

	var totalChunks int
	if config.totalSize > 0 {
		totalChunks = int(config.totalSize / int64(config.chunkSize))
		if config.totalSize%int64(config.chunkSize) != 0 {
			totalChunks++
		}
	} else {
		// Default to 1 if size is unknown (-1) or empty (0). Note: This will only
		// work correctly if the file actually fits in a single chunk. For larger
		// streaming uploads, the caller MUST provide the total size via WithTotalSize.
		totalChunks = 1
	}

	// Reuse buffer to reduce GC pressure
	chunkBuffer := make([]byte, config.chunkSize)

	for chunkNum := 1; ; chunkNum++ {
		n, err := io.ReadFull(r, chunkBuffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, nil, fmt.Errorf("openrelik: failed to read chunk %d: %w", chunkNum, err)
		}
		currentChunkData := chunkBuffer[:n]

		// If we read 0 bytes and it's not the first chunk, we're done
		if n == 0 && chunkNum > 1 {
			break
		}

		// Retry loop for individual chunk upload
		var resp *http.Response
		var uploadErr error
		file := new(File)

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				if config.retryFunc != nil {
					config.retryFunc(chunkNum, attempt, uploadErr)
				}

				delay := retryDelay * time.Duration(attempt)
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, nil, ctx.Err()
				case <-timer.C:
				}
			}

			resp, uploadErr = s.uploadChunk(ctx, folderID, filename, resumableIdentifier, chunkNum, totalChunks, config.totalSize, currentChunkData, file)
			if uploadErr == nil {
				break
			}

			// If we got a 429, the server has given up on this upload
			if apiErr, ok := uploadErr.(*Error); ok && apiErr.StatusCode == http.StatusTooManyRequests {
				return nil, resp, uploadErr
			}

			// Only retry on 503 or network errors
			if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
				return nil, resp, uploadErr
			}
		}

		if uploadErr != nil {
			return nil, resp, fmt.Errorf("openrelik: failed to upload chunk %d after %d retries: %w", chunkNum, maxRetries, uploadErr)
		}

		bytesSent += int64(n)
		if config.progressFunc != nil {
			config.progressFunc(bytesSent, config.totalSize)
		}

		lastResp = resp
		lastFile = file

		// If we read less than chunkSize, it's the last chunk
		if n < config.chunkSize || (config.totalSize >= 0 && bytesSent >= config.totalSize) {
			break
		}
	}

	return lastFile, lastResp, nil
}

func (s *FilesService) uploadChunk(ctx context.Context, folderID int, filename, identifier string, chunkNum, totalChunks int, totalSize int64, data []byte, v *File) (*http.Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodPost, "files/upload", nil)
	if err != nil {
		return nil, err
	}

	// Construct the query parameters
	q := req.URL.Query()
	q.Set("resumableChunkNumber", strconv.Itoa(chunkNum))
	q.Set("resumableTotalChunks", strconv.Itoa(totalChunks))
	q.Set("resumableIdentifier", identifier)
	q.Set("resumableFilename", filename)
	q.Set("folder_id", strconv.Itoa(folderID))
	if totalSize > 0 {
		q.Set("resumableTotalSize", strconv.FormatInt(totalSize, 10))
	}
	req.URL.RawQuery = q.Encode()

	// Prepare multipart body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	writer.Close()

	// Captured body content for GetBody
	bodyBytes := body.Bytes()

	// Implement GetBody to support authentication token refresh retries
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.ContentLength = int64(len(bodyBytes))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return s.client.Do(req, v)
}

func generateRandomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
