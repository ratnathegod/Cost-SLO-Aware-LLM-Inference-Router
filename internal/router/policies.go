package router

import (
	"math/rand"
	"sort"
	"sync"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/providers"
)

type Strategy string

const (
	Cheapest     Strategy = "cheapest"
	FastestP95   Strategy = "fastest_p95"
	SLOBurnAware Strategy = "slo_burn_aware"
	Canary       Strategy = "canary"
)

type Engine struct {
	mu        sync.RWMutex
	provs     []*providers.ResilientProvider
	sloTarget float64
	rng       *rand.Rand

	canary struct {
		candidate string
		stages    []float64
		stageIdx  int
		calls     int
	}
}

func NewEngine(providersList []*providers.ResilientProvider) *Engine {
	e := &Engine{
		provs:     providersList,
		sloTarget: 0.01, // 99% success target
		rng:       rand.New(rand.NewSource(42)),
	}
	e.canary.stages = []float64{0.01, 0.05, 0.25}
	if len(providersList) > 1 {
		// default candidate = second cheapest
		primary, candidate := e.cheapestPair("")
		_ = primary
		e.canary.candidate = candidate.Name()
	}
	return e
}

func (e *Engine) providers() []*providers.ResilientProvider {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.provs
}

func (e *Engine) cheapest(model string) *providers.ResilientProvider {
	ps := e.providers()
	if len(ps) == 0 {
		return nil
	}
	best := ps[0]
	bestCost := best.CostPer1kTokensUSD(model)
	for _, p := range ps[1:] {
		if c := p.CostPer1kTokensUSD(model); c < bestCost {
			best = p
			bestCost = c
		}
	}
	return best
}

func (e *Engine) cheapestPair(model string) (*providers.ResilientProvider, *providers.ResilientProvider) {
	ps := e.providers()
	if len(ps) < 2 {
		return nil, nil
	}
	sort.Slice(ps, func(i, j int) bool {
		return ps[i].CostPer1kTokensUSD(model) < ps[j].CostPer1kTokensUSD(model)
	})
	return ps[0], ps[1]
}

func (e *Engine) fastestP95() *providers.ResilientProvider {
	ps := e.providers()
	if len(ps) == 0 {
		return nil
	}
	best := ps[0]
	bestP := best.Stats().P95LatencyMs()
	for _, p := range ps[1:] {
		if v := p.Stats().P95LatencyMs(); v > 0 && (bestP == 0 || v < bestP) {
			best = p
			bestP = v
		}
	}
	if bestP == 0 { // no data, fallback to cheapest
		return e.cheapest("")
	}
	return best
}

func (e *Engine) healthyAlternative(model string) *providers.ResilientProvider {
	ps := e.providers()
	if len(ps) == 0 {
		return nil
	}
	// choose lowest error rate, tie-break by cost
	best := ps[0]
	bestErr := best.Stats().ErrorRate()
	for _, p := range ps[1:] {
		er := p.Stats().ErrorRate()
		if er < bestErr || (er == bestErr && p.CostPer1kTokensUSD(model) < best.CostPer1kTokensUSD(model)) {
			best = p
			bestErr = er
		}
	}
	return best
}

// Choose selects a provider based on the policy and current stats
func (e *Engine) Choose(policy string, model string) *providers.ResilientProvider {
	switch Strategy(policy) {
	case Cheapest:
		return e.cheapest(model)
	case FastestP95:
		return e.fastestP95()
	case SLOBurnAware:
		alt := e.healthyAlternative(model)
		// if cheapest is burning error budget, pick healthier alt
		cheapest := e.cheapest(model)
		if cheapest == nil {
			return alt
		}
		burn := cheapest.Stats().ErrorRate() / e.sloTarget
		if burn > 1.0 {
			return alt
		}
		return cheapest
	case Canary:
		primary, candidate := e.cheapestPair(model)
		if primary == nil || candidate == nil {
			return primary
		}
		p := e.canary.stages[e.canary.stageIdx]
		if e.rng.Float64() < p {
			return candidate
		}
		return primary
	default:
		return e.cheapest(model)
	}
}

// RecordResult lets the engine update state for strategies like canary
func (e *Engine) RecordResult(providerName string, failed bool) {
	if StrategyCanary := true; StrategyCanary { // cheap hook; no per-policy switch needed now
		e.mu.Lock()
		defer e.mu.Unlock()
		if providerName == e.canary.candidate {
			e.canary.calls++
			// simple stage progression: every 200 calls without excessive failures, advance
			// rollback on failure rate > 2x SLO
			calls := e.canary.calls
			if calls%200 == 0 {
				// check failure rate of candidate
				var cand *providers.ResilientProvider
				for _, p := range e.provs {
					if p.Name() == providerName {
						cand = p
						break
					}
				}
				if cand != nil {
					burn := cand.Stats().ErrorRate() / e.sloTarget
					if burn > 2.0 {
						// rollback to first stage
						e.canary.stageIdx = 0
						return
					}
				}
				if e.canary.stageIdx+1 < len(e.canary.stages) {
					e.canary.stageIdx++
				}
			}
		}
	}
}
