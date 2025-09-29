package config

import (
	"os"
)

type Config struct {
	Port          string
	DefaultPolicy string
	OpenAIKey     string
	BedrockRegion string
	OtelEndpoint  string
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	return Config{
		Port:          getenv("PORT", "8080"),
		DefaultPolicy: getenv("ROUTER_POLICY", "cheapest"),
		OpenAIKey:     getenv("OPENAI_API_KEY", ""),
		BedrockRegion: getenv("BEDROCK_REGION", "us-east-1"),
		OtelEndpoint:  getenv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
	}
}