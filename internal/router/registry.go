package router

import (
	"sync"

	"github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/providers"
)

var (
	regMu    sync.RWMutex
	regProvs []*providers.ResilientProvider
	regEng   *Engine
	defPol   string
)

func SetProviders(ps []*providers.ResilientProvider) {
	regMu.Lock()
	defer regMu.Unlock()
	regProvs = ps
}

func GetProviders() []*providers.ResilientProvider {
	regMu.RLock()
	defer regMu.RUnlock()
	return regProvs
}

func SetEngine(e *Engine) {
	regMu.Lock()
	defer regMu.Unlock()
	regEng = e
}

func GetEngine() *Engine {
	regMu.RLock()
	defer regMu.RUnlock()
	return regEng
}

func SetDefaultPolicy(p string) {
	regMu.Lock()
	defer regMu.Unlock()
	defPol = p
}

func GetDefaultPolicy() string {
	regMu.RLock()
	defer regMu.RUnlock()
	return defPol
}
