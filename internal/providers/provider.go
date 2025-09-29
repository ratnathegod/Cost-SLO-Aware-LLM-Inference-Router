package providers

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// CompletionRequest represents a text completion request
type CompletionRequest struct {
	Model  string
	Prompt string
	MaxTok int
	Stream bool
}

// CompletionResponse represents a text completion response
type CompletionResponse struct {
	Text string
}

// Provider is the interface implemented by all LLM providers
type Provider interface {
	Name() string
	// CostPer1kTokensUSD returns the nominal list price used by policies like Cheapest
	CostPer1kTokensUSD(model string) float64
	Complete(ctx context.Context, req CompletionRequest) (resp CompletionResponse, costUSD float64, latencyMs int64, err error)
}

// ---- Resilience and Metrics Wrappers ----

// Outcome holds a single call result
type Outcome struct {
	LatencyMs int64
	Err       bool
	At        time.Time
}

// Stats maintains a rolling window of outcomes for p95 and error rate
type Stats struct {
	mu         sync.RWMutex
	outcomes   []Outcome
	windowSize int
}

func NewStats(window int) *Stats {
	return &Stats{windowSize: window}
}

func (s *Stats) Record(latencyMs int64, err bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes = append(s.outcomes, Outcome{LatencyMs: latencyMs, Err: err, At: time.Now()})
	if len(s.outcomes) > s.windowSize {
		s.outcomes = s.outcomes[len(s.outcomes)-s.windowSize:]
	}
}

func (s *Stats) ErrorRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.outcomes) == 0 {
		return 0
	}
	var errs int
	for _, o := range s.outcomes {
		if o.Err {
			errs++
		}
	}
	return float64(errs) / float64(len(s.outcomes))
}

func (s *Stats) P95LatencyMs() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.outcomes) == 0 {
		return 0
	}
	vals := make([]int, 0, len(s.outcomes))
	for _, o := range s.outcomes {
		if !o.Err { // consider only successful calls for p95
			vals = append(vals, int(o.LatencyMs))
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Ints(vals)
	idx := int(math.Ceil(0.95*float64(len(vals)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	return int64(vals[idx])
}

// ErrorRateSince computes error rate over outcomes within the last d duration
func (s *Stats) ErrorRateSince(d time.Duration) float64 {
	s.mu.RLock(); defer s.mu.RUnlock()
	if len(s.outcomes) == 0 { return 0 }
	cutoff := time.Now().Add(-d)
	var total, errs int
	for _, o := range s.outcomes {
		if o.At.After(cutoff) {
			total++
			if o.Err { errs++ }
		}
	}
	if total == 0 { return 0 }
	return float64(errs) / float64(total)
}

// CBStateValue exposes 0=open,1=half,2=closed
func (cb *CircuitBreaker) StateValue() float64 {
	cb.mu.Lock(); defer cb.mu.Unlock()
	if cb.open {
		if cb.halfOpenProbe {
			return 1
		}
		return 0
	}
	return 2
}

// CircuitBreaker implements a simple sliding window error-rate breaker
type CircuitBreaker struct {
	mu            sync.Mutex
	window        []bool // last N results: true if error
	windowSize    int
	open          bool
	openedAt      time.Time
	cooldown      time.Duration
	halfOpenProbe bool
}

func NewCircuitBreaker(windowSize int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{windowSize: windowSize, cooldown: cooldown}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if !cb.open {
		return true
	}
	// if open and cooldown passed, allow a half-open probe
	if time.Since(cb.openedAt) >= cb.cooldown {
		if !cb.halfOpenProbe {
			cb.halfOpenProbe = true
			return true
		}
	}
	return false
}

func (cb *CircuitBreaker) OnResult(err bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.window = append(cb.window, err)
	if len(cb.window) > cb.windowSize {
		cb.window = cb.window[len(cb.window)-cb.windowSize:]
	}

	// handle half-open state
	if cb.halfOpenProbe {
		cb.halfOpenProbe = false
		if !err {
			// close
			cb.open = false
			cb.window = nil
			return
		} else {
			// reopen
			cb.open = true
			cb.openedAt = time.Now()
			return
		}
	}

	// compute error rate
	var errs int
	for _, e := range cb.window {
		if e {
			errs++
		}
	}
	if len(cb.window) >= cb.windowSize && float64(errs)/float64(len(cb.window)) > 0.5 {
		cb.open = true
		cb.openedAt = time.Now()
	}
}

// ResilienceOptions controls wrapping behaviors
type ResilienceOptions struct {
	Timeout      time.Duration
	MaxRetries   int
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	JitterFrac   float64 // 0..1 of backoff
	CBWindowSize int
	CBCooldown   time.Duration
}

// ResilientProvider wraps a provider with timeout, retry, and circuit breaker, while recording stats
type ResilientProvider struct {
	inner Provider
	opts  ResilienceOptions

	stats *Stats
	cb    *CircuitBreaker
}

func WithResilience(p Provider, opts ResilienceOptions) *ResilientProvider {
	stats := NewStats(100)
	cb := NewCircuitBreaker(opts.CBWindowSize, opts.CBCooldown)
	return &ResilientProvider{inner: p, opts: opts, stats: stats, cb: cb}
}

func (rp *ResilientProvider) Name() string { return rp.inner.Name() }
func (rp *ResilientProvider) CostPer1kTokensUSD(model string) float64 {
	return rp.inner.CostPer1kTokensUSD(model)
}

func (rp *ResilientProvider) Stats() *Stats { return rp.stats }

// CBStateValue returns 0=open,1=half,2=closed for the inner circuit breaker
func (rp *ResilientProvider) CBStateValue() float64 { return rp.cb.StateValue() }

func randomJitter(d time.Duration, frac float64) time.Duration {
	if frac <= 0 {
		return d
	}
	f := (rand.Float64()*2 - 1) * frac // +/- frac
	return time.Duration(float64(d) * (1 + f))
}

func (rp *ResilientProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, float64, int64, error) {
	// circuit breaker gate
	if !rp.cb.Allow() {
		return CompletionResponse{}, 0, 0, errors.New("circuit open")
	}

	var attempt int
	var lastErr error
	start := time.Now()
	defer func() {
		// No-op here; actual latency recorded on success or final failure below
		_ = start
	}()

	for {
		attempt++
		callCtx := ctx
		cancel := func() {}
		if rp.opts.Timeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, rp.opts.Timeout)
		}
		t0 := time.Now()
		resp, cost, _, err := rp.inner.Complete(callCtx, req)
		cancel()
		lat := time.Since(t0).Milliseconds()

		if err == nil {
			rp.stats.Record(lat, false)
			rp.cb.OnResult(false)
			return resp, cost, lat, nil
		}

		lastErr = err
		rp.stats.Record(lat, true)
		rp.cb.OnResult(true)

		if attempt > rp.opts.MaxRetries {
			break
		}
		// exponential backoff with jitter
		backoff := rp.opts.BaseBackoff * time.Duration(1<<uint(attempt-1))
		if rp.opts.MaxBackoff > 0 && backoff > rp.opts.MaxBackoff {
			backoff = rp.opts.MaxBackoff
		}
		time.Sleep(randomJitter(backoff, rp.opts.JitterFrac))
	}

	// failed after retries; estimate latency as elapsed
	lat := time.Since(start).Milliseconds()
	return CompletionResponse{}, 0, lat, lastErr
}
