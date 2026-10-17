package durable

import (
	"path/filepath"
	"testing"
)

func TestOpenLocalSetsBusyTimeoutAndWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memo.sqlite")
	store, err := OpenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var mode string
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	var busy int
	if err := store.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" || busy != 5000 {
		t.Fatalf("journal=%s busy=%d", mode, busy)
	}
}
