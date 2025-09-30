package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Problem represents RFC 7807 Problem Details for HTTP APIs
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail"`
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id,omitempty"`
}

// Error implements error interface for Problem
func (p Problem) Error() string {
	return fmt.Sprintf("%s: %s", p.Title, p.Detail)
}

// Common problem types
const (
	ProblemTypeValidation     = "https://llm-router.example.com/problems/validation-error"
	ProblemTypeAuth          = "https://llm-router.example.com/problems/authentication-error"
	ProblemTypeRateLimit     = "https://llm-router.example.com/problems/rate-limit-exceeded"
	ProblemTypeProvider      = "https://llm-router.example.com/problems/provider-error"
	ProblemTypeNotFound      = "https://llm-router.example.com/problems/not-found"
	ProblemTypeInternal      = "https://llm-router.example.com/problems/internal-error"
	ProblemTypeUsageExceeded = "https://llm-router.example.com/problems/usage-limit-exceeded"
)

// ResponseWriter helps write consistent HTTP responses
type ResponseWriter struct {
	w         http.ResponseWriter
	requestID string
	traceID   string
}

// NewResponseWriter creates a new response writer with request tracking
func NewResponseWriter(w http.ResponseWriter, r *http.Request) *ResponseWriter {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}
	
	traceID := r.Header.Get("X-Trace-ID")
	
	// Set response headers
	w.Header().Set("X-Request-ID", requestID)
	if traceID != "" {
		w.Header().Set("X-Trace-ID", traceID)
	}
	
	return &ResponseWriter{
		w:         w,
		requestID: requestID,
		traceID:   traceID,
	}
}

// WriteJSON writes a JSON response with proper headers
func (rw *ResponseWriter) WriteJSON(status int, data interface{}) error {
	rw.w.Header().Set("Content-Type", "application/json")
	rw.w.WriteHeader(status)
	return json.NewEncoder(rw.w).Encode(data)
}

// WriteProblem writes a Problem response according to RFC 7807
func (rw *ResponseWriter) WriteProblem(problemType, title string, status int, detail string) error {
	problem := Problem{
		Type:      problemType,
		Title:     title,
		Status:    status,
		Detail:    detail,
		RequestID: rw.requestID,
		TraceID:   rw.traceID,
	}
	
	return rw.WriteJSON(status, problem)
}

// WriteValidationError writes a validation error response
func (rw *ResponseWriter) WriteValidationError(field, message string) error {
	detail := fmt.Sprintf("Validation failed for field '%s': %s", field, message)
	return rw.WriteProblem(
		ProblemTypeValidation,
		"Validation Error",
		http.StatusBadRequest,
		detail,
	)
}

// WriteAuthError writes an authentication error response
func (rw *ResponseWriter) WriteAuthError(message string) error {
	return rw.WriteProblem(
		ProblemTypeAuth,
		"Authentication Failed",
		http.StatusUnauthorized,
		message,
	)
}

// WriteRateLimitError writes a rate limit error response
func (rw *ResponseWriter) WriteRateLimitError(limit int, windowSec int) error {
	detail := fmt.Sprintf("Rate limit of %d requests per %d seconds exceeded", limit, windowSec)
	return rw.WriteProblem(
		ProblemTypeRateLimit,
		"Rate Limit Exceeded",
		http.StatusTooManyRequests,
		detail,
	)
}

// WriteProviderError writes a provider error response
func (rw *ResponseWriter) WriteProviderError(provider string, err error) error {
	detail := fmt.Sprintf("Provider '%s' failed: %s", provider, err.Error())
	return rw.WriteProblem(
		ProblemTypeProvider,
		"Provider Error",
		http.StatusBadGateway,
		detail,
	)
}

// WriteNotFoundError writes a not found error response
func (rw *ResponseWriter) WriteNotFoundError(resource string) error {
	detail := fmt.Sprintf("Resource not found: %s", resource)
	return rw.WriteProblem(
		ProblemTypeNotFound,
		"Not Found",
		http.StatusNotFound,
		detail,
	)
}

// WriteInternalError writes an internal server error response
func (rw *ResponseWriter) WriteInternalError(message string) error {
	return rw.WriteProblem(
		ProblemTypeInternal,
		"Internal Server Error",
		http.StatusInternalServerError,
		message,
	)
}

