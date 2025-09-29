package api

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/config"
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
	return func(w http.ResponseWriter, r *http.Request) {
		var req InferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Policy == "" {
			req.Policy = cfg.DefaultPolicy
		}

		// Stub response for now; Copilot will replace with real routing/providers.
		resp := InferResponse{
			Provider: "mock",
			Text:     "(stub) hello from llm-router",
			CostUSD:  0.0001,
			LatencyMs: 12,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error().Err(err).Msg("encode resp")
		}
	}
}