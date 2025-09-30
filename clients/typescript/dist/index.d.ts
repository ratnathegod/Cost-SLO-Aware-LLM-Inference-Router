/**
 * TypeScript client for LLM Router API
 *
 * This client provides type-safe access to all LLM Router endpoints including
 * inference, usage tracking, and administrative operations.
 *
 * @example Basic Usage
 * ```typescript
 * import { LLMRouterClient } from '@llm-router/client';
 *
 * const client = new LLMRouterClient('https://api.llm-router.example.com', 'your-api-key');
 *
 * // Make an inference request
 * const response = await client.infer({
 *   prompt: 'What is the capital of France?',
 *   model: 'gpt-4o'
 * });
 *
 * console.log(`Response: ${response.text} (Cost: $${response.cost_usd})`);
 * ```
 *
 * @example Admin Operations
 * ```typescript
 * import { LLMRouterAdminClient } from '@llm-router/client';
 *
 * const admin = new LLMRouterAdminClient('https://api.llm-router.example.com', 'admin-token');
 *
 * // Get system status
 * const status = await admin.getAdminStatus();
 * console.log(`Uptime: ${status.uptime}, Requests: ${status.total_requests}`);
 * ```
 */
interface InferRequest {
    model?: string;
    prompt: string;
    max_tokens?: number;
    stream?: boolean;
    policy?: 'cheapest' | 'fastest_p95' | 'slo_burn_aware' | 'canary';
    idempotency_key?: string;
}
interface InferResponse {
    provider: string;
    text: string;
    cost_usd: number;
    latency_ms: number;
    request_id: string;
}
interface UsageDaily {
    date: string;
    requests: number;
    successes: number;
    failures: number;
    tokens_in: number;
    tokens_out: number;
    cost_usd: number;
}
interface UsageRecentItem {
    ts: string;
    provider: string;
    model: string;
    status: 'ok' | 'error';
    cost_usd: number;
    latency_ms: number;
    idempotency_key?: string;
}
interface Problem {
    type: string;
    title: string;
    status: number;
    detail: string;
    request_id: string;
    trace_id?: string;
}
interface AdminStatus {
    build: {
        version: string;
        commit: string;
        build_date: string;
    };
    uptime: string;
    default_policy: string;
    providers: Array<{
        name: string;
        cb_state: number;
        error_rate_1m: number;
        error_rate_5m: number;
        error_rate_1h: number;
        p95_latency_ms: number;
        cost_per_1k_tokens_usd: number;
    }>;
    burn_rates: {
        burn_rate_1m: number;
        burn_rate_5m: number;
        burn_rate_1h: number;
    };
    total_requests: number;
    canary_stage_percent: number;
}
interface CanaryStatus {
    stage: number;
    percent: number;
    candidate?: string;
    window: number;
    last_transition?: {
        ts: string;
        reason: string;
    };
}
interface CreateTenantRequest {
    name: string;
    plan: string;
    rps_limit: number;
    daily_token_limit: number;
    enabled?: boolean;
}
interface CreateTenantResponse {
    tenant_id: string;
    api_key: string;
    name: string;
    plan: string;
    rps_limit: number;
    daily_token_limit: number;
    enabled: boolean;
    created_at: string;
    updated_at: string;
}
interface InferOptions {
    idempotencyKey?: string;
}
/**
 * Custom error class for LLM Router API errors
 */
declare class LLMRouterError extends Error {
    readonly type: string;
    readonly status: number;
    readonly detail: string;
    readonly requestId: string;
    readonly traceId?: string;
    constructor(problem: Problem);
}
/**
 * Base client configuration
 */
interface ClientConfig {
    timeout?: number;
    headers?: Record<string, string>;
    fetch?: typeof fetch;
}
/**
 * Main client for tenant operations (inference, usage tracking)
 */
declare class LLMRouterClient {
    private readonly baseUrl;
    private readonly apiKey;
    private readonly config;
    constructor(baseUrl: string, apiKey: string, config?: ClientConfig);
    /**
     * Submit a prompt for LLM inference
     */
    infer(request: InferRequest, options?: InferOptions): Promise<InferResponse>;
    /**
     * Get daily usage statistics
     */
    getDailyUsage(days?: number): Promise<UsageDaily[]>;
    /**
     * Get recent usage records
     */
    getRecentUsage(): Promise<UsageRecentItem[]>;
    private request;
    private handleErrorResponse;
}
/**
 * Admin client for administrative operations
 */
declare class LLMRouterAdminClient {
    private readonly baseUrl;
    private readonly adminToken;
    private readonly config;
    constructor(baseUrl: string, adminToken: string, config?: ClientConfig);
    /**
     * Get comprehensive system status
     */
    getAdminStatus(): Promise<AdminStatus>;
    /**
     * Get canary deployment status
     */
    getCanaryStatus(): Promise<CanaryStatus>;
    /**
     * Advance canary to next stage
     */
    advanceCanary(): Promise<void>;
    /**
     * Rollback canary deployment
     */
    rollbackCanary(): Promise<void>;
    /**
     * Update default routing policy
     */
    updatePolicy(policy: string): Promise<void>;
    /**
     * Create a new tenant
     */
    createTenant(request: CreateTenantRequest): Promise<CreateTenantResponse>;
    /**
     * Get usage statistics for a specific tenant
     */
    getTenantUsage(tenantId: string, options?: {
        since?: string;
        until?: string;
    }): Promise<UsageDaily[]>;
    private request;
    private handleErrorResponse;
}

declare const _default: {
    LLMRouterClient: typeof LLMRouterClient;
    LLMRouterAdminClient: typeof LLMRouterAdminClient;
    LLMRouterError: typeof LLMRouterError;
};

export { AdminStatus, CanaryStatus, CreateTenantRequest, CreateTenantResponse, InferOptions, InferRequest, InferResponse, LLMRouterAdminClient, LLMRouterClient, LLMRouterError, Problem, UsageDaily, UsageRecentItem, _default as default };
