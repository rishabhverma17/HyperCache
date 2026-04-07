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

## cluster: Start a local N-node cluster (default: NODES=3)
##   Usage: make cluster NODES=5
cluster: build
	@NODES=$${NODES:-3}; \
	mkdir -p logs; \
	SEEDS=""; \
	for i in $$(seq 1 $$NODES); do \
		GOSSIP=$$((7945 + $$i)); \
		if [ -n "$$SEEDS" ]; then SEEDS="$$SEEDS,"; fi; \
		SEEDS="$${SEEDS}\"127.0.0.1:$$GOSSIP\""; \
	done; \
	for i in $$(seq 1 $$NODES); do \
		NODE_ID="node-$$i"; \
		RESP_PORT=$$((8079 + $$i)); \
		HTTP_PORT=$$((9079 + $$i)); \
		GOSSIP_PORT=$$((7945 + $$i)); \
		DATA_DIR="data/$$NODE_ID"; \
		mkdir -p "$$DATA_DIR"; \
		CFG="/tmp/hypercache-$$NODE_ID.yaml"; \
		sed -e "s/\$${NODE_ID}/$$NODE_ID/g" \
		    -e "s/\$${RESP_PORT}/$$RESP_PORT/g" \
		    -e "s/\$${HTTP_PORT}/$$HTTP_PORT/g" \
		    -e "s/\$${GOSSIP_PORT}/$$GOSSIP_PORT/g" \
		    -e "s|\$${CLUSTER_SEEDS}|$$SEEDS|g" \
		    -e "s/\$${LOG_LEVEL}/info/g" \
		    templates/node-config.yaml.template > "$$CFG"; \
		echo "Starting $$NODE_ID (RESP=$$RESP_PORT HTTP=$$HTTP_PORT Gossip=$$GOSSIP_PORT)..."; \
		./$(BINARY) -protocol resp -config "$$CFG" > "logs/$$NODE_ID.log" 2>&1 & \
		sleep 2; \
	done; \
	sleep 2; \
	echo "Cluster health checks:"; \
	for i in $$(seq 1 $$NODES); do \
		HTTP_PORT=$$((9079 + $$i)); \
		curl -s "http://localhost:$$HTTP_PORT/health" | head -1 || echo "node-$$i: not ready"; \
	done

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
