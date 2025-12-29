This file defines the **authoritative rules for all AI agents** (Claude, Codex, Gemini, etc.) interacting with the `jobplane` project.

Its purpose is to **prevent hallucination, scope creep, and architectural drift** by establishing a single, shared contract for reasoning, design, and code generation.

The `jobplane` Design_Document.md is the **Single Source of Truth**. These instructions explain how to use it.

---

## 1. Source of Truth

* The **Project Design Document (Design_Document.md)** is authoritative.
* If information is missing or ambiguous:

  * Do **not** guess.
  * Either ask for clarification **or** propose a clearly labeled design addition.
* Never override or reinterpret documented decisions implicitly.

---

## 2. Architecture Invariants (Non‑Negotiable)

All agents must preserve the following invariants:

1. **Control Plane vs Data Plane**

   * Control Plane:

     * Stateless HTTP API
     * Owns intent, validation, lifecycle, persistence
     * Never executes jobs
   * Data Plane:

     * Executes jobs
     * Owns concurrency, timeouts, retries, process/runtime management

2. **Execution Ownership**

   * Jobs run **outside** the Control Plane
   * Execution happens via a pluggable **Runtime** abstraction

3. **PostgreSQL as System of Record**

   * PostgreSQL owns all durable state
   * PostgreSQL is also used as a transactional queue
   * Queue semantics rely on `SELECT … FOR UPDATE SKIP LOCKED`

4. **At‑Least‑Once Semantics**

   * Duplicate execution is possible
   * Systems must be designed for idempotency

5. **Multi‑Tenancy**

   * Every entity and operation is scoped by `tenant_id`
   * No cross‑tenant data access, logs, artifacts, or metrics

---

## 3. Scope Discipline

Agents must **not** introduce the following unless explicitly requested:

* UI or frontend
* Cron scheduling or time‑based triggers
* Kubernetes operators or CRDs
* Multi‑region replication or HA control planes
* Complex authentication (OAuth, JWT, SSO)
* Priority queues or fair scheduling algorithms

If such features are discussed, they must be framed as **out‑of‑scope or future work**.

---

## 4. Reliability Model

All suggestions and implementations must assume:

* **Graceful shutdown** via `SIGTERM`

  * Stop dequeuing new work
  * Allow in‑flight executions to finish (with deadline)

* **Timeout enforcement**

  * Every execution runs under `context.WithTimeout`

* **Retries**

  * Explicit retry count
  * Backoff via queue visibility (`visible_after`)

* **Crash safety**

  * Worker crashes must not lose jobs
  * Locks and transactions define ownership

---

## 5. Observability Expectations

Agents should default to:

* Structured logging (JSON)
* Correlation / request IDs propagated via context
* Explicit execution state transitions
* Auditability via immutable execution history

Avoid vague guidance like “add monitoring.” Be concrete.

---

## 6. Go‑Specific Principles

All Go code and advice must follow these principles:

* Explicit error handling and wrapping
* `context.Context` passed through all boundaries
* Small, stable interfaces
* No global mutable state
* Bounded goroutines (no leaks)
* Preference for clarity over abstraction

Frameworks should be avoided unless they add clear value.

---

## 7. Interfaces Are Contracts

* The `Runtime` interface defines **how jobs run**
* The `Queue` interface defines **how work is claimed**

Agents must treat these as long‑term contracts:

* Extend only with strong justification
* Never break compatibility casually

---

## 8. Output Expectations

When responding, agents should:

* Be explicit and concrete
* Prefer diagrams, schemas, state machines, and examples
* Clearly label:

  * facts (from the document)
  * proposals (new ideas)
  * tradeoffs (pros/cons)

Avoid speculation presented as truth.

---

## 9. Conflict Resolution

If an agent’s suggestion conflicts with the Design Document:

1. The document **wins**
2. The agent should point out the conflict explicitly
3. A proposed revision may be suggested, but not assumed

---

## 10. Guiding Principle

> **This project is a platform, not a toy.**

All reasoning should reflect production‑grade thinking, even when the implementation is intentionally minimal.
