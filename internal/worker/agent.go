// Package worker contains the worker-specific logic for job execution.
package worker

import (
	"context"

	"jobplane/internal/store"
	"jobplane/internal/worker/runtime"
)

// Agent is the main worker agent that runs the pull-loop for job execution.
type Agent struct {
	queue   store.Queue
	runtime runtime.Runtime
}

// New creates a new worker agent.
func New(queue store.Queue, rt runtime.Runtime) *Agent {
	return &Agent{
		queue:   queue,
		runtime: rt,
	}
}

// Run starts the main pull-loop. It blocks until the context is cancelled.
// On SIGTERM, it stops dequeuing new work and allows in-flight executions to finish.
func (a *Agent) Run(ctx context.Context) error {
	// TODO: Implement pull-loop with graceful shutdown
	return nil
}
