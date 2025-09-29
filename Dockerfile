# syntax=docker/dockerfile:1

ARG GO_VERSION=1.23

FROM golang:${GO_VERSION} AS builder
WORKDIR /src

# Enable module mode and caching
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w" -o /out/llm-router ./cmd/server && \
    go build -trimpath -ldflags "-s -w" -o /out/loadgen ./cmd/loadgen


FROM gcr.io/distroless/base-debian12 AS runner
WORKDIR /app

# Copy CA certs if needed (distroless already has them, this is extra safety)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=builder /out/llm-router /app/llm-router
COPY --from=builder /out/loadgen /app/loadgen

USER nonroot:nonroot
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/app/llm-router"]
