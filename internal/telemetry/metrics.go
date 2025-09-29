package telemetry

import (
    "net/http"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    RequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "router_requests_total",
            Help: "Total requests processed by the router",
        },
        []string{"provider", "policy", "code"},
    )

    LatencyMs = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "router_latency_ms",
            Help:    "Latency of completions in milliseconds",
            Buckets: prometheus.ExponentialBuckets(10, 1.5, 12),
        },
        []string{"provider", "policy"},
    )

    CostUSDTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "router_cost_usd_total",
            Help: "Accumulated provider cost in USD",
        },
        []string{"provider"},
    )

    ErrorsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "router_errors_total",
            Help: "Total errors by provider and reason",
        },
        []string{"provider", "reason"},
    )

    CBState = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "router_cb_state",
            Help: "Circuit breaker state per provider (0=open,1=half,2=closed)",
        },
        []string{"provider"},
    )

    BurnRate = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "router_burn_rate",
            Help: "Error-budget burn rate over rolling windows",
        },
        []string{"window"},
    )
)

func MustRegisterMetrics() {
    prometheus.MustRegister(RequestsTotal, LatencyMs, CostUSDTotal, ErrorsTotal, CBState, BurnRate)
}

func MetricsHandler() http.Handler { return promhttp.Handler() }
