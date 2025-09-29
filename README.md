# Cost-SLO-Aware-LLM-Inference-Router

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
- PORT (default 8080)
- ROUTER_POLICY (default cheapest)
- OPENAI_API_KEY, OPENAI_MODEL (default gpt-4o)
- AWS_PROFILE or AWS_ACCESS_KEY_ID/SECRET (enables Bedrock)
- BEDROCK_REGION (default us-east-1), BEDROCK_MODEL_ID
- OTEL_EXPORTER_OTLP_ENDPOINT (optional)
