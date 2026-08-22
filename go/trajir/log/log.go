// Package nodelog provides an append only, content addressed node log on SQLite.
// Behaviour matches pkg/trajectory_ir/runtime/log.py so Go and Python share one IR story.
//
// The import path is trajir/log; the package name is nodelog to avoid clashing with the
// standard library log package at call sites.
package nodelog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/postgres"

	_ "modernc.org/sqlite"
)

var ErrSlotConflict = postgres.ErrSlotConflict

// NodeLog is an append only SQLite log keyed by content addressed node id.
// INSERT OR IGNORE makes replaying the same node a no op, which is what durable
// step replay relies on.
type NodeLog struct {
	db *sql.DB
	mu sync.Mutex
}

// Open creates or opens a SQLite database at path and ensures the schema exists.
func Open(path string) (*NodeLog, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// One connection is enough for the local profile and keeps write order simple.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set wal: %w", err)
	}

	schema := `
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    trajectory_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    step_n INTEGER,
    seq INTEGER NOT NULL,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    ts REAL NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_slot
ON nodes (tenant_id, trajectory_id, IFNULL(step_n, -1), seq, kind);
CREATE INDEX IF NOT EXISTS idx_nodes_traj_tenant
ON nodes (trajectory_id, tenant_id);
`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &NodeLog{db: db}, nil
}

