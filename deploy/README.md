# Kubernetes Watch Event Service - Deployment Guide

This directory contains Kubernetes manifests for deploying the watch event service that monitors cluster resources and provides a REST API for querying historical events.

## Overview

The watch server:
- Monitors Kubernetes resources cluster-wide using controller-runtime
- Stores full object snapshots in BadgerDB with 14-day retention
- Provides REST API compatible with the MCP server client
- Auto-discovers and watches custom CRDs
- Correlates Kubernetes Events with their target objects

## Prerequisites

- Kubernetes cluster (v1.28+)
- kubectl configured with cluster access
- Storage class for PersistentVolume provisioning
- Docker registry access for the watch-server image

## Building the Container Image

```bash
# From the repository root
cd /home/moritz/dev/mcp-toolkit

# Build the Go binary
go build -o watch-server ./cmd/watch-server

# Build Docker image (create Dockerfile first - see below)
docker build -t k8s-watch-server:latest -f deploy/Dockerfile .

# Push to your registry
docker tag k8s-watch-server:latest <your-registry>/k8s-watch-server:latest
docker push <your-registry>/k8s-watch-server:latest
```

## Deployment Steps

### 1. Create Namespace (Optional)

```bash
kubectl create namespace watch-system
```

If using a custom namespace, update the `namespace` field in all manifests.

### 2. Deploy RBAC

```bash
kubectl apply -f deploy/rbac.yaml
```

This creates:
- ServiceAccount `k8s-watch-server`
- ClusterRole with `get`, `list`, `watch` permissions on all resources
- ClusterRoleBinding

### 3. Deploy ConfigMap

```bash
kubectl apply -f deploy/configmap.yaml
```

The ConfigMap contains the resource watch configuration. Edit to:
- Add/remove resource types to watch
- Adjust retention period (default: 14 days)
- Change max query limit (default: 1000 events)

### 4. Deploy Service

```bash
kubectl apply -f deploy/service.yaml
```

This creates a ClusterIP service exposing port 8080.

### 5. Deploy StatefulSet

Before deploying, update the image reference in `statefulset.yaml`:

```yaml
image: <your-registry>/k8s-watch-server:latest
```

Optionally adjust:
- PVC storage size (default: 50Gi)
- Resource requests/limits
- StorageClass name

```bash
kubectl apply -f deploy/statefulset.yaml
```

### 6. Verify Deployment

```bash
# Check pod status
kubectl get pods -l app=k8s-watch-server

# Check logs
kubectl logs -f k8s-watch-server-0

# Check PVC
kubectl get pvc

# Test health endpoint
kubectl port-forward k8s-watch-server-0 8080:8080
curl http://localhost:8080/health
```

Expected log output:
```
Starting controller-runtime manager
Started watching /v1 (Pod)
Started watching /v1 (Node)
...
Cache synced successfully
Starting HTTP server on port 8080
```

## Configuration

### Resource Watch Configuration

Edit `deploy/configmap.yaml` to customize watched resources:

```yaml
resources:
  # Add custom resource
  - group: "example.com"
    version: v1
    kind: MyCustomResource
    plural: mycustomresources
    namespaced: true
```

Apply changes:
```bash
kubectl apply -f deploy/configmap.yaml
kubectl rollout restart statefulset/k8s-watch-server
```

### Environment Variables

Override configuration via environment variables in `statefulset.yaml`:

```yaml
env:
  - name: BADGER_PATH
    value: /data/watch-events
  - name: SERVER_PORT
    value: "8080"
  - name: CONFIG_PATH
    value: /config/resources.yaml
```

### Storage Configuration

Adjust PVC size based on cluster scale:

- **Small cluster** (<50 nodes, <1000 pods): 20Gi
- **Medium cluster** (50-200 nodes, 1000-10000 pods): 50Gi (default)
- **Large cluster** (>200 nodes, >10000 pods): 100Gi+

Storage estimation: ~5KB per event × event rate × 14 days

## API Usage

### Query Events by Time Range

```bash
curl "http://k8s-watch-server:8080/api/v1/events?start=2024-11-21T00:00:00Z&end=2024-11-21T23:59:59Z&namespace=default&resourceType=pods"
```

Parameters:
- `start`: RFC3339 timestamp (optional)
- `end`: RFC3339 timestamp (optional)
- `namespace`: Filter by namespace (optional)
- `resourceType`: Filter by resource type, e.g., `pods`, `deployments` (optional)
- `resourceName`: Filter by resource name (optional)
- `verb`: Filter by action: `create`, `update`, `delete` (optional)
- `user`: Filter by user (optional)
- `limit`: Max results (default/max: 1000)

Response headers:
- `X-Total-Count`: Number of events returned
- `X-Has-More`: `true` if more events available

### Get Object History

```bash
curl "http://k8s-watch-server:8080/api/v1/events/default/pods/my-app-abc123"
```

