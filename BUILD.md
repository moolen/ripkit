# Build and Deployment Guide

This guide covers building, containerizing, and deploying the Kubernetes watch event service.

## Quick Start

### 1. Build Binaries Locally

```bash
# Build both MCP and watch server
make build-local

# Or build individually
make build-mcp      # Builds build/k8s-audit-server
make build-watch    # Builds build/k8s-watch-server

# Binaries will be in the build/ directory
ls -lh build/
```

### 2. Build Docker Image

```bash
# Build with default registry and version
make docker-build

# Build with custom registry and version
make docker-build REGISTRY=myregistry.io IMAGE_TAG=v1.0.0

# Build and push to registry
make docker-build-push REGISTRY=myregistry.io
```

### 3. Deploy with Helm

```bash
# Install with defaults
make helm-install

# Or manually
helm install k8s-watch-server helm/k8s-watch-server

# With custom image
helm install k8s-watch-server helm/k8s-watch-server \
  --set image.repository=myregistry.io/k8s-watch-server \
  --set image.tag=v1.0.0
```

## Makefile Reference

### Build Targets

```bash
make build              # Build both binaries for linux/amd64
make build-mcp          # Build MCP server only
make build-watch        # Build watch server only
make build-local        # Build for local OS (development)
```

### Docker Targets

```bash
make docker-build       # Build Docker image
make docker-push        # Push image to registry
make docker-build-push  # Build and push
make docker-run         # Run container locally
```

**Variables**:
- `REGISTRY` - Container registry (default: `docker.io`)
- `IMAGE_TAG` - Image tag (default: git version)
- `VERSION` - Software version (default: git describe)

### Development Targets

```bash
make test               # Run tests
make test-coverage      # Run tests with HTML coverage report
make fmt                # Format code with go fmt
make lint               # Run golangci-lint
make vet                # Run go vet
make tidy               # Tidy go modules
make deps               # Download dependencies
```

### Kubernetes Targets

```bash
make k8s-deploy         # Deploy using kubectl
make k8s-delete         # Delete resources
make k8s-logs           # Tail pod logs
make k8s-status         # Show deployment status
make k8s-port-forward   # Port forward to service
```

### Helm Targets

```bash
make helm-package       # Package chart to build/
make helm-install       # Install chart
make helm-upgrade       # Upgrade release
make helm-uninstall     # Uninstall release
make helm-template      # Template chart (dry-run)
make helm-lint          # Lint chart
```

### Release Targets

```bash
make release            # Full release: build, push, package
make release-local      # Build binaries locally
make version            # Show version info
```

### Cleanup Targets

```bash
make clean              # Remove build artifacts
make clean-docker       # Remove Docker images
make clean-all          # Clean everything
```

## Docker Build Examples

### Build with Custom Registry

```bash
# Docker Hub
make docker-build REGISTRY=docker.io IMAGE_REPO=myuser/k8s-watch-server

# GitHub Container Registry
make docker-build REGISTRY=ghcr.io IMAGE_REPO=myorg/k8s-watch-server

# Private registry
make docker-build REGISTRY=registry.example.com IMAGE_REPO=k8s/watch-server
```

### Build with Version Tag

```bash
# Explicit version
make docker-build IMAGE_TAG=v1.2.3

# Use git tag
make docker-build IMAGE_TAG=$(git describe --tags)

# Latest tag (default)
make docker-build
```

### Build Multi-Architecture Images

```bash
# Using Docker buildx
docker buildx create --use
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t myregistry.io/k8s-watch-server:latest \
  -f deploy/Dockerfile \
  --push \
  .
```

## Helm Deployment Examples

### Install with Default Values

```bash
helm install k8s-watch-server helm/k8s-watch-server
```

### Install with Custom Values File

Create `custom-values.yaml`:

```yaml
image:
  repository: myregistry.io/k8s-watch-server
  tag: v1.0.0

persistence:
  size: 100Gi
  storageClassName: fast-ssd

resources:
  limits:
    memory: 8Gi
    cpu: 4000m

config:
  retentionDays: 7
  maxQueryLimit: 5000
```

Install:

```bash
helm install k8s-watch-server helm/k8s-watch-server -f custom-values.yaml
```

### Install in Custom Namespace

```bash
kubectl create namespace watch-system
helm install k8s-watch-server helm/k8s-watch-server \
  --namespace watch-system
```

### Install with Inline Overrides

```bash
helm install k8s-watch-server helm/k8s-watch-server \
  --set image.repository=myregistry.io/k8s-watch-server \
  --set image.tag=v1.0.0 \
  --set persistence.size=100Gi \
  --set config.retentionDays=7
```

### Upgrade Existing Release

```bash
# Upgrade with new image
helm upgrade k8s-watch-server helm/k8s-watch-server \
  --set image.tag=v1.1.0

# Upgrade with new values
helm upgrade k8s-watch-server helm/k8s-watch-server \
  -f custom-values.yaml

# Upgrade with reuse of values
helm upgrade k8s-watch-server helm/k8s-watch-server \
  --reuse-values \
  --set image.tag=v1.2.0
```

### Verify Installation

```bash
# Check status
helm status k8s-watch-server

# List releases
helm list

# Get values
helm get values k8s-watch-server

# Get manifest
helm get manifest k8s-watch-server
```

