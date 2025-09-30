package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/providers"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/router"
	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/telemetry"
)

func TestAdminAuthMiddleware(t *testing.T) {
	// Setup
	telemetry.MustRegisterMetrics()

	// Create a test handler that should be protected
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("authorized"))
	})

	// Auth middleware
	authMiddleware := func(token string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				const prefix = "Bearer "
				if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix || auth[len(prefix):] != token {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	expectedToken := "test-secret-token"
	protectedHandler := authMiddleware(expectedToken)(testHandler)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "no auth header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "unauthorized",
		},
		{
			name:           "invalid auth format",
			authHeader:     "Basic dGVzdA==",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "unauthorized",
		},
		{
			name:           "wrong token",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "unauthorized",
		},
		{
			name:           "correct token",
			authHeader:     "Bearer test-secret-token",
			expectedStatus: http.StatusOK,
			expectedBody:   "authorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			protectedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			body := strings.TrimSpace(rr.Body.String())
			if !strings.Contains(body, tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, body)
			}
		})
	}
}

func TestAdminStatus(t *testing.T) {
	// Setup test providers
	mockProvider := providers.NewMockProvider(50, 100, 0.01, 0.002)
	resilientProvider := providers.WithResilience(mockProvider, providers.ResilienceOptions{
		Timeout:      30 * 1_000_000_000,
		MaxRetries:   1,
		BaseBackoff:  100 * 1_000_000,
		MaxBackoff:   1 * 1_000_000_000,
		JitterFrac:   0.2,
		CBWindowSize: 20,
		CBCooldown:   10 * 1_000_000_000,
	})

	provs := []*providers.ResilientProvider{resilientProvider}
	router.SetProviders(provs)

	eng := router.NewEngine(provs)
	router.SetEngine(eng)
	router.SetDefaultPolicy("cheapest")

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil)
	rr := httptest.NewRecorder()

	handler := HandleAdminStatus()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp AdminStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Validate response structure
	if resp.BuildInfo.Version == "" {
		t.Error("expected build version to be set")
	}
	if resp.Uptime == "" {
		t.Error("expected uptime to be set")
	}
	if resp.DefaultPolicy != "cheapest" {
		t.Errorf("expected default policy 'cheapest', got %q", resp.DefaultPolicy)
	}
	if len(resp.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(resp.Providers))
	}
	if len(resp.Providers) > 0 {
		p := resp.Providers[0]
		if p.Name != "mock" {
			t.Errorf("expected provider name 'mock', got %q", p.Name)
		}
		if p.CostPer1k != 0.002 {
			t.Errorf("expected cost per 1k 0.002, got %f", p.CostPer1k)
		}
	}
}

func TestCanaryStatus(t *testing.T) {
	// Setup test engine
	mockProvider1 := providers.NewMockProvider(50, 100, 0.01, 0.001)
	mockProvider2 := providers.NewMockProvider(60, 120, 0.01, 0.002)

	rp1 := providers.WithResilience(mockProvider1, providers.ResilienceOptions{
		Timeout: 30 * 1_000_000_000, MaxRetries: 1, BaseBackoff: 100 * 1_000_000,
		MaxBackoff: 1 * 1_000_000_000, JitterFrac: 0.2, CBWindowSize: 20, CBCooldown: 10 * 1_000_000_000,
	})
	rp2 := providers.WithResilience(mockProvider2, providers.ResilienceOptions{
		Timeout: 30 * 1_000_000_000, MaxRetries: 1, BaseBackoff: 100 * 1_000_000,
		MaxBackoff: 1 * 1_000_000_000, JitterFrac: 0.2, CBWindowSize: 20, CBCooldown: 10 * 1_000_000_000,
	})

	provs := []*providers.ResilientProvider{rp1, rp2}
	eng := router.NewEngine(provs)
	eng.ConfigureCanary([]float64{1, 5, 25}, 200, 2.0)
	router.SetEngine(eng)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/canary/status", nil)
	rr := httptest.NewRecorder()

	handler := HandleCanaryStatus()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp CanaryStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Validate canary status
	if resp.Percent != 1.0 {
		t.Errorf("expected initial canary percent 1.0, got %f", resp.Percent)
	}
	if resp.Stage != 0 {
		t.Errorf("expected initial stage 0, got %d", resp.Stage)
	}
	if resp.WindowSize != 200 {
		t.Errorf("expected window size 200, got %d", resp.WindowSize)
	}
	if resp.CandidateProvider != "mock" {
		t.Errorf("expected candidate provider 'mock', got %q", resp.CandidateProvider)
	}
}

