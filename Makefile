# Makefile for Kubernetes Watch Event Service

# Variables
APP_NAME := k8s-watch-server
MCP_NAME := k8s-audit-server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Container registry settings
REGISTRY ?= docker.io
IMAGE_REPO ?= $(REGISTRY)/$(APP_NAME)
IMAGE_TAG ?= $(VERSION)
IMAGE := $(IMAGE_REPO):$(IMAGE_TAG)

# Go build settings
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0
LDFLAGS := -w -s -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

# Directories
BUILD_DIR := build
DEPLOY_DIR := deploy
HELM_DIR := helm/k8s-watch-server

# Targets
.PHONY: all
all: clean build

.PHONY: help
help: ## Display this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

.PHONY: build
build: build-mcp build-watch ## Build both MCP and watch server binaries

.PHONY: build-mcp
build-mcp: ## Build MCP server binary
	@echo "Building MCP server..."
	@mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(MCP_NAME) \
		./cmd/server

.PHONY: build-watch
build-watch: ## Build watch server binary
	@echo "Building watch server..."
	@mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP_NAME) \
		./cmd/watch-server

.PHONY: build-local
build-local: ## Build binaries for local OS
	@echo "Building for local platform..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(MCP_NAME) ./cmd/server
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/watch-server

##@ Docker

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image: $(IMAGE)"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE) \
		-t $(IMAGE_REPO):latest \
		-f $(DEPLOY_DIR)/Dockerfile \
		.

.PHONY: docker-push
docker-push: ## Push Docker image to registry
	@echo "Pushing Docker image: $(IMAGE)"
	docker push $(IMAGE)
	docker push $(IMAGE_REPO):latest

.PHONY: docker-build-push
docker-build-push: docker-build docker-push ## Build and push Docker image

.PHONY: docker-run
docker-run: ## Run Docker container locally
	@echo "Running Docker container..."
	docker run --rm -it \
		-p 8080:8080 \
		-v $(PWD)/data:/data \
		-v $(PWD)/deploy/configmap.yaml:/config/resources.yaml \
		$(IMAGE)

##@ Development

.PHONY: test
test: ## Run tests
	go test -v -race -coverprofile=coverage.out ./...

.PHONY: test-coverage
test-coverage: test ## Run tests with coverage report
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: fmt
fmt: ## Format Go code
	go fmt ./...

.PHONY: lint
lint: ## Run linters
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Tidy go modules
	go mod tidy

.PHONY: deps
deps: ## Download dependencies
	go mod download

##@ Kubernetes

.PHONY: k8s-deploy
k8s-deploy: ## Deploy to Kubernetes using kubectl
	kubectl apply -f $(DEPLOY_DIR)/rbac.yaml
	kubectl apply -f $(DEPLOY_DIR)/configmap.yaml
	kubectl apply -f $(DEPLOY_DIR)/service.yaml
	kubectl apply -f $(DEPLOY_DIR)/statefulset.yaml

.PHONY: k8s-delete
k8s-delete: ## Delete Kubernetes resources
	kubectl delete -f $(DEPLOY_DIR)/statefulset.yaml --ignore-not-found=true
	kubectl delete -f $(DEPLOY_DIR)/service.yaml --ignore-not-found=true
	kubectl delete -f $(DEPLOY_DIR)/configmap.yaml --ignore-not-found=true
	kubectl delete -f $(DEPLOY_DIR)/rbac.yaml --ignore-not-found=true

.PHONY: k8s-logs
k8s-logs: ## Tail logs from watch server pod
	kubectl logs -f -l app=k8s-watch-server

.PHONY: k8s-status
k8s-status: ## Check status of watch server deployment
	@echo "=== Pods ==="
	kubectl get pods -l app=k8s-watch-server
	@echo "\n=== Service ==="
	kubectl get svc k8s-watch-server
	@echo "\n=== PVC ==="
	kubectl get pvc -l app=k8s-watch-server

.PHONY: k8s-port-forward
k8s-port-forward: ## Port forward to watch server
	kubectl port-forward svc/k8s-watch-server 8080:8080

##@ Helm

.PHONY: helm-package
helm-package: ## Package Helm chart
	@echo "Packaging Helm chart..."
	helm package $(HELM_DIR) -d $(BUILD_DIR)

.PHONY: helm-install
helm-install: ## Install Helm chart
	helm upgrade --install k8s-watch-server $(HELM_DIR) \
		--set image.repository=$(IMAGE_REPO) \
		--set image.tag=$(IMAGE_TAG)

.PHONY: helm-upgrade
helm-upgrade: ## Upgrade Helm release
	helm upgrade k8s-watch-server $(HELM_DIR) \
		--set image.repository=$(IMAGE_REPO) \
		--set image.tag=$(IMAGE_TAG)

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall Helm release
	helm uninstall k8s-watch-server

.PHONY: helm-template
helm-template: ## Template Helm chart (dry-run)
	helm template k8s-watch-server $(HELM_DIR) \
		--set image.repository=$(IMAGE_REPO) \
		--set image.tag=$(IMAGE_TAG)

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	helm lint $(HELM_DIR)

##@ Release

.PHONY: release
release: clean build docker-build-push helm-package ## Build, push image, and package Helm chart

.PHONY: release-local
release-local: clean build-local ## Build binaries for local use

##@ Cleanup

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -f k8s-audit-server watch-server

.PHONY: clean-docker
clean-docker: ## Remove Docker images
	docker rmi $(IMAGE) $(IMAGE_REPO):latest || true

.PHONY: clean-all
clean-all: clean clean-docker ## Clean everything

##@ Utilities

.PHONY: version
version: ## Display version information
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Image:      $(IMAGE)"

.PHONY: install-tools
install-tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Default target
.DEFAULT_GOAL := help