// Append builds a node via trajir/nodes and stores it. Same content id is
// ignored (idempotent replay). Different payload at the same logical slot
// returns ErrSlotConflict.
func (l *NodeLog) Append(
	kind string,
	stepN *int,
	payload map[string]any,
	trajectoryID, tenantID string,
	seq int,
) (*nodes.Node, error) {
	n, err := nodes.NewNode(kind, trajectoryID, tenantID, stepN, seq, payload)
	if err != nil {
		return nil, err
	}

	payloadJSON, err := json.Marshal(n.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	var step any
	if n.StepN != nil {
		step = *n.StepN
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	res, err := l.db.Exec(
		`INSERT OR IGNORE INTO nodes
		 (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID,
		n.TrajectoryID,
		n.TenantID,
		step,
		n.Seq,
		n.Kind,
		string(payloadJSON),
		n.TS,
	)
	if err != nil {
		return nil, fmt.Errorf("insert node: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 1 {
		return n, nil
	}

	owner, err := l.slotOwner(tenantID, trajectoryID, step, seq, kind)
	if err != nil {
		return nil, fmt.Errorf("slot owner: %w", err)
	}
	if owner != "" && owner != n.ID {
		return nil, fmt.Errorf("%w: trajectory=%s step=%v seq=%d kind=%s",
			ErrSlotConflict, trajectoryID, step, seq, kind)
	}
	return n, nil
}

func (l *NodeLog) slotOwner(tenantID, trajectoryID string, step any, seq int, kind string) (string, error) {
	var id string
	err := l.db.QueryRow(
		`SELECT id FROM nodes
		 WHERE tenant_id = ? AND trajectory_id = ?
		   AND IFNULL(step_n, -1) = IFNULL(?, -1)
		   AND seq = ? AND kind = ?`,
		tenantID, trajectoryID, step, seq, kind,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// ClaimToolCall atomically inserts a TOOL_CALL at (trajectory, step, seq).
// Returns claimed=true if this caller owns the slot and may run the tool.
// Returns claimed=false if the slot was already taken (block-and-gate).
func (l *NodeLog) ClaimToolCall(
	stepN int,
	payload map[string]any,
	trajectoryID, tenantID string,
	seq int,
) (claimed bool, err error) {
	n, err := nodes.NewNode("TOOL_CALL", trajectoryID, tenantID, &stepN, seq, payload)
	if err != nil {
		return false, err
	}
	payloadJSON, err := json.Marshal(n.Payload)
	if err != nil {
		return false, fmt.Errorf("marshal payload: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	tx, err := l.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var one int
	qerr := tx.QueryRow(
		`SELECT 1 FROM nodes WHERE tenant_id = ? AND trajectory_id = ? AND step_n = ? AND kind = ? AND seq = ? LIMIT 1`,
		tenantID, trajectoryID, stepN, "TOOL_CALL", seq,
	).Scan(&one)
	if qerr == nil {
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	if qerr != sql.ErrNoRows {
		err = qerr
		return false, err
	}

	_, err = tx.Exec(
		`INSERT INTO nodes
		 (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID,
		n.TrajectoryID,
		n.TenantID,
		stepN,
		n.Seq,
		n.Kind,
		string(payloadJSON),
		n.TS,
	)
	if err != nil {
		// Unique slot conflict under race: treat as not claimed.
		_ = tx.Rollback()
		return false, nil
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// Has reports whether a node of kind exists for trajectory and step scoped to tenantID.
// When seq is non nil, the match is limited to that sequence slot.
// tenantID must be non-empty; use HasAllTenants for cross-tenant administrative checks.
func (l *NodeLog) Has(trajectoryID, tenantID string, stepN int, kind string, seq *int) (bool, error) {
	if tenantID == "" {
		return false, fmt.Errorf("tenantID must be a non-empty string")
	}
	return l.has(trajectoryID, &tenantID, stepN, kind, seq)
}

// HasAllTenants reports whether a node of kind exists for trajectory and step across any tenant.
// This bypasses tenant isolation and is intended for administrative or diagnostic tools only.
func (l *NodeLog) HasAllTenants(trajectoryID string, stepN int, kind string, seq *int) (bool, error) {
	return l.has(trajectoryID, nil, stepN, kind, seq)
}

func (l *NodeLog) has(trajectoryID string, tenantID *string, stepN int, kind string, seq *int) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	q := `SELECT 1 FROM nodes WHERE trajectory_id = ? AND step_n = ? AND kind = ?`
	args := []any{trajectoryID, stepN, kind}
	if tenantID != nil {
		q += ` AND tenant_id = ?`
		args = append(args, *tenantID)
	}
	if seq != nil {
		q += ` AND seq = ?`
		args = append(args, *seq)
	}
	q += ` LIMIT 1`

	var one int
	err := l.db.QueryRow(q, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Count returns how many rows share nodeID (0 or 1 for a healthy log).
func (l *NodeLog) Count(nodeID string) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var n int
	err := l.db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, nodeID).Scan(&n)
	return n, err
}

// ListNodes returns nodes for a trajectory scoped to tenantID. tenantID is
// required so this method always enforces tenant isolation at the read path.
// Callers that genuinely need rows across tenants (e.g. tir.Export's own
// mixed-tenant check) must use ListNodesAllTenants explicitly instead.
func (l *NodeLog) ListNodes(trajectoryID, tenantID string) ([]map[string]any, error) {
	return l.listNodes(trajectoryID, &tenantID)
}

// ListNodesAllTenants returns nodes for a trajectory across all tenants.
// This bypasses tenant isolation and should only be used by code that
// performs its own tenant-scoping check on the result, such as tir.Export
// detecting a trajectory with mixed tenant_id values.
func (l *NodeLog) ListNodesAllTenants(trajectoryID string) ([]map[string]any, error) {
	return l.listNodes(trajectoryID, nil)
}

func (l *NodeLog) listNodes(trajectoryID string, tenantID *string) ([]map[string]any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	q := `SELECT id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts
	      FROM nodes WHERE trajectory_id = ?`
	args := []any{trajectoryID}
	if tenantID != nil {
		q += ` AND tenant_id = ?`
		args = append(args, *tenantID)
	}
	q += ` ORDER BY COALESCE(step_n, -1), seq`

	rows, err := l.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id, traj, tenant, kind, payloadJSON string
			stepN                               sql.NullInt64
			seq                                 int
			ts                                  float64
		)
		if err := rows.Scan(&id, &traj, &tenant, &stepN, &seq, &kind, &payloadJSON, &ts); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return nil, err
		}
		rec := map[string]any{
			"id":            id,
			"trajectory_id": traj,
			"tenant_id":     tenant,
			"seq":           seq,
			"kind":          kind,
			"payload":       payload,
			"ts":            ts,
		}
		if stepN.Valid {
			rec["step_n"] = int(stepN.Int64)
		} else {
			rec["step_n"] = nil
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Close releases the database handle.
func (l *NodeLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db == nil {
		return nil
	}
	err := l.db.Close()
	l.db = nil
	return err
}
