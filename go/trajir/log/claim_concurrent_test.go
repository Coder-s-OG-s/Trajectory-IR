package nodelog

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// Each hook is its own process with its own connection, and sessions in one
// project share nodes.sqlite. ClaimToolCall reads and then writes, so it must
// wait for the other writers instead of failing with SQLITE_BUSY.
func TestClaimToolCallWaitsForOtherConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()

	const writers, calls = 4, 40
	errs := make(chan error, writers*calls)
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			nl, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			defer nl.Close()
			traj := fmt.Sprintf("t%d", w)
			for step := 1; step <= calls; step++ {
				payload := map[string]any{"tool": "Bash", "args": map[string]any{"n": step}}
				claimed, err := nl.ClaimToolCall(step, payload, traj, "local", 2)
				if err != nil {
					errs <- fmt.Errorf("%s step %d: %w", traj, step, err)
					continue
				}
				if !claimed {
					errs <- fmt.Errorf("%s step %d: not claimed", traj, step)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// Hooks of two sessions can open a project's new nodes.sqlite at the same
// moment. Only one of them gets to switch it to WAL. The others must still
// open it, or a pre hook would block its tool call.
func TestOpenNewFileFromManyConnections(t *testing.T) {
	dir := t.TempDir()
	for round := 0; round < 20; round++ {
		path := filepath.Join(dir, fmt.Sprintf("nodes-%d.sqlite", round))
		errs := make(chan error, 6)
		var wg sync.WaitGroup
		for w := 0; w < 6; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				nl, err := Open(path)
				if err != nil {
					errs <- err
					return
				}
				_ = nl.Close()
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("round %d: %v", round, err)
		}
	}
}
