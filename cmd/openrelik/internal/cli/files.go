package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/openrelik/openrelik-go-client"
	"github.com/openrelik/openrelik-go-client/cmd/cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	chunkSize int
)

func newFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Manage files",
	}

	cmd.AddCommand(newListFilesCmd())
	cmd.AddCommand(newFileInfoCmd())
	cmd.AddCommand(newFileDownloadCmd())
	cmd.AddCommand(newFileUploadCmd())
	return cmd
}

func newListFilesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [FOLDER_ID]",
		Short: "List files in a folder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid folder ID: %w", err)
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			files, _, err := client.Folders().ListFiles(cmd.Context(), fID)
			if err != nil {
				return err
			}

			return formatAndPrint(cmd, files)
		},
	}
}

func newFileInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info [ID]",
		Short: "Get file metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			file, _, err := client.Files().Info(cmd.Context(), fileID)
			if err != nil {
				return err
			}

			return formatAndPrint(cmd, file)
		},
	}
}

func newFileDownloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "download [ID] [DESTINATION]",
		Short:        "Download a file",
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			// Get file info first for filename and size
			file, _, err := client.Files().Info(cmd.Context(), fileID)
			if err != nil {
				return err
			}

			// Determine destination path
			destPath := file.DisplayName
			if len(args) > 1 {
				destPath = args[1]
				info, err := os.Stat(destPath)
				if err == nil && info.IsDir() {
					destPath = filepath.Join(destPath, file.DisplayName)
				}
			}

			// Check if folder exists
			parentDir := filepath.Dir(destPath)
			if _, err := os.Stat(parentDir); os.IsNotExist(err) {
				return fmt.Errorf("destination folder %q does not exist", parentDir)
			}

			// Check for overwrite
			if _, err := os.Stat(destPath); err == nil {
				confirmed, err := util.Confirm(cmd.OutOrStdout(), cmd.InOrStdin(), fmt.Sprintf("File %q already exists. Overwrite?", destPath))
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("download cancelled")
				}
			}

			// Download file
			body, _, err := client.Files().Download(cmd.Context(), fileID)
			if err != nil {
				return err
			}
			defer body.Close()

			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()

			var r io.Reader = body
			if !quiet {
				r = util.NewProgressReader(body, file.Filesize, cmd.OutOrStdout())
			}

			_, err = io.Copy(out, r)
			return err
		},
	}
}

func newFileUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "upload [FILE_PATH] [FOLDER_ID]",
		Short:        "Upload a file",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			fID, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid folder ID: %w", err)
			}

			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer file.Close()

			fileInfo, err := file.Stat()
			if err != nil {
				return err
			}

			client, err := newClient()
			if err != nil {
				return err
			}

			var tracker *util.ProgressTracker
			if !quiet {
				tracker = util.NewProgressTracker(cmd.OutOrStdout(), fileInfo.Size(), "Uploading")
				// Calculate total chunks for initial display
				totalChunks := int(fileInfo.Size() / int64(chunkSize))
				if fileInfo.Size()%int64(chunkSize) != 0 {
					totalChunks++
				}
				if totalChunks == 0 {
					totalChunks = 1
				}
				tracker.SetTotalChunks(totalChunks)
			}

			opts := []openrelik.UploadOption{
				openrelik.WithChunkSize(chunkSize),
			}

			// Track chunks and progress
			lastChunkNum := 0
			if tracker != nil {
				opts = append(opts, openrelik.WithUploadProgress(func(bytesSent, totalBytes int64) {
					currentChunk := int(bytesSent / int64(chunkSize))
					if bytesSent%int64(chunkSize) != 0 {
						currentChunk++
					}
					if currentChunk > lastChunkNum {
						for i := 0; i < currentChunk-lastChunkNum; i++ {
							tracker.IncrementChunk()
						}
						lastChunkNum = currentChunk
					}
					tracker.Update(bytesSent)
				}))
				opts = append(opts, openrelik.WithUploadRetry(func(chunkNum, attempt int, err error) {
					tracker.IncrementRetry()
				}))
			}

			result, _, err := client.Files().Upload(cmd.Context(), fID, filepath.Base(filePath), file, opts...)
			if err != nil {
				return err
			}

			if tracker != nil {
				tracker.Finish()
			}

			return formatAndPrint(cmd, result)
		},
	}

	cmd.Flags().IntVar(&chunkSize, "chunk-size", 4*1024*1024, "Chunk size in bytes")
	return cmd
}
