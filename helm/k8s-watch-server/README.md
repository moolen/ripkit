# k8s-watch-server Helm Chart

This Helm chart deploys the Kubernetes watch event service for incident investigation.

## Prerequisites

- Kubernetes 1.28+
- Helm 3.0+
- PV provisioner support in the underlying infrastructure (if persistence is enabled)

## Installing the Chart

To install the chart with the release name `k8s-watch-server`:

```bash
helm install k8s-watch-server ./helm/k8s-watch-server
```

Or from the repository root using the Makefile:

```bash
make helm-install
```

## Uninstalling the Chart

To uninstall/delete the `k8s-watch-server` deployment:

```bash
helm uninstall k8s-watch-server
```

Or using the Makefile:

```bash
make helm-uninstall
```

## Configuration

The following table lists the configurable parameters of the k8s-watch-server chart and their default values.

### General Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas (must be 1 for BadgerDB) | `1` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full chart name | `""` |

### Image Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Image repository | `k8s-watch-server` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Image tag (defaults to chart appVersion) | `""` |
| `imagePullSecrets` | Image pull secrets | `[]` |

### Service Account

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` |

### Service Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `8080` |
| `service.annotations` | Service annotations | `{}` |

### Resource Limits

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `2000m` |
| `resources.limits.memory` | Memory limit | `4Gi` |
| `resources.requests.cpu` | CPU request | `500m` |
| `resources.requests.memory` | Memory request | `2Gi` |

### Persistence

| Parameter | Description | Default |
|-----------|-------------|---------|
| `persistence.enabled` | Enable persistence | `true` |
| `persistence.storageClassName` | Storage class name | `""` |
| `persistence.accessMode` | Access mode | `ReadWriteOnce` |
| `persistence.size` | Volume size | `50Gi` |
| `persistence.annotations` | PVC annotations | `{}` |

### Watch Server Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.discoverCRDs` | Auto-discover CRDs | `true` |
| `config.storagePath` | BadgerDB storage path | `/data/watch-events` |
| `config.retentionDays` | Event retention period | `14` |
| `config.serverPort` | HTTP server port | `8080` |
| `config.maxQueryLimit` | Maximum query result limit | `1000` |
| `config.resources` | List of resources to watch | See `values.yaml` |

### Security

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext.fsGroup` | Pod FSGroup | `1000` |
| `podSecurityContext.runAsNonRoot` | Run as non-root | `true` |
| `podSecurityContext.runAsUser` | Run as user | `1000` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false` |
| `securityContext.capabilities.drop` | Capabilities to drop | `["ALL"]` |
| `securityContext.readOnlyRootFilesystem` | Read-only root filesystem | `false` |

### Probes

| Parameter | Description | Default |
|-----------|-------------|---------|
| `livenessProbe` | Liveness probe configuration | See `values.yaml` |
| `readinessProbe` | Readiness probe configuration | See `values.yaml` |

### RBAC

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rbac.create` | Create RBAC resources | `true` |

### Scheduling

| Parameter | Description | Default |
|-----------|-------------|---------|
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity | `{}` |
| `podAnnotations` | Pod annotations | `{}` |

## Examples

### Installing with custom image

```bash
helm install k8s-watch-server ./helm/k8s-watch-server \
  --set image.repository=myregistry/k8s-watch-server \
  --set image.tag=v1.0.0
```

### Installing with custom storage size

```bash
helm install k8s-watch-server ./helm/k8s-watch-server \
  --set persistence.size=100Gi \
  --set persistence.storageClassName=fast-ssd
```

### Installing with custom retention period

```bash
helm install k8s-watch-server ./helm/k8s-watch-server \
  --set config.retentionDays=7
```

### Installing with custom resource limits

```bash
helm install k8s-watch-server ./helm/k8s-watch-server \
  --set resources.limits.memory=8Gi \
  --set resources.limits.cpu=4000m
```

### Adding custom resources to watch

Create a custom `values.yaml`:

```yaml
config:
  resources:
    # Include default resources
    - group: ""
      version: v1
      kind: Pod
      plural: pods
      namespaced: true
    # ... other defaults
    
    # Add custom resource
    - group: example.com
      version: v1
      kind: MyCustomResource
      plural: mycustomresources
      namespaced: true
```

Install with custom values:

```bash
helm install k8s-watch-server ./helm/k8s-watch-server -f custom-values.yaml
```

## Upgrading

To upgrade the release with new configuration:

```bash
helm upgrade k8s-watch-server ./helm/k8s-watch-server \
  --set image.tag=v1.1.0
```

Or using the Makefile:

```bash
make helm-upgrade VERSION=v1.1.0
```

## Accessing the Service

After installation, follow the instructions in the NOTES to access the service:

```bash
# Port forward
kubectl port-forward svc/k8s-watch-server 8080:8080

# Test the API
curl http://localhost:8080/health
curl "http://localhost:8080/api/v1/events?limit=10"
```

## Integration with MCP Server

Configure the MCP server to use the watch server:

```bash
export AUDIT_API_URL="http://k8s-watch-server:8080"
```

If the MCP server runs outside the cluster, use port-forwarding as shown above.

## Troubleshooting

### Check pod status

```bash
kubectl get pods -l app.kubernetes.io/name=k8s-watch-server
```

### View logs

```bash
kubectl logs -f -l app.kubernetes.io/name=k8s-watch-server
```

### Check PVC

```bash
kubectl get pvc -l app.kubernetes.io/name=k8s-watch-server
```

### Verify RBAC permissions

```bash
kubectl auth can-i list pods --as=system:serviceaccount:default:k8s-watch-server
```

## Persistence

The chart mounts a PersistentVolume at `/data` for BadgerDB storage. By default, a PVC is created with 50Gi storage. The PVC is not deleted when the chart is uninstalled.

To delete the PVC:

```bash
kubectl delete pvc -l app.kubernetes.io/name=k8s-watch-server
```

## Security Considerations

- The chart runs the container as non-root user (UID 1000)
- All capabilities are dropped
- The service account has cluster-wide read permissions (watch, list, get) on all resources

## Notes

- **Single Replica**: The chart enforces `replicaCount: 1` because BadgerDB is a single-writer database
- **Storage**: Adjust `persistence.size` based on cluster scale and event volume
- **Retention**: Adjust `config.retentionDays` to balance storage vs. history depth
- **Resources**: Monitor memory usage and adjust limits if needed for large clusters
