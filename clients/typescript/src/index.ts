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

// Types
export interface InferRequest {
  model?: string;
  prompt: string;
  max_tokens?: number;
  stream?: boolean;
  policy?: 'cheapest' | 'fastest_p95' | 'slo_burn_aware' | 'canary';
  idempotency_key?: string;
}

export interface InferResponse {
  provider: string;
  text: string;
  cost_usd: number;
  latency_ms: number;
  request_id: string;
}

export interface UsageDaily {
  date: string;
  requests: number;
  successes: number;
  failures: number;
  tokens_in: number;
  tokens_out: number;
  cost_usd: number;
}

export interface UsageRecentItem {
  ts: string;
  provider: string;
  model: string;
  status: 'ok' | 'error';
  cost_usd: number;
  latency_ms: number;
  idempotency_key?: string;
}

export interface Problem {
  type: string;
  title: string;
  status: number;
  detail: string;
  request_id: string;
  trace_id?: string;
}

export interface AdminStatus {
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

export interface CanaryStatus {
  stage: number;
  percent: number;
  candidate?: string;
  window: number;
  last_transition?: {
    ts: string;
    reason: string;
  };
}

export interface CreateTenantRequest {
  name: string;
  plan: string;
  rps_limit: number;
  daily_token_limit: number;
  enabled?: boolean;
}

export interface CreateTenantResponse {
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

export interface InferOptions {
  idempotencyKey?: string;
}

/**
 * Custom error class for LLM Router API errors
 */
export class LLMRouterError extends Error {
  public readonly type: string;
  public readonly status: number;
  public readonly detail: string;
  public readonly requestId: string;
  public readonly traceId?: string;

  constructor(problem: Problem) {
    super(problem.title);
    this.name = 'LLMRouterError';
    this.type = problem.type;
    this.status = problem.status;
    this.detail = problem.detail;
    this.requestId = problem.request_id;
    this.traceId = problem.trace_id || undefined;
  }
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
export class LLMRouterClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly config: ClientConfig;

  constructor(baseUrl: string, apiKey: string, config: ClientConfig = {}) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.apiKey = apiKey;
    this.config = {
      timeout: 30000,
      ...config,
    };
  }

  /**
   * Submit a prompt for LLM inference
   */
  async infer(request: InferRequest, options: InferOptions = {}): Promise<InferResponse> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-API-Key': this.apiKey,
      ...this.config.headers,
    };

    if (options.idempotencyKey) {
      headers['Idempotency-Key'] = options.idempotencyKey;
    }

    return this.request<InferResponse>('POST', '/v1/infer', {
      headers,
      body: JSON.stringify(request),
    });
  }

  /**
   * Get daily usage statistics
   */
  async getDailyUsage(days?: number): Promise<UsageDaily[]> {
    const url = days ? `/v1/usage/daily?days=${days}` : '/v1/usage/daily';
    
    return this.request<UsageDaily[]>('GET', url, {
      headers: {
        'X-API-Key': this.apiKey,
        ...this.config.headers,
      },
    });
  }

  /**
   * Get recent usage records
   */
  async getRecentUsage(): Promise<UsageRecentItem[]> {
    return this.request<UsageRecentItem[]>('GET', '/v1/usage/recent', {
      headers: {
        'X-API-Key': this.apiKey,
        ...this.config.headers,
      },
    });
  }

  private async request<T>(
    method: string,
    path: string,
    options: RequestInit = {}
  ): Promise<T> {
    const fetchFn = this.config.fetch || globalThis.fetch;
    const url = this.baseUrl + path;

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.config.timeout);

    try {
      const response = await fetchFn(url, {
        method,
        signal: controller.signal,
        ...options,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        await this.handleErrorResponse(response);
      }

      return response.json() as Promise<T>;
    } catch (error) {
      clearTimeout(timeoutId);
      
      if (error instanceof Error && error.name === 'AbortError') {
        throw new Error(`Request timeout after ${this.config.timeout}ms`);
      }
      
      throw error;
    }
  }

  private async handleErrorResponse(response: Response): Promise<never> {
    try {
      const problem: Problem = await response.json();
      throw new LLMRouterError(problem);
    } catch (error) {
      if (error instanceof LLMRouterError) {
        throw error;
      }
      
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
  }
}

/**
 * Admin client for administrative operations
 */
export class LLMRouterAdminClient {
  private readonly baseUrl: string;
  private readonly adminToken: string;
  private readonly config: ClientConfig;

  constructor(baseUrl: string, adminToken: string, config: ClientConfig = {}) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.adminToken = adminToken;
    this.config = {
      timeout: 30000,
      ...config,
    };
  }

  /**
   * Get comprehensive system status
   */
  async getAdminStatus(): Promise<AdminStatus> {
    return this.request<AdminStatus>('GET', '/v1/admin/status');
  }

  /**
   * Get canary deployment status
   */
  async getCanaryStatus(): Promise<CanaryStatus> {
    return this.request<CanaryStatus>('GET', '/v1/admin/canary/status');
  }

  /**
   * Advance canary to next stage
   */
  async advanceCanary(): Promise<void> {
    await this.request<void>('POST', '/v1/admin/canary/advance');
  }

  /**
   * Rollback canary deployment
   */
  async rollbackCanary(): Promise<void> {
    await this.request<void>('POST', '/v1/admin/canary/rollback');
  }

  /**
   * Update default routing policy
   */
  async updatePolicy(policy: string): Promise<void> {
    await this.request<void>('POST', '/v1/admin/policy', {
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ default_policy: policy }),
    });
  }

  /**
   * Create a new tenant
   */
  async createTenant(request: CreateTenantRequest): Promise<CreateTenantResponse> {
    return this.request<CreateTenantResponse>('POST', '/v1/admin/tenants', {
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    });
  }

  /**
   * Get usage statistics for a specific tenant
   */
  async getTenantUsage(
    tenantId: string,
    options: { since?: string; until?: string } = {}
  ): Promise<UsageDaily[]> {
    const params = new URLSearchParams();
    if (options.since) params.set('since', options.since);
    if (options.until) params.set('until', options.until);
    
    const query = params.toString();
    const path = `/v1/admin/tenants/${tenantId}/usage${query ? `?${query}` : ''}`;
    
    return this.request<UsageDaily[]>('GET', path);
  }

  private async request<T>(
    method: string,
    path: string,
    options: RequestInit = {}
  ): Promise<T> {
    const fetchFn = this.config.fetch || globalThis.fetch;
    const url = this.baseUrl + path;

    const headers = {
      'Authorization': `Bearer ${this.adminToken}`,
      ...this.config.headers,
      ...((options.headers as Record<string, string>) || {}),
    };

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.config.timeout);

    try {
      const response = await fetchFn(url, {
        method,
        signal: controller.signal,
        ...options,
        headers,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        await this.handleErrorResponse(response);
      }

      if (response.status === 204) {
        return undefined as T;
      }

      return response.json() as Promise<T>;
    } catch (error) {
      clearTimeout(timeoutId);
      
      if (error instanceof Error && error.name === 'AbortError') {
        throw new Error(`Request timeout after ${this.config.timeout}ms`);
      }
      
      throw error;
    }
  }

  private async handleErrorResponse(response: Response): Promise<never> {
    try {
      const problem: Problem = await response.json();
      throw new LLMRouterError(problem);
    } catch (error) {
      if (error instanceof LLMRouterError) {
        throw error;
      }
      
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
  }
}

// Re-export everything for convenience
export * from './index';

// Default export for common usage
export default {
  LLMRouterClient,
  LLMRouterAdminClient,
  LLMRouterError,
};