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
	"sync"
	"time"

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
		TraverseChildren: true,
	}

	// Global run flags
	runCmd.PersistentFlags().StringP("directory", "d", ".", "Output directory for downloads")
	runCmd.PersistentFlags().String("download", "none", "Download policy (final, all, none)")
	runCmd.PersistentFlags().Lookup("download").NoOptDefVal = "final"
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
			// Slice arguments by --then or --and
			segments, delimiter, err := sliceArgs(cmd.Name(), args)
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

			// Get global flags for polling/downloading
			downloadPolicy, _ := cmd.Flags().GetString("download")
			outputDir, _ := cmd.Flags().GetString("directory")
			taskFolders, _ := cmd.Flags().GetBool("task-folders")

			outputSpecified := false
			if f := cmd.Flags().Lookup("output"); f != nil {
				outputSpecified = f.Changed
			}

			showProgress := !quiet && !outputSpecified

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
			taskDownloadPrefs := make(map[string]bool) // UUID -> should download
			taskShortNames := []string{}               // command aliases, e.g. ["strings", "grep"]
			taskUUIDs := []string{}                    // local UUIDs generated for the spec
			uuidToShort := make(map[string]string)     // UUID -> "strings"

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

				taskShortNames = append(taskShortNames, segment[0])

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

				// Allow unknown flags in temp command so global flags don't break it
				tempCmd.FParseErrWhitelist.UnknownFlags = true

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

				uuid := util.GenerateUUID()
				taskUUIDs = append(taskUUIDs, uuid)
				uuidToShort[uuid] = segment[0]

				// Handle per-task download preferences
				download, _ := tempCmd.Flags().GetBool("download-task")
				noDownload, _ := tempCmd.Flags().GetBool("no-download-task")
				if download {
					taskDownloadPrefs[uuid] = true
				} else if noDownload {
					taskDownloadPrefs[uuid] = false
				}

				newTask := WorkflowTask{
					UUID:        uuid,
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
						currentParent = &spec.Workflow.Tasks[len(spec.Workflow.Tasks)-1]
					} else {
						currentParent.Tasks = append(currentParent.Tasks, newTask)
						currentParent = &currentParent.Tasks[len(currentParent.Tasks)-1]
					}
				}
			}

			if dryRun {
				if showProgress {
					fmt.Println("Dry run: generating workflow spec...")
					fmt.Printf("Inputs: %v\n", positionalArgs)
				}
				if !quiet {
					encoder := json.NewEncoder(os.Stdout)
					encoder.SetIndent("", "  ")
					if err := encoder.Encode(spec); err != nil {
						return fmt.Errorf("failed to encode workflow spec: %w", err)
					}
				}
				return nil
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			fileIDs, totalUploaded, err := resolveInputs(cmd.Context(), client, positionalArgs, showProgress)
			if err != nil {
				return err
			}

			if len(fileIDs) == 0 {
				return fmt.Errorf("no inputs provided (IDs or file paths)")
			}

			startTime := time.Now()

			// Step 1: Create Workflow
			workflow, _, err := client.Workflows().Create(cmd.Context(), 0, fileIDs, nil, nil)
			if err != nil {
				return fmt.Errorf("failed to create workflow: %w", err)
			}

			// Step 2: Run Workflow
			specBytes, err := json.Marshal(spec)
			if err != nil {
				return fmt.Errorf("failed to marshal workflow spec: %w", err)
			}
			specJSON := string(specBytes)
			workflow, _, err = client.Workflows().Run(cmd.Context(), workflow.Folder.ID, workflow.ID, &specJSON)
			if err != nil {
				return fmt.Errorf("failed to run workflow: %w", err)
			}

			interactive := isInteractiveTTY() && showProgress

			taskStarted := make(map[string]time.Time) // UUID → first non-pending
			taskEnded := make(map[string]time.Time)   // UUID → first terminal

			var lastStatus *openrelik.WorkflowStatus

			if interactive {
				separator := " " + util.ColorDim + "→" + util.ColorReset + " "
				if delimiter == "--and" {
					separator = " " + util.ColorDim + "&" + util.ColorReset + " "
				}
				fmt.Printf("%s⚙ Workflow:%s %s\n", util.ColorBold, util.ColorReset, strings.Join(taskShortNames, separator))
				for _, name := range taskShortNames {
					fmt.Printf("  %s·%s  %-14s %spending%s\033[K\n", util.ColorDim, util.ColorReset, name, util.ColorDim, util.ColorReset)
				}

				var mu sync.Mutex
				taskStatusDisplay := make(map[string]string) // UUID → status
				stopSpinner := make(chan struct{})
				spinnerDone := make(chan struct{})

				// Spinner goroutine: redraws task lines at 100ms independent of API polling.
				go func() {
					defer close(spinnerDone)
					frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}
					idx := 0
					ticker := time.NewTicker(100 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ticker.C:
							mu.Lock()
							spinner := frames[idx%len(frames)]
							idx++
							fmt.Printf("\033[%dA", len(taskShortNames))
							for i, shortName := range taskShortNames {
								uuid := taskUUIDs[i]
								ts := taskStatusDisplay[uuid]
								if ts == "" {
									ts = "PENDING"
								}
								if strings.EqualFold(ts, "FAILURE") {
									fmt.Printf("  %s✖%s  %-14s %sfailed%s\033[K\n", util.ColorRed, util.ColorReset, shortName, util.ColorRed, util.ColorReset)
								} else if isTerminalTaskStatus(ts) {
									elapsed := taskEnded[uuid].Sub(taskStarted[uuid])
									fmt.Printf("  %s✔%s  %-14s %s%s%.1fs%s\033[K\n", util.ColorGreen, util.ColorReset, shortName, util.ColorDim, "[", elapsed.Seconds(), "]"+util.ColorReset)
								} else if strings.EqualFold(ts, "PENDING") {
									fmt.Printf("  %s·%s  %-14s %spending%s\033[K\n", util.ColorDim, util.ColorReset, shortName, util.ColorDim, util.ColorReset)
								} else {
									fmt.Printf("  %s%s%s  %-14s %s%s%s\033[K\n", util.ColorCyan, spinner, util.ColorReset, shortName, util.ColorCyan, strings.ToLower(ts), util.ColorReset)
								}
							}
							mu.Unlock()
						case <-stopSpinner:
							return
						}
					}
				}()

				// API polling loop: updates shared state every 2s.
				var workflowFailed bool
				for {
					status, _, err := client.Workflows().Status(cmd.Context(), workflow.Folder.ID, workflow.ID)
					if err != nil {
						close(stopSpinner)
						<-spinnerDone
						return fmt.Errorf("failed to get workflow status: %w", err)
					}
					lastStatus = status

					mu.Lock()
					now := time.Now()
					for _, task := range status.Tasks {
						ts := "PENDING"
						if task.StatusShort != nil {
							ts = *task.StatusShort
						}
						taskStatusDisplay[task.UUID] = ts
						if !strings.EqualFold(ts, "PENDING") && taskStarted[task.UUID].IsZero() {
							taskStarted[task.UUID] = now
						}
						if isTerminalTaskStatus(ts) && taskEnded[task.UUID].IsZero() {
							taskEnded[task.UUID] = now
						}
					}
					done := isTerminalTaskStatus(status.Status)
					workflowFailed = strings.EqualFold(status.Status, "FAILURE")
					mu.Unlock()

					if done {
						break
					}
					time.Sleep(2 * time.Second)
				}

				close(stopSpinner)
				<-spinnerDone

				// Final render: replace spinner lines with checkmarks/crosses.
				fmt.Printf("\033[%dA", len(taskShortNames))
				for i, shortName := range taskShortNames {
					uuid := taskUUIDs[i]
					ts := taskStatusDisplay[uuid]
					if strings.EqualFold(ts, "FAILURE") {
						fmt.Printf("  %s✖%s  %-14s %sfailed%s\033[K\n", util.ColorRed, util.ColorReset, shortName, util.ColorRed, util.ColorReset)
					} else {
						elapsed := taskEnded[uuid].Sub(taskStarted[uuid])
						fmt.Printf("  %s✔%s  %-14s %s%s%.1fs%s\033[K\n", util.ColorGreen, util.ColorReset, shortName, util.ColorDim, "[", elapsed.Seconds(), "]"+util.ColorReset)
					}
				}

				if workflowFailed {
					return fmt.Errorf("workflow failed")
				}

			} else {
				// Non-interactive (Option A): print a line only when a task reaches terminal state.
				if showProgress {
					fmt.Println("Running workflow...")
				}
				prevTaskStatuses := make(map[string]string) // UUID → status

				for {
					status, _, err := client.Workflows().Status(cmd.Context(), workflow.Folder.ID, workflow.ID)
					if err != nil {
						return fmt.Errorf("failed to get workflow status: %w", err)
					}
					lastStatus = status

					now := time.Now()
					for _, task := range status.Tasks {
						ts := "PENDING"
						if task.StatusShort != nil {
							ts = *task.StatusShort
						}
						if !strings.EqualFold(ts, "PENDING") && taskStarted[task.UUID].IsZero() {
							taskStarted[task.UUID] = now
						}
						if isTerminalTaskStatus(ts) && taskEnded[task.UUID].IsZero() {
							taskEnded[task.UUID] = now
						}
						if prevTaskStatuses[task.UUID] == ts || !isTerminalTaskStatus(ts) {
							continue
						}
						shortName := uuidToShort[task.UUID]
						if shortName == "" {
							shortName = strings.ToLower(task.DisplayName)
						}
						elapsed := ""
						if !taskStarted[task.UUID].IsZero() {
							d := taskEnded[task.UUID].Sub(taskStarted[task.UUID])
							elapsed = fmt.Sprintf(" %.1fs", d.Seconds())
						}
						if showProgress {
							if strings.EqualFold(ts, "FAILURE") {
								fmt.Printf("✗ %-14s failed%s\n", shortName, elapsed)
							} else {
								fmt.Printf("✓ %-14s done%s\n", shortName, elapsed)
							}
						}
						prevTaskStatuses[task.UUID] = ts
					}

					done := isTerminalTaskStatus(status.Status)
					if done {
						if strings.EqualFold(status.Status, "FAILURE") {
							return fmt.Errorf("workflow failed")
						}
						break
					}
					time.Sleep(2 * time.Second)
				}
			}

			// Phase 4: Download results
			var fullWorkflow *openrelik.Workflow
			var totalDownloaded int64
			if lastStatus != nil && (downloadPolicy != "none" || outputSpecified || outputFormat != "text") {
				// The status endpoint doesn't include output_files; fetch the full workflow.
				var err error
				fullWorkflow, _, err = client.Workflows().Get(cmd.Context(), workflow.ID)
				if err != nil {
					return fmt.Errorf("failed to fetch workflow details: %w", err)
				}
			}

			if lastStatus != nil && downloadPolicy != "none" && fullWorkflow != nil {
				tasksToDownload := []openrelik.Task{}

				for i, task := range fullWorkflow.Tasks {
					shouldDownload := false

					// Check per-task preference first
					if pref, ok := taskDownloadPrefs[task.UUID]; ok {
						shouldDownload = pref
					} else {
						// Fallback to global policy
						if downloadPolicy == "all" {
							shouldDownload = true
						} else if downloadPolicy == "final" {
							shouldDownload = i == len(fullWorkflow.Tasks)-1
						}
					}

					if shouldDownload {
						tasksToDownload = append(tasksToDownload, task)
					}
				}

				for _, task := range tasksToDownload {
					for _, outputFile := range task.OutputFiles {
						destDir := outputDir
						if taskFolders {
							folderName := fmt.Sprintf("%s_%d", strings.ReplaceAll(task.DisplayName, " ", "_"), task.ID)
							destDir = filepath.Join(outputDir, folderName)
							if err := os.MkdirAll(destDir, 0755); err != nil {
								return fmt.Errorf("failed to create task folder: %w", err)
							}
						}

						destPath := filepath.Join(destDir, outputFile.DisplayName)
						f, err := os.Create(destPath)
						if err != nil {
							return fmt.Errorf("failed to create file %s: %w", destPath, err)
						}

						body, _, err := client.Files().Download(cmd.Context(), outputFile.ID)
						if err != nil {
							f.Close()
							return fmt.Errorf("failed to download file %s: %w", outputFile.DisplayName, err)
						}

						action := "Download: " + outputFile.DisplayName
						var trackerWriter io.Writer = os.Stderr
						if !showProgress {
							trackerWriter = nil
						}
						progressReader := &util.ProgressReader{
							Reader:  body,
							Tracker: util.NewProgressTracker(trackerWriter, outputFile.Filesize, action),
						}
						n, err := io.Copy(f, progressReader)
						body.Close()
						f.Close()

						if err != nil {
							return fmt.Errorf("failed to save file %s: %w", outputFile.DisplayName, err)
						}
						totalDownloaded += n
					}
				}
			}

			if (outputSpecified || outputFormat != "text") && fullWorkflow != nil {
				return formatAndPrint(cmd, fullWorkflow)
			}

			if showProgress {
				duration := time.Since(startTime).Round(time.Millisecond)
				sep := fmt.Sprintf(" %s·%s ", util.ColorDim, util.ColorReset)
				fmt.Printf("\nTasks: %d%sDuration: %s%sUploaded: %s%sDownloaded: %s\n",
					len(taskShortNames), sep,
					duration, sep,
					util.FormatBytes(totalUploaded), sep,
					util.FormatBytes(totalDownloaded))
			}

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
	cmd.Flags().Bool("download-task", false, "Download results for this task")
	cmd.Flags().Bool("no-download-task", false, "Do not download results for this task")

	return cmd
}

