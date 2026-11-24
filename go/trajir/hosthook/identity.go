package hosthook

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Phase is which hook point fired.
type Phase string

const (
	PhasePre          Phase = "pre"
	PhasePost         Phase = "post"
	PhasePostFailure  Phase = "post-failure"
	PhaseSessionStart Phase = "session-start"
)

// Event is one host hook payload in host-neutral form. ToolUseID is empty
// when the host sends none. Failed marks a post event that reports a failed
// tool, for hosts that have no separate failure hook.
type Event struct {
	Host, SessionID, ToolUseID, ToolName, Cwd string
	Args, Annotations                         map[string]any
	Result, Error                             any
	Failed                                    bool
}

// Identity is where and as what a hook records.
type Identity struct {
	WorkDir, TenantID, TrajectoryID, ProjectDir string
	workDirFromEnv                              bool
}

var safeSessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,120}$`)

// ResolveIdentity picks the work dir, tenant and trajectory for one event.
// Env wins: TRAJIR_WORK_DIR, TRAJIR_TENANT_ID, TRAJIR_TRAJECTORY_ID.
// Otherwise runs go under <project>/.trajir/runs/<host>, tenant "local",
// trajectory "<host>-<session>". A session id that is not a plain token is
// hashed, so it never becomes part of a path.
func ResolveIdentity(host string, ev Event, getenv func(string) string) (Identity, error) {
	project := ev.Cwd
	if project == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Identity{}, err
		}
		project = wd
	}
	id := Identity{ProjectDir: project, TenantID: "local"}
	if v := strings.TrimSpace(getenv("TRAJIR_WORK_DIR")); v != "" {
		id.WorkDir, id.workDirFromEnv = v, true
	} else {
		id.WorkDir = filepath.Join(project, ".trajir", "runs", host)
	}
	if v := strings.TrimSpace(getenv("TRAJIR_TENANT_ID")); v != "" {
		id.TenantID = v
	}
	if v := strings.TrimSpace(getenv("TRAJIR_TRAJECTORY_ID")); v != "" {
		id.TrajectoryID = v
		return id, nil
	}
	session := strings.TrimSpace(ev.SessionID)
	switch {
	case session == "":
		session = "nosession"
	case !safeSessionID.MatchString(session):
		sum := sha256.Sum256([]byte(session))
		session = "s" + hex.EncodeToString(sum[:])[:16]
	}
	id.TrajectoryID = host + "-" + session
	return id, nil
}

// Prepare creates the work dir. When it creates <project>/.trajir it also
// writes a .gitignore holding "*", so runs are not committed by accident.
func (id Identity) Prepare() error {
	if !id.workDirFromEnv {
		dot := filepath.Join(id.ProjectDir, ".trajir")
		if err := os.MkdirAll(dot, 0o755); err != nil {
			return id.prepareErr(err)
		}
		ignore := filepath.Join(dot, ".gitignore")
		if _, err := os.Stat(ignore); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
				return id.prepareErr(err)
			}
		}
	}
	if err := os.MkdirAll(id.WorkDir, 0o700); err != nil {
		return id.prepareErr(err)
	}
	return nil
}

func (id Identity) prepareErr(err error) error {
	return fmt.Errorf("cannot write runs under %s (%v); set TRAJIR_WORK_DIR to a writable directory", id.WorkDir, err)
}
