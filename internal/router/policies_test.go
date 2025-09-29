package router

import (
	"context"
	"testing"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/providers"
)

type mockProv struct {
	name string
	cost float64
	lat  int64
	fail bool
}

func (m *mockProv) Name() string                            { return m.name }
func (m *mockProv) CostPer1kTokensUSD(model string) float64 { return m.cost }
func (m *mockProv) Complete(ctx context.Context, req providers.CompletionRequest) (providers.CompletionResponse, float64, int64, error) {
	if m.fail {
		return providers.CompletionResponse{}, 0, m.lat, context.DeadlineExceeded
	}
	return providers.CompletionResponse{Text: "ok"}, m.cost / 1000.0, m.lat, nil
}

func rp(p providers.Provider) *providers.ResilientProvider {
	return providers.WithResilience(p, providers.ResilienceOptions{CBWindowSize: 20})
}

func TestCheapest(t *testing.T) {
	e := NewEngine([]*providers.ResilientProvider{rp(&mockProv{name: "a", cost: 1}), rp(&mockProv{name: "b", cost: 2})})
	got := e.Choose("cheapest", "")
	if got == nil || got.Name() != "a" {
		t.Fatalf("want a, got %v", got)
	}
}

func TestFastestP95(t *testing.T) {
	a := rp(&mockProv{name: "a", cost: 2})
	b := rp(&mockProv{name: "b", cost: 1})
	// record latencies
	for i := 0; i < 50; i++ {
		a.Stats().Record(50, false)
		b.Stats().Record(100, false)
	}
	e := NewEngine([]*providers.ResilientProvider{a, b})
	got := e.Choose("fastest_p95", "")
	if got == nil || got.Name() != "a" {
		t.Fatalf("want a, got %v", got)
	}
}

func TestSLOBurnAware(t *testing.T) {
	a := rp(&mockProv{name: "a", cost: 1})
	b := rp(&mockProv{name: "b", cost: 2})
	for i := 0; i < 20; i++ {
		a.Stats().Record(100, true)
	} // high error on cheapest
	e := NewEngine([]*providers.ResilientProvider{a, b})
	got := e.Choose("slo_burn_aware", "")
	if got == nil || got.Name() != "b" {
		t.Fatalf("want b, got %v", got)
	}
}

func TestCanaryRolloutAndRollback(t *testing.T) {
	a := rp(&mockProv{name: "a", cost: 1})
	b := rp(&mockProv{name: "b", cost: 1})
	e := NewEngine([]*providers.ResilientProvider{a, b})

	// simulate successes on candidate to progress stages
	for i := 0; i < 400; i++ {
		chosen := e.Choose("canary", "")
		e.RecordResult(chosen.Name(), false)
	}
	// now induce failures and ensure rollback (stage reset to 0)
	for i := 0; i < 200; i++ {
		// pretend candidate failed
		e.RecordResult(b.Name(), true)
	}
	// Select many times; ensure some fraction is still primary (indicating rollback). We can't access stageIdx directly, so probabilistic check
	primaryCount := 0
	for i := 0; i < 200; i++ {
		if e.Choose("canary", "").Name() == a.Name() {
			primaryCount++
		}
	}
	if primaryCount < 100 {
		t.Fatalf("expected rollback keeping majority on primary, primaryCount=%d", primaryCount)
	}
}
