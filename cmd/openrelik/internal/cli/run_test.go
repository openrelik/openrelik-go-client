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

package cli

import (
	"os"
	"testing"

	"github.com/openrelik/openrelik-go-client"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/config"
)

func TestDynamicWorkerCommands(t *testing.T) {
	// Create a temporary directory for config
	tmpDir, err := os.MkdirTemp("", "openrelik-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config.SetBaseDir(tmpDir)

	// Pre-populate workers cache
	testWorkers := []openrelik.Worker{
		{
			TaskName:    "openrelik-worker-strings.tasks.strings",
			DisplayName: "Strings",
			Description: "Extract strings from files",
			TaskConfig: []openrelik.TaskConfig{
				{
					Name:        "min-len",
					Description: "Minimum string length",
					Type:        "integer",
					Default:     4,
				},
			},
		},
		{
			TaskName:    "openrelik-worker-grep.tasks.grep",
			DisplayName: "Grep",
			Description: "Search for patterns in files",
			TaskConfig: []openrelik.TaskConfig{
				{
					Name:        "regex",
					Description: "Regex pattern",
					Type:        "string",
					Required:    true,
				},
			},
		},
	}

	if err := config.SaveWorkersCache(testWorkers); err != nil {
		t.Fatalf("Failed to save workers cache: %v", err)
	}

	// Create run command
	runCmd := newRunCmd()

	// Verify subcommands exist
	subCommands := runCmd.Commands()
	if len(subCommands) != 2 {
		t.Errorf("Expected 2 subcommands, got %d", len(subCommands))
	}

	foundStrings := false
	foundGrep := false
	for _, cmd := range subCommands {
		if cmd.Name() == "strings" {
			foundStrings = true
			// Check flags
			if cmd.Flags().Lookup("min-len") == nil {
				t.Errorf("strings command missing 'min-len' flag")
			}
		}
		if cmd.Name() == "grep" {
			foundGrep = true
			// Check flags
			if cmd.Flags().Lookup("regex") == nil {
				t.Errorf("grep command missing 'regex' flag")
			}
		}
	}

	if !foundStrings {
		t.Errorf("strings subcommand not found")
	}
	if !foundGrep {
		t.Errorf("grep subcommand not found")
	}
}
