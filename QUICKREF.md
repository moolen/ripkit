# Quick Reference - Kubernetes Watch Event Service

## Build Commands

```bash
# Build MCP server
go build -o k8s-audit-server ./cmd/server

# Build watch server
go build -o watch-server ./cmd/watch-server

# Build Docker image
docker build -t k8s-watch-server:latest -f deploy/Dockerfile .
```

## Deployment Commands

```bash
# Deploy complete stack
kubectl apply -f deploy/rbac.yaml
kubectl apply -f deploy/configmap.yaml
kubectl apply -f deploy/service.yaml
kubectl apply -f deploy/statefulset.yaml

# Verify deployment
kubectl get pods -l app=k8s-watch-server
kubectl logs -f k8s-watch-server-0
kubectl get pvc

# Check service
kubectl get svc k8s-watch-server
```

## API Testing

```bash
# Port forward for local testing
kubectl port-forward svc/k8s-watch-server 8080:8080

# Health check
curl http://localhost:8080/health

# Query events
curl "http://localhost:8080/api/v1/events?start=2024-11-21T00:00:00Z&limit=10"

# Query by namespace
curl "http://localhost:8080/api/v1/events?namespace=default&resourceType=pods"

# Get object history
curl "http://localhost:8080/api/v1/events/default/pods/my-pod-name"
```

## Configuration Updates

```bash
# Edit ConfigMap
kubectl edit configmap k8s-watch-config

# Or update from file
kubectl apply -f deploy/configmap.yaml

# Restart to apply changes
kubectl rollout restart statefulset/k8s-watch-server
```

## Monitoring

```bash
# Watch logs
kubectl logs -f k8s-watch-server-0

# Check storage usage
kubectl exec k8s-watch-server-0 -- du -sh /data/watch-events

# Check resource usage
kubectl top pod k8s-watch-server-0

# Describe pod
kubectl describe pod k8s-watch-server-0
```

## Troubleshooting

```bash
# Check RBAC permissions
kubectl auth can-i list pods --as=system:serviceaccount:default:k8s-watch-server

# Check PVC status
kubectl describe pvc data-k8s-watch-server-0

# Get events
kubectl get events --field-selector involvedObject.name=k8s-watch-server-0

# Execute shell in pod
kubectl exec -it k8s-watch-server-0 -- sh

# Check API response headers
curl -v "http://localhost:8080/api/v1/events?limit=1"
```

## Backup & Recovery

```bash
# Backup BadgerDB
kubectl exec k8s-watch-server-0 -- tar czf /tmp/backup.tar.gz /data/watch-events
kubectl cp k8s-watch-server-0:/tmp/backup.tar.gz ./badger-backup-$(date +%Y%m%d).tar.gz

# Restore (WARNING: destructive)
kubectl scale statefulset k8s-watch-server --replicas=0
kubectl delete pvc data-k8s-watch-server-0
kubectl scale statefulset k8s-watch-server --replicas=1
# Wait for pod ready
kubectl cp ./badger-backup.tar.gz k8s-watch-server-0:/tmp/
kubectl exec k8s-watch-server-0 -- tar xzf /tmp/backup.tar.gz -C /
kubectl delete pod k8s-watch-server-0
```

## Scaling Storage

```bash
# Check current PVC size
kubectl get pvc data-k8s-watch-server-0

# Edit PVC (requires storage class support for expansion)
kubectl edit pvc data-k8s-watch-server-0
# Update spec.resources.requests.storage to desired size

# Or patch directly
kubectl patch pvc data-k8s-watch-server-0 -p '{"spec":{"resources":{"requests":{"storage":"100Gi"}}}}'
```

## MCP Server Integration

```bash
# Run MCP server with watch service
export AUDIT_API_URL="http://localhost:8080"
./k8s-audit-server

# Test with MCP inspector
mcp-inspector ./k8s-audit-server

# Claude Desktop config (macOS)
cat > ~/Library/Application\ Support/Claude/claude_desktop_config.json <<EOF
{
  "mcpServers": {
    "k8s-audit": {
      "command": "/path/to/k8s-audit-server",
      "env": {
        "AUDIT_API_URL": "http://localhost:8080"
      }
    }
  }
}
EOF
```

## Common Query Examples

```bash
# Get all pod events in last hour
curl "http://localhost:8080/api/v1/events?resourceType=pods&start=$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)&end=$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Get create events only
curl "http://localhost:8080/api/v1/events?verb=create&limit=100"

# Get events for specific namespace
curl "http://localhost:8080/api/v1/events?namespace=kube-system"

# Get deployment rollout history
curl "http://localhost:8080/api/v1/events/default/deployments/my-app"

# Get pod with related Events
curl "http://localhost:8080/api/v1/events/default/pods/my-pod-abc123" | jq .
```

## Performance Tuning

```bash
# Adjust retention period (in ConfigMap)
retentionDays: 7  # Reduce from 14 to save space

# Increase memory limits (in StatefulSet)
resources:
  limits:
    memory: "8Gi"

# Reduce watched resources (in ConfigMap)
# Comment out high-churn resources like Events or Pods in certain namespaces
```

## Cleanup

```bash
# Delete all resources
kubectl delete -f deploy/statefulset.yaml
kubectl delete -f deploy/service.yaml
kubectl delete -f deploy/configmap.yaml
kubectl delete -f deploy/rbac.yaml

# Delete PVC (WARNING: destroys all data)
kubectl delete pvc data-k8s-watch-server-0
```

## Development

```bash
# Run locally (requires kubeconfig)
export KUBECONFIG=~/.kube/config
go run ./cmd/watch-server

# Run with custom config
export CONFIG_PATH=./local-config.yaml
go run ./cmd/watch-server

# Build with race detector
go build -race -o watch-server ./cmd/watch-server

# Run tests (when implemented)
go test ./internal/watch/...
```

## Useful kubectl Aliases

```bash
# Add to ~/.bashrc or ~/.zshrc
alias kwatch='kubectl get pods -l app=k8s-watch-server -w'
alias klogs='kubectl logs -f k8s-watch-server-0'
alias kexec='kubectl exec -it k8s-watch-server-0 -- sh'
alias kpf='kubectl port-forward svc/k8s-watch-server 8080:8080'
```
