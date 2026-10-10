package nodelog

import (
	"path/filepath"
	"testing"
)

func TestOpenSetsBusyTimeoutAndWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	nl, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	var mode string
	if err := nl.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	var busy int
	if err := nl.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" || busy != 5000 {
		t.Fatalf("journal=%s busy=%d", mode, busy)
	}
}
