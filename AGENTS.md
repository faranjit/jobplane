This file defines the **authoritative rules for all AI agents** (Claude, Codex, Gemini, etc.) interacting with the `jobplane` project.

Its purpose is to **prevent hallucination, scope creep, and architectural drift** by establishing a single, shared contract for reasoning, design, and code generation.

The `jobplane` Design_Document.md is the **Single Source of Truth**. These instructions explain how to use it. The `roadmap.md` defines the **development plan and feature sequencing** — agents should be aware of it but the Design Document takes precedence for architectural questions.

---

## 1. Source of Truth

* The **Project Design Document (Design_Document.md)** is authoritative for architecture, data model, interfaces, and API contracts.
* The **Roadmap (roadmap.md)** is authoritative for feature scope, sequencing, and priorities.
* If information is missing or ambiguous:

  * Do **not** guess.
  * Either ask for clarification **or** propose a clearly labeled design addition.
* Never override or reinterpret documented decisions implicitly.

---

## 2. Architecture Invariants (Non‑Negotiable)

All agents must preserve the following invariants:

### 2.1 Control Plane vs Data Plane

* **Control Plane:**

  * Stateless HTTP API
  * Owns intent, validation, lifecycle, persistence
  * Hosts the Workflow Engine (orchestration logic only)
  * Delivers webhook callbacks
  * **Never executes jobs**

* **Data Plane:**

  * Executes jobs
  * Owns concurrency, timeouts, retries, process/runtime management
  * Collects results and artifacts from containers
  * Registers with controller and reports labels

### 2.2 Execution Ownership

* Jobs run **outside** the Control Plane
* Execution happens via a pluggable **Runtime** abstraction
* The Workflow Engine enqueues steps via the existing Queue interface — it does **not** execute anything

### 2.3 PostgreSQL as System of Record

* PostgreSQL owns all durable state
* PostgreSQL is also used as a transactional queue
* Queue semantics rely on `SELECT … FOR UPDATE SKIP LOCKED`
* No Redis, no Kafka, no external message broker — this is a core design principle, not a temporary simplification

### 2.4 At‑Least‑Once Semantics

* Duplicate execution is possible
* Systems must be designed for idempotency

### 2.5 Multi‑Tenancy

* Every entity and operation is scoped by `tenant_id`
* No cross‑tenant data access, logs, artifacts, metrics, or workflow visibility

---

## 3. Scope Discipline

### 3.1 In Scope (Implemented or on Roadmap)

The following features exist or are planned. Agents may work on them freely within the architectural invariants:

* Priority queues (0–100) with ordered dequeue
* Scheduled execution via `visible_after` / `scheduled_at`
* Dead letter queue with retry lineage
* Per-tenant rate limiting (API + execution concurrency)
* OpenTelemetry traces + Prometheus metrics
* Kubernetes Job runtime + Helm chart
* Workflow / DAG execution (engine inside controller)
* Structured execution results (`result` JSONB column)
* Webhook callbacks on execution completion
* Artifact storage (local filesystem, later S3/GCS)
* Worker registration with labels
* Constraint-based job routing (JSONB containment)
* Worker heartbeat and discovery
* Fan-out / fan-in workflow patterns
* Conditional step branching on result values
* Execution context injection (env vars)
* Go SDK and Python SDK
* Progress reporting for long-running jobs

### 3.2 Out of Scope (Do Not Introduce Unless Explicitly Requested)

Agents must **not** introduce the following unless the user explicitly asks:

* UI or frontend (a read-only dashboard is on the roadmap but is a separate phase)
* Kubernetes operators or CRDs (backlog — do not implement speculatively)
* Multi-region replication or HA control planes (backlog)
* Complex authentication (OAuth, JWT, SSO) — API key auth is the current model
* External message brokers (Redis, Kafka, RabbitMQ, NATS)
* Caching layers that break controller statelessness
* SDK-required job integration — the file convention (`/jobplane/output/result.json`) must always work without an SDK

