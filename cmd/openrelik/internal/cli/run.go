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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openrelik/openrelik-go-client"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/config"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/util"
	"github.com/spf13/cobra"
)

type WorkflowTaskConfig struct {
	Name        string      `json:"name"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Type        string      `json:"type"`
	Required    bool        `json:"required,omitempty"`
	Value       interface{} `json:"value"`
}

type WorkflowTask struct {
	UUID        string               `json:"uuid"`
	TaskName    string               `json:"task_name"`
	QueueName   string               `json:"queue_name"`
	DisplayName string               `json:"display_name"`
	Description string               `json:"description"`
	TaskConfig  []WorkflowTaskConfig `json:"task_config"`
	Type        string               `json:"type"`
	Tasks       []WorkflowTask       `json:"tasks"` // For nested tasks if needed
}

type WorkflowSpecInner struct {
	Type   string         `json:"type"`
	IsRoot bool           `json:"isRoot"`
	Tasks  []WorkflowTask `json:"tasks"`
}

type WorkflowSpec struct {
	Workflow WorkflowSpecInner `json:"workflow"`
}

func newRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run a worker on files",
		Long: `Execute OpenRelik workers on files.
Subcommands are dynamically generated based on registered workers.

Command chaining is supported using --then:
openrelik run strings --then grep --regex "foo" 123

Parallel execution is supported using --and:
openrelik run strings --and grep 123`,
	}

	// Global run flags (placeholders for now)
	runCmd.PersistentFlags().StringP("directory", "d", ".", "Output directory for downloads")
	runCmd.PersistentFlags().String("download", "none", "Download policy (final, all, none)")
	runCmd.PersistentFlags().Bool("task-folders", false, "Organize downloads into task folders")
	runCmd.PersistentFlags().String("then", "", "Chain workers (use as delimiter)")
	runCmd.PersistentFlags().String("and", "", "Run workers in parallel (use as delimiter)")

	// dry-run is only for the 'run' command itself, not subcommands
	runCmd.Flags().Bool("dry-run", false, "Generate and display workflow spec without executing")

	// Load workers from cache to build dynamic subcommands
	workers, err := config.LoadWorkersCache()
	if err != nil {
		// If cache load fails, we don't add dynamic subcommands.
		// User can run 'openrelik workers list --refresh' to populate cache.
		return runCmd
	}

	for _, worker := range workers {
		workerCmd := createWorkerCmd(worker, workers)
		runCmd.AddCommand(workerCmd)
	}

	return runCmd
}

func createWorkerCmd(worker openrelik.Worker, allWorkers []openrelik.Worker) *cobra.Command {
	// Worker names are usually something like "openrelik-worker-strings.tasks.strings"
	// We want a shorter alias if possible, but for now we'll use the task name parts.
	use := worker.TaskName
	parts := strings.Split(worker.TaskName, ".")
	if len(parts) > 0 {
		use = parts[len(parts)-1]
	}

	cmd := &cobra.Command{
		Use:                use,
		Short:              worker.DisplayName,
		Long:               worker.Description,
		DisableFlagParsing: true, // We will parse flags manually for each segment
		RunE: func(cmd *cobra.Command, args []string) error {
			// Slice arguments by --then or --and, skipping 'run' command flags
			segments, delimiter, err := sliceArgs(os.Args, allWorkers)
			if err != nil {
				return err
			}

			// Check if help was requested in ANY segment.
			// We show help for the LAST segment that has it.
			var helpWorker *openrelik.Worker
			for i := len(segments) - 1; i >= 0; i-- {
				segment := segments[i]
				hasHelp := false
				for _, arg := range segment {
					if arg == "--help" || arg == "-h" {
						hasHelp = true
						break
					}
				}
				if hasHelp {
					workerName := segment[0]
					for _, w := range allWorkers {
						wUse := w.TaskName
						wParts := strings.Split(w.TaskName, ".")
						if len(wParts) > 0 {
							wUse = wParts[len(wParts)-1]
						}
						if wUse == workerName || w.TaskName == workerName {
							helpWorker = &w
							break
						}
					}
					if helpWorker != nil {
						break
					}
				}
			}

			if helpWorker != nil {
				tempCmd := createWorkerCmd(*helpWorker, nil)
				tempCmd.DisableFlagParsing = false
				return tempCmd.Help()
			}

			// Get global flags from the run command
			runCmd := cmd.Parent()
			for runCmd != nil && runCmd.Name() != "run" {
				runCmd = runCmd.Parent()
			}

			var dryRun bool
			if runCmd != nil {
				// dry-run is a non-persistent flag on the 'run' command
				dryRun, _ = runCmd.Flags().GetBool("dry-run")
			}

			// Root workflow type is always 'chain'
			spec := WorkflowSpec{
				Workflow: WorkflowSpecInner{
					Type:   "chain",
					IsRoot: true,
					Tasks:  []WorkflowTask{},
				},
			}

			var positionalArgs []string
			var currentParent *WorkflowTask

			// We need to find the worker info for each segment.
			for _, segment := range segments {
				if len(segment) == 0 {
					continue
				}

				// segment[0] is the worker name (or alias)
				workerName := segment[0]
				var currentWorker *openrelik.Worker
				for _, w := range allWorkers {
					wUse := w.TaskName
					wParts := strings.Split(w.TaskName, ".")
					if len(wParts) > 0 {
						wUse = wParts[len(wParts)-1]
					}
					if wUse == workerName || w.TaskName == workerName {
						currentWorker = &w
						break
					}
				}

				if currentWorker == nil {
					return fmt.Errorf("unknown worker: %s", workerName)
				}

				// Create a temporary command to parse flags for this segment
				tempCmd := createWorkerCmd(*currentWorker, nil)
				// We MUST enable flag parsing for the temp command so Execute() parses them
				tempCmd.DisableFlagParsing = false

				// Add persistent flags from run command to temp command
				if runCmd != nil {
					tempCmd.PersistentFlags().AddFlagSet(runCmd.PersistentFlags())
				}

				// We need to strip the worker name from the segment for parsing
				tempCmd.SetArgs(segment[1:])
				// We don't want it to actually run, just parse
				tempCmd.RunE = func(c *cobra.Command, a []string) error { return nil }

				// Redirect output to avoid cluttering
				tempCmd.SetOut(io.Discard)
				tempCmd.SetErr(io.Discard)

				if err := tempCmd.Execute(); err != nil {
					return fmt.Errorf("failed to parse flags for worker %s: %w", workerName, err)
				}

				// Collect positional args from this segment
				positionalArgs = append(positionalArgs, tempCmd.Flags().Args()...)

				taskConfig := []WorkflowTaskConfig{}
				for _, cfg := range currentWorker.TaskConfig {
					wCfg := WorkflowTaskConfig{
						Name:        cfg.Name,
						Label:       cfg.Label,
						Description: cfg.Description,
						Type:        cfg.Type,
						Required:    cfg.Required,
					}
					switch cfg.Type {
					case "string", "text", "textarea", "artifacts", "autocomplete", "select":
						wCfg.Value, _ = tempCmd.Flags().GetString(cfg.Name)
					case "boolean", "checkbox":
						wCfg.Value, _ = tempCmd.Flags().GetBool(cfg.Name)
					case "integer":
						wCfg.Value, _ = tempCmd.Flags().GetInt(cfg.Name)
					}
					taskConfig = append(taskConfig, wCfg)
				}

				newTask := WorkflowTask{
					UUID:        util.GenerateUUID(),
					TaskName:    currentWorker.TaskName,
					QueueName:   currentWorker.QueueName,
					DisplayName: currentWorker.DisplayName,
					Description: currentWorker.Description,
					TaskConfig:  taskConfig,
					Type:        "task",
					Tasks:       []WorkflowTask{},
				}

				if delimiter == "--and" {
					// Parallel execution: tasks are flat in the root 'tasks' array
					spec.Workflow.Tasks = append(spec.Workflow.Tasks, newTask)
				} else {
					// Sequential execution (or single worker): tasks are nested
					if currentParent == nil {
						spec.Workflow.Tasks = append(spec.Workflow.Tasks, newTask)
						currentParent = &spec.Workflow.Tasks[0]
					} else {
						currentParent.Tasks = append(currentParent.Tasks, newTask)
						currentParent = &currentParent.Tasks[0]
					}
				}
			}

			if dryRun {
				fmt.Println("Dry run: generating workflow spec...")
				fmt.Printf("Inputs: %v\n", positionalArgs)
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(spec); err != nil {
					return fmt.Errorf("failed to encode workflow spec: %w", err)
				}
				return nil
			}

			// Phase 3: Resolve inputs and execute
			client, err := newClient()
			if err != nil {
				return err
			}

			fileIDs, err := resolveInputs(cmd.Context(), client, positionalArgs)
			if err != nil {
				return err
			}

			if len(fileIDs) == 0 {
				return fmt.Errorf("no inputs provided (IDs or file paths)")
			}

			// Step 1: Create Workflow
			workflow, _, err := client.Workflows().Create(cmd.Context(), 0, fileIDs, nil, nil)
			if err != nil {
				return fmt.Errorf("failed to create workflow: %w", err)
			}

			// Step 2: Run Workflow
			specBytes, _ := json.Marshal(spec)
			specJSON := string(specBytes)
			workflow, _, err = client.Workflows().Run(cmd.Context(), workflow.Folder.ID, workflow.ID, &specJSON)
			if err != nil {
				return fmt.Errorf("failed to run workflow: %w", err)
			}

			fmt.Printf("Workflow %d created and started in folder %d\n", workflow.ID, workflow.Folder.ID)
			return nil
		},
	}

	// Add worker-specific flags based on TaskConfig
	for _, cfg := range worker.TaskConfig {
		switch cfg.Type {
		case "string", "text", "textarea", "artifacts", "autocomplete", "select":
			var def string
			if cfg.Default != nil {
				def = fmt.Sprintf("%v", cfg.Default)
			}
			cmd.Flags().String(cfg.Name, def, cfg.Description)
		case "boolean", "checkbox":
			var def bool
			if b, ok := cfg.Default.(bool); ok {
				def = b
			}
			cmd.Flags().Bool(cfg.Name, def, cfg.Description)
		case "integer":
			var def int
			if i, ok := cfg.Default.(float64); ok { // JSON unmarshals numbers as float64
				def = int(i)
			}
			cmd.Flags().Int(cfg.Name, def, cfg.Description)
		}
	}

	// Every worker command also gets its own download override flags
	cmd.Flags().Bool("download", false, "Download results for this task")
	cmd.Flags().Bool("no-download", false, "Do not download results for this task")

	return cmd
}

func sliceArgs(args []string, allWorkers []openrelik.Worker) ([][]string, string, error) {
	var segments [][]string
	var current []string
	var delimiter string

	// Find where "run" is in the args
	runIdx := -1
	for i, arg := range args {
		if arg == "run" {
			runIdx = i
			break
		}
	}

	if runIdx == -1 || runIdx == len(args)-1 {
		return nil, "", nil
	}

	// Skip any leading flags that belong to 'run' command
	// We stop at the first argument that is a known worker name
	startIdx := runIdx + 1
	for startIdx < len(args) {
		arg := args[startIdx]
		if !strings.HasPrefix(arg, "-") {
			// Check if it's a known worker
			isWorker := false
			for _, w := range allWorkers {
				wUse := w.TaskName
				wParts := strings.Split(w.TaskName, ".")
				if len(wParts) > 0 {
					wUse = wParts[len(wParts)-1]
				}
				if wUse == arg || w.TaskName == arg {
					isWorker = true
					break
				}
			}
			if isWorker {
				break
			}
		}
		startIdx++
	}

	// Start from the first worker name
	for i := startIdx; i < len(args); i++ {
		arg := args[i]
		if arg == "--then" || arg == "--and" {
			if delimiter != "" && delimiter != arg {
				return nil, "", fmt.Errorf("cannot mix --then and --and in the same command")
			}
			delimiter = arg
			if len(current) > 0 {
				segments = append(segments, current)
			}
			current = []string{}
		} else {
			current = append(current, arg)
		}
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}

	return segments, delimiter, nil
}

func resolveInputs(ctx context.Context, client *openrelik.Client, args []string) ([]int, error) {
	var fileIDs []int
	var filesToUpload []string

	for _, arg := range args {
		if id, err := strconv.Atoi(arg); err == nil {
			fileIDs = append(fileIDs, id)
		} else {
			// Check if it's a local file
			if info, err := os.Stat(arg); err == nil && !info.IsDir() {
				filesToUpload = append(filesToUpload, arg)
			} else if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to stat file %s: %w", arg, err)
			} else {
				return nil, fmt.Errorf("argument %s is neither a File ID nor a local file path", arg)
			}
		}
	}

	if len(filesToUpload) > 0 {
		// Create or find "CLI Uploads" folder
		folder, err := getOrCreateCLIUploadsFolder(ctx, client)
		if err != nil {
			return nil, err
		}

		for _, path := range filesToUpload {
			f, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer f.Close()

			filename := filepath.Base(path)
			fmt.Printf("Uploading %s...\n", filename)

			progress := func(bytesSent, totalBytes int64) {
				// Simple progress for now
			}

			uploadedFile, _, err := client.Files().Upload(ctx, folder.ID, filename, f, openrelik.WithUploadProgress(progress))
			if err != nil {
				return nil, fmt.Errorf("failed to upload file %s: %w", path, err)
			}
			fileIDs = append(fileIDs, uploadedFile.ID)
		}
	}

	return fileIDs, nil
}

func getOrCreateCLIUploadsFolder(ctx context.Context, client *openrelik.Client) (*openrelik.Folder, error) {
	const folderName = "CLI Uploads"

	folders, _, err := client.Folders().ListRootFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list root folders: %w", err)
	}

	for _, f := range folders {
		if f.DisplayName == folderName {
			return &f, nil
		}
	}

	// Create it
	folder, _, err := client.Folders().CreateRootFolder(ctx, folderName)
	if err != nil {
		return nil, fmt.Errorf("failed to create CLI Uploads folder: %w", err)
	}

	return folder, nil
}
