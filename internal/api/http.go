package api

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/auth"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/config"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/providers"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/router"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/telemetry"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/usage"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type InferRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	MaxTok int    `json:"max_tokens,omitempty"`
	Stream bool   `json:"stream,omitempty"`
	Policy string `json:"policy,omitempty"` // e.g., cheapest|fastest_p95|slo_burn_aware|canary
}

type InferResponse struct {
	Provider  string  `json:"provider"`
	Text      string  `json:"text"`
	CostUSD   float64 `json:"cost_usd"`
	LatencyMs int64   `json:"latency_ms"`
}

func HandleInfer(cfg config.Config) http.HandlerFunc {
	// Build providers with resilience once per handler creation
	provs := make([]*providers.ResilientProvider, 0, 2)
	if cfg.OpenAIKey != "" {
		op := providers.NewOpenAIProvider(cfg.OpenAIKey)
		provs = append(provs, providers.WithResilience(op, providers.ResilienceOptions{
			Timeout:      30 * 1_000_000_000, // 30s
			MaxRetries:   2,
			BaseBackoff:  200 * 1_000_000,   // 200ms
			MaxBackoff:   2 * 1_000_000_000, // 2s
			JitterFrac:   0.2,
			CBWindowSize: 20,
			CBCooldown:   30 * 1_000_000_000, // 30s
		}))
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" {
		if br, err := providers.NewBedrockProvider(cfg.BedrockModelID, cfg.BedrockRegion); err == nil {
			provs = append(provs, providers.WithResilience(br, providers.ResilienceOptions{
				Timeout:      30 * 1_000_000_000,
				MaxRetries:   2,
				BaseBackoff:  200 * 1_000_000,
				MaxBackoff:   2 * 1_000_000_000,
				JitterFrac:   0.2,
				CBWindowSize: 20,
				CBCooldown:   30 * 1_000_000_000,
			}))
		} else {
			log.Warn().Err(err).Msg("bedrock init failed")
		}
	}
	// Optional Mock provider for local/dev testing
	if cfg.EnableMockProvider {
		mp := providers.NewMockProvider(float64(cfg.MockMeanLatencyMs), float64(cfg.MockP95LatencyMs), cfg.MockErrorRate, cfg.MockCostPer1kUSD)
		provs = append(provs, providers.WithResilience(mp, providers.ResilienceOptions{
			Timeout:      30 * 1_000_000_000,
			MaxRetries:   1,
			BaseBackoff:  100 * 1_000_000,
			MaxBackoff:   1 * 1_000_000_000,
			JitterFrac:   0.2,
			CBWindowSize: 20,
			CBCooldown:   10 * 1_000_000_000,
		}))
	}
	// publish providers to registry for readiness checks
	router.SetProviders(provs)
	eng := router.NewEngine(provs)
	eng.ConfigureCanary(cfg.CanaryStages, cfg.CanaryWindow, cfg.CanaryBurnMultiplier)
	router.SetEngine(eng)
	if cfg.DefaultPolicy != "" {
		router.SetDefaultPolicy(cfg.DefaultPolicy)
	}
	// export initial canary stage metric
	telemetry.CanaryStage.Set(eng.CanaryPercent())
	return func(w http.ResponseWriter, r *http.Request) {
		var req InferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Policy == "" {
			// use runtime default policy which admin can update
			if p := router.GetDefaultPolicy(); p != "" {
				req.Policy = p
			} else {
				req.Policy = cfg.DefaultPolicy
			}
		}
		if req.Model == "" {
			req.Model = cfg.OpenAIModel
		}

		// Choose provider via policy engine
		chosen := eng.Choose(req.Policy, req.Model)
		if chosen == nil {
			http.Error(w, "no providers available", http.StatusServiceUnavailable)
			return
		}
		// Start span
		tracer := otel.Tracer("llm-router")
		ctx, span := tracer.Start(r.Context(), "infer")
		span.SetAttributes(
			attribute.String("policy", req.Policy),
			attribute.String("model", req.Model),
			attribute.String("provider", chosen.Name()),
		)
		defer span.End()
		// Call provider
		pReq := providers.CompletionRequest{Model: req.Model, Prompt: req.Prompt, MaxTok: req.MaxTok, Stream: req.Stream}
		out, cost, latency, err := chosen.Complete(ctx, pReq)
		failed := err != nil
		eng.RecordResult(chosen.Name(), failed)
		telemetry.CanaryStage.Set(eng.CanaryPercent())
		// Metrics
		code := "200"
		reason := ""
		if err != nil {
			code = "502"
			reason = "provider_error"
		}
		telemetry.RequestsTotal.WithLabelValues(chosen.Name(), req.Policy, code).Inc()
		telemetry.LatencyMs.WithLabelValues(chosen.Name(), req.Policy).Observe(float64(latency))
		if !failed {
			telemetry.CostUSDTotal.WithLabelValues(chosen.Name()).Add(cost)
		} else {
			telemetry.ErrorsTotal.WithLabelValues(chosen.Name(), reason).Inc()
		}
		// Span attrs
		span.SetAttributes(
			attribute.Float64("cost_usd", cost),
			attribute.Int64("latency_ms", latency),
			attribute.Bool("success", !failed),
		)
		// Export CB state gauge
		if rp, ok := any(chosen).(*providers.ResilientProvider); ok {
			telemetry.CBState.WithLabelValues(chosen.Name()).Set(rp.CBStateValue())
		}
		// Burn rate windows
		if rp, ok := any(chosen).(*providers.ResilientProvider); ok {
			er1m := rp.Stats().ErrorRateSince(1 * 60 * 1e9)
			er5m := rp.Stats().ErrorRateSince(5 * 60 * 1e9)
			er1h := rp.Stats().ErrorRateSince(60 * 60 * 1e9)
			// Assume SLO target 1%
			burn1m := er1m / 0.01
			burn5m := er5m / 0.01
			burn1h := er1h / 0.01
			telemetry.BurnRate.WithLabelValues("1m").Set(burn1m)
			telemetry.BurnRate.WithLabelValues("5m").Set(burn5m)
			telemetry.BurnRate.WithLabelValues("1h").Set(burn1h)
			if burn1m > 1.0 || burn5m > 1.0 || burn1h > 1.0 {
				log.Warn().Str("provider", chosen.Name()).Float64("burn_1m", burn1m).Float64("burn_5m", burn5m).Float64("burn_1h", burn1h).Msg("error budget burning")
			}
		}
		if err != nil {
			log.Error().Err(err).Str("provider", chosen.Name()).Msg("completion failed")
			http.Error(w, "provider error", http.StatusBadGateway)
			return
		}
		resp := InferResponse{Provider: chosen.Name(), Text: out.Text, CostUSD: cost, LatencyMs: latency}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error().Err(err).Msg("encode resp")
		}
	}
}

