# API Documentation & SDK System

This directory contains the complete API contract system for the LLM Router, including OpenAPI specification, interactive documentation, and client SDKs.

## 📋 Overview

The LLM Router provides a first-class API contract with:
- **OpenAPI 3.1 Specification** - Complete, standards-compliant API documentation
- **Interactive Swagger UI** - Browser-based API explorer with admin endpoint badges
- **Go SDK** - Type-safe client with zero external dependencies
- **TypeScript SDK** - ESM/CJS compatible with comprehensive type definitions
- **CI/CD Integration** - Automated validation and SDK freshness checks

## 📁 Directory Structure

```
api/
├── openapi.yaml           # Complete OpenAPI 3.1 specification
└── README.md             # This documentation

internal/docs/
├── swagger.go            # Swagger UI server with embedded assets
├── openapi.yaml         # Embedded copy of OpenAPI spec
└── swagger-ui/          # Embedded Swagger UI assets (auto-downloaded)

clients/
├── go/                  # Go SDK
│   ├── client.go        # Type-safe Go client implementation
│   ├── go.mod          # Module definition
│   └── README.md       # Go SDK documentation
└── typescript/         # TypeScript SDK
    ├── src/index.ts    # TypeScript client implementation
    ├── package.json    # NPM package configuration
    ├── tsconfig.json   # TypeScript configuration
    └── README.md       # TypeScript SDK documentation

Makefile.api            # API development and validation targets
```

## 🚀 Quick Start

### View API Documentation

Start the server and visit the interactive documentation:

```bash
# Start the LLM Router server
go run ./cmd/server

# Visit the documentation
open http://localhost:8080/docs
```

### Use Go SDK

```go
import "github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/clients/go"

client := llmrouter.NewClient("https://api.example.com", "your-api-key")

response, err := client.Infer(context.Background(), llmrouter.InferRequest{
    Prompt: "What is the capital of France?",
    Model:  "gpt-4o",
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Response: %s (Cost: $%.4f)\n", response.Text, response.CostUSD)
```

### Use TypeScript SDK

```bash
npm install @llm-router/client
```

```typescript
import { LLMRouterClient } from '@llm-router/client';

const client = new LLMRouterClient('https://api.example.com', 'your-api-key');

const response = await client.infer({
  prompt: 'What is the capital of France?',
  model: 'gpt-4o'
});

console.log(`Response: ${response.text} (Cost: $${response.cost_usd})`);
```

## 🛠️ Development Workflow

### API Validation

```bash
# Validate OpenAPI specification
make -f Makefile.api api-validate

# Lint with Spectral (requires npm install -g @stoplight/spectral-cli)
make -f Makefile.api api-lint

# Check if SDKs are up to date with API spec
make -f Makefile.api check-sdk-freshness
```

### SDK Development

```bash
# Build and test Go SDK
make -f Makefile.api sdk-go

# Build and test TypeScript SDK  
make -f Makefile.api sdk-ts

# Run all validations and builds
make -f Makefile.api all
```

### Documentation

```bash
# Build embedded documentation assets
make -f Makefile.api docs-build

# Serve documentation locally
make -f Makefile.api docs-serve
```

## 📖 API Specification Highlights

### Core Endpoints

- **POST /v1/infer** - Submit prompts for LLM inference
- **GET /v1/usage/daily** - Retrieve daily usage statistics  
- **GET /v1/usage/recent** - Get recent usage records

### Admin Endpoints

- **GET /v1/admin/status** - Comprehensive system status
- **GET /v1/admin/canary/status** - Canary deployment status
- **POST /v1/admin/canary/advance** - Advance canary stage
- **POST /v1/admin/canary/rollback** - Rollback canary
- **POST /v1/admin/policy** - Update routing policy
- **POST /v1/admin/tenants** - Create new tenant
- **GET /v1/admin/tenants/{id}/usage** - Tenant usage statistics

### Authentication

- **API Key Authentication**: Use `X-API-Key` header for tenant endpoints
- **Bearer Token Authentication**: Use `Authorization: Bearer <token>` for admin endpoints

### Error Handling

All endpoints return RFC 7807 Problem Details for consistent error responses:

