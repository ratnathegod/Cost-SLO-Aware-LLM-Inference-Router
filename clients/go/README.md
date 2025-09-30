# Go SDK for LLM Router API

This package provides a Go client for the LLM Router API with type-safe access to all endpoints.

## Installation

```bash
go get github.com/ratnathegod/llm-router-clients/go
```

## Usage

### Basic Client (Tenant Operations)

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    llmrouter "github.com/ratnathegod/llm-router-clients/go"
)

func main() {
    // Create a client with your tenant API key
    client := llmrouter.NewClient("https://api.llm-router.example.com", "your-tenant-api-key")
    
    ctx := context.Background()
    
    // Make an inference request
    resp, err := client.Infer(ctx, llmrouter.InferRequest{
        Prompt: "What is the capital of France?",
        Model:  StringPtr("gpt-4o"),
        Policy: StringPtr("cheapest"),
    })
    
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Response: %s\n", resp.Text)
    fmt.Printf("Provider: %s\n", resp.Provider)
    fmt.Printf("Cost: $%.4f\n", resp.CostUsd)
    fmt.Printf("Latency: %dms\n", resp.LatencyMs)
    
    // Get daily usage
    usage, err := client.GetDailyUsage(ctx, IntPtr(7)) // Last 7 days
    if err != nil {
        log.Fatal(err)
    }
    
    for _, day := range usage {
        fmt.Printf("Date: %s, Requests: %d, Cost: $%.2f\n", 
            day.Date, day.Requests, day.CostUsd)
    }
    
    // Get recent usage
    recent, err := client.GetRecentUsage(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Recent requests: %d\n", len(recent))
}

// Helper functions for optional fields
func StringPtr(s string) *string { return &s }
func IntPtr(i int) *int { return &i }
func BoolPtr(b bool) *bool { return &b }
```

### Admin Client

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    llmrouter "github.com/ratnathegod/llm-router-clients/go"
)

func main() {
    // Create admin client with admin token
    admin := llmrouter.NewAdminClient("https://api.llm-router.example.com", "your-admin-token")
    
    ctx := context.Background()
    
    // Get system status
    status, err := admin.GetAdminStatus(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Uptime: %s\n", status.Uptime)
    fmt.Printf("Total Requests: %d\n", status.TotalRequests)
    fmt.Printf("Default Policy: %s\n", status.DefaultPolicy)
    
    for _, provider := range status.Providers {
        fmt.Printf("Provider %s: Error Rate 1m=%.2f%%, Cost=$%.4f/1k\n", 
            provider.Name, provider.ErrorRate1m*100, provider.CostPer1kTokensUsd)
    }
    
    // Create a new tenant
    tenant, err := admin.CreateTenant(ctx, llmrouter.CreateTenantRequest{
        Name:            "Acme Corp",
        Plan:            "enterprise",
        RpsLimit:        100,
        DailyTokenLimit: 1000000,
        Enabled:         BoolPtr(true),
    })
    
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Created tenant: %s\n", tenant.TenantId)
    fmt.Printf("API Key: %s\n", tenant.ApiKey)
    
    // Get canary status
    canary, err := admin.GetCanaryStatus(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Canary Stage: %d (%.1f%%)\n", canary.Stage, canary.Percent)
    
    // Update routing policy
    err = admin.UpdatePolicy(ctx, "fastest_p95")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Updated default policy to fastest_p95")
}

func BoolPtr(b bool) *bool { return &b }
```

### Idempotent Requests

```go
// Use idempotency for critical requests
idempotencyKey := "user-12345-request-67890"

resp, err := client.Infer(ctx, llmrouter.InferRequest{
    Prompt: "Important calculation request",
    Model:  StringPtr("gpt-4o"),
}, llmrouter.InferOptions{
    IdempotencyKey: &idempotencyKey,
})
```

### Custom HTTP Client

```go
// Use custom HTTP client with timeout/retry logic
httpClient := &http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        MaxRetries: 3,
    },
}

client := llmrouter.NewClient("https://api.llm-router.example.com", "api-key").
    WithHTTPClient(httpClient)
```

## Error Handling

The client returns structured errors that implement the `Problem` type from RFC 7807:

```go
resp, err := client.Infer(ctx, req)
if err != nil {
    if problem, ok := err.(llmrouter.Problem); ok {
        fmt.Printf("API Error: %s (%d)\n", problem.Title, problem.Status)
        fmt.Printf("Detail: %s\n", problem.Detail)
        fmt.Printf("Request ID: %s\n", problem.RequestId)
        
        // Handle specific error types
        switch problem.Status {
        case 401:
            // Invalid API key
        case 429:
            // Rate limit exceeded
        case 503:
            // Service unavailable
        }
    } else {
        // Network or other error
        log.Printf("Request failed: %v", err)
    }
}
```

## Types

All request and response types are provided with proper JSON tags and validation:

- `InferRequest` / `InferResponse` - LLM inference operations
- `UsageDaily` / `UsageRecentItem` - Usage tracking data
- `AdminStatus` - Comprehensive system status
- `CanaryStatus` - Canary deployment information  
- `CreateTenantRequest` / `CreateTenantResponse` - Tenant management
- `Problem` - RFC 7807 error responses

## Thread Safety

The client is thread-safe and can be used concurrently from multiple goroutines. Each client maintains its own HTTP client which handles connection pooling automatically.