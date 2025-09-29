# Cost-SLO-Aware-LLM-Inference-Router

[![CI](https://github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/actions/workflows/go-ci.yml/badge.svg)](https://github.com/ratnathegod/Cost-SLO-Aware-LLM-Inference-Router/actions/workflows/go-ci.yml)

Run the server:

```bash
go run ./cmd/server
```

Endpoints:
- GET /v1/healthz
- POST /v1/infer
- GET /metrics (Prometheus)

Observability:
- Prometheus metrics at /metrics.
- OpenTelemetry traces exported if OTEL_EXPORTER_OTLP_ENDPOINT is set (e.g., localhost:4317).
- X-Request-ID middleware sets and propagates request IDs.

Key env vars:
Mock provider (dev only):
- ENABLE_MOCK_PROVIDER=1 to enable
- MOCK_MEAN_LATENCY_MS (default 40)
- MOCK_P95_LATENCY_MS (default 120)
- MOCK_ERROR_RATE (default 0.01)
- MOCK_COST_PER_1K_TOKENS_USD (default 0.002)

Load generator:
- Build and run:
	- make loadgen
- Custom run example:
	- go run ./cmd/loadgen --qps 500 --concurrency 128 --duration 60s --policy cheapest --prompt "ping"
- Output: live QPS and final JSON summary; optional CSV via --csv-out.

Expected local mock benchmark (localhost):
- mean=40ms, p95=120ms, error=0.5% -> expect ~500 QPS with p95 < 150ms on a typical laptop.

- PORT (default 8080)
- ROUTER_POLICY (default cheapest)
- OPENAI_API_KEY, OPENAI_MODEL (default gpt-4o)
- AWS_PROFILE or AWS_ACCESS_KEY_ID/SECRET (enables Bedrock)
- BEDROCK_REGION (default us-east-1), BEDROCK_MODEL_ID
- OTEL_EXPORTER_OTLP_ENDPOINT (optional)

Docker

Build and run locally:

```bash
docker build -t ghcr.io/ratnathegod/llm-router:local .
docker run --rm -p 8080:8080 ghcr.io/ratnathegod/llm-router:local
```

Smoke test locally:

```bash
ENABLE_MOCK_PROVIDER=1 go run ./cmd/server & sleep 2
go run ./cmd/loadgen --qps 200 --concurrency 64 --duration 15s --policy cheapest --prompt "ping"
pkill -f "go run ./cmd/server"
```

Release

Tag and push to cut a release:

```bash
git tag v0.1.0
git push --tags
```
