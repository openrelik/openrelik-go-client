package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	openrelik "github.com/openrelik/openrelik-go-client"
)

func main() {
	// Configuration from environment variables
	apiServerURL := os.Getenv("OPENRELIK_API_URL")
	if apiServerURL == "" {
		apiServerURL = "http://localhost:8710"
	}

	apiKey := os.Getenv("OPENRELIK_API_KEY")
	if apiKey == "" {
		apiKey = "<your-api-token>"
	}

	ctx := context.Background()

	// Create a new client instance
	client, err := openrelik.NewClient(apiServerURL, apiKey)
	if err != nil {
		log.Fatalf("Failed to create OpenRelik client: %v", err)
	}

	exampleHighLevel(ctx, client)
	exampleLowLevel(ctx, client)
	exampleRawJSON(ctx, client)
}

// exampleHighLevel demonstrates the recommended high-level service pattern.
func exampleHighLevel(ctx context.Context, client *openrelik.Client) {
	fmt.Println("--- High-Level Service Pattern ---")

	// User service example
	user, _, err := client.Users().GetMe(ctx)
	if err != nil {
		log.Printf("User Service Error: %v\n", err)
	} else {
		email := "N/A"
		if user.Email != nil {
			email = *user.Email
		}
		fmt.Printf("User:    %s (%s)\n\n", user.Username, email)
	}

	// Folder service example
	folders, _, err := client.Folders().ListRootFolders(ctx)
	if err != nil {
		log.Printf("Folder Service Error: %v\n", err)
	} else {
		fmt.Printf("Folders: Found %d root folders\n", len(folders))
		for _, f := range folders {
			fmt.Printf("  - [%d] %s (Created by: %s)\n", f.ID, f.DisplayName, f.User.Username)
		}
	}
}

// exampleLowLevel demonstrates decoding into a struct using low-level methods.
func exampleLowLevel(ctx context.Context, client *openrelik.Client) {
	fmt.Println("\n--- Low-Level Abstraction ---")

	user := &openrelik.User{}
	_, err := client.Get(ctx, "/users/me/", user)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("User Struct: %+v\n", user)
}

// exampleRawJSON demonstrates handling raw JSON response bodies.
func exampleRawJSON(ctx context.Context, client *openrelik.Client) {
	fmt.Println("\n--- Raw JSON (Low-Level) ---")

	resp, err := client.Get(ctx, "/users/me/", nil)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading body: %v\n", err)
		return
	}

	fmt.Printf("Raw JSON: %s\n", string(body))
}
