# Orbo - Video Alarm System
# Author: Marco Pennelli
#
# Run from project root: make -C deploy <target>
# Or from deploy dir:    make <target>

.PHONY: help gen clean build build-fresh test run docker lint fmt check-opencv install-deps dev test-camera minikube-build minikube-deploy

SERVICE_NAME := orbo
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
REGISTRY := localhost
NAMESPACE := $(or $(NAMESPACE),orbo)

# Project root (parent of deploy/)
ROOT_DIR := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
PROJECT_DIR := $(ROOT_DIR)/..

# Default target - show help
help: ## Show this help message
	@echo "$(SERVICE_NAME) - Deployment Commands"
	@echo ""
	@echo "Usage: make -C deploy [target]  (from project root)"
	@echo "       make [target]            (from deploy/)"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Generate code from design
gen: ## Generate Goa code from design
	cd $(PROJECT_DIR) && goa gen $(SERVICE_NAME)/design

# Clean generated files and binaries
clean: ## Clean generated files and binaries
	rm -rf $(PROJECT_DIR)/bin/
	rm -rf $(PROJECT_DIR)/gen/

# Build the service (without regenerating)
build: ## Build the service binary
	cd $(PROJECT_DIR) && CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" \
		-o bin/$(SERVICE_NAME) ./cmd/$(SERVICE_NAME)

# Build fresh (regenerate + build)
build-fresh: gen build ## Regenerate Goa code and build

# Run tests
test: ## Run tests with coverage
	cd $(PROJECT_DIR) && go test -v -race -coverprofile=coverage.out ./internal/...
	@if [ -f $(PROJECT_DIR)/coverage.out ]; then cd $(PROJECT_DIR) && go tool cover -html=coverage.out -o coverage.html; fi

# Run the service locally
run: build ## Run the service locally
	$(PROJECT_DIR)/bin/$(SERVICE_NAME) --host 0.0.0.0

# Run with environment variables
run-dev: build ## Run with development settings
	PRIMARY_DETECTOR=basic \
	YOLO_ENABLED=false \
	DINOV3_ENABLED=false \
	$(PROJECT_DIR)/bin/$(SERVICE_NAME) --host 0.0.0.0 --debug

# Build Docker image
docker: ## Build Docker image
	docker build -t $(REGISTRY)/$(SERVICE_NAME):$(VERSION) \
		-f $(ROOT_DIR)/Dockerfile $(PROJECT_DIR)

# Build YOLO service Docker image
docker-yolo: ## Build YOLO service Docker image
	docker build -t $(REGISTRY)/$(SERVICE_NAME)-yolo:$(VERSION) \
		-f $(PROJECT_DIR)/yolo-service/Dockerfile $(PROJECT_DIR)/yolo-service

# Lint code
lint: ## Lint code (requires golangci-lint)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install with:"; \
		echo "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	}
	cd $(PROJECT_DIR) && golangci-lint run ./...

# Format code
fmt: ## Format code
	cd $(PROJECT_DIR) && go fmt ./...
	cd $(PROJECT_DIR) && gofmt -s -w .

# Check if OpenCV is available
check-opencv: ## Check OpenCV installation
	@pkg-config --modversion opencv4 2>/dev/null || { \
		echo "OpenCV4 not found. Please install OpenCV development libraries."; \
		echo "Ubuntu/Debian: sudo apt-get install libopencv-dev"; \
		echo "macOS: brew install opencv"; \
		echo "Arch Linux: sudo pacman -S opencv"; \
		exit 1; \
	}
	@echo "✓ OpenCV $$(pkg-config --modversion opencv4) found"

# Install development dependencies
install-deps: ## Install development dependencies
	cd $(PROJECT_DIR) && go mod download
	go install goa.design/goa/v3/cmd/goa@latest

# Run in development mode with hot reload (requires air)
dev: ## Run with hot reload (requires 'air')
	@command -v air >/dev/null 2>&1 || { \
		echo "air not found. Install with:"; \
		echo "go install github.com/cosmtrek/air@latest"; \
		exit 1; \
	}
	cd $(PROJECT_DIR) && air

# Quick test for camera access
test-camera: ## Test camera access
	@ls -la /dev/video* 2>/dev/null || echo "No video devices found"
	@echo "Available video devices:"
	@for dev in /dev/video*; do \
		if [ -e "$$dev" ]; then \
			echo "  $$dev"; \
		fi; \
	done 2>/dev/null || echo "  None found"

# Minikube targets
minikube-build: ## Build and load image to Minikube
	eval $$(minikube docker-env) && \
	docker build -t $(SERVICE_NAME):$(VERSION) -f $(ROOT_DIR)/Dockerfile $(PROJECT_DIR)

minikube-build-yolo: ## Build and load YOLO image to Minikube
	eval $$(minikube docker-env) && \
	docker build -t $(SERVICE_NAME)-yolo:$(VERSION) -f $(PROJECT_DIR)/yolo-service/Dockerfile $(PROJECT_DIR)/yolo-service

