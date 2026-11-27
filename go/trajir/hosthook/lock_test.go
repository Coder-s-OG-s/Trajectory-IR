package hosthook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStaleLockIsBroken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.x.lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-20 * time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	release, err := acquireLock(path, time.Second)
	if err != nil {
		t.Fatalf("stale lock not broken: %v", err)
	}
	release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock left behind: %v", err)
	}
}

func TestLockTimesOut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.x.lock")
	release, err := acquireLock(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = acquireLock(path, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("want timeout naming %s, got %v", path, err)
	}
}
