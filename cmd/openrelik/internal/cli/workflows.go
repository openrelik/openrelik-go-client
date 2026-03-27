package cli

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func newWorkflowsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "Manage workflows",
	}

	cmd.AddCommand(newWorkflowsCreateCmd())
	cmd.AddCommand(newWorkflowsInfoCmd())
	cmd.AddCommand(newWorkflowsStatusCmd())
	cmd.AddCommand(newWorkflowsRunCmd())
	return cmd
}

func newWorkflowsCreateCmd() *cobra.Command {
	var folderID int
	var fileIDs []int
	var templateID int
	var params string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(fileIDs) == 0 {
				return fmt.Errorf("at least one file ID is required (use --file)")
			}

			var parsedParams map[string]any
			if params != "" {
				if err := json.Unmarshal([]byte(params), &parsedParams); err != nil {
					return fmt.Errorf("invalid JSON for --params: %w", err)
				}
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			var tID *int
			if cmd.Flags().Changed("template") {
				tID = &templateID
			}

			workflow, _, err := client.Workflows().Create(cmd.Context(), folderID, fileIDs, tID, parsedParams)
			if err != nil {
				return err
			}

			return formatAndPrint(cmd, workflow)
		},
	}

	cmd.Flags().IntVarP(&folderID, "folder", "f", 0, "Folder ID (optional, resolved from first file if omitted)")
	cmd.Flags().IntSliceVarP(&fileIDs, "file", "i", nil, "File IDs to include (can be specified multiple times)")
	cmd.Flags().IntVarP(&templateID, "template", "t", 0, "Template ID to use")
	cmd.Flags().StringVarP(&params, "params", "p", "", "JSON string of parameters")

	return cmd
}

func newWorkflowsInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info [WORKFLOW_ID]",
		Short: "Get workflow metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid workflow ID: %w", err)
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			workflow, _, err := client.Workflows().Get(cmd.Context(), wID)
			if err != nil {
				return err
			}

			return formatAndPrint(cmd, workflow)
		},
	}
}

func newWorkflowsStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [WORKFLOW_ID]",
		Short: "Get workflow status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid workflow ID: %w", err)
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			// First, get the workflow to resolve the folder ID
			workflow, _, err := client.Workflows().Get(cmd.Context(), wID)
			if err != nil {
				return err
			}

			status, _, err := client.Workflows().Status(cmd.Context(), workflow.Folder.ID, wID)
			if err != nil {
				return err
			}

			return formatAndPrint(cmd, status)
		},
	}
}

func newWorkflowsRunCmd() *cobra.Command {
	var spec string

	cmd := &cobra.Command{
		Use:   "run [WORKFLOW_ID]",
		Short: "Run a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid workflow ID: %w", err)
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			// First, get the workflow to resolve the folder ID and fetch existing spec
			workflow, _, err := client.Workflows().Get(cmd.Context(), wID)
			if err != nil {
				return err
			}

			var specPtr *string
			if spec != "" {
				specPtr = &spec
			} else if workflow.SpecJSON != nil {
				specPtr = workflow.SpecJSON
			}

			updatedWorkflow, _, err := client.Workflows().Run(cmd.Context(), workflow.Folder.ID, wID, specPtr)
			if err != nil {
				return err
			}

			return formatAndPrint(cmd, updatedWorkflow)
		},
	}

	cmd.Flags().StringVarP(&spec, "spec", "s", "", "JSON string of workflow specification")
	return cmd
}
