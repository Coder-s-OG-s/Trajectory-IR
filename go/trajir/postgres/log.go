// Package postgres provides a PostgreSQL backed NodeLog matching SQLite semantics.
//
// Hash identity stays in trajir/nodes (RFC 8785 + SHA-256). This package only
// persists rows. Schema aligns with drivers/postgres and infrastructure.md.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const defaultPGConnectTimeout = 10 * time.Second

// ErrSlotConflict is raised when a different payload already occupies the slot.
var ErrSlotConflict = errors.New("postgres: slot conflict")

// ErrNotStored means the append did not leave this node in the slot.
var ErrNotStored = errors.New("node not stored")

// NodeLog is an append only, content addressed IR log on PostgreSQL.
type NodeLog struct {
	db *sql.DB
	mu sync.Mutex
}

// OpenDSN opens a Postgres NodeLog from a connection string.
// Ping and schema setup use TRAJIR_PG_CONNECT_TIMEOUT (seconds), else the DSN
// connect_timeout, else 10s.
func OpenDSN(dsn string) (*NodeLog, error) {
	if dsn == "" {
		return nil, errors.New("postgres: DSN is required")
	}
	timeout, err := resolvePGConnectTimeout(dsn)
	if err != nil {
		return nil, err
	}
	effective, err := dsnWithConnectTimeout(dsn, timeout)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", effective)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	db.SetMaxOpenConns(4)
	pingCtx, pingCancel := context.WithTimeout(context.Background(), timeout)
	err = db.PingContext(pingCtx)
	pingCancel()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	nl := &NodeLog{db: db}
	schemaCtx, schemaCancel := context.WithTimeout(context.Background(), timeout)
	err = nl.ensureSchema(schemaCtx)
	schemaCancel()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return nl, nil
}

func resolvePGConnectTimeout(dsn string) (time.Duration, error) {
	if raw := strings.TrimSpace(os.Getenv("TRAJIR_PG_CONNECT_TIMEOUT")); raw != "" {
		sec, err := strconv.Atoi(raw)
		if err != nil || sec <= 0 {
			return 0, errors.New("postgres: TRAJIR_PG_CONNECT_TIMEOUT must be a positive integer number of seconds")
		}
		return time.Duration(sec) * time.Second, nil
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return 0, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	if cfg.ConnectTimeout > 0 {
		return cfg.ConnectTimeout, nil
	}
	return defaultPGConnectTimeout, nil
}

func dsnWithConnectTimeout(dsn string, timeout time.Duration) (string, error) {
	seconds := int(timeout / time.Second)
	if seconds < 1 {
		return "", errors.New("postgres: connect timeout must be at least 1s")
	}
	want := strconv.Itoa(seconds)
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("postgres: parse dsn: %w", err)
		}
		q := u.Query()
		if q.Get("connect_timeout") == want {
			return dsn, nil
		}
		q.Set("connect_timeout", want)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	parts := strings.Fields(dsn)
	for _, part := range parts {
		key, val, ok := strings.Cut(part, "=")
		if ok && strings.EqualFold(key, "connect_timeout") && val == want {
			return dsn, nil
		}
	}
	kept := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		key, _, ok := strings.Cut(part, "=")
		if ok && strings.EqualFold(key, "connect_timeout") {
			continue
		}
		kept = append(kept, part)
	}
	kept = append(kept, "connect_timeout="+want)
	return strings.Join(kept, " "), nil
}

// OpenFromEnv resolves TRAJIR_DATABASE_URL or DATABASE_URL and opens a log.
func OpenFromEnv() (*NodeLog, error) {
	dsn := os.Getenv("TRAJIR_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return nil, errors.New("postgres: set TRAJIR_DATABASE_URL or DATABASE_URL")
	}
	return OpenDSN(dsn)
}

// OpenDB wraps an existing *sql.DB (for tests or advanced wiring).
// The caller owns the connection lifecycle unless Close is used after OpenDB
// when the NodeLog is no longer needed; OpenDB does not close the DB on error
// after schema setup fails on a caller owned handle.
func OpenDB(db *sql.DB) (*NodeLog, error) {
	if db == nil {
		return nil, errors.New("postgres: db is required")
	}
	nl := &NodeLog{db: db}
	if err := nl.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return nl, nil
}