If such features are discussed, they must be framed as **out‑of‑scope or future work**.

---

## 4. Workflow Engine Rules

The Workflow Engine is a critical component with specific constraints:

### 4.1 Location

* The engine lives **inside the Controller**, not as a separate service.
* Step transitions are triggered synchronously by `InternalUpdateResult` when an execution completes.

### 4.2 Responsibilities

* Validate DAGs at workflow creation time (cycle detection via topological sort).
* On step completion: mark step terminal → check failure policy → find ready downstream steps → inject dependency results → enqueue via Queue interface.
* Track aggregate workflow status from step states.

### 4.3 Constraints

* The engine must **never** execute jobs directly. It enqueues work via the existing Queue interface.
* The engine must handle concurrent step completion safely. When parallel steps complete simultaneously and share a downstream dependency, only one should trigger enqueue. Use database-level guards (e.g., `UPDATE ... WHERE status = 'WAITING'` with affected-row checks).
* Step output passing uses environment variables (`JOBPLANE_DEP_{STEP_ID}_RESULT`) and/or mounted files (`/jobplane/input/{step_id}/result.json`). Both mechanisms should be supported.
* Failure strategies are workflow-level: `fail_fast` (cancel remaining on first failure) and `continue_on_failure` (run everything possible). Per-step retry is handled by the existing job retry mechanism, not by the workflow engine.

### 4.4 What Agents Should NOT Do

* Do not propose a separate workflow execution service or a dedicated workflow queue.
* Do not introduce an event bus or pub/sub for step transitions — database state transitions are sufficient.
* Do not implement saga-pattern compensation or rollback steps unless explicitly requested.

---

## 5. Result & Artifact Rules

### 5.1 Structured Results

* Jobs write to `/jobplane/output/result.json` (file convention, language-agnostic).
* Workers read this file after job completion and include it in the result report to the controller.
* Results are stored as JSONB on the `executions` table.
* Size limit: 1MB. Larger outputs should use artifact storage.

### 5.2 Artifacts

* Jobs write to `/jobplane/output/artifacts/` directory.
* Workers upload artifacts to the configured `ArtifactStore` after job completion.
* Artifacts are scoped by `tenant_id` and `execution_id`.
* The `ArtifactStore` interface abstracts the storage backend. Do not hardcode filesystem paths in business logic.

### 5.3 Webhook Callbacks

* Callbacks are delivered asynchronously by the controller via a lightweight goroutine pool.
* Callbacks are **not** enqueued through the job queue (avoid circular dependency).
* Retry with exponential backoff (3 attempts). Callback failures do not affect execution status.
* Callback delivery is best-effort, not guaranteed. The `callback_status` field tracks delivery state.

---

## 6. Worker Registration & Routing Rules

### 6.1 Worker Lifecycle

* Workers self-assign a UUID on startup.
* Workers register with the controller (`POST /internal/workers/register`) reporting their labels.
* Workers send periodic heartbeats. The controller marks workers as `offline` if heartbeats go stale.
* Worker state is ephemeral — if a worker restarts, it re-registers.

### 6.2 Label-Based Routing

* Workers declare capabilities as JSONB labels: `{"gpu": "true", "region": "eu-west"}`.
* Jobs declare requirements as JSONB constraints: `{"gpu": "true"}`.
* Dequeue uses `constraints <@ labels` (JSONB containment) — the worker's labels must be a superset of the job's constraints.
* Jobs with no constraints (`NULL`) run on any worker.
* Agents must not introduce a separate routing/scheduler service. Routing happens at dequeue time via SQL.

---

## 7. Reliability Model

All suggestions and implementations must assume:

* **Graceful shutdown** via `SIGTERM`

  * Stop dequeuing new work
  * Allow in‑flight executions to finish (with deadline)

* **Timeout enforcement**

  * Every execution runs under `context.WithTimeout`

* **Retries**

  * Explicit retry count
  * Backoff via queue visibility (`visible_after`)
  * Exhausted retries → Dead Letter Queue