// WriteUsageExceededError writes a usage limit exceeded error
func (rw *ResponseWriter) WriteUsageExceededError(limitType string, current, limit int) error {
	detail := fmt.Sprintf("%s usage limit exceeded: %d/%d", limitType, current, limit)
	return rw.WriteProblem(
		ProblemTypeUsageExceeded,
		"Usage Limit Exceeded",
		http.StatusForbidden,
		detail,
	)
}

// Validation helpers

// ValidateInferRequest validates an InferRequest according to OpenAPI spec
func ValidateInferRequest(req *InferRequest) error {
	if req.Prompt == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}
	
	if len(req.Prompt) > 100000 {
		return fmt.Errorf("prompt exceeds maximum length of 100,000 characters")
	}
	
	if req.MaxTok > 0 && (req.MaxTok < 1 || req.MaxTok > 8192) {
		return fmt.Errorf("max_tokens must be between 1 and 8192")
	}
	
	if req.Policy != "" {
		validPolicies := map[string]bool{
			"cheapest":         true,
			"fastest_p95":      true,
			"slo_burn_aware":   true,
			"canary":          true,
		}
		if !validPolicies[req.Policy] {
			return fmt.Errorf("policy must be one of: cheapest, fastest_p95, slo_burn_aware, canary")
		}
	}
	
	return nil
}

// ValidateCreateTenantRequest validates a CreateTenantRequest
func ValidateCreateTenantRequest(req *CreateTenantRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required and cannot be empty")
	}
	
	if len(req.Name) > 100 {
		return fmt.Errorf("name exceeds maximum length of 100 characters")
	}
	
	// Validate name format (alphanumeric, hyphens, underscores only)
	for _, r := range req.Name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || 
			 (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("name can only contain alphanumeric characters, hyphens, and underscores")
		}
	}
	
	if req.Plan == "" {
		return fmt.Errorf("plan is required")
	}
	
	validPlans := map[string]bool{
		"free":       true,
		"starter":    true,
		"growth":     true,
		"enterprise": true,
	}
	if !validPlans[req.Plan] {
		return fmt.Errorf("plan must be one of: free, starter, growth, enterprise")
	}
	
	if req.RPSLimit < 1 || req.RPSLimit > 10000 {
		return fmt.Errorf("rps_limit must be between 1 and 10000")
	}
	
	if req.DailyTokenLimit < 1000 || req.DailyTokenLimit > 100000000 {
		return fmt.Errorf("daily_token_limit must be between 1000 and 100000000")
	}
	
	return nil
}

// ParseDaysParam parses and validates the 'days' query parameter
func ParseDaysParam(r *http.Request) (int, error) {
	daysStr := r.URL.Query().Get("days")
	if daysStr == "" {
		return 30, nil // default
	}
	
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		return 0, fmt.Errorf("days must be a valid integer")
	}
	
	if days < 1 || days > 365 {
		return 0, fmt.Errorf("days must be between 1 and 365")
	}
	
	return days, nil
}

// ParseTimeParams parses and validates 'since' and 'until' query parameters
func ParseTimeParams(r *http.Request) (since, until *time.Time, err error) {
	sinceStr := r.URL.Query().Get("since")
	untilStr := r.URL.Query().Get("until")
	
	if sinceStr != "" {
		s, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return nil, nil, fmt.Errorf("since must be in RFC3339 format (e.g., 2023-01-01T00:00:00Z)")
		}
		since = &s
	}
	
	if untilStr != "" {
		u, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			return nil, nil, fmt.Errorf("until must be in RFC3339 format (e.g., 2023-01-01T00:00:00Z)")
		}
		until = &u
	}
	
	if since != nil && until != nil && since.After(*until) {
		return nil, nil, fmt.Errorf("since must be before until")
	}
	
	return since, until, nil
}

// ExtractTenantID extracts and validates tenant ID from URL path
func ExtractTenantID(path string) (string, error) {
	// Expected format: /v1/admin/tenants/{tenantId}/...
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "v1" || parts[1] != "admin" || parts[2] != "tenants" {
		return "", fmt.Errorf("invalid tenant path format")
	}
	
	tenantID := parts[3]
	if tenantID == "" {
		return "", fmt.Errorf("tenant ID cannot be empty")
	}
	
	// Validate UUID format
	if _, err := uuid.Parse(tenantID); err != nil {
		return "", fmt.Errorf("tenant ID must be a valid UUID")
	}
	
	return tenantID, nil
}

// CORS middleware
func EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization, Idempotency-Key")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Trace-ID")
		w.Header().Set("Access-Control-Max-Age", "3600")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}