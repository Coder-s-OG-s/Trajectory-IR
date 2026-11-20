// Package logerr holds the error values shared by every NodeLog
// implementation (SQLite in trajir/log, PostgreSQL in trajir/postgres).
//
// It has no dependencies, so importing trajir/log does not link a database
// driver the caller never uses. Both log and postgres re-export these values
// under their old names, and errors.Is works across all of them.
package logerr

import "errors"

// ErrSlotConflict is returned when a different payload already occupies the
// logical slot (tenant, trajectory, step, seq, kind).
//
// The message keeps its original "postgres:" prefix so code that matched on
// the old text keeps working.
var ErrSlotConflict = errors.New("postgres: slot conflict")

// ErrNotStored means the append did not leave this node in the slot.
var ErrNotStored = errors.New("node not stored")
