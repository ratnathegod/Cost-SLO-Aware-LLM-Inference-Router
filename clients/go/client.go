// Package llmrouter provides a Go client for the LLM Router API
//
// This client is auto-generated from the OpenAPI specification.
// It provides type-safe access to all LLM Router endpoints including
// inference, usage tracking, and administrative operations.
//
// Basic Usage:
//
//	client := llmrouter.NewClient("https://api.llm-router.example.com", "your-api-key")
//
//	// Make an inference request
//	resp, err := client.Infer(ctx, llmrouter.InferRequest{
//		Prompt: "What is the capital of France?",
//		Model:  "gpt-4o",
//	})
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	fmt.Printf("Response: %s (Provider: %s, Cost: $%.4f)\n", 
//		resp.Text, resp.Provider, resp.CostUsd)
//
// Administrative Usage:
//
//	adminClient := llmrouter.NewAdminClient("https://api.llm-router.example.com", "admin-token")
//
//	// Get system status
//	status, err := adminClient.GetAdminStatus(ctx)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	fmt.Printf("Uptime: %s, Total Requests: %d\n", status.Uptime, status.TotalRequests)
//
package llmrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client provides access to the LLM Router API
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// AdminClient provides access to administrative endpoints
type AdminClient struct {
	baseURL     string
	adminToken  string
	httpClient  *http.Client
}

// NewClient creates a new LLM Router API client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewAdminClient creates a new admin client for administrative operations
func NewAdminClient(baseURL, adminToken string) *AdminClient {
	return &AdminClient{
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		adminToken:  adminToken,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// WithHTTPClient allows customization of the underlying HTTP client
func (c *Client) WithHTTPClient(client *http.Client) *Client {
	c.httpClient = client
	return c
}

// WithHTTPClient allows customization of the underlying HTTP client for admin operations
func (c *AdminClient) WithHTTPClient(client *http.Client) *AdminClient {
	c.httpClient = client
	return c
}

// InferRequest represents a request to generate LLM inference
type InferRequest struct {
	Model           *string `json:"model,omitempty"`
	Prompt          string  `json:"prompt"`
	MaxTokens       *int    `json:"max_tokens,omitempty"`
	Stream          *bool   `json:"stream,omitempty"`
	Policy          *string `json:"policy,omitempty"`
	IdempotencyKey  *string `json:"idempotency_key,omitempty"`
}

// InferResponse represents the response from an inference request
type InferResponse struct {
	Provider  string  `json:"provider"`
	Text      string  `json:"text"`
	CostUsd   float64 `json:"cost_usd"`
	LatencyMs int     `json:"latency_ms"`
	RequestId string  `json:"request_id"`
}

// UsageDaily represents daily usage statistics
type UsageDaily struct {
	Date       string  `json:"date"`
	Requests   int     `json:"requests"`
	Successes  int     `json:"successes"`
	Failures   int     `json:"failures"`
	TokensIn   int     `json:"tokens_in"`
	TokensOut  int     `json:"tokens_out"`
	CostUsd    float64 `json:"cost_usd"`
}

// UsageRecentItem represents a recent usage record
type UsageRecentItem struct {
	Ts             string  `json:"ts"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	Status         string  `json:"status"`
	CostUsd        float64 `json:"cost_usd"`
	LatencyMs      int     `json:"latency_ms"`
	IdempotencyKey *string `json:"idempotency_key,omitempty"`
}

// Problem represents an RFC 7807 problem response
type Problem struct {
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	Status    int     `json:"status"`
	Detail    string  `json:"detail"`
	RequestId string  `json:"request_id"`
	TraceId   *string `json:"trace_id,omitempty"`
}

// Error implements the error interface for Problem
func (p Problem) Error() string {
	return fmt.Sprintf("%s (%d): %s", p.Title, p.Status, p.Detail)
}

// AdminStatus represents comprehensive system status
type AdminStatus struct {
	Build struct {
		Version   string    `json:"version"`
		Commit    string    `json:"commit"`
		BuildDate time.Time `json:"build_date"`
	} `json:"build"`
	Uptime             string `json:"uptime"`
	DefaultPolicy      string `json:"default_policy"`
	Providers          []Provider `json:"providers"`
	BurnRates          BurnRates  `json:"burn_rates"`
	TotalRequests      int        `json:"total_requests"`
	CanaryStagePercent float64    `json:"canary_stage_percent"`
}

// Provider represents provider status information
type Provider struct {
	Name               string  `json:"name"`
	CbState            float64 `json:"cb_state"`
	ErrorRate1m        float64 `json:"error_rate_1m"`
	ErrorRate5m        float64 `json:"error_rate_5m"`
	ErrorRate1h        float64 `json:"error_rate_1h"`
	P95LatencyMs       float64 `json:"p95_latency_ms"`
	CostPer1kTokensUsd float64 `json:"cost_per_1k_tokens_usd"`
}

// BurnRates represents error budget burn rates
type BurnRates struct {
	BurnRate1m float64 `json:"burn_rate_1m"`
	BurnRate5m float64 `json:"burn_rate_5m"`
	BurnRate1h float64 `json:"burn_rate_1h"`
}

// CanaryStatus represents canary deployment status
type CanaryStatus struct {
	Stage      int    `json:"stage"`
	Percent    float64 `json:"percent"`
	Candidate  *string `json:"candidate,omitempty"`
	Window     int    `json:"window"`
	LastTransition *struct {
		Ts     time.Time `json:"ts"`
		Reason string    `json:"reason"`
	} `json:"last_transition,omitempty"`
}

// CreateTenantRequest represents a request to create a new tenant
type CreateTenantRequest struct {
	Name            string `json:"name"`
	Plan            string `json:"plan"`
	RpsLimit        int    `json:"rps_limit"`
	DailyTokenLimit int64  `json:"daily_token_limit"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

// CreateTenantResponse represents the response when creating a tenant
type CreateTenantResponse struct {
	TenantId        string    `json:"tenant_id"`
	ApiKey          string    `json:"api_key"`
	Name            string    `json:"name"`
	Plan            string    `json:"plan"`
	RpsLimit        int       `json:"rps_limit"`
	DailyTokenLimit int64     `json:"daily_token_limit"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// InferOptions provides additional options for inference requests
type InferOptions struct {
	IdempotencyKey *string
}

// Infer submits a prompt for LLM inference
func (c *Client) Infer(ctx context.Context, req InferRequest, opts ...InferOptions) (*InferResponse, error) {
	var opt InferOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/infer", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
	
	if opt.IdempotencyKey != nil {
		httpReq.Header.Set("Idempotency-Key", *opt.IdempotencyKey)
	}
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}
	
	var result InferResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	return &result, nil
}

// GetDailyUsage retrieves daily usage statistics
func (c *Client) GetDailyUsage(ctx context.Context, days *int) ([]UsageDaily, error) {
	url := c.baseURL + "/v1/usage/daily"
	if days != nil {
		url += "?days=" + strconv.Itoa(*days)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("X-API-Key", c.apiKey)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}
	
	var result []UsageDaily
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	return result, nil
}

// GetRecentUsage retrieves recent usage records
func (c *Client) GetRecentUsage(ctx context.Context) ([]UsageRecentItem, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/usage/recent", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("X-API-Key", c.apiKey)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}
	
	var result []UsageRecentItem
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	return result, nil
}

// GetAdminStatus retrieves comprehensive system status
func (c *AdminClient) GetAdminStatus(ctx context.Context) (*AdminStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/admin/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Authorization", "Bearer "+c.adminToken)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponseAdmin(resp)
	}
	
	var result AdminStatus
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	return &result, nil
}

// GetCanaryStatus retrieves canary deployment status
func (c *AdminClient) GetCanaryStatus(ctx context.Context) (*CanaryStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/admin/canary/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Authorization", "Bearer "+c.adminToken)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponseAdmin(resp)
	}
	
	var result CanaryStatus
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	return &result, nil
}

// AdvanceCanary moves canary to the next stage
func (c *AdminClient) AdvanceCanary(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/admin/canary/advance", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Authorization", "Bearer "+c.adminToken)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusNoContent {
		return c.handleErrorResponseAdmin(resp)
	}
	
	return nil
}

// RollbackCanary rolls back canary deployment
func (c *AdminClient) RollbackCanary(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/admin/canary/rollback", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Authorization", "Bearer "+c.adminToken)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusNoContent {
		return c.handleErrorResponseAdmin(resp)
	}
	
	return nil
}

// UpdatePolicy updates the default routing policy
func (c *AdminClient) UpdatePolicy(ctx context.Context, policy string) error {
	body, err := json.Marshal(map[string]string{"default_policy": policy})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/admin/policy", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.adminToken)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusNoContent {
		return c.handleErrorResponseAdmin(resp)
	}
	
	return nil
}

// CreateTenant creates a new tenant
func (c *AdminClient) CreateTenant(ctx context.Context, req CreateTenantRequest) (*CreateTenantResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/admin/tenants", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.adminToken)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		return nil, c.handleErrorResponseAdmin(resp)
	}
	
	var result CreateTenantResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	return &result, nil
}

// GetTenantUsage retrieves usage statistics for a specific tenant
func (c *AdminClient) GetTenantUsage(ctx context.Context, tenantID string, since, until *string) ([]UsageDaily, error) {
	u, err := url.Parse(c.baseURL + "/v1/admin/tenants/" + tenantID + "/usage")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	
	q := u.Query()
	if since != nil {
		q.Set("since", *since)
	}
	if until != nil {
		q.Set("until", *until)
	}
	u.RawQuery = q.Encode()
	
	httpReq, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	
	httpReq.Header.Set("Authorization", "Bearer "+c.adminToken)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponseAdmin(resp)
	}
	
	var result []UsageDaily
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	
	return result, nil
}

func (c *Client) handleErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d: failed to read error response", resp.StatusCode)
	}
	
	var problem Problem
	if err := json.Unmarshal(body, &problem); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	
	return problem
}

func (c *AdminClient) handleErrorResponseAdmin(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d: failed to read error response", resp.StatusCode)
	}
	
	var problem Problem
	if err := json.Unmarshal(body, &problem); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	
	return problem
}