func (l *NodeLog) ensureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			trajectory_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			step_n INTEGER,
			seq INTEGER NOT NULL,
			kind TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			ts DOUBLE PRECISION NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_slot
			ON nodes (tenant_id, trajectory_id, COALESCE(step_n, -1), seq, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_traj_tenant
			ON nodes (trajectory_id, tenant_id)`,
	}
	for _, s := range stmts {
		if _, err := l.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("postgres: schema: %w", err)
		}
	}
	return nil
}

// Append stores a node. Same content id is ignored. Different payload at the
// same logical slot returns ErrSlotConflict. If the insert does not leave this
// node in the slot, Append returns an error.
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
		return nil, err
	}
	var step any
	if n.StepN != nil {
		step = *n.StepN
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	tx, err := l.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("SAVEPOINT insert_node"); err != nil {
		return nil, err
	}

	_, err = tx.Exec(
		`INSERT INTO nodes
		 (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (id) DO NOTHING`,
		n.ID, n.TrajectoryID, n.TenantID, step, n.Seq, n.Kind, string(payloadJSON), n.TS,
	)
	if err != nil {
		// Unique slot index: different id same slot. Postgres aborts the
		// transaction on any statement error, so roll back to the savepoint
		// first or the slot lookup below would fail too.
		if _, rerr := tx.Exec("ROLLBACK TO SAVEPOINT insert_node"); rerr != nil {
			return nil, rerr
		}
		owner, oerr := slotOwner(tx, tenantID, trajectoryID, stepN, seq, kind)
		if oerr != nil {
			return nil, err
		}
		if owner == n.ID {
			return n, nil
		}
		if owner != "" {
			return nil, fmt.Errorf("%w: trajectory=%s step=%v seq=%d kind=%s",
				ErrSlotConflict, trajectoryID, stepN, seq, kind)
		}
		return nil, fmt.Errorf("%w: %w", ErrNotStored, err)
	}
	owner, err := slotOwner(tx, tenantID, trajectoryID, stepN, seq, kind)
	if err != nil {
		return nil, err
	}
	if owner == "" {
		return nil, fmt.Errorf("%w: trajectory=%s step=%v seq=%d kind=%s",
			ErrNotStored, trajectoryID, stepN, seq, kind)
	}
	if owner != n.ID {
		return nil, fmt.Errorf("%w: trajectory=%s step=%v seq=%d kind=%s",
			ErrSlotConflict, trajectoryID, stepN, seq, kind)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return n, nil
}

func slotOwner(tx *sql.Tx, tenantID, trajectoryID string, stepN *int, seq int, kind string) (string, error) {
	var step any
	if stepN != nil {
		step = *stepN
	}
	var id string
	err := tx.QueryRow(
		`SELECT id FROM nodes
		 WHERE tenant_id = $1 AND trajectory_id = $2
		   AND COALESCE(step_n, -1) = COALESCE($3::integer, -1)
		   AND seq = $4 AND kind = $5`,
		tenantID, trajectoryID, step, seq, kind,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// ClaimToolCall claims a TOOL_CALL slot. claimed=false means lost the race.
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
		return false, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	tx, err := l.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var one int
	qerr := tx.QueryRow(
		`SELECT 1 FROM nodes
		 WHERE tenant_id = $1 AND trajectory_id = $2
		   AND step_n = $3 AND kind = $4 AND seq = $5
		 LIMIT 1
		 FOR UPDATE`,
		tenantID, trajectoryID, stepN, "TOOL_CALL", seq,
	).Scan(&one)
	if qerr == nil {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	if qerr != sql.ErrNoRows {
		return false, qerr
	}

	_, err = tx.Exec(
		`INSERT INTO nodes
		 (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		n.ID, n.TrajectoryID, n.TenantID, stepN, n.Seq, n.Kind, string(payloadJSON), n.TS,
	)
	if err != nil {
		// Unique violation under race: not claimed.
		return false, nil
	}
	if err := tx.Commit(); err != nil {
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

	q := `SELECT 1 FROM nodes WHERE trajectory_id = $1 AND step_n = $2 AND kind = $3`
	args := []any{trajectoryID, stepN, kind}
	if tenantID != nil {
		q += fmt.Sprintf(" AND tenant_id = $%d", len(args)+1)
		args = append(args, *tenantID)
	}
	if seq != nil {
		q += fmt.Sprintf(" AND seq = $%d", len(args)+1)
		args = append(args, *seq)
	}
	q += ` LIMIT 1`
	var one int
	err := l.db.QueryRow(q, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ListNodes returns nodes for a trajectory scoped to tenantID.
func (l *NodeLog) ListNodes(trajectoryID, tenantID string) ([]map[string]any, error) {
	return l.listNodes(trajectoryID, &tenantID)
}

// ListNodesAllTenants returns nodes for a trajectory across all tenants.
func (l *NodeLog) ListNodesAllTenants(trajectoryID string) ([]map[string]any, error) {
	return l.listNodes(trajectoryID, nil)
}

func (l *NodeLog) listNodes(trajectoryID string, tenantID *string) ([]map[string]any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	q := `SELECT id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts
	      FROM nodes WHERE trajectory_id = $1`
	args := []any{trajectoryID}
	if tenantID != nil {
		q += ` AND tenant_id = $2`
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
