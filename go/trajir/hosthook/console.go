package hosthook

import (
	stdlog "log"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
)

// defaultRuntime labels console events when the host gave no name.
const defaultRuntime = "trajir-hook"

// Console events follow docs/CONSOLE_EVENTS.md, the same shapes the Go
// client sends. A nil Sink sends nothing. Sink errors are logged by
// emit.SafeEmit and never fail the hook.

func (s Session) event(kind string, payload map[string]any) {
	runtime := s.Runtime
	if runtime == "" {
		runtime = defaultRuntime
	}
	emit.SafeEmit(s.Sink, emit.Event{
		Kind:         kind,
		Source:       "go",
		Runtime:      runtime,
		TrajectoryID: s.TrajectoryID,
		TenantID:     s.TenantID,
		Payload:      payload,
	})
}

func (s Session) emitSeal(step int, name string, ctx, decision *nodes.Node) {
	if s.Sink == nil {
		return
	}
	for _, n := range []*nodes.Node{ctx, decision} {
		s.event(emit.KindNodeAppended, map[string]any{
			"node_id":      n.ID,
			"kind":         n.Kind,
			"seq":          n.Seq,
			"step_n":       step,
			"content_hash": n.PHash,
		})
	}
	s.event(emit.KindSealCreated, map[string]any{
		"node_id":      decision.ID,
		"step_n":       step,
		"content_hash": decision.PHash,
		"tool_names":   []string{name},
	})
}

// emitOutcome reports the TOOL_CALL, outcome and COMMIT_STEP rows of one
// step. The effect class is host knowledge, so it is added to the TOOL_CALL
// event and not to the stored node.
func (s Session) emitOutcome(nl *nodelog.NodeLog, rec Record) {
	if s.Sink == nil {
		return
	}
	rows, err := nl.ListStepNodes(s.TrajectoryID, s.TenantID, rec.StepN)
	if err != nil {
		stdlog.Printf("console sink: %v", err)
		return
	}
	for _, row := range rows {
		seq, _ := row["seq"].(int)
		if seq < toolSeq {
			continue
		}
		payload := map[string]any{
			"node_id": row["id"],
			"kind":    row["kind"],
			"seq":     seq,
			"step_n":  rec.StepN,
		}
		if row["kind"] == "TOOL_CALL" {
			body, _ := row["payload"].(map[string]any)
			if tool, ok := body["tool"].(string); ok && tool != "" {
				payload["tool"] = tool
			}
			if key, ok := body["idempotency_key"].(string); ok && key != "" {
				payload["idempotency_key"] = key
			}
			payload["effect_class"] = string(rec.EffectClass)
		}
		s.event(emit.KindNodeAppended, payload)
	}
}
