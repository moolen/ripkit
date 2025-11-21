# Kubernetes Audit Log MCP Toolkit

A comprehensive toolkit for investigating Kubernetes incidents, consisting of:

1. **MCP Server** - Model Context Protocol server for Claude Desktop integration
2. **Watch Event Service** - In-cluster service that monitors Kubernetes resources and provides a REST API

## Overview

This toolkit enables deep investigation of Kubernetes cluster issues by capturing watch events from all cluster resources and making them queryable through a REST API. The MCP server provides Claude Desktop with tools and prompts to analyze these events effectively.

## Components

### 1. MCP Server (`k8s-audit-server`)

An MCP server that provides Claude Desktop with diagnostic tools for investigating Kubernetes incidents.

## Features

### Diagnostic Tools

- **check_node_health** - Detect NotReady nodes, resource pressure, network issues, and kubelet failures
- **check_pod_issues** - Find CrashLoopBackOff, ImagePullBackOff, OOMKilled, and probe failures
- **check_volume_issues** - Identify PVC pending, binding failures, and StorageClass errors
- **analyze_recent_changes** - Review recent deployments, configs, secrets, and network policy changes
- **investigate_pod_startup** - Deep dive into why a specific pod won't start
- **check_resource_limits** - Analyze CPU throttling, OOM kills, and resource exhaustion

### Resources

Direct access to audit log data via URIs:

- `audit://events/{namespace}` - All events for a namespace (last 24h)
- `audit://events/{namespace}/{resource-type}` - Filtered by resource type
- `audit://changes/{time-range}` - Recent modifications (1h, 24h, 7d)
- `audit://node-events/{node-name}` - Node-specific events

### Investigation Prompts

Guided workflows for common scenarios:

- **investigate_pod_failure** - Step-by-step pod failure investigation
- **diagnose_cluster_health** - Comprehensive cluster health check
- **analyze_deployment_rollout** - Deployment rollout troubleshooting
- **troubleshoot_volume_issues** - Volume and PVC problem resolution

### Watch Event Service

**Binary**: `watch-server` (64 MB)
**Source**: `cmd/watch-server/`
**Purpose**: Runs in-cluster to monitor Kubernetes resources and store events in BadgerDB

**Features**:
- Cluster-wide resource watching using controller-runtime
- BadgerDB storage with 14-day automatic retention
- Full object snapshots for historical analysis
- REST API compatible with MCP server
- Auto-discovery of custom CRDs
- Event correlation (Kubernetes Events linked to target objects)

**API Endpoints**:
- `GET /api/v1/events?start=...&end=...&namespace=...&resourceType=...` - Query events
- `GET /api/v1/events/{namespace}/{resourceType}/{name}` - Get object history with related events
- `GET /health` - Health check

See `deploy/README.md` for deployment guide.

## Prerequisites

- Go 1.24 or later
- Kubernetes cluster (for watch-server deployment)
- kubectl configured with cluster access (for watch-server)

## Quick Start

### Option 1: Use with Existing Audit API

If you already have a Kubernetes audit log REST API:

```bash
# Build the MCP server
go build -o k8s-audit-server ./cmd/server

# Configure and run
export AUDIT_API_URL="http://your-audit-api:8080"
./k8s-audit-server
```

### Option 2: Deploy Complete Stack (Recommended)

Deploy the watch event service to your cluster to capture real-time events:

```bash
# Build the watch server
go build -o watch-server ./cmd/watch-server

# Deploy to Kubernetes (see deploy/README.md for details)
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/configmap.yaml
kubectl apply -f deploy/service.yaml
kubectl apply -f deploy/statefulset.yaml

# Build the MCP server
go build -o k8s-audit-server ./cmd/server

# Configure to use the watch server
export AUDIT_API_URL="http://k8s-watch-server:8080"
```

See `deploy/README.md` for detailed deployment instructions.

## Components

### MCP Server

**Binary**: `k8s-audit-server` (9.9 MB)
**Source**: `cmd/server/`
**Purpose**: Provides Claude Desktop with tools and prompts for investigating Kubernetes issues

## Installation

See "Quick Start" section above for deployment options.

### Building from Source

```bash
# Clone the repository
git clone <repository-url>
cd mcp-toolkit

# Install dependencies
go mod download

# Build both binaries
go build -o k8s-audit-server ./cmd/server
go build -o watch-server ./cmd/watch-server
```

## Configuration

### MCP Server

Set the audit API URL via environment variable:

```bash
export AUDIT_API_URL="http://k8s-watch-server:8080"
```

If not set, defaults to `http://localhost:8080`.

Debugging MCP server

```
npx @modelcontextprotocol/inspector ./build/k8s-audit-server

```

