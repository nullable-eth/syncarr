.PHONY: build test clean docker docker-build docker-run fmt vet lint

# Build variables
BINARY_NAME=syncarr
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)"

# Default target
all: fmt vet test build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/syncarr

# Build for Linux (useful for Docker)
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY_NAME)-linux ./cmd/syncarr

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Vet code
vet:
	@echo "Vetting code..."
	go vet ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	golangci-lint run

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME) $(BINARY_NAME)-linux
	rm -f coverage.out coverage.html
	docker rmi syncarr:latest 2>/dev/null || true

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t syncarr:latest .

# Docker run (requires environment variables to be set)
docker-run: docker-build
	@echo "Running Docker container..."
	docker run --rm syncarr:latest --version

# Docker compose up
docker-up:
	@echo "Starting with Docker Compose..."
	docker-compose up -d

# Docker compose down
docker-down:
	@echo "Stopping Docker Compose..."
	docker-compose down

# Docker compose logs
docker-logs:
	docker-compose logs -f syncarr

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Update dependencies
update-deps:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

# Run the application (oneshot mode)
run-oneshot:
	@echo "Running $(BINARY_NAME) in oneshot mode..."
	./$(BINARY_NAME) --oneshot

# Validate configuration
validate:
	@echo "Validating configuration..."
	./$(BINARY_NAME) --validate

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  build-linux   - Build for Linux (Docker)"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  lint          - Lint code (requires golangci-lint)"
	@echo "  clean         - Clean build artifacts"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  docker-up     - Start with Docker Compose"
	@echo "  docker-down   - Stop Docker Compose"
	@echo "  docker-logs   - Show Docker Compose logs"
	@echo "  deps          - Install dependencies"
	@echo "  update-deps   - Update dependencies"
	@echo "  run-oneshot   - Run application in oneshot mode"
	@echo "  validate      - Validate configuration"
	@echo "  help          - Show this help" 