func TestCanaryAdvance(t *testing.T) {
	// Setup test engine
	mockProvider1 := providers.NewMockProvider(50, 100, 0.01, 0.001)
	mockProvider2 := providers.NewMockProvider(60, 120, 0.01, 0.002)

	rp1 := providers.WithResilience(mockProvider1, providers.ResilienceOptions{
		Timeout: 30 * 1_000_000_000, MaxRetries: 1, BaseBackoff: 100 * 1_000_000,
		MaxBackoff: 1 * 1_000_000_000, JitterFrac: 0.2, CBWindowSize: 20, CBCooldown: 10 * 1_000_000_000,
	})
	rp2 := providers.WithResilience(mockProvider2, providers.ResilienceOptions{
		Timeout: 30 * 1_000_000_000, MaxRetries: 1, BaseBackoff: 100 * 1_000_000,
		MaxBackoff: 1 * 1_000_000_000, JitterFrac: 0.2, CBWindowSize: 20, CBCooldown: 10 * 1_000_000_000,
	})

	provs := []*providers.ResilientProvider{rp1, rp2}
	eng := router.NewEngine(provs)
	eng.ConfigureCanary([]float64{1, 5, 25}, 200, 2.0)
	router.SetEngine(eng)

	// Test force advance
	body := `{"force": true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/canary/advance", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler := HandleCanaryAdvance()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rr.Code)
	}

	// Verify stage advanced
	if eng.CanaryStageIndex() != 1 {
		t.Errorf("expected stage 1 after advance, got %d", eng.CanaryStageIndex())
	}
	if eng.CanaryPercent() != 5.0 {
		t.Errorf("expected percent 5.0 after advance, got %f", eng.CanaryPercent())
	}
}

func TestCanaryRollback(t *testing.T) {
	// Setup test engine
	mockProvider1 := providers.NewMockProvider(50, 100, 0.01, 0.001)
	mockProvider2 := providers.NewMockProvider(60, 120, 0.01, 0.002)

	rp1 := providers.WithResilience(mockProvider1, providers.ResilienceOptions{
		Timeout: 30 * 1_000_000_000, MaxRetries: 1, BaseBackoff: 100 * 1_000_000,
		MaxBackoff: 1 * 1_000_000_000, JitterFrac: 0.2, CBWindowSize: 20, CBCooldown: 10 * 1_000_000_000,
	})
	rp2 := providers.WithResilience(mockProvider2, providers.ResilienceOptions{
		Timeout: 30 * 1_000_000_000, MaxRetries: 1, BaseBackoff: 100 * 1_000_000,
		MaxBackoff: 1 * 1_000_000_000, JitterFrac: 0.2, CBWindowSize: 20, CBCooldown: 10 * 1_000_000_000,
	})

	provs := []*providers.ResilientProvider{rp1, rp2}
	eng := router.NewEngine(provs)
	eng.ConfigureCanary([]float64{1, 5, 25}, 200, 2.0)
	router.SetEngine(eng)

	// Advance to stage 1 first
	eng.CanaryAdvance()
	if eng.CanaryStageIndex() != 1 {
		t.Fatalf("setup failed: expected stage 1, got %d", eng.CanaryStageIndex())
	}

	// Test rollback
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/canary/rollback", nil)
	rr := httptest.NewRecorder()

	handler := HandleCanaryRollback()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rr.Code)
	}

	// Verify stage rolled back
	if eng.CanaryStageIndex() != 0 {
		t.Errorf("expected stage 0 after rollback, got %d", eng.CanaryStageIndex())
	}
	if eng.CanaryPercent() != 1.0 {
		t.Errorf("expected percent 1.0 after rollback, got %f", eng.CanaryPercent())
	}
}

func TestPolicyUpdate(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedPolicy string
	}{
		{
			name:           "valid policy update",
			body:           `{"default_policy": "fastest_p95"}`,
			expectedStatus: http.StatusNoContent,
			expectedPolicy: "fastest_p95",
		},
		{
			name:           "invalid policy",
			body:           `{"default_policy": "invalid_policy"}`,
			expectedStatus: http.StatusBadRequest,
			expectedPolicy: "", // should not change
		},
		{
			name:           "invalid JSON",
			body:           `invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedPolicy: "", // should not change
		},
		{
			name:           "empty policy",
			body:           `{"default_policy": ""}`,
			expectedStatus: http.StatusBadRequest,
			expectedPolicy: "", // should not change
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset to known state
			router.SetDefaultPolicy("cheapest")

			req := httptest.NewRequest(http.MethodPost, "/v1/admin/policy", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler := HandlePolicyUpdate()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedPolicy != "" {
				if router.GetDefaultPolicy() != tt.expectedPolicy {
					t.Errorf("expected policy %q, got %q", tt.expectedPolicy, router.GetDefaultPolicy())
				}
			}
		})
	}
}

func TestProvidersReload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/providers/reload", nil)
	rr := httptest.NewRecorder()

	handler := HandleProvidersReload()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "not implemented") {
		t.Errorf("expected 'not implemented' in response body, got %q", rr.Body.String())
	}
}
