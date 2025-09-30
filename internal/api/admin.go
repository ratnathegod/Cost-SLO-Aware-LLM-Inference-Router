package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/auth"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/router"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/telemetry"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/usage"
	"github.com/rs/zerolog/log"
)

var (
	startTime = time.Now()
	buildInfo = struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"build_date"`
	}{
		Version: "dev",
		Commit:  "unknown",
		Date:    time.Now().Format(time.RFC3339),
	}
)

// AdminStatusResponse represents the admin status endpoint response
type AdminStatusResponse struct {
	BuildInfo struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"build_date"`
	} `json:"build_info"`
	Uptime        string `json:"uptime"`
	DefaultPolicy string `json:"default_policy"`
	Providers     []struct {
		Name         string  `json:"name"`
		CBState      float64 `json:"cb_state"`
		ErrorRate1m  float64 `json:"error_rate_1m"`
		ErrorRate5m  float64 `json:"error_rate_5m"`
		ErrorRate1h  float64 `json:"error_rate_1h"`
		P95LatencyMs float64 `json:"p95_latency_ms"`
		CostPer1k    float64 `json:"cost_per_1k_tokens_usd"`
	} `json:"providers"`
	TotalRequests int64 `json:"total_requests"`
	BurnRates     struct {
		Rate1m float64 `json:"burn_rate_1m"`
		Rate5m float64 `json:"burn_rate_5m"`
		Rate1h float64 `json:"burn_rate_1h"`
	} `json:"burn_rates"`
	CanaryStagePercent float64 `json:"canary_stage_percent"`
}

// CanaryStatusResponse represents the canary status endpoint response
type CanaryStatusResponse struct {
	Percent           float64   `json:"percent"`
	Stage             int       `json:"stage_index"`
	CandidateProvider string    `json:"candidate_provider"`
	WindowSize        int       `json:"window_size"`
	LastTransition    time.Time `json:"last_transition"`
	LastReason        string    `json:"last_reason"`
}

// HandleAdminStatus returns the comprehensive status information
func HandleAdminStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ps := router.GetProviders()
		e := router.GetEngine()

		resp := AdminStatusResponse{
			BuildInfo:     buildInfo,
			Uptime:        time.Since(startTime).String(),
			DefaultPolicy: router.GetDefaultPolicy(),
		}

		var totalReqs int64
		var maxBurn1m, maxBurn5m, maxBurn1h float64

		for _, p := range ps {
			er1m := p.Stats().ErrorRateSince(1 * 60 * 1e9)
			er5m := p.Stats().ErrorRateSince(5 * 60 * 1e9)
			er1h := p.Stats().ErrorRateSince(60 * 60 * 1e9)

			// Calculate burn rates (assuming 1% SLO target)
			burn1m := er1m / 0.01
			burn5m := er5m / 0.01
			burn1h := er1h / 0.01

			if burn1m > maxBurn1m {
				maxBurn1m = burn1m
			}
			if burn5m > maxBurn5m {
				maxBurn5m = burn5m
			}
			if burn1h > maxBurn1h {
				maxBurn1h = burn1h
			}

			resp.Providers = append(resp.Providers, struct {
				Name         string  `json:"name"`
				CBState      float64 `json:"cb_state"`
				ErrorRate1m  float64 `json:"error_rate_1m"`
				ErrorRate5m  float64 `json:"error_rate_5m"`
				ErrorRate1h  float64 `json:"error_rate_1h"`
				P95LatencyMs float64 `json:"p95_latency_ms"`
				CostPer1k    float64 `json:"cost_per_1k_tokens_usd"`
			}{
				Name:         p.Name(),
				CBState:      p.CBStateValue(),
				ErrorRate1m:  er1m,
				ErrorRate5m:  er5m,
				ErrorRate1h:  er1h,
				P95LatencyMs: float64(p.Stats().P95LatencyMs()),
				CostPer1k:    p.CostPer1kTokensUSD(""),
			})
		}

		resp.TotalRequests = totalReqs
		resp.BurnRates.Rate1m = maxBurn1m
		resp.BurnRates.Rate5m = maxBurn5m
		resp.BurnRates.Rate1h = maxBurn1h

		if e != nil {
			resp.CanaryStagePercent = e.CanaryPercent()
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error().Err(err).Msg("failed to encode admin status response")
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}
}

