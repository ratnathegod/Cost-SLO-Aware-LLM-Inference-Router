package router

import (
    "sync"

    "github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/internal/providers"
)

var (
    regMu     sync.RWMutex
    regProvs  []*providers.ResilientProvider
)

func SetProviders(ps []*providers.ResilientProvider) {
    regMu.Lock(); defer regMu.Unlock()
    regProvs = ps
}

func GetProviders() []*providers.ResilientProvider {
    regMu.RLock(); defer regMu.RUnlock()
    return regProvs
}
