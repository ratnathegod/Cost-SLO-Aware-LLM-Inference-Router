"use strict";
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/index.ts
var src_exports = {};
__export(src_exports, {
  LLMRouterAdminClient: () => LLMRouterAdminClient,
  LLMRouterClient: () => LLMRouterClient,
  LLMRouterError: () => LLMRouterError,
  default: () => src_default
});
module.exports = __toCommonJS(src_exports);
var LLMRouterError = class extends Error {
  constructor(problem) {
    super(problem.title);
    this.name = "LLMRouterError";
    this.type = problem.type;
    this.status = problem.status;
    this.detail = problem.detail;
    this.requestId = problem.request_id;
    this.traceId = problem.trace_id || void 0;
  }
};
var LLMRouterClient = class {
  constructor(baseUrl, apiKey, config = {}) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
    this.apiKey = apiKey;
    this.config = {
      timeout: 3e4,
      ...config
    };
  }
  /**
   * Submit a prompt for LLM inference
   */
  async infer(request, options = {}) {
    const headers = {
      "Content-Type": "application/json",
      "X-API-Key": this.apiKey,
      ...this.config.headers
    };
    if (options.idempotencyKey) {
      headers["Idempotency-Key"] = options.idempotencyKey;
    }
    return this.request("POST", "/v1/infer", {
      headers,
      body: JSON.stringify(request)
    });
  }
  /**
   * Get daily usage statistics
   */
  async getDailyUsage(days) {
    const url = days ? `/v1/usage/daily?days=${days}` : "/v1/usage/daily";
    return this.request("GET", url, {
      headers: {
        "X-API-Key": this.apiKey,
        ...this.config.headers
      }
    });
  }
  /**
   * Get recent usage records
   */
  async getRecentUsage() {
    return this.request("GET", "/v1/usage/recent", {
      headers: {
        "X-API-Key": this.apiKey,
        ...this.config.headers
      }
    });
  }
  async request(method, path, options = {}) {
    const fetchFn = this.config.fetch || globalThis.fetch;
    const url = this.baseUrl + path;
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.config.timeout);
    try {
      const response = await fetchFn(url, {
        method,
        signal: controller.signal,
        ...options
      });
      clearTimeout(timeoutId);
      if (!response.ok) {
        await this.handleErrorResponse(response);
      }
      return response.json();
    } catch (error) {
      clearTimeout(timeoutId);
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(`Request timeout after ${this.config.timeout}ms`);
      }
      throw error;
    }
  }
  async handleErrorResponse(response) {
    try {
      const problem = await response.json();
      throw new LLMRouterError(problem);
    } catch (error) {
      if (error instanceof LLMRouterError) {
        throw error;
      }
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
  }
};
var LLMRouterAdminClient = class {
  constructor(baseUrl, adminToken, config = {}) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
    this.adminToken = adminToken;
    this.config = {
      timeout: 3e4,
      ...config
    };
  }
  /**
   * Get comprehensive system status
   */
  async getAdminStatus() {
    return this.request("GET", "/v1/admin/status");
  }
  /**
   * Get canary deployment status
   */
  async getCanaryStatus() {
    return this.request("GET", "/v1/admin/canary/status");
  }
  /**
   * Advance canary to next stage
   */
  async advanceCanary() {
    await this.request("POST", "/v1/admin/canary/advance");
  }
  /**
   * Rollback canary deployment
   */
  async rollbackCanary() {
    await this.request("POST", "/v1/admin/canary/rollback");
  }
  /**
   * Update default routing policy
   */
  async updatePolicy(policy) {
    await this.request("POST", "/v1/admin/policy", {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ default_policy: policy })
    });
  }
  /**
   * Create a new tenant
   */
  async createTenant(request) {
    return this.request("POST", "/v1/admin/tenants", {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request)
    });
  }
  /**
   * Get usage statistics for a specific tenant
   */
  async getTenantUsage(tenantId, options = {}) {
    const params = new URLSearchParams();
    if (options.since)
      params.set("since", options.since);
    if (options.until)
      params.set("until", options.until);
    const query = params.toString();
    const path = `/v1/admin/tenants/${tenantId}/usage${query ? `?${query}` : ""}`;
    return this.request("GET", path);
  }
  async request(method, path, options = {}) {
    const fetchFn = this.config.fetch || globalThis.fetch;
    const url = this.baseUrl + path;
    const headers = {
      "Authorization": `Bearer ${this.adminToken}`,
      ...this.config.headers,
      ...options.headers || {}
    };
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.config.timeout);
    try {
      const response = await fetchFn(url, {
        method,
        signal: controller.signal,
        ...options,
        headers
      });
      clearTimeout(timeoutId);
      if (!response.ok) {
        await this.handleErrorResponse(response);
      }
      if (response.status === 204) {
        return void 0;
      }
      return response.json();
    } catch (error) {
      clearTimeout(timeoutId);
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(`Request timeout after ${this.config.timeout}ms`);
      }
      throw error;
    }
  }
  async handleErrorResponse(response) {
    try {
      const problem = await response.json();
      throw new LLMRouterError(problem);
    } catch (error) {
      if (error instanceof LLMRouterError) {
        throw error;
      }
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
  }
};
var src_default = {
  LLMRouterClient,
  LLMRouterAdminClient,
  LLMRouterError
};
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  LLMRouterAdminClient,
  LLMRouterClient,
  LLMRouterError
});
