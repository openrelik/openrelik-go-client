package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	openrelik "github.com/openrelik/openrelik-go-client"
	"github.com/spf13/cobra"
)

var (
	parentID    int
	displayName string
	maxDepth    int
)

func newFolderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "folder",
		Short: "Manage folders",
	}

	cmd.AddCommand(newListFoldersCmd())
	cmd.AddCommand(newCreateFolderCmd())
	cmd.AddCommand(newMirrorFolderCmd())
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

func newMirrorFolderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mirror <LOCAL_FOLDER> [FOLDER_ID]",
		Short:        "Mirror a local folder tree into OpenRelik",
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			localPath := args[0]

			client, err := newClient()
			if err != nil {
				return err
			}

			var rootFolderID int
			if len(args) == 2 {
				rootFolderID, err = strconv.Atoi(args[1])
				if err != nil {
					return fmt.Errorf("invalid folder ID: %w", err)
				}
			} else {
				folder, _, err := client.Folders().CreateRootFolder(cmd.Context(), filepath.Base(localPath))
				if err != nil {
					return fmt.Errorf("failed to create root folder: %w", err)
				}
				rootFolderID = folder.ID
			}

			return mirrorDir(cmd.Context(), cmd.OutOrStdout(), client, localPath, rootFolderID, 0)
		},
	}

	cmd.Flags().IntVarP(&maxDepth, "depth", "d", 3, "Maximum subfolder depth to mirror")
	cmd.Flags().IntVar(&chunkSize, "chunk-size", 4*1024*1024, "Chunk size in bytes for uploads")
	return cmd
}

func mirrorDir(ctx context.Context, out io.Writer, client *openrelik.Client, localPath string, remoteFolderID int, depth int) error {
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", localPath, err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(localPath, entry.Name())
		if entry.IsDir() {
			if depth >= maxDepth {
				continue
			}
			folder, _, err := client.Folders().CreateSubFolder(ctx, remoteFolderID, entry.Name())
			if err != nil {
				return fmt.Errorf("failed to create subfolder %s: %w", entry.Name(), err)
			}
			if err := mirrorDir(ctx, out, client, entryPath, folder.ID, depth+1); err != nil {
				return err
			}
		} else {
			if _, err := uploadFileWithProgress(ctx, out, client, entryPath, remoteFolderID); err != nil {
				return err
			}
		}
	}
	return nil
}
