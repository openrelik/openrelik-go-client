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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupWorkersTestServer(t *testing.T) (mux *http.ServeMux, server *httptest.Server, client *Client) {
	mux = http.NewServeMux()
	server = httptest.NewServer(mux)
	var err error
	client, err = NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return
}

func TestWorkersService_Registered(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		mux, server, client := setupWorkersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/taskqueue/tasks/registered", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("Expected method GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[
				{
					"task_name": "openrelik-worker-grep.tasks.grep",
					"queue_name": "openrelik-worker-grep",
					"display_name": "Grep",
					"description": "Search for a regular expression in a file (case insensitive).",
					"task_config": [
						{
							"name": "regex",
							"label": "[a-f][0-9]+",
							"description": "Regular expression to grep for",
							"type": "text",
							"required": true
						}
					]
				},
				{
					"task_name": "openrelik-worker-capa.tasks.capa",
					"queue_name": "openrelik-worker-capa",
					"display_name": "Capa Malware Analysis",
					"description": "Detect capabilities from executable files"
				},
				{
					"task_name": "openrelik-worker-strings.tasks.strings",
					"queue_name": "openrelik-worker-strings",
					"display_name": "Strings",
					"description": "Extract strings from files",
					"task_config": [
						{
							"name": "UTF16LE",
							"label": "Extract Unicode strings",
							"description": "This will tell the strings command to extract UTF-16LE (little endian) encoded strings",
							"type": "checkbox",
							"default": true
						},
						{
							"name": "ASCII",
							"label": "Extract ASCII strings",
							"description": "This will tell the strings command to extract ASCII (single-7-bit-byte) encoded strings",
							"type": "checkbox",
							"default": true
						}
					]
				}
			]`)
		})

		workers, resp, err := client.Workers().Registered(ctx)
		if err != nil {
			t.Fatalf("Registered returned error: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if len(workers) != 3 {
			t.Errorf("Expected 3 workers, got %d", len(workers))
		}

		if workers[0].TaskName != "openrelik-worker-grep.tasks.grep" {
			t.Errorf("Expected task name 'openrelik-worker-grep.tasks.grep', got %s", workers[0].TaskName)
		}

		if len(workers[0].TaskConfig) != 1 {
			t.Errorf("Expected 1 task config for worker 0, got %d", len(workers[0].TaskConfig))
		}

		if workers[0].TaskConfig[0].Required != true {
			t.Errorf("Expected task config to be required")
		}

		if workers[2].TaskConfig[0].Default != true {
			t.Errorf("Expected task config default to be true, got %v", workers[2].TaskConfig[0].Default)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		mux, server, client := setupWorkersTestServer(t)
		defer server.Close()

		mux.HandleFunc("/api/v1/taskqueue/tasks/registered", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		_, _, err := client.Workers().Registered(ctx)
		if err == nil {
			t.Error("Expected error for 404 status code")
		}
	})
}
