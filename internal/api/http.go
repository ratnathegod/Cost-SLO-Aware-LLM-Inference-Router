package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/config"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/providers"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/router"
)

type InferRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	MaxTok  int    `json:"max_tokens,omitempty"`
	Stream  bool   `json:"stream,omitempty"`
	Policy  string `json:"policy,omitempty"` // e.g., cheapest|fastest_p95|slo_burn_aware|canary
}

type InferResponse struct {
	Provider string  `json:"provider"`
	Text     string  `json:"text"`
	CostUSD  float64 `json:"cost_usd"`
	LatencyMs int64  `json:"latency_ms"`
}

func HandleInfer(cfg config.Config) http.HandlerFunc {
	// Build providers with resilience once per handler creation
	provs := make([]*providers.ResilientProvider, 0, 2)
	if cfg.OpenAIKey != "" {
		op := providers.NewOpenAIProvider(cfg.OpenAIKey)
		provs = append(provs, providers.WithResilience(op, providers.ResilienceOptions{
			Timeout:     30 * 1_000_000_000, // 30s
			MaxRetries:  2,
			BaseBackoff: 200 * 1_000_000,   // 200ms
			MaxBackoff:  2 * 1_000_000_000, // 2s
			JitterFrac:  0.2,
			CBWindowSize: 20,
			CBCooldown:  30 * 1_000_000_000, // 30s
		}))
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" {
		if br, err := providers.NewBedrockProvider(cfg.BedrockModelID, cfg.BedrockRegion); err == nil {
		provs = append(provs, providers.WithResilience(br, providers.ResilienceOptions{
			Timeout:     30 * 1_000_000_000,
			MaxRetries:  2,
			BaseBackoff: 200 * 1_000_000,
			MaxBackoff:  2 * 1_000_000_000,
			JitterFrac:  0.2,
			CBWindowSize: 20,
			CBCooldown:  30 * 1_000_000_000,
		}))
		} else {
			log.Warn().Err(err).Msg("bedrock init failed")
		}
	}
	eng := router.NewEngine(provs)
	return func(w http.ResponseWriter, r *http.Request) {
		var req InferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Policy == "" {
			req.Policy = cfg.DefaultPolicy
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
		// Call provider
		pReq := providers.CompletionRequest{Model: req.Model, Prompt: req.Prompt, MaxTok: req.MaxTok, Stream: req.Stream}
		out, cost, latency, err := chosen.Complete(r.Context(), pReq)
		failed := err != nil
		eng.RecordResult(chosen.Name(), failed)
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