```json
{
  "type": "https://llm-router.example.com/problems/validation-error",
  "title": "Validation Error", 
  "status": 400,
  "detail": "Validation failed for field 'prompt': prompt is required",
  "request_id": "req_123456789"
}
```

## 🏗️ Architecture Details

### Embedded Documentation

The Swagger UI is embedded directly into the Go binary using `//go:embed`:
- Zero external dependencies for documentation serving
- Swagger UI assets automatically downloaded during build
- Custom HTML template with admin endpoint badges
- OpenAPI spec served as both YAML and embedded asset

### SDK Design Principles

**Go SDK:**
- Zero external dependencies (uses only standard library)
- Comprehensive error handling with structured Problem responses
- Context support for cancellation and timeouts
- Separate tenant and admin clients for clear separation of concerns

**TypeScript SDK:**
- Dual ESM/CJS exports for maximum compatibility
- Tree-shakeable for optimal bundle sizes
- Fetch-based with custom fetch support for Node.js
- Comprehensive TypeScript definitions

### CI/CD Integration

GitHub Actions workflow includes:
- OpenAPI specification validation with Spectral
- SDK freshness checks (compare modification times)
- Go SDK compilation and testing
- TypeScript SDK build and type checking
- Documentation accessibility testing

## 🔧 Configuration

### Spectral Linting Rules

The `.spectral.yml` configuration enforces:
- Required operation IDs and summaries
- Proper error response definitions (4xx responses required)
- Valid examples and schemas
- Consistent tagging and descriptions

### Build Dependencies

**Development:**
- `@stoplight/spectral-cli` - API linting
- `swagger` CLI - OpenAPI validation
- Node.js 18+ - TypeScript SDK builds
- Go 1.23+ - Go SDK and server builds

**Runtime:**
- Go 1.23 embed directive - Asset embedding
- Chi router - HTTP routing
- No external dependencies for core functionality

## 📚 API Design Standards

### OpenAPI 3.1 Features Used

- **Discriminator Objects** - For polymorphic responses
- **Example Objects** - Comprehensive request/response examples  
- **Security Schemes** - API Key and Bearer token definitions
- **Problem Details** - RFC 7807 error response format
- **Server Objects** - Multiple environment definitions

### Consistency Guidelines

1. **Naming**: Use snake_case for JSON properties
2. **Timestamps**: ISO 8601 (RFC 3339) format
3. **IDs**: UUID v4 format for all resource identifiers
4. **Pagination**: Cursor-based with `limit` and `cursor` parameters
5. **Versioning**: URL path versioning (`/v1`, `/v2`, etc.)

### Error Response Standards

All error responses follow RFC 7807 Problem Details:
- `type`: URI identifying the problem type
- `title`: Human-readable summary
- `status`: HTTP status code
- `detail`: Human-readable explanation
- `request_id`: Unique identifier for tracing

## 🚨 Security Considerations

### API Key Management

- API keys are UUIDs stored in DynamoDB or JSON files
- Each tenant has a unique API key with associated rate limits
- Keys are validated on every request via middleware

### Admin Token Security  

- Admin endpoints require Bearer token authentication
- Tokens should be rotated regularly and stored securely
- Admin operations are logged for audit purposes

### CORS Configuration

- Configured for cross-origin requests in browser environments
- Exposes necessary headers: `X-Request-ID`, `X-Trace-ID`
- Preflight support for complex requests

## 🎯 Future Enhancements

### Planned Features

1. **GraphQL Schema** - Alternative query interface
2. **gRPC Definitions** - High-performance RPC interface  
3. **Additional SDKs** - Python, Rust, Java clients
4. **OpenTelemetry Tracing** - Distributed tracing integration
5. **Webhook Support** - Event-driven notifications

### SDK Improvements

1. **Retry Logic** - Configurable retry policies with exponential backoff
2. **Circuit Breaker** - Client-side circuit breaker implementation
3. **Caching** - Response caching with TTL support
4. **Pagination** - Helper methods for paginated endpoints
5. **Streaming** - WebSocket and SSE support for real-time updates

---

For more details, see the individual SDK documentation in `clients/go/README.md` and `clients/typescript/README.md`.