// Package worker contains the worker-specific logic for job execution.
package worker

import (
	"context"
	"time"

	"jobplane/internal/store"
	"jobplane/internal/worker/runtime"

	"github.com/google/uuid"
)

// Agent is the main worker agent that runs the pull-loop for job execution.
type Agent struct {
	queue     store.Queue
	runtime   runtime.Runtime
	tenantIDs []uuid.UUID
}

// New creates a new worker agent.
// tenantIDs: Optional. If provided, this worker only pulls jobs for these specific tenants.
func New(queue store.Queue, rt runtime.Runtime, tenantIDs []uuid.UUID) *Agent {
	return &Agent{
		queue:     queue,
		runtime:   rt,
		tenantIDs: tenantIDs,
	}
}

// Run starts the main pull-loop. It blocks until the context is cancelled.
// On SIGTERM, it stops dequeuing new work and allows in-flight executions to finish.
func (a *Agent) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.processOne(ctx)
		}
	}
}

func (a *Agent) processOne(ctx context.Context) {
	execID, payload, err := a.queue.Dequeue(ctx, a.tenantIDs)
	if err != nil {
		return
	}

	// Start Runtime
	handle, err := a.runtime.Start(ctx, runtime.StartOptions{Payload: payload})
	if err != nil {
		a.queue.Fail(ctx, nil, execID, "Failed to start runtime")
		return
	}

	// Wait for result
	result, err := handle.Wait(ctx)

	// Update Queue
	if result.ExitCode == 0 {
		a.queue.Complete(ctx, nil, execID, 0)
	} else {
		a.queue.Fail(ctx, nil, execID, result.Error.Error())
	}
}
