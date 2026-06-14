# =============================================================================
# ZenGate AI — Makefile
# =============================================================================

.PHONY: build run test lint clean docker docker-up docker-down fmt vet

# Go settings
BINARY_NAME = zengate
MODULE = github.com/zengate-ai/zengate
GO = go
GOFLAGS = -ldflags="-s -w"

# --- Build ---

build:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME) ./cmd/gateway

build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/gateway

build-arm:
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/gateway

# --- Run ---

run: build
	./bin/$(BINARY_NAME)

dev:
	$(GO) run ./cmd/gateway

# --- Test ---

test:
	$(GO) test ./... -v -cover -race

test-short:
	$(GO) test ./... -short

test-integration:
	$(GO) test ./... -tags=integration -v

coverage:
	$(GO) test ./... -coverprofile=coverage.out -race
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# --- Quality ---

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

# --- Docker ---

docker:
	docker build -t zengate:latest .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f gateway

# --- Clean ---

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# --- ADK Agents ---

adk-install:
	cd adk && uv sync

adk-run:
	cd adk && uv run python -m agents.orchestrator

# --- Help ---

help:
	@echo "ZenGate AI - Distributed API Gateway"
	@echo ""
	@echo "Usage:"
	@echo "  make build          Build the gateway binary"
	@echo "  make run            Build and run the gateway"
	@echo "  make dev            Run the gateway with go run"
	@echo "  make test           Run all tests with race detection"
	@echo "  make coverage       Generate coverage report"
	@echo "  make docker         Build Docker image"
	@echo "  make docker-up      Start Docker Compose stack"
	@echo "  make docker-down    Stop Docker Compose stack"
	@echo "  make adk-install    Install ADK agent dependencies"
	@echo "  make adk-run        Run the ADK orchestrator"
	@echo "  make clean          Remove build artifacts"
