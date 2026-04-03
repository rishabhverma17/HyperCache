.PHONY: build test test-unit test-integration lint fmt vet clean run docker-build docker-up docker-down cluster cluster-stop help

BINARY=bin/hypercache
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-s -w"

## help: Show this help message
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'

## build: Build the HyperCache binary
build:
	@mkdir -p bin
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY) ./cmd/hypercache

## run: Build and run a single node (RESP protocol)
run: build
	./$(BINARY) -protocol resp -node-id node-1 -config configs/hypercache.yaml

## test: Run all tests
test: test-unit

## test-unit: Run unit tests with race detection and coverage
test-unit:
	@mkdir -p test-results
	$(GO) test -race -coverprofile=test-results/coverage.out -covermode=atomic -timeout=5m ./tests/unit/... ./internal/... ./pkg/...
	@echo "Coverage report:"
	@$(GO) tool cover -func=test-results/coverage.out | tail -1

## test-integration: Run integration tests (requires running cluster)
test-integration:
	$(GO) test -v -timeout=5m ./tests/integration/...

## test-coverage-html: Generate HTML coverage report and open it
test-coverage-html: test-unit
	$(GO) tool cover -html=test-results/coverage.out -o test-results/coverage.html
	@echo "Coverage report: test-results/coverage.html"

## bench: Run benchmarks
bench:
	$(GO) test -bench=. -benchmem ./internal/...

## lint: Run golangci-lint
lint:
	golangci-lint run --timeout=5m

## fmt: Format all Go files
fmt:
	gofmt -s -w .

## vet: Run go vet
vet:
	$(GO) vet ./...

## clean: Remove build artifacts and data
clean:
	rm -rf bin/ test-results/ logs/ data/
	rm -f main hypercache

## cluster: Start a local 3-node cluster
cluster: build
	@mkdir -p logs data/node-1 data/node-2 data/node-3
	@echo "Starting node-1..."
	./$(BINARY) -protocol resp -config configs/node1-config.yaml > logs/node-1.log 2>&1 &
	@sleep 2
	@echo "Starting node-2..."
	./$(BINARY) -protocol resp -config configs/node2-config.yaml > logs/node-2.log 2>&1 &
	@sleep 2
	@echo "Starting node-3..."
	./$(BINARY) -protocol resp -config configs/node3-config.yaml > logs/node-3.log 2>&1 &
	@sleep 3
	@echo "Cluster started. Health checks:"
	@curl -s http://localhost:9080/health | head -1 || echo "node-1: not ready"
	@curl -s http://localhost:9081/health | head -1 || echo "node-2: not ready"
	@curl -s http://localhost:9082/health | head -1 || echo "node-3: not ready"

## cluster-stop: Stop all local HyperCache processes
cluster-stop:
	@pkill -f "$(BINARY)" 2>/dev/null || true
	@echo "Cluster stopped."

## docker-build: Build Docker image
docker-build:
	docker build -t rishabhverma17/hypercache:latest .

## docker-up: Start full Docker stack (cluster + monitoring)
docker-up:
	docker compose -f docker-compose.cluster.yml up -d

## docker-down: Stop Docker stack
docker-down:
	docker compose -f docker-compose.cluster.yml down

## docker-logs: Tail Docker cluster logs
docker-logs:
	docker compose -f docker-compose.cluster.yml logs -f --tail=50

## deps: Download and verify dependencies
deps:
	$(GO) mod download
	$(GO) mod verify
	$(GO) mod tidy
