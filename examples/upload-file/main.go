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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	openrelik "github.com/openrelik/openrelik-go-client"
)

func main() {
	// Simple command line flags
	apiURL := flag.String("url", os.Getenv("OPENRELIK_API_URL"), "OpenRelik API server URL")
	apiKey := flag.String("key", os.Getenv("OPENRELIK_API_KEY"), "OpenRelik API key")
	folderID := flag.Int("folder", 0, "Folder ID to upload to")
	filePath := flag.String("file", "", "Path to the file to upload")
	flag.Parse()

	if *apiURL == "" || *apiKey == "" || *filePath == "" {
		fmt.Println("Usage: go run examples/upload-file/main.go -url <URL> -key <KEY> -file <PATH> [-folder <ID>]")
		os.Exit(1)
	}

	// Open the file
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// Get file info for the total size
	fileInfo, err := file.Stat()
	if err != nil {
		log.Fatalf("Failed to get file info: %v", err)
	}
	totalSize := fileInfo.Size()

	// Initialize the OpenRelik client
	client, err := openrelik.NewClient(*apiURL, *apiKey)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	start := time.Now()

	fmt.Printf("Uploading %s (%d bytes) to folder %d...\n", fileInfo.Name(), totalSize, *folderID)

	// Define the progress callback
	progress := func(bytesSent, totalBytes int64) {
		percent := float64(bytesSent) / float64(totalBytes) * 100
		elapsed := time.Since(start).Seconds()
		bps := float64(bytesSent) / elapsed
		mbps := bps / (1024 * 1024)

		// Print a simple progress bar
		fmt.Printf("\rProgress: [%-50s] %.2f%% (%.2f MB/s)",
			getProgressBar(percent), percent, mbps)
	}

	// Perform the chunked upload
	uploadedFile, _, err := client.Files().Upload(
		ctx,
		*folderID,
		fileInfo.Name(),
		file,
		openrelik.WithUploadProgress(progress),
		openrelik.WithTotalSize(totalSize),
		openrelik.WithChunkSize(4*1024*1024), // 4MB chunks
	)

	if err != nil {
		fmt.Printf("\nUpload failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n\nUpload successful! Fetching metadata...\n")

	// Fetch full metadata for the uploaded file
	meta, _, err := client.Files().Info(ctx, uploadedFile.ID)
	if err != nil {
		fmt.Printf("Failed to fetch metadata: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n--- File Metadata ---")
	fmt.Printf("ID:          %d\n", meta.ID)
	fmt.Printf("UUID:        %s\n", meta.UUID)
	fmt.Printf("Display Name:%s\n", meta.DisplayName)
	fmt.Printf("Size:        %d bytes\n", meta.Filesize)
	fmt.Printf("MIME Type:   %s\n", meta.MagicMime)
	fmt.Printf("Created At:  %v\n\n", meta.CreatedAt)
}

func getProgressBar(percent float64) string {
	bars := int(percent / 2) // 50 total characters
	res := ""
	for i := 0; i < bars; i++ {
		res += "="
	}
	return res
}
