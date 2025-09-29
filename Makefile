run:
	go run ./cmd/server

run-mock:
	ENABLE_MOCK_PROVIDER=1 go run ./cmd/server

loadgen:
	go run ./cmd/loadgen --qps 500 --concurrency 128 --duration 60s --policy cheapest --prompt "ping"

test:
	go test ./...

build:
	go build ./...
