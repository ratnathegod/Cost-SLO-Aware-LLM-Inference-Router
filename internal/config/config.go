package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port           string
	DefaultPolicy  string
	OpenAIKey      string
	OpenAIModel    string
	BedrockRegion  string
	BedrockModelID string
	OtelEndpoint   string

	EnableMockProvider bool
	MockMeanLatencyMs  int
	MockP95LatencyMs   int
	MockErrorRate      float64
	MockCostPer1kUSD   float64
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	cfg := Config{
		Port:               getenv("PORT", "8080"),
		DefaultPolicy:      getenv("ROUTER_POLICY", "cheapest"),
		OpenAIKey:          getenv("OPENAI_API_KEY", ""),
		OpenAIModel:        getenv("OPENAI_MODEL", "gpt-4o"),
		BedrockRegion:      getenv("BEDROCK_REGION", "us-east-1"),
		BedrockModelID:     getenv("BEDROCK_MODEL_ID", "anthropic.claude-3-haiku"),
		OtelEndpoint:       getenv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		EnableMockProvider: getenv("ENABLE_MOCK_PROVIDER", "") != "" && getenv("ENABLE_MOCK_PROVIDER", "") != "0",
	}
	// defaults
	cfg.MockMeanLatencyMs = 40
	cfg.MockP95LatencyMs = 120
	cfg.MockErrorRate = 0.01
	cfg.MockCostPer1kUSD = 0.002

	if v, err := strconv.Atoi(getenv("MOCK_MEAN_LATENCY_MS", "")); err == nil && v > 0 {
		cfg.MockMeanLatencyMs = v
	}
	if v, err := strconv.Atoi(getenv("MOCK_P95_LATENCY_MS", "")); err == nil && v > 0 {
		cfg.MockP95LatencyMs = v
	}
	if v, err := strconv.ParseFloat(getenv("MOCK_ERROR_RATE", ""), 64); err == nil && v >= 0 && v <= 1 {
		cfg.MockErrorRate = v
	}
	if v, err := strconv.ParseFloat(getenv("MOCK_COST_PER_1K_TOKENS_USD", ""), 64); err == nil && v >= 0 {
		cfg.MockCostPer1kUSD = v
	}
	return cfg
}
