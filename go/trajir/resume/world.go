package resume

import (
	"fmt"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

// WorldMismatch is one sealed snapshot key that no longer matches.
type WorldMismatch struct {
	Key      string
	Sealed   string
	Observed string
}

// WorldDrift is raised when a sealed WORLD_SNAPSHOT diverges from observation.
type WorldDrift struct {
	StepN      int
	Mismatches []WorldMismatch
}

func (e *WorldDrift) Error() string {
	return fmt.Sprintf(
		"WORLD_DRIFT: step %d: %d sealed snapshot key(s) diverged",
		e.StepN,
		len(e.Mismatches),
	)
}

// DecisionPayload builds a DECISION payload. Empty snapshots are omitted so
// existing content hashes stay stable for callers that do not opt in.
func DecisionPayload(plan map[string]any, world map[string]string) map[string]any {
	if plan == nil {
		plan = map[string]any{}
	}
	payload := map[string]any{"plan": plan}
	if snap := NormalizeWorldSnapshot(world); len(snap) > 0 {
		payload["world_snapshot"] = snap
	}
	return payload
}

// NormalizeWorldSnapshot copies a host-declared snapshot. Nil/empty → nil.
func NormalizeWorldSnapshot(snapshot map[string]string) map[string]string {
	if len(snapshot) == 0 {
		return nil
	}
	out := make(map[string]string, len(snapshot))
	for k, v := range snapshot {
		out[k] = v
	}
	return out
}

// CheckWorld compares observed values against the sealed WORLD_SNAPSHOT.
// Extra keys in observed are ignored. Missing or changed sealed keys raise
// WorldDrift. No DECISION is an error. An empty sealed snapshot is a no-op.
func CheckWorld(log *nodelog.NodeLog, trajectoryID, tenantID string, stepN int, observed map[string]string) error {
	if log == nil {
		return fmt.Errorf("resume: CheckWorld requires a NodeLog")
	}
	sealed, ok, err := sealedWorldSnapshot(log, trajectoryID, tenantID, stepN)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("resume: no DECISION for step %d; cannot check world snapshot", stepN)
	}
	if len(sealed) == 0 {
		return nil
	}
	var mismatches []WorldMismatch
	for key, want := range sealed {
		got, present := observed[key]
		if !present || got != want {
			if !present {
				got = ""
			}
			mismatches = append(mismatches, WorldMismatch{
				Key:      key,
				Sealed:   want,
				Observed: got,
			})
		}
	}
	if len(mismatches) > 0 {
		return &WorldDrift{StepN: stepN, Mismatches: mismatches}
	}
	return nil
}

func sealedWorldSnapshot(log *nodelog.NodeLog, trajectoryID, tenantID string, stepN int) (map[string]string, bool, error) {
	rows, err := log.ListNodes(trajectoryID, tenantID)
	if err != nil {
		return nil, false, err
	}
	for _, row := range rows {
		if row["kind"] != "DECISION" {
			continue
		}
		if row["step_n"] != stepN {
			continue
		}
		payload, _ := row["payload"].(map[string]any)
		if payload == nil {
			return nil, true, nil
		}
		raw, exists := payload["world_snapshot"]
		if !exists || raw == nil {
			return nil, true, nil
		}
		snap, err := snapshotFromAny(raw)
		if err != nil {
			return nil, true, err
		}
		return snap, true, nil
	}
	return nil, false, nil
}

func snapshotFromAny(v any) (map[string]string, error) {
	switch m := v.(type) {
	case map[string]string:
		return NormalizeWorldSnapshot(m), nil
	case map[string]any:
		if len(m) == 0 {
			return nil, nil
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("resume: world_snapshot[%q] must be a string", k)
			}
			out[k] = s
		}
		return out, nil
	default:
		return nil, fmt.Errorf("resume: world_snapshot must be an object")
	}
}