// HandleInferWithUsageTracking is the multi-tenant version with usage tracking
func HandleInferWithUsageTracking(cfg config.Config, usageStore *usage.Store) http.HandlerFunc {
	// Reuse the same provider setup logic
	provs := make([]*providers.ResilientProvider, 0, 2)
	if cfg.OpenAIKey != "" {
		op := providers.NewOpenAIProvider(cfg.OpenAIKey)
		provs = append(provs, providers.WithResilience(op, providers.ResilienceOptions{
			Timeout:      30 * 1_000_000_000,
			MaxRetries:   2,
			BaseBackoff:  200 * 1_000_000,
			MaxBackoff:   2 * 1_000_000_000,
			JitterFrac:   0.2,
			CBWindowSize: 20,
			CBCooldown:   30 * 1_000_000_000,
		}))
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" {
		if br, err := providers.NewBedrockProvider(cfg.BedrockModelID, cfg.BedrockRegion); err == nil {
			provs = append(provs, providers.WithResilience(br, providers.ResilienceOptions{
				Timeout:      30 * 1_000_000_000,
				MaxRetries:   2,
				BaseBackoff:  200 * 1_000_000,
				MaxBackoff:   2 * 1_000_000_000,
				JitterFrac:   0.2,
				CBWindowSize: 20,
				CBCooldown:   30 * 1_000_000_000,
			}))
		} else {
			log.Warn().Err(err).Msg("bedrock init failed")
		}
	}
	if cfg.EnableMockProvider {
		mp := providers.NewMockProvider(float64(cfg.MockMeanLatencyMs), float64(cfg.MockP95LatencyMs), cfg.MockErrorRate, cfg.MockCostPer1kUSD)
		provs = append(provs, providers.WithResilience(mp, providers.ResilienceOptions{
			Timeout:      30 * 1_000_000_000,
			MaxRetries:   1,
			BaseBackoff:  100 * 1_000_000,
			MaxBackoff:   1 * 1_000_000_000,
			JitterFrac:   0.2,
			CBWindowSize: 20,
			CBCooldown:   10 * 1_000_000_000,
		}))
	}

	router.SetProviders(provs)
	eng := router.NewEngine(provs)
	eng.ConfigureCanary(cfg.CanaryStages, cfg.CanaryWindow, cfg.CanaryBurnMultiplier)
	router.SetEngine(eng)
	if cfg.DefaultPolicy != "" {
		router.SetDefaultPolicy(cfg.DefaultPolicy)
	}
	telemetry.CanaryStage.Set(eng.CanaryPercent())

	estimator := usage.NewTokenEstimator()

	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Get tenant from context (added by auth middleware)
		tenant, ok := auth.GetTenantFromContext(r.Context())
		if !ok {
			http.Error(w, "no tenant context", http.StatusInternalServerError)
			return
		}

		var req InferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Policy == "" {
			if p := router.GetDefaultPolicy(); p != "" {
				req.Policy = p
			} else {
				req.Policy = cfg.DefaultPolicy
			}
		}
		if req.Model == "" {
			req.Model = cfg.OpenAIModel
		}

		// Estimate tokens for usage tracking
		promptTokens := estimator.EstimatePromptTokens(req.Prompt, req.Model)

		chosen := eng.Choose(req.Policy, req.Model)
		if chosen == nil {
			http.Error(w, "no providers available", http.StatusServiceUnavailable)
			return
		}

		tracer := otel.Tracer("llm-router")
		ctx, span := tracer.Start(r.Context(), "infer")
		span.SetAttributes(
			attribute.String("policy", req.Policy),
			attribute.String("model", req.Model),
			attribute.String("provider", chosen.Name()),
			attribute.String("tenant_id", tenant.TenantID),
		)
		defer span.End()

		pReq := providers.CompletionRequest{Model: req.Model, Prompt: req.Prompt, MaxTok: req.MaxTok, Stream: req.Stream}
		out, cost, latency, err := chosen.Complete(ctx, pReq)
		failed := err != nil
		eng.RecordResult(chosen.Name(), failed)
		telemetry.CanaryStage.Set(eng.CanaryPercent())

		// Estimate completion tokens from actual response
		var completionTokens int64
		if out.Text != "" {
			completionTokens = estimator.EstimateTokens(out.Text, req.Model)
		} else {
			completionTokens = estimator.EstimateCompletionTokens(req.Prompt, req.Model)
		}

		// Record usage
		usageRecord := usage.UsageRecord{
			TenantID:            tenant.TenantID,
			Timestamp:           startTime,
			RequestID:           r.Header.Get("X-Request-ID"),
			Provider:            chosen.Name(),
			Model:               req.Model,
			EstPromptTokens:     promptTokens,
			EstCompletionTokens: completionTokens,
			CostUSD:             cost,
			LatencyMs:           latency,
			Status:              "ok",
			IdempotencyKey:      r.Header.Get("Idempotency-Key"),
		}

		if failed {
			usageRecord.Status = "error"
		}

		if usageStore != nil {
			if err := usageStore.RecordUsage(r.Context(), usageRecord); err != nil {
				log.Error().Err(err).Msg("failed to record usage")
			}
		}

		// Metrics (same as original handler)
		code := "200"
		reason := ""
		if err != nil {
			code = "502"
			reason = "provider_error"
		}
		telemetry.RequestsTotal.WithLabelValues(chosen.Name(), req.Policy, code).Inc()
		telemetry.LatencyMs.WithLabelValues(chosen.Name(), req.Policy).Observe(float64(latency))
		if !failed {
			telemetry.CostUSDTotal.WithLabelValues(chosen.Name()).Add(cost)
		} else {
			telemetry.ErrorsTotal.WithLabelValues(chosen.Name(), reason).Inc()
		}

		span.SetAttributes(
			attribute.Float64("cost_usd", cost),
			attribute.Int64("latency_ms", latency),
			attribute.Bool("success", !failed),
			attribute.Int64("prompt_tokens", promptTokens),
			attribute.Int64("completion_tokens", completionTokens),
		)

		if rp, ok := any(chosen).(*providers.ResilientProvider); ok {
			telemetry.CBState.WithLabelValues(chosen.Name()).Set(rp.CBStateValue())
		}

		if rp, ok := any(chosen).(*providers.ResilientProvider); ok {
			er1m := rp.Stats().ErrorRateSince(1 * 60 * 1e9)
			er5m := rp.Stats().ErrorRateSince(5 * 60 * 1e9)
			er1h := rp.Stats().ErrorRateSince(60 * 60 * 1e9)
			burn1m := er1m / 0.01
			burn5m := er5m / 0.01
			burn1h := er1h / 0.01
			telemetry.BurnRate.WithLabelValues("1m").Set(burn1m)
			telemetry.BurnRate.WithLabelValues("5m").Set(burn5m)
			telemetry.BurnRate.WithLabelValues("1h").Set(burn1h)
			if burn1m > 1.0 || burn5m > 1.0 || burn1h > 1.0 {
				log.Warn().Str("provider", chosen.Name()).Float64("burn_1m", burn1m).Float64("burn_5m", burn5m).Float64("burn_1h", burn1h).Msg("error budget burning")
			}
		}

		if err != nil {
			log.Error().Err(err).Str("provider", chosen.Name()).Str("tenant", tenant.TenantID).Msg("completion failed")
			http.Error(w, "provider error", http.StatusBadGateway)
			return
		}

		resp := InferResponse{Provider: chosen.Name(), Text: out.Text, CostUSD: cost, LatencyMs: latency}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error().Err(err).Msg("encode resp")
		}
	}
}
