package rate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/auth"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/telemetry"
)

// Bucket represents a token bucket for rate limiting
type Bucket struct {
	mu            sync.Mutex
	capacity      int64
	tokens        int64
	refillRate    int64 // tokens per second
	lastRefill    time.Time
	burstCapacity int64
}

func NewBucket(capacity, refillRate int64) *Bucket {
	burstCapacity := capacity * 2 // Allow 2x burst
	return &Bucket{
		capacity:      capacity,
		tokens:        capacity,
		refillRate:    refillRate,
		lastRefill:    time.Now(),
		burstCapacity: burstCapacity,
	}
}

func (b *Bucket) Allow(tokensNeeded int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()

	// Refill tokens
	tokensToAdd := int64(elapsed * float64(b.refillRate))
	b.tokens = min(b.burstCapacity, b.tokens+tokensToAdd)
	b.lastRefill = now

	if b.tokens >= tokensNeeded {
		b.tokens -= tokensNeeded
		return true
	}
	return false
}

func (b *Bucket) TokensRemaining() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens
}

func (b *Bucket) NextRefillTime(tokensNeeded int64) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.tokens >= tokensNeeded {
		return time.Now()
	}

	tokensNeededFromRefill := tokensNeeded - b.tokens
	secondsUntilRefill := float64(tokensNeededFromRefill) / float64(b.refillRate)
	return time.Now().Add(time.Duration(secondsUntilRefill * float64(time.Second)))
}

// DailyUsage tracks daily token usage
type DailyUsage struct {
	mu         sync.RWMutex
	usage      map[string]int64     // tenantID -> tokens used today
	resetTimes map[string]time.Time // tenantID -> next reset time
}

func NewDailyUsage() *DailyUsage {
	return &DailyUsage{
		usage:      make(map[string]int64),
		resetTimes: make(map[string]time.Time),
	}
}

func (du *DailyUsage) AddTokens(tenantID string, tokens int64) {
	du.mu.Lock()
	defer du.mu.Unlock()

	now := time.Now()

	// Check if we need to reset (new day)
	if resetTime, exists := du.resetTimes[tenantID]; exists {
		if now.After(resetTime) {
			du.usage[tenantID] = 0
		}
	}

	// Set next reset time (midnight tomorrow)
	tomorrow := now.AddDate(0, 0, 1)
	midnight := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
	du.resetTimes[tenantID] = midnight

	du.usage[tenantID] += tokens
}

func (du *DailyUsage) GetUsage(tenantID string) int64 {
	du.mu.RLock()
	defer du.mu.RUnlock()

	now := time.Now()

	// Check if usage has expired (new day)
	if resetTime, exists := du.resetTimes[tenantID]; exists {
		if now.After(resetTime) {
			return 0
		}
	}

	return du.usage[tenantID]
}

func (du *DailyUsage) GetReset(tenantID string) time.Time {
	du.mu.RLock()
	defer du.mu.RUnlock()

	if resetTime, exists := du.resetTimes[tenantID]; exists {
		return resetTime
	}

	// Return midnight tomorrow if no reset time set
	tomorrow := time.Now().AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
}

// Limiter manages rate limiting for tenants
type Limiter struct {
	rpsBuckets map[string]*Bucket
	dailyUsage *DailyUsage
	mu         sync.RWMutex
}

func NewLimiter() *Limiter {
	return &Limiter{
		rpsBuckets: make(map[string]*Bucket),
		dailyUsage: NewDailyUsage(),
	}
}

func (l *Limiter) getRPSBucket(tenantID string, rpsLimit int) *Bucket {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.rpsBuckets[tenantID]
	if !exists {
		bucket = NewBucket(int64(rpsLimit), int64(rpsLimit))
		l.rpsBuckets[tenantID] = bucket
	}
	return bucket
}

// CheckRPS verifies if the request is within RPS limits
func (l *Limiter) CheckRPS(tenantID string, rpsLimit int) (bool, int64, time.Time) {
	bucket := l.getRPSBucket(tenantID, rpsLimit)

	allowed := bucket.Allow(1)
	remaining := bucket.TokensRemaining()
	resetTime := bucket.NextRefillTime(1)

	return allowed, remaining, resetTime
}

