# HyperCache RESP Demo Makefile

.PHONY: demo clean deps server help

# Default target
demo: deps
	@echo "🚀 Running HyperCache RESP Demo..."
	@echo "Make sure HyperCache server is running on localhost:6379"
	@echo "Use 'make server' in another terminal to start the server"
	@echo ""
	go run main.go

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	go mod tidy
	go mod download

# Start HyperCache server (requires navigating to main project)
server:
	@echo "🔧 Starting HyperCache RESP server on localhost:6379..."
	@echo "Note: This assumes you're running from the demo directory"
	@echo "Starting server in main project..."
	cd ../.. && go run cmd/hypercache/main.go --config configs/hypercache.yaml --protocol resp --port 6379

# Clean up
clean:
	@echo "🧹 Cleaning up..."
	go clean
	go mod tidy

# Full demo with server startup instruction
full-demo:
	@echo "🚀 HyperCache Full Demo Setup"
	@echo "==============================="
	@echo ""
	@echo "1. First, start the HyperCache server:"
	@echo "   make server"
	@echo ""
	@echo "2. Then in another terminal, run the demo:"
	@echo "   make demo"
	@echo ""
	@echo "Or run both commands in separate terminals."

# Show help
help:
	@echo "HyperCache RESP Demo Commands:"
	@echo "=============================="
	@echo ""
	@echo "make demo      - Run the demo (requires server to be running)"
	@echo "make server    - Start HyperCache RESP server on localhost:6379"
	@echo "make deps      - Install Go dependencies"
	@echo "make clean     - Clean up build artifacts"
	@echo "make full-demo - Show instructions for complete demo setup"
	@echo "make help      - Show this help message"
	@echo ""
	@echo "Quick Start:"
	@echo "1. Terminal 1: make server"
	@echo "2. Terminal 2: make demo"
