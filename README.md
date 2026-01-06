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
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│   │   HTTP API  │───▶│  Job Store  │───▶│    Queue    │        │
│   └─────────────┘    └─────────────┘    └─────────────┘        │
│         ▲                                      │                │
│         │                                      ▼                │
│   ┌─────┴─────┐                        ┌─────────────┐         │
│   │    CLI    │                        │  PostgreSQL │         │
│   └───────────┘                        └──────┬──────┘         │
└───────────────────────────────────────────────┼─────────────────┘
                                                │
┌───────────────────────────────────────────────┼─────────────────┐
│                        Data Plane             │                 │
│                                               ▼                 │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│   │   Worker    │───▶│   Runtime   │───▶│   Docker    │        │
│   │   Agent     │    │  Interface  │    │   / Exec    │        │
│   └─────────────┘    └─────────────┘    └─────────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

## Key Features

- **Multi-Tenant** – Every operation scoped by `tenant_id`
- **At-Least-Once Execution** – Designed for idempotency with retry semantics
- **Pluggable Runtimes** – Docker containers or raw processes
- **Postgres-Backed Queue** – `SELECT FOR UPDATE SKIP LOCKED` for reliable job claiming
- **Graceful Shutdown** – SIGTERM handling with in-flight execution completion
- **Log Streaming** – Real-time log streaming from workers to controller with batch optimization
- **Timeout Enforcement** – Hard deadlines via `context.WithTimeout`
- **Heartbeat-Based Visibility** – Long-running jobs extend queue visibility to prevent duplicate pickup

## Project Structure

```
jobplane/
├── cmd/
│   ├── controller/     # HTTP API server ("Brain")
│   ├── worker/         # Job executor agent ("Muscle")
│   └── cli/            # Developer terminal tool
├── internal/
│   ├── config/         # Environment configuration
│   ├── store/          # Database layer + queue
│   ├── worker/         # Agent logic + runtime interface
│   ├── controller/     # HTTP handlers + middleware
│   └── logger/         # Structured logging (slog)
└── pkg/api/            # Shared request/response types
```

## Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 16+
- Docker (optional, for container runtime)

### Build

```bash
make build-all
```

### Run

```bash
# Start PostgreSQL
docker run -d --name jobplane-db \
  -e POSTGRES_PASSWORD=secret \
  -e POSTGRES_DB=jobplane \
  -p 5432:5432 postgres:16

# Run controller
DATABASE_URL="postgres://postgres:secret@localhost:5432/jobplane?sslmode=disable" \
  ./bin/controller

# Run worker (in another terminal)
DATABASE_URL="postgres://postgres:secret@localhost:5432/jobplane?sslmode=disable" \
  ./bin/worker
```

### Submit a Job

```bash
./bin/jobctl submit --name "hello" --image "alpine:latest" --command "echo", "Hello, jobplane!"
```

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

# Lint
make lint

# Format
go fmt ./...
```

## License

MIT