func sliceArgs(workerName string, args []string) ([][]string, string, error) {
	var segments [][]string
	var current []string
	var delimiter string

	current = append(current, workerName)
	for _, arg := range args {
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

func resolveInputs(ctx context.Context, client *openrelik.Client, args []string, showProgress bool) ([]int, int64, error) {
	var fileIDs []int
	var filesToUpload []string
	var totalUploaded int64

	for _, arg := range args {
		if id, err := strconv.Atoi(arg); err == nil {
			fileIDs = append(fileIDs, id)
		} else {
			// Check if it's a local file
			if info, err := os.Stat(arg); err == nil && !info.IsDir() {
				filesToUpload = append(filesToUpload, arg)
			} else if err != nil && !os.IsNotExist(err) {
				return nil, 0, fmt.Errorf("failed to stat file %s: %w", arg, err)
			} else {
				return nil, 0, fmt.Errorf("argument %s is neither a File ID nor a local file path", arg)
			}
		}
	}

	if len(filesToUpload) > 0 {
		// Create or find "CLI Uploads" folder
		folder, err := getOrCreateCLIUploadsFolder(ctx, client)
		if err != nil {
			return nil, 0, err
		}

		for _, path := range filesToUpload {
			err := func() error {
				f, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("failed to open file %s: %w", path, err)
				}
				defer f.Close()

				stat, err := f.Stat()
				if err != nil {
					return fmt.Errorf("failed to stat file %s: %w", path, err)
				}
				filename := filepath.Base(path)

				var trackerWriter io.Writer = os.Stderr
				if !showProgress {
					trackerWriter = nil
				}

				action := "Upload: " + filename
				tracker := util.NewProgressTracker(trackerWriter, stat.Size(), action)
				progress := func(bytesSent, totalBytes int64) {
					tracker.Update(bytesSent)
				}

				uploadedFile, _, err := client.Files().Upload(ctx, folder.ID, filename, f, openrelik.WithUploadProgress(progress))
				if err != nil {
					tracker.Finish()
					return fmt.Errorf("failed to upload file %s: %w", path, err)
				}
				tracker.Finish()
				fileIDs = append(fileIDs, uploadedFile.ID)
				totalUploaded += stat.Size()
				return nil
			}()
			if err != nil {
				return nil, 0, err
			}
		}
	}

	return fileIDs, totalUploaded, nil
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

// isInteractiveTTY returns true when stderr is connected to a real terminal.
// Progress output goes to stderr, so that is the relevant fd to check.
func isInteractiveTTY() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// isTerminalTaskStatus returns true for task statuses that represent a final state.
func isTerminalTaskStatus(status string) bool {
	s := strings.ToLower(status)
	return s == "success" || s == "failure" || s == "complete" || s == "completed"
}
