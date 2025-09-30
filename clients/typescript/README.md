# TypeScript SDK for LLM Router

A type-safe TypeScript/JavaScript client for the LLM Router API with support for both ESM and CommonJS modules.

## Installation

```bash
npm install @llm-router/client
# or
yarn add @llm-router/client
```

## Quick Start

### Basic Inference

```typescript
import { LLMRouterClient } from '@llm-router/client';

const client = new LLMRouterClient('https://api.llm-router.example.com', 'your-api-key');

// Make an inference request
const response = await client.infer({
  prompt: 'What is the capital of France?',
  model: 'gpt-4o'
});

console.log(`Response: ${response.text}`);
console.log(`Cost: $${response.cost_usd}`);
console.log(`Latency: ${response.latency_ms}ms`);
```

### Administrative Operations

```typescript
import { LLMRouterAdminClient } from '@llm-router/client';

const admin = new LLMRouterAdminClient('https://api.llm-router.example.com', 'admin-token');

// Get system status
const status = await admin.getAdminStatus();
console.log(`Uptime: ${status.uptime}`);
console.log(`Total requests: ${status.total_requests}`);

// Create a new tenant
const tenant = await admin.createTenant({
  name: 'acme-corp',
  plan: 'enterprise',
  rps_limit: 100,
  daily_token_limit: 1000000
});

console.log(`Created tenant ${tenant.tenant_id} with API key: ${tenant.api_key}`);
```

## Configuration

Both clients accept an optional configuration object:

```typescript
const config = {
  timeout: 30000,        // Request timeout in milliseconds
  headers: {             // Additional headers to send with requests
    'X-Custom': 'value'
  },
  fetch: customFetch     // Custom fetch implementation (useful for Node.js)
};

const client = new LLMRouterClient(baseUrl, apiKey, config);
```

## Error Handling

The SDK provides structured error handling with the `LLMRouterError` class:

```typescript
import { LLMRouterClient, LLMRouterError } from '@llm-router/client';

try {
  const response = await client.infer({
    prompt: 'Hello world',
    model: 'invalid-model'
  });
} catch (error) {
  if (error instanceof LLMRouterError) {
    console.error(`API Error: ${error.title}`);
    console.error(`Status: ${error.status}`);
    console.error(`Detail: ${error.detail}`);
    console.error(`Request ID: ${error.requestId}`);
  } else {
    console.error('Network or other error:', error.message);
  }
}
```

## API Reference

### LLMRouterClient

Main client for tenant operations:

#### `infer(request: InferRequest, options?: InferOptions): Promise<InferResponse>`

Submit a prompt for LLM inference.

**Parameters:**
- `request.prompt` (string, required): The input prompt
- `request.model` (string, optional): Specific model to use
- `request.max_tokens` (number, optional): Maximum tokens to generate
- `request.stream` (boolean, optional): Enable streaming response
- `request.policy` (string, optional): Routing policy ('cheapest', 'fastest_p95', 'slo_burn_aware', 'canary')
- `options.idempotencyKey` (string, optional): Idempotency key for duplicate prevention

#### `getDailyUsage(days?: number): Promise<UsageDaily[]>`

Get daily usage statistics for the tenant.

#### `getRecentUsage(): Promise<UsageRecentItem[]>`

Get recent usage records for the tenant.

### LLMRouterAdminClient

Administrative client for system management:

#### `getAdminStatus(): Promise<AdminStatus>`

Get comprehensive system status including provider health and burn rates.

#### `getCanaryStatus(): Promise<CanaryStatus>`

Get canary deployment status.

#### `advanceCanary(): Promise<void>`

Advance canary deployment to next stage.

#### `rollbackCanary(): Promise<void>`

Rollback canary deployment.

#### `updatePolicy(policy: string): Promise<void>`

Update the default routing policy.

#### `createTenant(request: CreateTenantRequest): Promise<CreateTenantResponse>`

Create a new tenant with specified limits and configuration.

#### `getTenantUsage(tenantId: string, options?: { since?: string; until?: string }): Promise<UsageDaily[]>`

Get usage statistics for a specific tenant.

## Types

All TypeScript types are exported for use in your applications:

```typescript
import type {
  InferRequest,
  InferResponse,
  UsageDaily,
  UsageRecentItem,
  AdminStatus,
  CanaryStatus,
  CreateTenantRequest,
  CreateTenantResponse,
  Problem
} from '@llm-router/client';
```

## Node.js Usage

For Node.js environments, you may need to provide a fetch implementation:

```typescript
import { LLMRouterClient } from '@llm-router/client';
import fetch from 'node-fetch';

const client = new LLMRouterClient('https://api.llm-router.example.com', 'api-key', {
  fetch: fetch as any
});
```

Or install a fetch polyfill:

```bash
npm install node-fetch
```

## Browser Usage

The SDK works in all modern browsers that support fetch and ES2018+. For older browsers, you may need to include polyfills.

## Contributing

This SDK is part of the LLM Router project. See the main repository for contribution guidelines.

## License

MIT - See LICENSE file in the main repository.