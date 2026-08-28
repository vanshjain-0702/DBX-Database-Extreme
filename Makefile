# DBX Makefile
# Usage: make <target>

BINARY_SERVER      := dbx-server
BINARY_ORCHESTRATOR := dbx-orchestrator
VERSION            := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS            := -ldflags="-X main.version=$(VERSION) -s -w"
BUILD_DIR          := ./bin

.PHONY: all build build-server build-orchestrator run-dev test python-check soak restore-drill clean docker-build docker-up lint site help

all: build

## build: Build all binaries
build: build-server build-orchestrator

## build-server: Build the DBX storage node binary
build-server:
	@echo "==> Building dbx-server..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_SERVER) ./cmd/dbx-server

## build-orchestrator: Build the DBX orchestrator (control plane) binary
build-orchestrator:
	@echo "==> Building dbx-orchestrator..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_ORCHESTRATOR) ./cmd/dbx-orchestrator

## run-dev: Start the orchestrator in insecure local dev mode
run-dev:
	@echo "==> Starting DBX Orchestrator in dev mode..."
	@export DBX_ADMIN_PASSWORD=adminadminadmin && \
	 export DBX_JWT_SECRET=supersecretjwtsecret1234567890123456 && \
	 export DBX_INTERNAL_API_TOKEN=internalapitoken1234567890123456 && \
	export DBX_DEFAULT_PASSWORD=adminadminadmin && \
	 export DBX_DATA_DIR=./data && \
	 export DBX_NODE_MEMORY_BUDGET=8gb && \
	 go run ./cmd/dbx-orchestrator -insecure-http=true

## run-dashboard: Start the Vite dev server for the dashboard
run-dashboard:
	@echo "==> Starting Dashboard dev server..."
	cd dashboard && npm run dev

## test: Run the Go test suite (no race detector; Linux CI adds -race)
test:
	go test -count=1 -timeout 15m ./...

## python-check: Flake8, Black, and pytest for the SDK and examples
python-check:
	python -m flake8 scripts sdk/python examples
	python -m black --check scripts sdk/python examples
	python -m pytest sdk/python examples --ignore=sdk/python/.venv

## site: Open the public HTML from disk (GitHub Pages is 404 while the repo is private)
site:
	python -c "import pathlib, webbrowser; webbrowser.open(pathlib.Path('website/index.html').resolve().as_uri())"

## soak: Engine density drill (100 idle / 25 active). Not a CI default.
soak:
	go run ./cmd/dbx-soak -idle 100 -active 25 -for 3s

## restore-drill: Backup/restore round-trip test
restore-drill:
	go test -count=1 -timeout 3m ./internal/orchestrator/ -run 'BackupRestore|Hibernate|TenantUsage'

## test-bench: Run benchmarks
test-bench:
	go test -bench=. -benchmem ./...

## lint: Run Go vet and strict linting if available
lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null; then \
		echo "==> Running golangci-lint..."; \
		golangci-lint run; \
	else \
		echo "==> golangci-lint not installed, skipping strict lint"; \
	fi

## helm-install: Install DBX via Helm locally
helm-install:
	helm upgrade --install dbx ./deploy/kubernetes/helm/dbx --namespace dbx-system --create-namespace

## clean: Remove build artifacts
clean:
	@echo "==> Cleaning..."
	rm -rf $(BUILD_DIR)

## docker-build: Build Docker image
docker-build:
	docker build -t dbx/dbx:$(VERSION) -f deploy/Dockerfile .

## docker-up: Start full stack with docker-compose
docker-up:
	docker compose -f deploy/docker-compose.yml up --build

## docker-down: Stop docker-compose stack
docker-down:
	docker compose -f deploy/docker-compose.yml down

## help: Show this help
help:
	@grep -h "^##" $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':' | sed -e 's/^/  /'