### Watch Server

Configure via `/config/resources.yaml` (see `deploy/configmap.yaml`):

```yaml
discoverCRDs: true
storagePath: /data/watch-events
retentionDays: 14
serverPort: 8080
maxQueryLimit: 1000

resources:
  - group: ""
    version: v1
    kind: Pod
    plural: pods
    namespaced: true
  # ... add more resources
```

Environment variables:
- `BADGER_PATH` - Storage path (default: `/data/watch-events`)
- `CONFIG_PATH` - Config file path (default: `/config/resources.yaml`)
- `SERVER_PORT` - HTTP port (default: `8080`)

## Usage with Claude Desktop

Add to your Claude Desktop configuration (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "k8s-audit": {
      "command": "/path/to/k8s-audit-server",
      "env": {
        "AUDIT_API_URL": "http://k8s-watch-server.default.svc:8080"
      }
    }
  }
}
```

**Note**: If the MCP server runs outside the cluster, use port-forwarding:

```bash
kubectl port-forward svc/k8s-watch-server 8080:8080
```

Then configure `AUDIT_API_URL` as `http://localhost:8080`.

## Audit API Requirements

The watch server provides a REST API compatible with the MCP server. If you're using an external audit API instead, it must implement:

```
GET /api/v1/events?start=<RFC3339>&end=<RFC3339>&namespace=<ns>&resourceType=<type>&verb=<verb>
```

Response format:
```json
[
  {
    "timestamp": "2024-01-01T12:00:00Z",
    "verb": "update",
    "user": "system:k8s-watcher",
    "namespace": "default",
    "resourceType": "pods",
    "resourceName": "my-pod",
    "responseStatus": 200,
    "message": "Update pods my-pod",
    "objectChanges": { /* full object snapshot */ },
    "stage": "ResponseComplete",
    "requestURI": "/api/v1/namespaces/default/pods/my-pod"
  }
]
```

The watch server automatically provides this API when deployed in-cluster.

## Example Usage

### Investigating a Pod Failure

1. Use the prompt template:
```
Get prompt: investigate_pod_failure
Arguments:
  - pod_name: "my-app-pod"
  - namespace: "production"
  - time_window: "2 hours"
```

2. Or use tools directly:
```
Call tool: investigate_pod_startup
Arguments:
  - pod_name: "my-app-pod"
  - namespace: "production"
  - start_time: "2024-01-01T10:00:00Z"
  - end_time: "2024-01-01T12:00:00Z"
```

### Checking Cluster Health

```
Get prompt: diagnose_cluster_health
Arguments:
  - time_window: "24 hours"
  - focus_area: "all"
```

## Project Structure

```
mcp-toolkit/
├── cmd/
│   ├── server/              # MCP server for Claude Desktop
│   │   └── main.go
│   └── watch-server/        # Kubernetes watch event service
│       └── main.go
├── internal/
│   ├── audit/
│   │   └── client.go        # REST API client
│   ├── tools/               # MCP diagnostic tools
│   │   ├── node_health.go
│   │   ├── pod_volume.go
│   │   └── analysis.go
│   ├── resources/
│   │   └── handlers.go      # MCP resource handlers
│   ├── prompts/
│   │   └── handlers.go      # Investigation prompts
│   └── watch/               # Watch server internals
│       ├── api/             # REST API handlers
│       ├── config/          # Configuration
│       ├── models/          # Event transformation
│       ├── storage/         # BadgerDB storage
│       └── watchers/        # Controller-runtime watchers
├── deploy/                  # Kubernetes manifests
│   ├── configmap.yaml
│   ├── rbac.yaml
│   ├── service.yaml
│   ├── statefulset.yaml
│   ├── Dockerfile
│   └── README.md           # Deployment guide
├── go.mod
└── README.md
```

## Development

### Running MCP Server Locally

```bash
# Set audit API URL
export AUDIT_API_URL="http://localhost:8080"

# Run the server
go run ./cmd/server
```

The server communicates over stdio using the MCP protocol.

### Running Watch Server Locally

**Note**: The watch server requires a Kubernetes cluster and in-cluster configuration.

```bash
# For local development with kind/minikube
export KUBECONFIG=~/.kube/config

# Run the server (requires cluster access)
go run ./cmd/watch-server
```

### Testing with MCP Inspector

```bash
# Install MCP Inspector
npm install -g @modelcontextprotocol/inspector

# Test the MCP server
mcp-inspector go run ./cmd/server
```

## Error Handling

The server returns descriptive errors when:
- Audit API is unavailable
- No data exists for the specified time range
- Required parameters are missing
- API returns non-200 status codes

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.
