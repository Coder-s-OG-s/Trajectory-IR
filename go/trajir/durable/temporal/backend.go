package temporal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

// Backend implements durable.Backend using Temporal workflows as durable memos.
//
// Each (workflowID, stepKey) maps to one Temporal workflow whose result is the
// JSON step payload. The first Save starts that workflow; later Saves are no-ops
// when the workflow already exists. Load returns the completed workflow result.
//
// A worker registered via NewWorker must be running for Save to complete.
// This matches LocalSQLite semantics: Infer/Tool still execute fn in-process,
// then persist the result through the Backend. Moving fn into activities is a
// later hardening step.
type Backend struct {
	client    client.Client
	taskQueue string
	mu        sync.Mutex
	closed    bool
}

// Dial connects to Temporal and returns a Backend.
func Dial(ctx context.Context, cfg Config) (*Backend, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	c, err := client.Dial(cfg.ClientOptions())
	if err != nil {
		return nil, fmt.Errorf("temporal dial: %w", err)
	}
	return &Backend{client: c, taskQueue: cfg.TaskQueue}, nil
}

// NewBackend wraps an existing Temporal client.
func NewBackend(c client.Client, taskQueue string) (*Backend, error) {
	if c == nil {
		return nil, fmt.Errorf("temporal: nil client")
	}
	if taskQueue == "" {
		return nil, fmt.Errorf("temporal: taskQueue is required")
	}
	return &Backend{client: c, taskQueue: taskQueue}, nil
}

// Load implements durable.Backend.
func (b *Backend) Load(ctx context.Context, workflowID, stepKey string) ([]byte, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, false, fmt.Errorf("temporal: backend closed")
	}

	id := MemoWorkflowID(workflowID, stepKey)
	run := b.client.GetWorkflow(ctx, id, "")
	var value []byte
	err := run.Get(ctx, &value)
	if err != nil {
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			return nil, false, nil
		}
		// Workflow may still be running; treat as not ready / not found for Load.
		if isNotFoundMessage(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return value, true, nil
}

// Save implements durable.Backend. First successful start wins.
func (b *Backend) Save(ctx context.Context, workflowID, stepKey string, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("temporal: backend closed")
	}

	id := MemoWorkflowID(workflowID, stepKey)
	opts := client.StartWorkflowOptions{
		ID:                                       id,
		TaskQueue:                                b.taskQueue,
		WorkflowIDReusePolicy:                    enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}
	we, err := b.client.ExecuteWorkflow(ctx, opts, MemoWorkflow, value)
	if err != nil {
		if isAlreadyStarted(err) {
			return nil
		}
		return err
	}
	// Wait for completion so Load can see the result immediately after Save.
	return we.Get(ctx, nil)
}

// Close implements durable.Backend.
func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.client.Close()
	return nil
}

func isAlreadyStarted(err error) bool {
	if err == nil {
		return false
	}
	var already *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &already) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "already started") || strings.Contains(msg, "AlreadyStarted")
}

func isNotFoundMessage(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "NotFound") || strings.Contains(msg, "no rows")
}
