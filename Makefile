# Load environment variables from .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

.PHONY: build-all build-controller build-worker build-cli run-dev migrate clean

# Build all binaries
build-all: build-controller build-worker build-cli

build-controller:
	go build -o bin/controller ./cmd/controller

build-worker:
	go build -o bin/worker ./cmd/worker

build-cli:
	go build -o bin/jobctl ./cmd/cli

# Run development servers
run-dev:
	@echo "Starting DB..."
	docker-compose up -d
	@echo "Waiting for DB..."
	@sleep 2
	@echo "Starting Controller..."
	go run cmd/controller/main.go

# Run Worker
run-worker:
	@echo "Starting Worker..."
	go run cmd/worker/main.go

# Run database migrations
migrate:
	@echo "Running migrations..."
	@echo "TODO: Add migration tool (e.g., golang-migrate)"

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test -v ./...

# Lint code
lint:
	go vet ./...
	@which golangci-lint > /dev/null || echo "Install golangci-lint for full linting"
