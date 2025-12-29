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
	@echo "Starting controller and worker in development mode..."
	@echo "TODO: Use docker-compose or process manager"

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