## Development Workflow

### Local Development Loop

```bash
# 1. Make code changes
vim internal/watch/...

# 2. Format and vet
make fmt vet

# 3. Build locally
make build-local

# 4. Test binary (requires kubeconfig)
./build/k8s-watch-server

# 5. Run tests
make test
```

### Building for Release

```bash
# 1. Tag the release
git tag v1.0.0
git push --tags

# 2. Build everything
make release REGISTRY=myregistry.io

# This will:
# - Clean build artifacts
# - Build linux/amd64 binaries
# - Build Docker image
# - Push to registry
# - Package Helm chart

# 3. Helm chart will be in build/
ls build/*.tgz
```

### CI/CD Integration

Example GitHub Actions workflow:

```yaml
name: Build and Push

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: docker/login-action@v2
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      
      - name: Build and push
        run: |
          make release \
            REGISTRY=ghcr.io \
            IMAGE_REPO=${{ github.repository }} \
            VERSION=${{ github.ref_name }}
```

## Testing

### Run All Tests

```bash
make test
```

### Generate Coverage Report

```bash
make test-coverage
# Opens coverage.html in browser
```

### Run Linters

```bash
# Install golangci-lint first
make install-tools

# Run linter
make lint
```

### Test Docker Image Locally

```bash
# Build image
make docker-build

# Run with local data directory
make docker-run

# Or run manually with custom config
docker run --rm -it \
  -p 8080:8080 \
  -v $(pwd)/test-data:/data \
  -v $(pwd)/test-config.yaml:/config/resources.yaml \
  k8s-watch-server:latest
```

### Test Helm Chart

```bash
# Lint chart
make helm-lint

# Template and review output
make helm-template > rendered.yaml
less rendered.yaml

# Dry-run install
helm install k8s-watch-server helm/k8s-watch-server --dry-run --debug

# Install to test cluster
make helm-install

# Check status
kubectl get pods -l app.kubernetes.io/name=k8s-watch-server
kubectl logs -f -l app.kubernetes.io/name=k8s-watch-server

# Test API
kubectl port-forward svc/k8s-watch-server 8080:8080 &
curl http://localhost:8080/health

# Uninstall
make helm-uninstall
```

## Troubleshooting

### Build Issues

**Problem**: `go build` fails with missing dependencies

```bash
# Solution: Download dependencies
make deps
make tidy
```

**Problem**: Docker build fails

```bash
# Check Dockerfile syntax
docker build -f deploy/Dockerfile . --no-cache

# Check build context
docker build -f deploy/Dockerfile . --progress=plain
```

### Helm Issues

**Problem**: Chart validation fails

```bash
# Lint the chart
make helm-lint

# Template to see rendered output
helm template k8s-watch-server helm/k8s-watch-server --debug
```

**Problem**: Installation fails

```bash
# Check values
helm get values k8s-watch-server

# Check events
kubectl get events --sort-by='.lastTimestamp'

# Check pod status
kubectl describe pod -l app.kubernetes.io/name=k8s-watch-server
```

### Runtime Issues

**Problem**: Pod won't start

```bash
# Check logs
kubectl logs -l app.kubernetes.io/name=k8s-watch-server

# Check RBAC
kubectl auth can-i list pods \
  --as=system:serviceaccount:default:k8s-watch-server

# Check PVC
kubectl describe pvc -l app.kubernetes.io/name=k8s-watch-server
```

## Advanced Scenarios

### Building for Different Platforms

```bash
# Build for ARM64
make build GOARCH=arm64

# Build for Windows
make build GOOS=windows GOARCH=amd64
```

### Custom Build Flags

```bash
# Build with race detector
go build -race -o build/watch-server ./cmd/watch-server

# Build with debug symbols
go build -gcflags="all=-N -l" -o build/watch-server ./cmd/watch-server

# Build with custom ldflags
make build LDFLAGS="-X main.customVar=value"
```

### Packaging Helm Chart for Distribution

```bash
# Package chart
make helm-package

# This creates build/k8s-watch-server-0.1.0.tgz

# Upload to chart repository
helm repo add myrepo https://charts.example.com
helm push build/k8s-watch-server-0.1.0.tgz myrepo
```

## Environment Variables

The Makefile respects these environment variables:

- `REGISTRY` - Container registry (default: docker.io)
- `IMAGE_REPO` - Image repository name
- `IMAGE_TAG` - Image tag (default: git version)
- `VERSION` - Software version
- `GOOS` - Target OS for Go build
- `GOARCH` - Target architecture for Go build
- `CGO_ENABLED` - Enable/disable CGO (default: 0)

Example:

```bash
export REGISTRY=ghcr.io
export IMAGE_REPO=myorg/k8s-watch-server
export IMAGE_TAG=v2.0.0

make docker-build-push
```

## Summary

The Makefile and Helm chart provide:

✅ **Simple build commands** - `make build-local` for development
✅ **Docker automation** - `make docker-build-push` for releases
✅ **Helm deployment** - `make helm-install` for Kubernetes
✅ **Development tools** - `make test`, `make lint`, `make fmt`
✅ **Complete workflows** - `make release` for full release process
✅ **Customizable** - Override variables for different environments

Start with:
```bash
make help
```
