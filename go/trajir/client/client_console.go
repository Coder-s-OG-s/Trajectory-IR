package client

import (
	"encoding/json"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/projector"
)

func (t *Trajectory) emitNode(n *nodes.Node) {
	if t == nil || n == nil {
		return
	}
	payload := map[string]any{
		"node_id":      n.ID,
		"kind":         n.Kind,
		"seq":          n.Seq,
		"content_hash": n.PHash,
	}
	if n.StepN != nil {
		payload["step_n"] = *n.StepN
	}
	emit.SafeEmit(t.sink, emit.Event{
		Kind:         emit.KindNodeAppended,
		Source:       "go",
		Runtime:      "go",
		TrajectoryID: t.TrajectoryID,
		TenantID:     t.TenantID,
		Payload:      payload,
	})
}

func (t *Trajectory) emitNodeRow(row map[string]any) {
	if t == nil || row == nil {
		return
	}
	payload := map[string]any{
		"node_id": row["id"],
		"kind":    row["kind"],
		"seq":     row["seq"],
	}
	if h, ok := row["payload_hash"].(string); ok && h != "" {
		payload["content_hash"] = h
	}
	if step, ok := asInt(row["step_n"]); ok {
		payload["step_n"] = step
	}
	if kind, _ := row["kind"].(string); kind == "TOOL_CALL" {
		if body, ok := row["payload"].(map[string]any); ok {
			if tool, ok := body["tool"].(string); ok && tool != "" {
				payload["tool"] = tool
			}
			if key, ok := body["idempotency_key"].(string); ok && key != "" {
				payload["idempotency_key"] = key
			}
			if eff, ok := body["effect_class"].(string); ok && eff != "" {
				payload["effect_class"] = eff
			}
		}
	}
	emit.SafeEmit(t.sink, emit.Event{
		Kind:         emit.KindNodeAppended,
		Source:       "go",
		Runtime:      "go",
		TrajectoryID: t.TrajectoryID,
		TenantID:     t.TenantID,
		Payload:      payload,
	})
}

func (t *Trajectory) emitSeal(n *nodes.Node, stepN int, plan map[string]any) {
	if t == nil || n == nil {
		return
	}
	payload := map[string]any{
		"node_id":      n.ID,
		"step_n":       stepN,
		"content_hash": n.PHash,
	}
	if names := toolNames(plan); len(names) > 0 {
		payload["tool_names"] = names
	}
	emit.SafeEmit(t.sink, emit.Event{
		Kind:         emit.KindSealCreated,
		Source:       "go",
		Runtime:      "go",
		TrajectoryID: t.TrajectoryID,
		TenantID:     t.TenantID,
		Payload:      payload,
	})
}

func (t *Trajectory) emitProjection(stepN int) {
	if t == nil || t.sink == nil || t.log == nil {
		return
	}
	rows, err := t.log.ListNodes(t.TrajectoryID, t.TenantID)
	if err != nil || len(rows) == 0 {
		return
	}
	rawUnits := 0
	for _, n := range rows {
		sz, err := projector.NodeSizeUnits(n)
		if err != nil {
			return
		}
		rawUnits += sz
	}
	res, err := projector.ProjectContext(rows, emit.ObservationBudget, nil)
	if err != nil {
		return
	}
	included := rowsByID(rows, res.IncludedIDs)
	emit.NoteProjection(t.sink, emit.ProjectionFact{
		TrajectoryID:     t.TrajectoryID,
		TenantID:         t.TenantID,
		StepN:            stepN,
		Budget:           res.Budget,
		Metric:           res.Metric,
		SizeUnits:        res.SizeUnits,
		RawSizeUnits:     rawUnits,
		IncludedIDs:      res.IncludedIDs,
		DroppedIDs:       res.DroppedIDs,
		RawCharLen:       charLenKindPayload(rows),
		ProjectedCharLen: charLenKindPayload(included),
		Source:           "go",
		Runtime:          "go",
	})
}

func (t *Trajectory) emitToolNodes(stepN, seq int, effect effects.EffectClass) {
	if t == nil || t.sink == nil || t.log == nil {
		return
	}
	rows, err := t.log.ListNodes(t.TrajectoryID, t.TenantID)
	if err != nil {
		return
	}
	for _, row := range rows {
		kind, _ := row["kind"].(string)
		if kind != "TOOL_CALL" && kind != "TOOL_RESULT" {
			continue
		}
		rowSeq, ok := asInt(row["seq"])
		if !ok || (rowSeq != seq && rowSeq != seq+1) {
			continue
		}
		if step, ok := asInt(row["step_n"]); ok && step != stepN {
			continue
		}
		if kind == "TOOL_CALL" && effect != "" {
			// Effect class is host knowledge. Copy the payload so the log row stays unchanged.
			body, _ := row["payload"].(map[string]any)
			copied := map[string]any{}
			for k, v := range body {
				copied[k] = v
			}
			if _, exists := copied["effect_class"]; !exists {
				copied["effect_class"] = string(effect)
			}
			next := make(map[string]any, len(row))
			for k, v := range row {
				next[k] = v
			}
			next["payload"] = copied
			row = next
		}
		t.emitNodeRow(row)
	}
}

func toolNames(plan map[string]any) []string {
	raw, ok := plan["tool_calls"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, item := range raw {
		call, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := call["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func rowsByID(rows []map[string]any, ids []string) []map[string]any {
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]map[string]any, 0, len(ids))
	for _, n := range rows {
		id, _ := n["id"].(string)
		if _, ok := want[id]; ok {
			out = append(out, n)
		}
	}
	return out
}

func charLenKindPayload(rows []map[string]any) int {
	items := make([]map[string]any, 0, len(rows))
	for _, n := range rows {
		items = append(items, map[string]any{"kind": n["kind"], "payload": n["payload"]})
	}
	b, err := json.Marshal(items)
	if err != nil {
		return 0
	}
	return len(b)
}
