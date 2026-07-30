//go:build temporal_integration

package temporal_test

import (
	"context"
	"testing"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable/temporal"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// Requires a local Temporal server and a worker on the task queue.
//
//	TEMPORAL_HOSTPORT=localhost:7233 go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
func TestTemporalBackendMemoRoundTrip(t *testing.T) {
	cfg := temporal.ConfigFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := client.Dial(client.Options{HostPort: cfg.HostPort, Namespace: cfg.Namespace})
	if err != nil {
		t.Skipf("temporal not available: %v", err)
	}
	defer c.Close()

	w, err := temporal.NewWorker(c, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = w.Run(worker.InterruptCh())
	}()
	defer w.Stop()

	// Give the worker a moment to poll.
	time.Sleep(500 * time.Millisecond)

	b, err := temporal.NewBackend(c, cfg.TaskQueue)
	if err != nil {
		t.Fatal(err)
	}
	// Do not Close b: it would close the shared client before worker drain.

	wf := "integration-wf-" + time.Now().Format("150405.000")
	key := "infer:model"

	// First Step executes fn and Saves via Temporal.
	var calls int
	v1, err := durable.Step(ctx, b, wf, key, func(context.Context) (any, error) {
		calls++
		return map[string]any{"n": float64(1)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}

	v2, err := durable.Step(ctx, b, wf, key, func(context.Context) (any, error) {
		calls++
		return map[string]any{"n": float64(99)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("fn re-ran: calls=%d", calls)
	}
	m1 := v1.(map[string]any)
	m2 := v2.(map[string]any)
	if m1["n"] != m2["n"] {
		t.Fatalf("%v vs %v", v1, v2)
	}
}
