// Package projector implements the default context projector (R04).
//
// Metric rfc8785_bytes: length of RFC 8785 (JCS) canonical form of
// {"kind":...,"payload":...} per node. Same JCS stack as trajir/nodes
// (github.com/gowebpki/jcs) so size units match Python rfc8785.
package projector

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gowebpki/jcs"
)

// SizeMetric is the documented budget unit name (parity with Python SIZE_METRIC).
const SizeMetric = "rfc8785_bytes"

// BudgetImpossible is raised when CONSTRAINT/pinned nodes alone exceed budget.
type BudgetImpossible struct {
	RequiredSize int
	Budget       int
}

func (e *BudgetImpossible) Error() string {
	return fmt.Sprintf(
		"BUDGET_IMPOSSIBLE: CONSTRAINT/pinned size %d exceeds budget %d",
		e.RequiredSize,
		e.Budget,
	)
}

// Result is the outcome of default projection.
type Result struct {
	IncludedIDs []string
	DroppedIDs  []string
	Context     map[string]any
	SizeUnits   int
	Budget      int
	Metric      string
}

// NodeSizeUnits returns the rfc8785_bytes size of one node record (JCS length).
func NodeSizeUnits(node map[string]any) (int, error) {
	kind, _ := node["kind"].(string)
	payload := node["payload"]
	if payload == nil {
		payload = map[string]any{}
	}
	// encoding/json then jcs.Transform — same pattern as nodes.PayloadHash.
	raw, err := json.Marshal(map[string]any{"kind": kind, "payload": payload})
	if err != nil {
		return 0, fmt.Errorf("json marshal: %w", err)
	}
	canon, err := jcs.Transform(raw)
	if err != nil {
		return 0, fmt.Errorf("jcs: %w", err)
	}
	return len(canon), nil
}

type sortKey struct {
	step int
	seq  int
	id   string
	node map[string]any
}

func nodeSortKey(n map[string]any) sortKey {
	step := 1_000_000_000
	if v, ok := n["step_n"].(int); ok {
		step = v
	} else if v, ok := n["step_n"].(float64); ok {
		step = int(v)
	}
	seq := 1_000_000_000
	if v, ok := n["seq"].(int); ok {
		seq = v
	} else if v, ok := n["seq"].(float64); ok {
		seq = int(v)
	}
	id, _ := n["id"].(string)
	return sortKey{step: step, seq: seq, id: id, node: n}
}

func sortNodes(nodes []map[string]any) []map[string]any {
	keys := make([]sortKey, len(nodes))
	for i, n := range nodes {
		keys[i] = nodeSortKey(n)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].step != keys[j].step {
			return keys[i].step < keys[j].step
		}
		if keys[i].seq != keys[j].seq {
			return keys[i].seq < keys[j].seq
		}
		return keys[i].id < keys[j].id
	})
	out := make([]map[string]any, len(keys))
	for i, k := range keys {
		out[i] = k.node
	}
	return out
}

// ProjectContext applies the default projector policy.
func ProjectContext(nodes []map[string]any, budget int, pinnedIDs map[string]struct{}) (*Result, error) {
	if budget < 0 {
		return nil, fmt.Errorf("budget must be non-negative")
	}
	if pinnedIDs == nil {
		pinnedIDs = map[string]struct{}{}
	}

	must := make([]map[string]any, 0)
	seen := map[string]struct{}{}
	for _, n := range nodes {
		id, _ := n["id"].(string)
		kind, _ := n["kind"].(string)
		_, pin := pinnedIDs[id]
		if kind == "CONSTRAINT" || pin {
			if id != "" {
				if _, ok := seen[id]; !ok {
					must = append(must, n)
					seen[id] = struct{}{}
				}
			}
		}
	}
	must = sortNodes(must)
	mustSize := 0
	for _, n := range must {
		sz, err := NodeSizeUnits(n)
		if err != nil {
			return nil, err
		}
		mustSize += sz
	}
	if mustSize > budget {
		return nil, &BudgetImpossible{RequiredSize: mustSize, Budget: budget}
	}

	included := make([]map[string]any, 0, len(must))
	included = append(included, must...)
	includedIDs := map[string]struct{}{}
	for _, n := range included {
		if id, ok := n["id"].(string); ok {
			includedIDs[id] = struct{}{}
		}
	}
	used := mustSize

	optional := make([]map[string]any, 0)
	for _, n := range nodes {
		id, _ := n["id"].(string)
		if _, ok := includedIDs[id]; !ok {
			optional = append(optional, n)
		}
	}
	optional = sortNodes(optional)
	for _, n := range optional {
		sz, err := NodeSizeUnits(n)
		if err != nil {
			return nil, err
		}
		if used+sz > budget {
			continue
		}
		included = append(included, n)
		if id, ok := n["id"].(string); ok {
			includedIDs[id] = struct{}{}
		}
		used += sz
	}
	included = sortNodes(included)

	dropped := make([]string, 0)
	for _, n := range nodes {
		id, _ := n["id"].(string)
		if id == "" {
			continue
		}
		if _, ok := includedIDs[id]; !ok {
			dropped = append(dropped, id)
		}
	}
	sort.Strings(dropped)

	ids := make([]string, 0, len(included))
	items := make([]map[string]any, 0, len(included))
	for _, n := range included {
		id, _ := n["id"].(string)
		ids = append(ids, id)
		items = append(items, map[string]any{
			"id":      n["id"],
			"kind":    n["kind"],
			"payload": n["payload"],
			"step_n":  n["step_n"],
			"seq":     n["seq"],
		})
	}
	ctx := map[string]any{
		"items":        items,
		"included_ids": ids,
		"budget":       budget,
		"size_units":   used,
		"metric":       SizeMetric,
	}
	return &Result{
		IncludedIDs: ids,
		DroppedIDs:  dropped,
		Context:     ctx,
		SizeUnits:   used,
		Budget:      budget,
		Metric:      SizeMetric,
	}, nil
}