// HandleCanaryStatus returns detailed canary information
func HandleCanaryStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := router.GetEngine()
		if e == nil {
			http.Error(w, "engine not ready", http.StatusServiceUnavailable)
			return
		}

		resp := CanaryStatusResponse{
			Percent:           e.CanaryPercent(),
			Stage:             e.CanaryStageIndex(),
			CandidateProvider: e.CanaryCandidateProvider(),
			WindowSize:        e.CanaryWindowSize(),
			LastTransition:    e.CanaryLastTransition(),
			LastReason:        e.CanaryLastReason(),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error().Err(err).Msg("failed to encode canary status response")
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}
}

// HandleCanaryAdvance advances canary to next stage with validation
func HandleCanaryAdvance() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Force bool `json:"force"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		e := router.GetEngine()
		if e == nil {
			http.Error(w, "engine not ready", http.StatusServiceUnavailable)
			return
		}

		// Guardrails check
		if !body.Force {
			// Check if canary provider has acceptable burn rate
			candProvider := e.CanaryCandidateProvider()
			if candProvider != "" {
				ps := router.GetProviders()
				for _, p := range ps {
					if p.Name() == candProvider {
						burnRate := p.Stats().ErrorRate() / 0.01 // 1% SLO
						if burnRate > 2.0 {
							http.Error(w, fmt.Sprintf("canary burn rate too high: %.2f", burnRate), http.StatusPreconditionFailed)
							return
						}
						break
					}
				}
			}
		}

		oldStage := e.CanaryStageIndex()
		oldPercent := e.CanaryPercent()

		e.CanaryAdvance()

		newStage := e.CanaryStageIndex()
		newPercent := e.CanaryPercent()

		// Emit structured canary event log
		log.Info().
			Str("event", "canary_advance").
			Str("provider", e.CanaryCandidateProvider()).
			Int("old_stage", oldStage).
			Int("new_stage", newStage).
			Float64("old_percent", oldPercent).
			Float64("new_percent", newPercent).
			Bool("forced", body.Force).
			Msg("canary stage advanced")

		telemetry.AdminActionsTotal.WithLabelValues("canary_advance").Inc()
		telemetry.CanaryStage.Set(e.CanaryPercent())

		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleCanaryRollback rolls back canary to stage 0
func HandleCanaryRollback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := router.GetEngine()
		if e == nil {
			http.Error(w, "engine not ready", http.StatusServiceUnavailable)
			return
		}

		oldStage := e.CanaryStageIndex()
		oldPercent := e.CanaryPercent()

		e.CanaryRollback()

		// Emit structured canary event log
		log.Info().
			Str("event", "canary_rollback").
			Str("provider", e.CanaryCandidateProvider()).
			Int("old_stage", oldStage).
			Int("new_stage", 0).
			Float64("old_percent", oldPercent).
			Float64("new_percent", e.CanaryPercent()).
			Str("reason", "manual_rollback").
			Msg("canary rolled back")

		telemetry.AdminActionsTotal.WithLabelValues("canary_rollback").Inc()
		telemetry.CanaryStage.Set(e.CanaryPercent())

		w.WriteHeader(http.StatusNoContent)
	}
}

// HandlePolicyUpdate updates the default policy
func HandlePolicyUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DefaultPolicy string `json:"default_policy"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate policy
		validPolicies := map[string]bool{
			"cheapest":       true,
			"fastest_p95":    true,
			"slo_burn_aware": true,
			"canary":         true,
		}

		if !validPolicies[body.DefaultPolicy] {
			http.Error(w, "invalid policy", http.StatusBadRequest)
			return
		}

		oldPolicy := router.GetDefaultPolicy()
		router.SetDefaultPolicy(body.DefaultPolicy)

		log.Info().
			Str("event", "policy_update").
			Str("old_policy", oldPolicy).
			Str("new_policy", body.DefaultPolicy).
			Msg("default policy updated")

		telemetry.AdminActionsTotal.WithLabelValues("set_policy").Inc()

		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleProvidersReload is a placeholder for future provider hot-reload
func HandleProvidersReload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info().
			Str("event", "providers_reload").
			Str("status", "not_implemented").
			Msg("providers reload requested")

		telemetry.AdminActionsTotal.WithLabelValues("providers_reload").Inc()
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}
}