minikube-deploy: ## Deploy to Minikube using Helm (uses values-cpu.yaml)
	helm upgrade --install $(SERVICE_NAME) $(ROOT_DIR)/helm/orbo \
		--namespace $(NAMESPACE) --create-namespace \
		-f $(ROOT_DIR)/helm/orbo/values-cpu.yaml \
		--set image.tag=$(VERSION)

minikube-deploy-cpu: ## Deploy with CPU-only detection (no GPU services)
	helm upgrade --install $(SERVICE_NAME) $(ROOT_DIR)/helm/orbo \
		--namespace $(NAMESPACE) --create-namespace \
		-f $(ROOT_DIR)/helm/orbo/values-cpu.yaml \
		--set image.tag=$(VERSION)

minikube-deploy-gpu: ## Deploy with GPU detection (YOLO)
	helm upgrade --install $(SERVICE_NAME) $(ROOT_DIR)/helm/orbo \
		--namespace $(NAMESPACE) --create-namespace \
		-f $(ROOT_DIR)/helm/orbo/values-gpu.yaml \
		--set image.tag=$(VERSION) \
		--set yolo.image.tag=$(VERSION)

minikube-deploy-test: ## Deploy with test configuration
	helm upgrade --install $(SERVICE_NAME) $(ROOT_DIR)/helm/orbo \
		--namespace $(NAMESPACE) --create-namespace \
		-f $(ROOT_DIR)/helm/orbo/values-test.yaml \
		--set image.tag=$(VERSION)

minikube-undeploy: ## Undeploy from Minikube
	helm uninstall $(SERVICE_NAME) --namespace $(NAMESPACE) || true

minikube-status: ## Check Minikube deployment status
	kubectl get pods -n $(NAMESPACE) -l app.kubernetes.io/name=$(SERVICE_NAME)
	kubectl get svc -n $(NAMESPACE) -l app.kubernetes.io/name=$(SERVICE_NAME)

minikube-logs: ## View Minikube deployment logs
	kubectl logs -n $(NAMESPACE) -l app.kubernetes.io/name=$(SERVICE_NAME) -f

# Docker Compose targets
compose-up: ## Start all services with docker-compose
	docker-compose -f $(ROOT_DIR)/docker-compose.dinov3.yml up -d

compose-down: ## Stop all services
	docker-compose -f $(ROOT_DIR)/docker-compose.dinov3.yml down

compose-logs: ## View docker-compose logs
	docker-compose -f $(ROOT_DIR)/docker-compose.dinov3.yml logs -f

# API documentation
docs: ## Show OpenAPI documentation location
	@echo "OpenAPI documentation is automatically generated in gen/http/"
	@echo "  - $(PROJECT_DIR)/gen/http/openapi.yaml"
	@echo "  - $(PROJECT_DIR)/gen/http/openapi3.yaml"
	@echo "  - $(PROJECT_DIR)/gen/http/openapi.json"
	@echo "  - $(PROJECT_DIR)/gen/http/openapi3.json"

# Test API endpoints using a curl pod
minikube-test: ## Test API endpoints in cluster
	@echo "Testing $(SERVICE_NAME) API endpoints..."
	@kubectl get pods -n $(NAMESPACE) -l app.kubernetes.io/name=$(SERVICE_NAME) --no-headers | grep -q Running || { echo "Error: No running pods found"; exit 1; }
	@SVC_IP=$$(kubectl get svc -n $(NAMESPACE) $(SERVICE_NAME) -o jsonpath='{.spec.clusterIP}'); \
	echo "Service IP: $$SVC_IP"; \
	echo ""; \
	echo "=== Health Check ==="; \
	kubectl run curl-test --image=curlimages/curl --rm -i --restart=Never -n $(NAMESPACE) -- curl -s http://$$SVC_IP:8080/healthz; \
	echo ""; \
	echo "=== System Status ==="; \
	kubectl run curl-test --image=curlimages/curl --rm -i --restart=Never -n $(NAMESPACE) -- curl -s http://$$SVC_IP:8080/api/v1/system/status; \
	echo ""; \
	echo "=== Detection Config ==="; \
	kubectl run curl-test --image=curlimages/curl --rm -i --restart=Never -n $(NAMESPACE) -- curl -s http://$$SVC_IP:8080/api/v1/config/detection; \
	echo ""; \
	echo "=== YOLO Config ==="; \
	kubectl run curl-test --image=curlimages/curl --rm -i --restart=Never -n $(NAMESPACE) -- curl -s http://$$SVC_IP:8080/api/v1/config/yolo; \
	echo ""; \
	echo "=== Cameras ==="; \
	kubectl run curl-test --image=curlimages/curl --rm -i --restart=Never -n $(NAMESPACE) -- curl -s http://$$SVC_IP:8080/api/v1/cameras; \
	echo ""

# Forward port for local access
minikube-port-forward: ## Forward port 8080 for local access
	@echo "Forwarding port 8080 to $(SERVICE_NAME) service..."
	@echo "Access API at: http://localhost:8080"
	@echo "Press Ctrl+C to stop"
	kubectl port-forward -n $(NAMESPACE) svc/$(SERVICE_NAME) 8080:8080

# End-to-end test
minikube-e2e: ## Run end-to-end API tests
	@$(ROOT_DIR)/scripts/e2e-test.sh $(NAMESPACE)
