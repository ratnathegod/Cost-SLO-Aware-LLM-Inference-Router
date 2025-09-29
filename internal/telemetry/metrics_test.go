package telemetry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	MustRegisterMetrics()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	MetricsHandler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if len(body) == 0 {
		t.Fatalf("expected metrics body")
	}
}
