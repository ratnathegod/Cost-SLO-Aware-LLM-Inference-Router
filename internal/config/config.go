package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
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

	AdminToken string

	// Multi-tenant configuration
	DDBTenantsTable     string
	DDBUsageTable       string
	TenantsJSONPath     string
	EnableUsageTracking bool

	CanaryStages         []float64
	CanaryWindow         int
	CanaryBurnMultiplier float64
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var dotenvOnce sync.Once

func loadDotEnv() {
	dotenvOnce.Do(func() {
		f, err := os.Open(".env")
		if err != nil {
			return
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			kv := strings.SplitN(line, "=", 2)
			if len(kv) != 2 {
				continue
			}
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	})
}

// ValidateConfig performs startup validation and warnings
func ValidateConfig(cfg Config) []string {
	var warnings []string

	// Check if canary policy has sufficient providers
	if cfg.DefaultPolicy == "canary" {
		providerCount := 0
		if cfg.OpenAIKey != "" {
			providerCount++
		}
		if os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_PROFILE") != "" {
			providerCount++
		}
		if cfg.EnableMockProvider {
			providerCount++
		}
		if providerCount < 2 {
			warnings = append(warnings, "canary policy requires at least 2 providers, falling back to cheapest")
		}
	}

	return warnings
}

// MaskSecrets returns a copy of config with secrets masked for logging
func (c Config) MaskSecrets() Config {
	masked := c
	if masked.OpenAIKey != "" {
		masked.OpenAIKey = "***masked***"
	}
	if masked.AdminToken != "" {
		masked.AdminToken = "***masked***"
	}
	return masked
}

// IsValidPolicy checks if a policy string is valid
func IsValidPolicy(policy string) bool {
	validPolicies := map[string]bool{
		"cheapest":       true,
		"fastest_p95":    true,
		"slo_burn_aware": true,
		"canary":         true,
	}
	return validPolicies[policy]
}

func Load() Config {
	loadDotEnv()
	cfg := Config{
		Port:               getenv("PORT", "8080"),
		DefaultPolicy:      getenv("ROUTER_POLICY", "cheapest"),
		OpenAIKey:          getenv("OPENAI_API_KEY", ""),
		OpenAIModel:        getenv("OPENAI_MODEL", "gpt-4o"),
		BedrockRegion:      getenv("BEDROCK_REGION", "us-east-1"),
		BedrockModelID:     getenv("BEDROCK_MODEL_ID", "anthropic.claude-3-haiku"),
		OtelEndpoint:       getenv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		EnableMockProvider: getenv("ENABLE_MOCK_PROVIDER", "") != "" && getenv("ENABLE_MOCK_PROVIDER", "") != "0",
		AdminToken:         getenv("ADMIN_TOKEN", ""),
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
	// Canary config with defaults
	cfg.CanaryStages = []float64{1, 5, 25}
	if s := getenv("CANARY_STAGES", ""); s != "" {
		parts := strings.Split(s, ",")
		var st []float64
		for _, p := range parts {
			if f, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil && f >= 0 {
				st = append(st, f)
			}
		}
		if len(st) >= 1 {
			cfg.CanaryStages = st
		}
	}
	cfg.CanaryWindow = 200
	if v, err := strconv.Atoi(getenv("CANARY_WINDOW", "")); err == nil && v > 0 {
		cfg.CanaryWindow = v
	}
	cfg.CanaryBurnMultiplier = 2.0
	if v, err := strconv.ParseFloat(getenv("CANARY_BURN_MULTIPLIER", ""), 64); err == nil && v > 0 {
		cfg.CanaryBurnMultiplier = v
	}

	// Multi-tenant config
	cfg.DDBTenantsTable = getenv("DDB_TENANTS_TABLE", "")
	cfg.DDBUsageTable = getenv("DDB_USAGE_TABLE", "")
	cfg.TenantsJSONPath = getenv("TENANTS_JSON", "")

	// Enable usage tracking if DDB tables are set or if explicitly enabled (for JSON fallback)
	cfg.EnableUsageTracking = (cfg.DDBTenantsTable != "" && cfg.DDBUsageTable != "") ||
		(getenv("ENABLE_USAGE_TRACKING", "") != "" && getenv("ENABLE_USAGE_TRACKING", "") != "0") ||
		cfg.TenantsJSONPath != ""

	return cfg
}
