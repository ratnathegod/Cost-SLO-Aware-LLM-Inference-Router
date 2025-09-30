package config

import (
	"os"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name             string
		config           Config
		envVars          map[string]string
		expectedWarnings int
		expectedWarning  string
	}{
		{
			name: "canary policy with sufficient providers",
			config: Config{
				DefaultPolicy:      "canary",
				OpenAIKey:          "test-key",
				EnableMockProvider: true,
			},
			envVars:          map[string]string{},
			expectedWarnings: 0,
		},
		{
			name: "canary policy with insufficient providers",
			config: Config{
				DefaultPolicy: "canary",
				OpenAIKey:     "", // no OpenAI
				// no AWS creds, no mock provider
			},
			envVars:          map[string]string{},
			expectedWarnings: 1,
			expectedWarning:  "canary policy requires at least 2 providers",
		},
		{
			name: "canary policy with AWS creds",
			config: Config{
				DefaultPolicy: "canary",
				OpenAIKey:     "test-key",
			},
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID": "test-key-id",
			},
			expectedWarnings: 0,
		},
		{
			name: "non-canary policy",
			config: Config{
				DefaultPolicy: "cheapest",
				// no providers configured, but that's okay for non-canary
			},
			envVars:          map[string]string{},
			expectedWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			oldEnvVars := make(map[string]string)
			for k, v := range tt.envVars {
				oldEnvVars[k] = os.Getenv(k)
				os.Setenv(k, v)
			}

			// Clean up environment after test
			defer func() {
				for k, oldVal := range oldEnvVars {
					if oldVal == "" {
						os.Unsetenv(k)
					} else {
						os.Setenv(k, oldVal)
					}
				}
			}()

			warnings := ValidateConfig(tt.config)

			if len(warnings) != tt.expectedWarnings {
				t.Errorf("expected %d warnings, got %d: %v", tt.expectedWarnings, len(warnings), warnings)
			}

			if tt.expectedWarning != "" && len(warnings) > 0 {
				found := false
				for _, warning := range warnings {
					if warning == tt.expectedWarning ||
						(tt.expectedWarning == "canary policy requires at least 2 providers" &&
							warning == "canary policy requires at least 2 providers, falling back to cheapest") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q, got %v", tt.expectedWarning, warnings)
				}
			}
		})
	}
}

func TestIsValidPolicy(t *testing.T) {
	tests := []struct {
		policy string
		valid  bool
	}{
		{"cheapest", true},
		{"fastest_p95", true},
		{"slo_burn_aware", true},
		{"canary", true},
		{"invalid_policy", false},
		{"", false},
		{"CHEAPEST", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			result := IsValidPolicy(tt.policy)
			if result != tt.valid {
				t.Errorf("IsValidPolicy(%q) = %v, want %v", tt.policy, result, tt.valid)
			}
		})
	}
}

func TestMaskSecrets(t *testing.T) {
	cfg := Config{
		OpenAIKey:     "sk-1234567890abcdef",
		AdminToken:    "secret-admin-token",
		DefaultPolicy: "cheapest",
		Port:          "8080",
	}

	masked := cfg.MaskSecrets()

	// Check that secrets are masked
	if masked.OpenAIKey != "***masked***" {
		t.Errorf("expected OpenAIKey to be masked, got %q", masked.OpenAIKey)
	}
	if masked.AdminToken != "***masked***" {
		t.Errorf("expected AdminToken to be masked, got %q", masked.AdminToken)
	}

	// Check that non-secrets are preserved
	if masked.DefaultPolicy != cfg.DefaultPolicy {
		t.Errorf("expected DefaultPolicy to be preserved, got %q", masked.DefaultPolicy)
	}
	if masked.Port != cfg.Port {
		t.Errorf("expected Port to be preserved, got %q", masked.Port)
	}

	// Check that original config is not modified
	if cfg.OpenAIKey == "***masked***" {
		t.Error("original config should not be modified")
	}
	if cfg.AdminToken == "***masked***" {
		t.Error("original config should not be modified")
	}
}

func TestMaskSecretsEmptyValues(t *testing.T) {
	cfg := Config{
		OpenAIKey:     "",
		AdminToken:    "",
		DefaultPolicy: "cheapest",
	}

	masked := cfg.MaskSecrets()

	// Empty secrets should remain empty (not masked)
	if masked.OpenAIKey != "" {
		t.Errorf("expected empty OpenAIKey to remain empty, got %q", masked.OpenAIKey)
	}
	if masked.AdminToken != "" {
		t.Errorf("expected empty AdminToken to remain empty, got %q", masked.AdminToken)
	}
}