* **Crash safety**

  * Worker crashes must not lose jobs
  * Locks and transactions define ownership
  * Workflows must not get stuck if a worker dies mid-step — the existing retry/DLQ mechanism handles recovery

* **Rate limiting**

  * Per-tenant API rate limiting (token bucket, in-memory)
  * Per-tenant execution concurrency cap (enforced at dequeue time)

---

## 8. Observability Expectations

Agents should default to:

* Structured logging (JSON via `slog`)
* Correlation / request IDs propagated via context
* OpenTelemetry traces for HTTP requests, queue operations, and job execution spans
* Prometheus metrics for queue depth, execution counts, durations, and worker status
* Explicit execution state transitions
* Auditability via immutable execution history

Avoid vague guidance like "add monitoring." Be concrete: specify the metric name, the label dimensions, the instrumentation point.

---

## 9. Go‑Specific Principles

All Go code and advice must follow these principles:

* Explicit error handling and wrapping (use `fmt.Errorf("context: %w", err)`)
* `context.Context` passed through all boundaries
* Small, stable interfaces
* No global mutable state
* Bounded goroutines (no leaks) — use `errgroup` or explicit lifecycle management
* Preference for clarity over abstraction
* Table-driven tests where applicable
* Frameworks should be avoided unless they add clear value

---

## 10. Interfaces Are Contracts

* The `Runtime` interface defines **how jobs run**
* The `Queue` interface defines **how work is claimed**
* The `WorkflowEngine` interface defines **how steps are orchestrated**
* The `ArtifactStore` interface defines **how outputs are stored**

Agents must treat these as long‑term contracts:

* Extend only with strong justification
* Never break compatibility casually
* New methods require a clear use case — do not add speculative interface methods

---

## 11. Output Expectations

When responding, agents should:

* Be explicit and concrete
* Prefer diagrams, schemas, state machines, and examples
* Clearly label:

  * **facts** (from the Design Document)
  * **proposals** (new ideas)
  * **tradeoffs** (pros/cons)

* When proposing code changes, specify:

  * Which files are affected
  * Which interfaces are involved
  * Whether the change requires a database migration
  * Whether the change affects the API contract

Avoid speculation presented as truth.

---

## 12. Conflict Resolution

If an agent's suggestion conflicts with the Design Document:

1. The document **wins**
2. The agent should point out the conflict explicitly
3. A proposed revision may be suggested, but not assumed

If an agent's suggestion conflicts with the Roadmap:

1. The roadmap defines priorities, not hard constraints
2. The agent should note the sequencing concern
3. Out-of-order implementation is acceptable if the user explicitly requests it

---

## 13. Common Agent Mistakes to Avoid

| Mistake | Correct Approach |
|---------|-----------------|
| Proposing Redis/Kafka for workflow events | Use database state transitions. Postgres is the only infrastructure dependency. |
| Building a separate workflow executor service | The workflow engine lives in the controller and enqueues via the Queue interface. |
| Adding SDK-required integration patterns | The file convention (`/jobplane/output/result.json`) must always work. SDKs are convenience wrappers, not requirements. |
| Introducing caching that breaks controller statelessness | All durable state lives in Postgres. In-memory state (rate limit counters, worker heartbeat cache) is acceptable only if it's reconstructable and loss-tolerant. |
| Hardcoding storage paths in business logic | Use the `ArtifactStore` interface. Storage backends are pluggable. |
| Modifying interface contracts without justification | Interfaces are long-term contracts. Propose changes explicitly with rationale. |
| Ignoring multi-tenancy in new features | Every new entity, query, API endpoint, and metric must be scoped by `tenant_id`. |
| Implementing workflow compensation/rollback | Out of scope unless explicitly requested. `fail_fast` and `continue_on_failure` are the supported strategies. |

---

## 14. Guiding Principle

> **This project is a platform, not a toy.**

All reasoning should reflect production‑grade thinking, even when the implementation is intentionally minimal. The goal is a real open-source product that people can deploy, trust, and build on — not a portfolio demo.
