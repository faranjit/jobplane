# jobplane

A multi-tenant distributed job execution platform with explicit **control plane / data plane** separation.

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-336791?style=flat&logo=postgresql)](https://postgresql.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## Overview

jobplane allows teams to define background jobs, enqueue executions, and run them in isolated environments through a pluggable runtime layer. It mirrors real-world platform infrastructure with production-grade execution semantics.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Control Plane                            │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│   │   HTTP API  │───▶│  Job Store  │───▶│    Queue    │         │
│   └─────────────┘    └─────────────┘    └─────────────┘         │
│         ▲                                      │                │
│         │                                      ▼                │
│   ┌─────┴─────┐                        ┌─────────────┐          │
│   │    CLI    │                        │  PostgreSQL │          │
│   └───────────┘                        └──────┬──────┘          │
└───────────────────────────────────────────────┼─────────────────┘
                                                │
┌───────────────────────────────────────────────┼─────────────────┐
│                        Data Plane             │                 │
│                                               ▼                 │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│   │   Worker    │───▶│   Runtime   │───▶│ Kubernetes  │         │
│   │   Agent     │    │  Interface  │    │ Docker/Exec │         │
│   └─────────────┘    └─────────────┘    └─────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

## Key Features

- **Multi-Tenant** – Every operation scoped by `tenant_id`, per-tenant rate limiting and concurrency controls
- **Pluggable Runtimes** – Supports **Kubernetes Jobs**, Docker containers, and local processes (`exec`)
- **Structured Result Collection** – Jobs write `result.json`; results are persisted as JSONB in the database
- **Scheduled Execution** – Schedule jobs to run at a specific future time (RFC3339)
- **Priority Queues** – Prioritize critical workloads (Critical, High, Normal, Low)
- **Dead Letter Queue (DLQ)** – Automatic handling, inspection, and retry of permanently failed jobs
- **Postgres-Backed Queue** – `SELECT FOR UPDATE SKIP LOCKED` for reliable, transactional job claiming
- **Graceful Shutdown** – SIGTERM handling with in-flight execution completion
- **Log Streaming** – Real-time log streaming from workers to controller
- **Timeout Enforcement** – Hard deadlines via `context.WithTimeout`
- **Heartbeat-Based Visibility** – Long-running jobs extend queue visibility to prevent duplicate pickup
- **Observability** – OpenTelemetry tracing with Jaeger integration, Prometheus metrics
- **Auto-Migrations** – Schema migrations run automatically with `--migrate` flag
- **Rate Limiting** – Per-tenant request rate limiting with configurable limits

## Project Structure

```
jobplane/
├── cmd/
│   ├── controller/     # HTTP API server ("Brain")
│   ├── worker/         # Job executor agent ("Muscle")
│   └── cli/            # Developer terminal tool (jobctl)
├── internal/
│   ├── auth/           # Token validation
│   ├── config/         # Environment configuration
│   ├── controller/     # HTTP handlers + middleware
│   ├── logger/         # Structured logging (slog)
│   ├── observability/  # OpenTelemetry tracing setup
│   ├── store/          # Database layer + queue + migrations
│   └── worker/         # Agent logic + runtime interface
├── pkg/api/            # Shared request/response types
└── charts/             # Helm charts for Kubernetes deployment
```

## Quick Start

### Prerequisites

- Go 1.25+
- Docker (for PostgreSQL + Jaeger, and optional container runtime)

### Setup

```bash

# Required environment variables:
DATABASE_URL=postgres://user:password@localhost:5432/jobplane?sslmode=disable
SYSTEM_SECRET=<your-super-secret-key>
JOBPLANE_TOKEN=<your-api-token>
```

### Build

```bash
make build-all         # Build controller, worker, and CLI
make build-cli         # Build CLI only
```

### Run

```bash
# Terminal 1: Start PostgreSQL + Jaeger, run controller with auto-migration
make run-migrate

# Terminal 2: Start worker (uses RUNTIME env var: exec, docker, or kubernetes)
make run-worker
```

### Submit a Job

```bash
# Create and immediately run
./bin/jobctl submit --name "hello" --image "alpine:latest" \
  -c "echo" -c "Hello, jobplane!"

# With priority
./bin/jobctl submit --name "urgent" --image "alpine:latest" \
  -c "echo" -c "Fast!" --priority 100
```

### Result Collection

Jobs can write structured results that are persisted with the execution:

- **Exec runtime**: Write `result.json` in your working directory
- **Docker runtime**: Write `result.json` to `$JOBPLANE_OUTPUT_DIR/`

```bash
# Create a job that writes a result
./bin/jobctl create --name "compute" --image "exec" \
  -c "bash" -c "-c" -c 'echo {"score":61} > result.json'

# Run it
./bin/jobctl run <job-id>

# Check execution status and result
./bin/jobctl status <execution-id>
```

Results are stored as JSONB and returned via the `GET /executions/{id}` endpoint. Max result size: 1MB.

### Schedule a Job

```bash
# Create the job
./bin/jobctl create --name "nightly" --image "alpine" -c "echo" -c "nightly run"

# Run it at a specific time
./bin/jobctl run <job-id> --schedule "2024-12-31T23:59:00Z"
```

### Manage Failed Jobs (DLQ)

```bash
./bin/jobctl dlq list
./bin/jobctl dlq retry <execution-id>
```

## API Endpoints

### Public (Token Auth)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/tenants` | Create a new tenant |
| `PATCH` | `/tenants/{id}` | Update tenant settings |
| `POST` | `/jobs` | Create a job definition |
| `POST` | `/jobs/{id}/run` | Run a job (creates execution) |
| `GET` | `/executions/{id}` | Get execution status + result |
| `GET` | `/executions/{id}/logs` | Get execution logs |
| `GET` | `/executions/dlq` | List dead-letter queue |
| `POST` | `/executions/dlq/{id}/retry` | Retry a DLQ execution |

### Internal (System Secret Auth)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/internal/executions/dequeue` | Worker claims jobs |
| `PUT` | `/internal/executions/{id}/heartbeat` | Extend visibility |
| `PUT` | `/internal/executions/{id}/result` | Submit execution result |
| `POST` | `/internal/executions/{id}/logs` | Push execution logs |

## Architecture Invariants

| Principle | Implementation |
|-----------|----------------|
| Control plane is stateless | No in-memory job state; PostgreSQL owns all |
| Data plane executes jobs | Workers pull from queue, controller never runs jobs |
| PostgreSQL as system of record | Transactional queue with visibility semantics |
| At-least-once delivery | Jobs may run multiple times; design for idempotency |
| Multi-tenancy | All queries scoped by `tenant_id` |

## Development

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Lint
make lint

# Format
go fmt ./...
```

### Observability

Jaeger UI is available at [http://localhost:16686](http://localhost:16686) when running with `docker-compose`.

## License

MIT