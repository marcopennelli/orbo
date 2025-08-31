# deploy/Makefile - Deployment Tasks
.PHONY: help build run install upgrade uninstall status logs clean

# Configuration
NAMESPACE ?= orbo-system
RELEASE_NAME ?= orbo
CHART_NAME ?= orbo
IMAGE_NAME = orbo
IMAGE_TAG ?= latest
VALUES_FILE ?= values.yaml
DEPLOYMENT_ENV ?= minikube
CONTAINER_TOOL ?= podman
REGISTRY ?= localhost

# Default target
help: ## Show this help message
	@echo "$(RELEASE_NAME) - Deployment Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Variables:"
	@echo "  NAMESPACE          Kubernetes namespace (default: $(NAMESPACE))"
	@echo "  RELEASE_NAME       Helm release name (default: $(RELEASE_NAME))"
	@echo "  IMAGE_TAG          Docker image tag (default: $(IMAGE_TAG))"
	@echo "  DEPLOYMENT_ENV     Deployment environment: minikube or production (default: $(DEPLOYMENT_ENV))"
	@echo "  VALUES_FILE        Helm values file (default: $(VALUES_FILE))"

# Container build targets
build: ## Build container image locally (using pre-built OpenCV)
	$(CONTAINER_TOOL) build -t $(IMAGE_NAME):$(IMAGE_TAG) -f Dockerfile.slim ..

build-full: ## Build container image with full OpenCV compilation
	$(CONTAINER_TOOL) build -t $(IMAGE_NAME):$(IMAGE_TAG) -f Dockerfile ..

run: ## Run container locally
	$(CONTAINER_TOOL) run --rm -it -p 8080:8080 \
		--device=/dev/video0:/dev/video0 \
		--group-add keep-groups \
		--privileged \
		-v /tmp/orbo-frames:/app/frames \
		-e LOG_LEVEL=debug \
		--name $(IMAGE_NAME) $(IMAGE_NAME):$(IMAGE_TAG)

# Minikube-specific targets
check-minikube: ## Check if Minikube is running
	@minikube status >/dev/null 2>&1 || { echo "ERROR: Minikube not running. Start with: minikube start"; exit 1; }
	@echo "✓ Minikube is running"

minikube-build: check-minikube ## Build Docker image in Minikube's Docker daemon
	@eval $$(minikube docker-env) && cd .. && docker build -t $(IMAGE_NAME):$(IMAGE_TAG) -f deploy/Dockerfile .

# Production build targets
docker-build: ## Build Docker image for production
	@cd .. && docker build -t $(IMAGE_NAME):$(IMAGE_TAG) -f deploy/Dockerfile .

docker-push: docker-build ## Push Docker image to registry
	@docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	@docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Kubernetes deployment targets
check-requirements: ## Check if required tools are installed
	@command -v kubectl >/dev/null 2>&1 || { echo "ERROR: kubectl required but not installed"; exit 1; }
	@command -v helm >/dev/null 2>&1 || { echo "ERROR: helm required but not installed"; exit 1; }
	@echo "✓ All requirements satisfied"

install: check-requirements ## Install the service using Helm
	@echo "Installing $(RELEASE_NAME)..."
ifeq ($(DEPLOYMENT_ENV),minikube)
	@$(MAKE) minikube-build
else
	@$(MAKE) docker-build
endif
	@helm upgrade --install $(RELEASE_NAME) ./helm/$(CHART_NAME) \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--values ./helm/$(CHART_NAME)/$(VALUES_FILE) \
		--set image.tag=$(IMAGE_TAG) \
		--wait --timeout 10m
	@$(MAKE) status

upgrade: check-requirements ## Upgrade the service
	@echo "Upgrading $(RELEASE_NAME)..."
ifeq ($(DEPLOYMENT_ENV),minikube)
	@$(MAKE) minikube-build
else
	@$(MAKE) docker-build
endif
	@helm upgrade $(RELEASE_NAME) ./helm/$(CHART_NAME) \
		--namespace $(NAMESPACE) \
		--values ./helm/$(CHART_NAME)/$(VALUES_FILE) \
		--set image.tag=$(IMAGE_TAG) \
		--wait --timeout 10m

uninstall: check-requirements ## Uninstall the service
	@helm uninstall $(RELEASE_NAME) --namespace $(NAMESPACE) || true

status: ## Show deployment status
	@echo "Deployment Status:"
	@echo "Namespace:"
	@kubectl get namespace $(NAMESPACE) 2>/dev/null || echo "  Namespace not found"
	@echo "Helm Release:"
	@helm list -n $(NAMESPACE) 2>/dev/null || echo "  No releases found"
	@echo "Pods:"
	@kubectl get pods -n $(NAMESPACE) -l app.kubernetes.io/name=$(CHART_NAME) 2>/dev/null || echo "  No pods found"

logs: ## Show application logs
	@kubectl logs -f deployment/$(RELEASE_NAME) -n $(NAMESPACE) --tail=50

debug: ## Debug deployment issues
	@echo "Debug Information:"
	@kubectl get pods -n $(NAMESPACE) -l app.kubernetes.io/name=$(CHART_NAME) --field-selector=status.phase!=Running 2>/dev/null || echo "No issues found"
	@kubectl get events -n $(NAMESPACE) --sort-by='.lastTimestamp' | tail -10

clean: ## Clean up resources
	@$(MAKE) uninstall
	@docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true

port-forward: ## Port forward the service
	@kubectl port-forward -n $(NAMESPACE) svc/$(RELEASE_NAME) 8080:8080

# Environment-specific shortcuts
install-minikube: ## Install with Minikube configuration
	@$(MAKE) install VALUES_FILE=values-minikube.yaml DEPLOYMENT_ENV=minikube

install-production: ## Install with production configuration
	@$(MAKE) install VALUES_FILE=values-production.yaml DEPLOYMENT_ENV=production

# Test camera access
test-camera: ## Test camera access in container
	@$(CONTAINER_TOOL) run --rm -it \
		--device=/dev/video0:/dev/video0 \
		$(IMAGE_NAME):$(IMAGE_TAG) \
		sh -c "ls -la /dev/video* || echo 'No video devices found'"