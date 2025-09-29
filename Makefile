run:
	go run ./cmd/server

run-mock:
	ENABLE_MOCK_PROVIDER=1 go run ./cmd/server

loadgen:
	go run ./cmd/loadgen --qps 500 --concurrency 128 --duration 60s --policy cheapest --prompt "ping"

test:
	go test ./... -race -coverprofile=coverage.out

build:
	go build ./cmd/server && go build ./cmd/loadgen

lint:
	golangci-lint run

docker-build:
	docker build -t ghcr.io/ratnathegod/llm-router:local .

docker-run:
	docker run --rm -p 8080:8080 ghcr.io/ratnathegod/llm-router:local

smoke-mock:
	ENABLE_MOCK_PROVIDER=1 go run ./cmd/server & echo $$! > server.pid; sleep 2; \
	go run ./cmd/loadgen --qps 200 --concurrency 64 --duration 15s --prompt "ping"; \
	kill `cat server.pid` || true
