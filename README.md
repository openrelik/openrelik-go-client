# OpenRelik Go API Client

An idiomatic, zero-dependency Go client library for the OpenRelik API. This library provides seamless authentication, automatic token refresh, and high-level service abstractions for building Go applications on top of the OpenRelik platform.

## Installation

```bash
go get github.com/openrelik/openrelik-go-client
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"github.com/openrelik/openrelik-go-client"
)

func main() {
	// Initialize the client
	// apiServerURL: The root URL of the OpenRelik server.
	// apiKey: The API token used for authentication.
	client, err := openrelik.NewClient("http://localhost:8710", "your-api-token")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Retrieve the currently authenticated user profile
	user, _, err := client.Users().GetMe(context.Background())
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Authenticated as: %s (%s)\n", user.DisplayName, user.Username)
}
```

## Key Features

- **Zero External Dependencies:** The core library only uses the Go standard library.
- **Service-Oriented Design:** API methods are grouped into logical services (`client.Users()`) for better discoverability.
- **Automated Authentication:** Handles access token injection and transparent token refresh (via `http.RoundTripper`) automatically.
- **Context Support:** All methods support `context.Context` for timeout and cancellation handling.
- **Concurrency Safe:** Transparently handles concurrent token refreshes.

## Advanced Usage

### Error Handling

The library returns a structured `openrelik.Error` for API-level failures (4xx/5xx status codes). You can use `errors.As` to inspect the status code or the raw response body. Network or timeout errors will be returned as-is.

```go
user, _, err := client.Users().GetMe(ctx)
if err != nil {
    var apiErr *openrelik.Error
    if errors.As(err, &apiErr) {
        // API error (e.g., 404 Not Found)
        fmt.Printf("API error %d: %s\n", apiErr.StatusCode, apiErr.Message)
    } else {
        // Network or cancellation error
        fmt.Printf("Network error: %v\n", err)
    }
}
```

### Custom HTTP Configuration

If you need to provide a custom `http.Client` or a custom `http.RoundTripper` (e.g., for proxies, custom TLS, or tracing), you can use functional options during initialization.

#### Custom Transport (e.g., Proxy)
```go
customTransport := &http.Transport{
    Proxy: http.ProxyURL(proxyURL),
}

client, err := openrelik.NewClient(
    "http://localhost:8710",
    "your-api-key",
    openrelik.WithBaseTransport(customTransport),
)
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
```

#### Custom HTTP Client
```go
customClient := &http.Client{
    Timeout: 30 * time.Second,
}

client, err := openrelik.NewClient(
    "http://localhost:8710",
    "your-api-key",
    openrelik.WithHTTPClient(customClient),
)
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
```

### File Uploads

The client supports uploading large files using a chunked, resumable mechanism. This approach is highly resilient to network failures, as it only retries the failing chunk rather than the entire file.

#### Basic Upload
```go
file, err := os.Open("large-file.dd")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// folderID: The ID of the folder to upload to.
// filename: The name the file will have on the server.
uploadedFile, _, err := client.Files().UploadFile(ctx, folderID, "large-file.dd", file)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("File uploaded successfully! ID: %d\n", uploadedFile.ID)
```

#### Upload with Progress Tracking
You can provide a progress callback to track the upload status. This is ideal for CLI applications.

```go
progress := func(bytesSent, totalBytes int64) {
    if totalBytes > 0 {
        percent := float64(bytesSent) / float64(totalBytes) * 100
        fmt.Printf("\rUploading... %.2f%%", percent)
    } else {
        fmt.Printf("\rUploading... %d bytes sent", bytesSent)
    }
}

uploadedFile, _, err := client.Files().UploadFile(
    ctx,
    folderID,
    "large-file.dd",
    file,
    openrelik.WithProgress(progress),
)
```

#### Advanced Upload Options
- `WithChunkSize(size int)`: Customize the size of each chunk (default is 4MB).
- `WithTotalSize(size int64)`: Explicitly set the total size (required for non-seeking readers like `stdin`).
- `WithRetry(fn RetryCallback)`: Observe retry attempts during network flakes.

### Low-Level API

For endpoints not yet covered by a typed service, you can use the low-level HTTP methods (`Get`, `Post`, `Put`, `Patch`, `Delete`). These methods handle authentication and automatic token refresh transparently.

Paths should be relative to the versioned API root (e.g. `/api/v1/`).

#### Decoding into a Struct
```go
user := &openrelik.User{}
_, err := client.Get(ctx, "/users/me/", user)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("User: %+v\n", user)
```

#### Raw Response Handling
```go
resp, err := client.Get(ctx, "/users/me/", nil)
if err != nil {
    log.Fatal(err)
}
defer resp.Body.Close()

body, err := io.ReadAll(resp.Body)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Raw JSON: %s\n", string(body))
```
