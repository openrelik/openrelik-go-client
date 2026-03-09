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
