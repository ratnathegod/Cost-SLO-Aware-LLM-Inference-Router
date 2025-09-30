package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/auth"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/usage"
	"github.com/rs/zerolog/log"
)

// UsageHandlers provides usage-related HTTP handlers
type UsageHandlers struct {
	store *usage.Store
}

func NewUsageHandlers(store *usage.Store) *UsageHandlers {
	return &UsageHandlers{store: store}
}

// DailyUsageResponse represents daily usage data
type DailyUsageResponse struct {
	Date      string  `json:"date"`
	Requests  int64   `json:"requests"`
	Successes int64   `json:"successes"`
	Failures  int64   `json:"failures"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CostUSD   float64 `json:"cost_usd"`
}

// RecentUsageResponse represents recent usage data
type RecentUsageResponse struct {
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Status    string    `json:"status"`
	CostUSD   float64   `json:"cost_usd"`
	LatencyMs int64     `json:"latency_ms"`
	TokensIn  int64     `json:"tokens_in"`
	TokensOut int64     `json:"tokens_out"`
}

// HandleDailyUsage returns daily usage aggregates
func (h *UsageHandlers) HandleDailyUsage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := auth.GetTenantFromContext(r.Context())
		if !ok {
			h.writeError(w, r, http.StatusUnauthorized, "No tenant context")
			return
		}

		// Parse query parameters
		daysStr := r.URL.Query().Get("days")
		days := 7 // default
		if daysStr != "" {
			if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 30 {
				days = d
			}
		}

		// Calculate date range
		until := time.Now()
		since := until.AddDate(0, 0, -days+1) // Include today

		aggregates, err := h.store.GetDailyUsage(r.Context(), tenant.TenantID, since, until)
		if err != nil {
			log.Error().Err(err).Str("tenant", tenant.TenantID).Msg("failed to get daily usage")
			h.writeError(w, r, http.StatusInternalServerError, "Failed to retrieve usage data")
			return
		}

		// Convert to response format
		var response []DailyUsageResponse
		for _, agg := range aggregates {
			response = append(response, DailyUsageResponse{
				Date:      agg.Date,
				Requests:  agg.Requests,
				Successes: agg.Successes,
				Failures:  agg.Failures,
				TokensIn:  agg.TokensIn,
				TokensOut: agg.TokensOut,
				CostUSD:   agg.CostUSD,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error().Err(err).Msg("failed to encode daily usage response")
		}
	}
}

// HandleRecentUsage returns recent usage records
func (h *UsageHandlers) HandleRecentUsage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := auth.GetTenantFromContext(r.Context())
		if !ok {
			h.writeError(w, r, http.StatusUnauthorized, "No tenant context")
			return
		}

		// Parse limit parameter
		limitStr := r.URL.Query().Get("limit")
		limit := 100 // default
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
				limit = l
			}
		}

		records, err := h.store.GetRecentUsage(r.Context(), tenant.TenantID, limit)
		if err != nil {
			log.Error().Err(err).Str("tenant", tenant.TenantID).Msg("failed to get recent usage")
			h.writeError(w, r, http.StatusInternalServerError, "Failed to retrieve usage data")
			return
		}

		// Convert to response format
		var response []RecentUsageResponse
		for _, record := range records {
			response = append(response, RecentUsageResponse{
				Timestamp: record.Timestamp,
				RequestID: record.RequestID,
				Provider:  record.Provider,
				Model:     record.Model,
				Status:    record.Status,
				CostUSD:   record.CostUSD,
				LatencyMs: record.LatencyMs,
				TokensIn:  record.EstPromptTokens,
				TokensOut: record.EstCompletionTokens,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error().Err(err).Msg("failed to encode recent usage response")
		}
	}
}

func (h *UsageHandlers) writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "application/problem+json")
	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	w.WriteHeader(status)

	response := map[string]interface{}{
		"type":   "https://example.com/errors/usage_error",
		"title":  "Usage API Error",
		"detail": message,
		"status": status,
	}

	_ = json.NewEncoder(w).Encode(response)
}
