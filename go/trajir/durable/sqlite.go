package durable

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// LocalSQLite is the Phase 1B coding default Backend. It is a test fake, a
// hand rolled first writer wins memoization store, not an integration with a
// real durable execution engine; see issue #92. Step memos survive process
// restart when the same database path is reopened (needed for crash style
// tests later).
type LocalSQLite struct {
	db *sql.DB
	mu sync.Mutex
}

// OpenLocal opens or creates a SQLite memo database at path.
func OpenLocal(path string) (*LocalSQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("durable sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)

	schema := `
CREATE TABLE IF NOT EXISTS step_memos (
    workflow_id TEXT NOT NULL,
    step_key TEXT NOT NULL,
    value_json BLOB NOT NULL,
    PRIMARY KEY (workflow_id, step_key)
)`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("durable sqlite schema: %w", err)
	}
	return &LocalSQLite{db: db}, nil
}

// Load implements Backend.
func (s *LocalSQLite) Load(ctx context.Context, workflowID, stepKey string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT value_json FROM step_memos WHERE workflow_id = ? AND step_key = ?`,
		workflowID, stepKey,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// Save implements Backend. First writer wins (INSERT OR IGNORE).
func (s *LocalSQLite) Save(ctx context.Context, workflowID, stepKey string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO step_memos (workflow_id, step_key, value_json) VALUES (?, ?, ?)`,
		workflowID, stepKey, value,
	)
	return err
}

// Close implements Backend.
func (s *LocalSQLite) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
