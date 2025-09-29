package config

import (
	"os"
)

type Config struct {
	Port           string
	DefaultPolicy  string
	OpenAIKey      string
	OpenAIModel    string
	BedrockRegion  string
	BedrockModelID string
	OtelEndpoint   string
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	return Config{
		Port:           getenv("PORT", "8080"),
		DefaultPolicy:  getenv("ROUTER_POLICY", "cheapest"),
		OpenAIKey:      getenv("OPENAI_API_KEY", ""),
		OpenAIModel:    getenv("OPENAI_MODEL", "gpt-4o"),
		BedrockRegion:  getenv("BEDROCK_REGION", "us-east-1"),
		BedrockModelID: getenv("BEDROCK_MODEL_ID", "anthropic.claude-3-haiku"),
		OtelEndpoint:   getenv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
	}
}
