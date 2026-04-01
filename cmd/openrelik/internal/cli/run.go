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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openrelik/openrelik-go-client"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/config"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/util"
	"github.com/spf13/cobra"
)

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

	// dry-run applies to all worker subcommands
	runCmd.PersistentFlags().Bool("dry-run", false, "Generate and display workflow spec without executing")

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
			segments, delimiter, err := util.SliceArgs(cmd.Name(), args, "--then", "--and")
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
					helpWorker = util.FindWorker(segment[0], allWorkers)
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

			// All run flags are persistent, so they are available via cmd.Flags()
			downloadPolicy, _ := cmd.Flags().GetString("download")
			outputDir, _ := cmd.Flags().GetString("directory")
			taskFolders, _ := cmd.Flags().GetBool("task-folders")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			outputSpecified := outputFormat != "text"
			showProgress := !quiet && !outputSpecified

			// Root workflow type is always 'chain'
			spec, positionalArgs, taskDownloadPrefs, meta, err := util.BuildWorkflowSpec(cmd, segments, delimiter, allWorkers, createWorkerCmd)
			if err != nil {
				return err
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

			fileIDs, totalUploaded, err := util.ResolveInputs(cmd.Context(), client, positionalArgs, showProgress)
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
				return fmt.Errorf("failed to create workflow with %d inputs: %w", len(fileIDs), err)
			}

			// Step 2: Run Workflow
			specBytes, err := json.Marshal(spec)
			if err != nil {
				return fmt.Errorf("failed to marshal workflow spec: %w", err)
			}
			specJSON := string(specBytes)
			workflow, _, err = client.Workflows().Run(cmd.Context(), workflow.Folder.ID, workflow.ID, &specJSON)
			if err != nil {
				return fmt.Errorf("failed to start workflow %d: %w", workflow.ID, err)
			}

			interactive := util.IsInteractiveTTY() && showProgress

			monitor := util.NewWorkflowMonitor(client, workflow, meta.TaskShortNames, meta.TaskUUIDs, meta.UUIDToShort, interactive, showProgress)
			var lastStatus *openrelik.WorkflowStatus
			lastStatus, err = monitor.Monitor(cmd.Context())
			if err != nil && lastStatus == nil {
				return err
			}

			// Phase 4: Download results
			var fullWorkflow *openrelik.Workflow
			var totalDownloaded int64
			if lastStatus != nil {
				totalDownloaded, fullWorkflow, err = util.DownloadResults(cmd.Context(), client, workflow.ID, downloadPolicy, taskDownloadPrefs, outputDir, taskFolders, outputFormat, showProgress)
				if err != nil {
					return err
				}
			}

			if outputFormat != "text" && fullWorkflow != nil {
				return formatAndPrint(cmd, fullWorkflow)
			}

			monitor.PrintSummary(startTime, totalUploaded, totalDownloaded)

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
