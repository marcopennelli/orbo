# Project Root Makefile - Development Tasks
.PHONY: help gen clean build test run docker docs lint fmt

SERVICE_NAME := orbo
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
REGISTRY := localhost

# Default target - show help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Development Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Generate code from design
gen: clean ## Generate code from design
	goa gen $(SERVICE_NAME)/design
	goa example $(SERVICE_NAME)/design

# Clean generated files
clean: ## Clean generated files (except main.go and http.go in cmd/)
	rm -f *.go

# Generate documentation
docs: ## Generate API documentation
	@mkdir -p docs/api
	goa gen $(SERVICE_NAME)/design -o docs/api/openapi.yaml

# Build the service
build: gen ## Build the service binary
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" \
		-o bin/$(SERVICE_NAME) ./cmd/$(SERVICE_NAME)

# Run tests
test: ## Run tests with coverage
	go test -v -race -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html

# Run the service locally
run: build ## Run the service locally
	./bin/$(SERVICE_NAME)

# Build Docker image
docker: test ## Build Docker image
	docker build -t $(REGISTRY)/$(SERVICE_NAME):$(VERSION) \
		-f deploy/Dockerfile .

# Lint code
lint: ## Lint code (requires golangci-lint)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install with:"; \
		echo "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	}
	golangci-lint run ./...

# Format code
fmt: ## Format code
	go fmt ./...
	gofmt -s -w .

# Check if OpenCV is available
check-opencv: ## Check OpenCV installation
	@pkg-config --modversion opencv4 2>/dev/null || { \
		echo "OpenCV4 not found. Please install OpenCV development libraries."; \
		echo "Ubuntu/Debian: sudo apt-get install libopencv-dev"; \
		echo "macOS: brew install opencv"; \
		echo "Arch Linux: sudo pacman -S opencv"; \
		exit 1; \
	}
	@echo "✓ OpenCV $(shell pkg-config --modversion opencv4) found"

# Install development dependencies
install-deps: ## Install development dependencies
	go mod download
	go install goa.design/goa/v3/cmd/goa@latest

# Run in development mode with hot reload (requires air)
dev: ## Run with hot reload (requires 'air')
	@command -v air >/dev/null 2>&1 || { \
		echo "air not found. Install with:"; \
		echo "go install github.com/cosmtrek/air@latest"; \
		exit 1; \
	}
	air

# Quick test for camera access
test-camera: ## Test camera access
	@ls -la /dev/video* 2>/dev/null || echo "No video devices found"
	@echo "Available video devices:"
	@for dev in /dev/video*; do \
		if [ -e "$$dev" ]; then \
			echo "  $$dev ($(shell stat -c '%A' $$dev 2>/dev/null || stat -f '%Sp' $$dev 2>/dev/null))"; \
		fi; \
	done 2>/dev/null || echo "  None found"