// CheckDailyTokens verifies if the request is within daily token limits
func (l *Limiter) CheckDailyTokens(tenantID string, tokensNeeded int64, dailyLimit int64) (bool, int64, time.Time) {
	currentUsage := l.dailyUsage.GetUsage(tenantID)

	if currentUsage+tokensNeeded > dailyLimit {
		remaining := max(0, dailyLimit-currentUsage)
		resetTime := l.dailyUsage.GetReset(tenantID)
		return false, remaining, resetTime
	}

	return true, dailyLimit - currentUsage - tokensNeeded, l.dailyUsage.GetReset(tenantID)
}

// RecordTokenUsage records token usage for daily limits
func (l *Limiter) RecordTokenUsage(tenantID string, tokens int64) {
	l.dailyUsage.AddTokens(tenantID, tokens)
}

// RateLimitMiddleware provides rate limiting for API requests
func (l *Limiter) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := auth.GetTenantFromContext(r.Context())
		if !ok {
			// If no tenant in context, pass through (should be caught by auth middleware)
			next.ServeHTTP(w, r)
			return
		}

		// Check RPS limit
		allowed, remaining, resetTime := l.CheckRPS(tenant.TenantID, tenant.RPSLimit)
		if !allowed {
			telemetry.RequestsTotal.WithLabelValues("rate_limited", "", "429").Inc()
			l.writeRateLimitError(w, r, "rps", tenant.RPSLimit, remaining, resetTime)
			return
		}

		// Set RPS headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(tenant.RPSLimit))
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

		// For token limits, we need to estimate tokens from the request
		// This is a simplified estimation - in practice you'd parse the request body
		estimatedTokens := l.estimateTokensFromRequest(r)

		tokenAllowed, tokenRemaining, tokenReset := l.CheckDailyTokens(tenant.TenantID, estimatedTokens, tenant.DailyTokenLimit)
		if !tokenAllowed {
			telemetry.RequestsTotal.WithLabelValues("token_limited", "", "429").Inc()
			l.writeTokenLimitError(w, r, tenant.DailyTokenLimit, tokenRemaining, tokenReset)
			return
		}

		// Set token limit headers
		w.Header().Set("X-TokenLimit-Limit", strconv.FormatInt(tenant.DailyTokenLimit, 10))
		w.Header().Set("X-TokenLimit-Remaining", strconv.FormatInt(tokenRemaining, 10))
		w.Header().Set("X-TokenLimit-Reset", strconv.FormatInt(tokenReset.Unix(), 10))

		// Record pre-request token usage estimate
		l.RecordTokenUsage(tenant.TenantID, estimatedTokens)

		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) estimateTokensFromRequest(r *http.Request) int64 {
	// Simple estimation - in practice, you'd parse the request body and use model-specific rates
	// For now, assume average request uses 1000 tokens
	return 1000
}

func (l *Limiter) writeRateLimitError(w http.ResponseWriter, r *http.Request, reason string, limit int, remaining int64, resetTime time.Time) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Retry-After", strconv.FormatInt(resetTime.Unix()-time.Now().Unix(), 10))
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"type":   "https://example.com/errors/rate_limit_exceeded",
		"title":  "Rate limit exceeded",
		"detail": fmt.Sprintf("Request rate limit of %d requests per second exceeded", limit),
		"status": http.StatusTooManyRequests,
	}

	_ = json.NewEncoder(w).Encode(response)
}

func (l *Limiter) writeTokenLimitError(w http.ResponseWriter, r *http.Request, limit, remaining int64, resetTime time.Time) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Retry-After", strconv.FormatInt(resetTime.Unix()-time.Now().Unix(), 10))
	w.Header().Set("X-TokenLimit-Limit", strconv.FormatInt(limit, 10))
	w.Header().Set("X-TokenLimit-Remaining", strconv.FormatInt(remaining, 10))
	w.Header().Set("X-TokenLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"type":   "https://example.com/errors/token_limit_exceeded",
		"title":  "Daily token limit exceeded",
		"detail": fmt.Sprintf("Daily token limit of %d tokens exceeded", limit),
		"status": http.StatusTooManyRequests,
	}

	_ = json.NewEncoder(w).Encode(response)
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
