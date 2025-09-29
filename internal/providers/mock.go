package providers

import (
    "context"
    "errors"
    "math"
    "math/rand"
    "time"
)

type MockProvider struct {
    name       string
    meanMs     float64
    p95Ms      float64
    errorRate  float64
    costPer1k  float64
}

func NewMockProvider(meanMs, p95Ms float64, errorRate float64, costPer1k float64) *MockProvider {
    return &MockProvider{
        name:      "mock",
        meanMs:    meanMs,
        p95Ms:     p95Ms,
        errorRate: errorRate,
        costPer1k: costPer1k,
    }
}

func (m *MockProvider) Name() string { return m.name }
func (m *MockProvider) CostPer1kTokensUSD(model string) float64 { return m.costPer1k }

// sampleLatency samples from a lognormal distribution configured to approximate given mean and p95
func (m *MockProvider) sampleLatency() time.Duration {
    // For lognormal X ~ logN(mu, sigma), mean = exp(mu + sigma^2/2)
    // p95 = exp(mu + z* sigma), z = 1.64485362695
    mean := m.meanMs
    p95 := m.p95Ms
    if p95 < mean { p95 = mean }
    z := 1.64485362695
    // Solve for sigma: p95/mean = exp(sigma*(z - sigma/2))
    // Use numeric search for sigma
    f := func(s float64) float64 { return math.Exp(s*(z - s/2)) - p95/mean }
    lo, hi := 1e-6, 3.0
    for i := 0; i < 40; i++ {
        mid := (lo + hi) / 2
        if f(mid) > 0 { hi = mid } else { lo = mid }
    }
    sigma := (lo + hi) / 2
    mu := math.Log(mean) - sigma*sigma/2
    // sample
    n := rand.NormFloat64()
    x := math.Exp(mu + sigma*n)
    // clamp to [0, 3*p95] to avoid extreme outliers
    if x < 0 { x = 0 }
    if x > 3*p95 { x = 3 * p95 }
    return time.Duration(x * float64(time.Millisecond))
}

func (m *MockProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, float64, int64, error) {
    d := m.sampleLatency()
    t := time.NewTimer(d)
    select {
    case <-ctx.Done():
        if !t.Stop() { <-t.C }
        return CompletionResponse{}, 0, 0, ctx.Err()
    case <-t.C:
    }
    // decide error
    if rand.Float64() < m.errorRate {
        return CompletionResponse{}, 0, int64(d/time.Millisecond), errors.New("mock error")
    }
    // cost estimation using request MaxTok or default 50
    toks := req.MaxTok
    if toks <= 0 { toks = 50 }
    cost := m.costPer1k * float64(toks) / 1000.0
    return CompletionResponse{Text: "(mock) hello"}, cost, int64(d/time.Millisecond), nil
}
