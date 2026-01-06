# jobplane Helm Chart

A Helm chart for deploying jobplane, a multi-tenant distributed job execution platform.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PostgreSQL database

## Installation

```bash
# Add the repository (if published)
# helm repo add jobplane https://...

# Install with default values
helm install jobplane ./charts/jobplane \
  --set database.host=my-postgres \
  --set database.password=secret

# Or use an existing database secret
helm install jobplane ./charts/jobplane \
  --set database.existingSecret=my-db-secret
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `ghcr.io/faranjit/jobplane` |
| `image.tag` | Image tag | `Chart.appVersion` |
| `controller.replicaCount` | Number of controller replicas | `1` |
| `controller.httpPort` | Controller API port | `6161` |
| `controller.migrate` | Run DB migrations on startup | `true` |
| `controller.resources` | Controller resource limits | See values.yaml |
| `worker.replicaCount` | Number of worker replicas | `1` |
| `worker.concurrency` | Jobs per worker | `1` |
| `worker.runtime` | Runtime type: docker/exec | `exec` |
| `worker.metricsPort` | Prometheus metrics port | `6162` |
| `worker.resources` | Worker resource limits | See values.yaml |
| `database.host` | PostgreSQL host | `""` |
| `database.port` | PostgreSQL port | `5432` |
| `database.name` | Database name | `jobplane` |
| `database.user` | Database user | `postgres` |
| `database.password` | Database password | `""` |
| `database.existingSecret` | Use existing secret | `""` |
| `observability.otelEndpoint` | OTEL collector endpoint | `localhost:4317` |
| `serviceAccount.create` | Create service account | `true` |

## Example: Production Deployment

```yaml
# values-production.yaml
controller:
  replicaCount: 2
  resources:
    limits:
      cpu: 1
      memory: 512Mi

worker:
  replicaCount: 5
  concurrency: 3
  resources:
    limits:
      cpu: 2
      memory: 1Gi

database:
  existingSecret: jobplane-db-credentials

observability:
  otelEndpoint: otel-collector:4317
```

```bash
helm install jobplane ./charts/jobplane -f values-production.yaml
```

## Upgrading

```bash
helm upgrade jobplane ./charts/jobplane
```

## Uninstalling

```bash
helm uninstall jobplane
```