// CreateTenantRequest represents the request to create a new tenant
type CreateTenantRequest struct {
	Name            string `json:"name"`
	Plan            string `json:"plan"`
	RPSLimit        int    `json:"rps_limit"`
	DailyTokenLimit int64  `json:"daily_token_limit"`
	Enabled         bool   `json:"enabled"`
}

// CreateTenantResponse represents the response when creating a tenant
type CreateTenantResponse struct {
	TenantID string `json:"tenant_id"`
	APIKey   string `json:"api_key"`
	*auth.Tenant
}

// UpdateTenantRequest represents the request to update a tenant
type UpdateTenantRequest struct {
	Name            *string `json:"name,omitempty"`
	Plan            *string `json:"plan,omitempty"`
	RPSLimit        *int    `json:"rps_limit,omitempty"`
	DailyTokenLimit *int64  `json:"daily_token_limit,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
	RotateKey       bool    `json:"rotate_key,omitempty"`
}

// TenantHandlers provides tenant management functionality
type TenantHandlers struct {
	keyManager *auth.APIKeyManager
	usageStore *usage.Store
}

func NewTenantHandlers(keyManager *auth.APIKeyManager, usageStore *usage.Store) *TenantHandlers {
	return &TenantHandlers{
		keyManager: keyManager,
		usageStore: usageStore,
	}
}

// HandleCreateTenant creates a new tenant
func (th *TenantHandlers) HandleCreateTenant() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if req.Name == "" || req.Plan == "" {
			http.Error(w, "name and plan are required", http.StatusBadRequest)
			return
		}

		// Set defaults
		if req.RPSLimit <= 0 {
			switch req.Plan {
			case "free":
				req.RPSLimit = 10
			case "pro":
				req.RPSLimit = 100
			case "enterprise":
				req.RPSLimit = 1000
			default:
				req.RPSLimit = 10
			}
		}

		if req.DailyTokenLimit <= 0 {
			switch req.Plan {
			case "free":
				req.DailyTokenLimit = 10000
			case "pro":
				req.DailyTokenLimit = 1000000
			case "enterprise":
				req.DailyTokenLimit = 10000000
			default:
				req.DailyTokenLimit = 10000
			}
		}

		tenant, apiKey, err := th.keyManager.CreateTenant(
			r.Context(),
			req.Name,
			req.Plan,
			req.RPSLimit,
			req.DailyTokenLimit,
		)
		if err != nil {
			log.Error().Err(err).Msg("failed to create tenant")
			http.Error(w, "failed to create tenant", http.StatusInternalServerError)
			return
		}

		log.Info().
			Str("event", "tenant_create").
			Str("tenant_id", tenant.TenantID).
			Str("name", tenant.Name).
			Str("plan", tenant.Plan).
			Msg("tenant created")

		telemetry.AdminActionsTotal.WithLabelValues("tenant_create").Inc()

		response := CreateTenantResponse{
			TenantID: tenant.TenantID,
			APIKey:   apiKey,
			Tenant:   tenant,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error().Err(err).Msg("failed to encode create tenant response")
		}
	}
}

// HandleGetTenantUsage returns usage data for a specific tenant
func (th *TenantHandlers) HandleGetTenantUsage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenant_id")
		if tenantID == "" {
			http.Error(w, "tenant_id required", http.StatusBadRequest)
			return
		}

		// Parse date parameters
		since := r.URL.Query().Get("since")
		until := r.URL.Query().Get("until")

		var sinceTime, untilTime time.Time
		var err error

		if since != "" {
			sinceTime, err = time.Parse("2006-01-02", since)
			if err != nil {
				http.Error(w, "invalid since date format (YYYY-MM-DD)", http.StatusBadRequest)
				return
			}
		} else {
			sinceTime = time.Now().AddDate(0, 0, -7) // Default to last 7 days
		}

		if until != "" {
			untilTime, err = time.Parse("2006-01-02", until)
			if err != nil {
				http.Error(w, "invalid until date format (YYYY-MM-DD)", http.StatusBadRequest)
				return
			}
		} else {
			untilTime = time.Now()
		}

		aggregates, err := th.usageStore.GetDailyUsage(r.Context(), tenantID, sinceTime, untilTime)
		if err != nil {
			log.Error().Err(err).Str("tenant_id", tenantID).Msg("failed to get tenant usage")
			http.Error(w, "failed to retrieve usage data", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(aggregates); err != nil {
			log.Error().Err(err).Msg("failed to encode tenant usage response")
		}
	}
}
