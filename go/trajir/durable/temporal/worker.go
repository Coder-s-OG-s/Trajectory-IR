package temporal

import (
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// NewWorker registers memo workflows on the configured task queue.
// The process must call Run (or Run with interrupt) to process tasks.
func NewWorker(c client.Client, cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("temporal: nil client")
	}
	w := worker.New(c, cfg.TaskQueue, worker.Options{})
	w.RegisterWorkflow(MemoWorkflow)
	return w, nil
}
