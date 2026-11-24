package hosthook

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// state is the per-trajectory hook ledger: the next step number, calls
// sealed but not yet recorded, and calls already finished.
type state struct {
	NextStep int               `json:"next_step"`
	Pending  map[string]Record `json:"pending"`
	Done     map[string]Record `json:"done"`
}

// fileKey names the state and lock files. The trajectory id comes from
// the host (a session id), so it is hashed and never used as a path.
func fileKey(tenantID, trajectoryID string) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + trajectoryID))
	return hex.EncodeToString(sum[:])[:16]
}

func (s Session) statePath() string {
	return filepath.Join(s.WorkDir, "hook_state."+fileKey(s.TenantID, s.TrajectoryID)+".json")
}

func (s Session) lockPath() string {
	return filepath.Join(s.WorkDir, "hook."+fileKey(s.TenantID, s.TrajectoryID)+".lock")
}

func (s Session) loadState() (*state, error) {
	st := &state{NextStep: 1, Pending: map[string]Record{}, Done: map[string]Record{}}
	raw, err := os.ReadFile(s.statePath())
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	// UseNumber keeps stored args exact. As float64, an integer above 2^53
	// would change between the seal and the TOOL_CALL.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(st); err != nil {
		return nil, fmt.Errorf("hosthook: %s: %w", s.statePath(), err)
	}
	if st.NextStep < 1 {
		st.NextStep = 1
	}
	if st.Pending == nil {
		st.Pending = map[string]Record{}
	}
	if st.Done == nil {
		st.Done = map[string]Record{}
	}
	return st, nil
}

func (s Session) saveState(st *state) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	path := s.statePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
