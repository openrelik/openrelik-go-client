package main

import (
	"context"
	"fmt"
	"io"
	"log"

	openrelik "github.com/openrelik/openrelik-go-client"
)

func main() {
	apiServerURL := "http://localhost:8710"
	apiKey := "<your-api-token>"
	ctx := context.Background()

	// Create a new client instance
	client, err := openrelik.NewClient(apiServerURL, apiKey)
	if err != nil {
		log.Fatalf("Failed to create OpenRelik client: %v", err)
	}

	// Using High-Level Service Pattern
	// This abstracts away the HTTP details and directly returns typed structs.
	fmt.Println("\n--- High-Level Service Pattern ---")
	userSvc, respSvc, errSvc := client.Users.GetMe(ctx)
	if errSvc != nil {
		fmt.Printf("Service Error (expected if server down): %v\n", errSvc)
	} else {
		email := "N/A"
		if userSvc.Email != nil {
			email = *userSvc.Email
		}
		fmt.Printf("User from Service: %s (%s)\n", userSvc.Username, email)
		fmt.Printf("Response Status: %s\n", respSvc.Status)
	}

	// Low-Level Abstraction (decoding into a struct)
	// This allows you to use the same method for any endpoint, but you need to provide the struct.
	fmt.Println("\n--- Low-Level Abstraction ---")
	user := &openrelik.User{}
	_, err = client.Get(ctx, "/users/me/", user)
	if err != nil {
		fmt.Printf("Error (expected if server down): %v\n", err)
	} else {
		fmt.Printf("User Struct: %+v\n", user)
	}

	// Getting Raw JSON (passing nil to low-level Get)
	// This is useful for debugging or when you want to handle the JSON yourself.
	fmt.Println("\n--- Raw JSON (Low-Level) ---")
	respRaw, errRaw := client.Get(ctx, "/users/me/", nil)
	if errRaw != nil {
		fmt.Printf("Error: %v\n", errRaw)
	} else {
		defer respRaw.Body.Close()
		body, _ := io.ReadAll(respRaw.Body)
		fmt.Printf("Raw JSON Body: %s\n\n", string(body))
	}
}