Returns:
```json
{
  "namespace": "default",
  "resourceType": "pods",
  "resourceName": "my-app-abc123",
  "watchEvents": [
    {
      "timestamp": "2024-11-21T10:30:00Z",
      "verb": "create",
      "resourceType": "pods",
      ...
    }
  ],
  "relatedEvents": [
    {
      "timestamp": "2024-11-21T10:30:05Z",
      "verb": "create",
      "resourceType": "events",
      "message": "Successfully pulled image",
      ...
    }
  ]
}
```

## Integration with MCP Server

Update the MCP server's `AUDIT_API_URL` environment variable:

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

Or if running the MCP server in-cluster:

```yaml
env:
  - name: AUDIT_API_URL
    value: http://k8s-watch-server:8080
```

## Monitoring

### Metrics

The server logs events as they're processed. Monitor logs for:
- Watch errors: `Error storing event`
- Query performance: Request durations in access logs
- Storage issues: `GC error`

### Health Check

```bash
kubectl exec k8s-watch-server-0 -- wget -qO- http://localhost:8080/health
```

### Database Statistics

BadgerDB files are in `/data/watch-events`. Check size:

```bash
kubectl exec k8s-watch-server-0 -- du -sh /data/watch-events
```

## Troubleshooting

### Pod Won't Start

Check RBAC permissions:
```bash
kubectl auth can-i list pods --as=system:serviceaccount:default:k8s-watch-server
```

Check PVC binding:
```bash
kubectl get pvc data-k8s-watch-server-0
kubectl describe pvc data-k8s-watch-server-0
```

### High Memory Usage

Reduce watched resource types in ConfigMap or increase memory limits:

```yaml
resources:
  limits:
    memory: "8Gi"  # Increase if needed
```

### Storage Full

Increase PVC size (requires storage class support for volume expansion):

```bash
kubectl edit pvc data-k8s-watch-server-0
# Update storage request, e.g., 100Gi
```

Or reduce retention period in ConfigMap:

```yaml
retentionDays: 7  # Reduce from 14 days
```

### No Events Returned

Check cache sync:
```bash
kubectl logs k8s-watch-server-0 | grep "Cache synced"
```

Verify watchers started:
```bash
kubectl logs k8s-watch-server-0 | grep "Started watching"
```

Test with broader query:
```bash
curl "http://k8s-watch-server:8080/api/v1/events?limit=10"
```

## Backup and Recovery

### Backup BadgerDB

```bash
# Create backup
kubectl exec k8s-watch-server-0 -- tar czf /tmp/backup.tar.gz /data/watch-events

# Copy to local machine
kubectl cp k8s-watch-server-0:/tmp/backup.tar.gz ./badger-backup.tar.gz
```

### Restore from Backup

```bash
# Scale down StatefulSet
kubectl scale statefulset k8s-watch-server --replicas=0

# Delete PVC (WARNING: destroys existing data)
kubectl delete pvc data-k8s-watch-server-0

# Recreate and restore
kubectl scale statefulset k8s-watch-server --replicas=1
# Wait for pod to be ready
kubectl cp ./badger-backup.tar.gz k8s-watch-server-0:/tmp/
kubectl exec k8s-watch-server-0 -- tar xzf /tmp/backup.tar.gz -C /
kubectl delete pod k8s-watch-server-0  # Restart to pick up restored data
```

## Scaling Considerations

### Single Replica Only

BadgerDB is a single-writer database. Do not scale replicas > 1.

For high availability:
- Use PVC with backup/snapshot capability
- Consider multi-AZ storage class
- Implement external backup strategy

### Large Clusters

For clusters exceeding 500 nodes or 50,000 pods:

1. **Selective watching**: Remove high-churn resources from ConfigMap (e.g., Events, Pods in certain namespaces)

2. **Increase resources**:
   ```yaml
   resources:
     requests:
       memory: "4Gi"
       cpu: "1000m"
     limits:
       memory: "8Gi"
       cpu: "4000m"
   ```

3. **Shorter retention**:
   ```yaml
   retentionDays: 7
   ```

4. **Larger storage**:
   ```yaml
   storage: 100Gi
   ```

## Uninstall

```bash
kubectl delete -f deploy/statefulset.yaml
kubectl delete -f deploy/service.yaml
kubectl delete -f deploy/configmap.yaml
kubectl delete -f deploy/rbac.yaml

# Delete PVC (WARNING: destroys all stored events)
kubectl delete pvc data-k8s-watch-server-0
```

## Support

For issues or questions:
1. Check logs: `kubectl logs k8s-watch-server-0`
2. Verify RBAC: Ensure ClusterRole permissions are correct
3. Check storage: Ensure PVC is bound and has sufficient space
4. Review configuration: Validate ConfigMap YAML syntax
