package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	parentID    int
	displayName string
)

func newFolderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "folder",
		Short: "Manage folders",
	}

	cmd.AddCommand(newListFoldersCmd())
	cmd.AddCommand(newCreateFolderCmd())
	return cmd
}

func newListFoldersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [PARENT_ID]",
		Short: "List folders",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var pID int
			var err error
			if len(args) > 0 {
				pID, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid parent ID: %w", err)
				}
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			var folders interface{}
			if pID != 0 {
				folders, _, err = client.Folders().ListSubFolders(cmd.Context(), pID)
			} else {
				folders, _, err = client.Folders().ListRootFolders(cmd.Context())
			}

			if err != nil {
				return err
			}

			return formatAndPrint(cmd, folders)
		},
	}
}

func newCreateFolderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient()
			if err != nil {
				return err
			}

			var folder interface{}
			if parentID != 0 {
				folder, _, err = client.Folders().CreateSubFolder(cmd.Context(), parentID, displayName)
			} else {
				folder, _, err = client.Folders().CreateRootFolder(cmd.Context(), displayName)
			}

			if err != nil {
				return err
			}

			return formatAndPrint(cmd, folder)
		},
	}

	cmd.Flags().StringVarP(&displayName, "name", "n", "", "Folder name")
	cmd.Flags().IntVarP(&parentID, "parent", "p", 0, "Parent folder ID")
	cmd.MarkFlagRequired("name")
	return cmd
}
