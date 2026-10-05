package console

import (
	"encoding/json"
	"sort"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
)

// EvidenceView is the TURNING_POINT operator rollup: seal-before-execute,
// open-world flags, idempotency keys, and last trajir verify / audit result.
type EvidenceView struct {
	SealBeforeExecuteOK *bool `json:"seal_before_execute_ok"`
	// SealBeforeExecuteDetail lists TOOL_CALL steps missing an earlier DECISION.
	SealBeforeExecuteGaps []EvidenceGap `json:"seal_before_execute_gaps,omitempty"`

	OpenWorldTools []string `json:"open_world_tools,omitempty"`

	ToolCalls []EvidenceToolCall `json:"tool_calls,omitempty"`

	AuditOK       *bool    `json:"audit_ok"`
	AuditFindings []string `json:"audit_findings,omitempty"`
	LastAuditTS   string   `json:"last_audit_ts,omitempty"`
	LastAuditPath string   `json:"last_audit_path,omitempty"`

	SealCount int `json:"seal_count"`
}

// EvidenceGap is one missing seal-before-execute case.
type EvidenceGap struct {
	StepN    int    `json:"step_n"`
	Seq      int    `json:"seq"`
	ToolName string `json:"tool_name,omitempty"`
}

// EvidenceToolCall is one observed TOOL_CALL with optional key / effect.
type EvidenceToolCall struct {
	TS             string `json:"ts,omitempty"`
	StepN          int    `json:"step_n,omitempty"`
	Seq            int    `json:"seq,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	EffectClass    string `json:"effect_class,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	OpenWorld      bool   `json:"open_world"`
	NodeID         string `json:"node_id,omitempty"`
}

// deriveEvidence builds EvidenceView from console events (live or derived).
// Decision/seal indexes are collected first so seal-before-execute does not
// depend on NDJSON/HTTP arrival order (mirrors audit.checkSealBeforeExecute).
func deriveEvidence(events []Event) EvidenceView {
	view := EvidenceView{}
	decisionSeq := map[int]int{} // step -> earliest DECISION seq
	decisionSteps := map[int]struct{}{}
	sealCreated := 0
	openWorld := map[string]struct{}{}

	for _, e := range events {
		switch e.Kind {
		case KindSealCreated:
			sealCreated++
			step, hasStep := payloadIntOK(e.Payload, "step_n")
			if hasStep && step > 0 {
				decisionSteps[step] = struct{}{}
				if prev, ok := decisionSeq[step]; !ok || 1 < prev {
					decisionSeq[step] = 1 // DECISION is always seq 1 in the IR
				}
			}
			if names, ok := payloadStringSlice(e.Payload, "tool_names"); ok {
				for _, n := range names {
					if effects.IsOpenWorldPrimitive(n) {
						openWorld[n] = struct{}{}
					}
				}
			}
			if names, ok := payloadStringSlice(e.Payload, "open_world_tools"); ok {
				for _, n := range names {
					openWorld[n] = struct{}{}
				}
			}
		case KindNodeAppended:
			kind, _ := payloadString(e.Payload, "kind")
			if kind != "DECISION" {
				continue
			}
			step, hasStep := payloadIntOK(e.Payload, "step_n")
			seq, hasSeq := payloadIntOK(e.Payload, "seq")
			if hasStep && hasSeq {
				decisionSteps[step] = struct{}{}
				if prev, ok := decisionSeq[step]; !ok || seq < prev {
					decisionSeq[step] = seq
				}
			}
		case KindAuditCompleted:
			ok, has := payloadBool(e.Payload, "ok")
			if has {
				view.AuditOK = &ok
			}
			view.LastAuditTS = e.TS
			if p, ok := payloadString(e.Payload, "path"); ok {
				view.LastAuditPath = p
			}
			if findings, ok := payloadStringSlice(e.Payload, "findings"); ok {
				view.AuditFindings = findings
			} else {
				view.AuditFindings = payloadFindingCodes(e.Payload)
			}
		}
	}

	var gaps []EvidenceGap
	var tools []EvidenceToolCall
	for _, e := range events {
		if e.Kind != KindNodeAppended {
			continue
		}
		kind, _ := payloadString(e.Payload, "kind")
		if kind != "TOOL_CALL" {
			continue
		}
		step, hasStep := payloadIntOK(e.Payload, "step_n")
		seq, hasSeq := payloadIntOK(e.Payload, "seq")
		name, _ := payloadString(e.Payload, "tool")
		if name == "" {
			name, _ = payloadString(e.Payload, "tool_name")
		}
		eff, _ := payloadString(e.Payload, "effect_class")
		key, _ := payloadString(e.Payload, "idempotency_key")
		nodeID, _ := payloadString(e.Payload, "node_id")
		ow := effects.IsOpenWorldPrimitive(name)
		if ow {
			openWorld[name] = struct{}{}
		}
		tools = append(tools, EvidenceToolCall{
			TS:             e.TS,
			StepN:          step,
			Seq:            seq,
			ToolName:       name,
			EffectClass:    eff,
			IdempotencyKey: key,
			OpenWorld:      ow,
			NodeID:         nodeID,
		})
		if hasStep && hasSeq {
			decSeq, has := decisionSeq[step]
			if !has || decSeq >= seq {
				gaps = append(gaps, EvidenceGap{StepN: step, Seq: seq, ToolName: name})
			}
		} else {
			gaps = append(gaps, EvidenceGap{ToolName: name})
		}
	}

	if sealCreated > 0 {
		view.SealCount = sealCreated
	} else {
		view.SealCount = len(decisionSteps)
	}
	view.ToolCalls = tools
	view.SealBeforeExecuteGaps = gaps
	if len(tools) > 0 {
		ok := len(gaps) == 0
		view.SealBeforeExecuteOK = &ok
	} else if view.SealCount > 0 {
		ok := true
		view.SealBeforeExecuteOK = &ok
	}
	if len(openWorld) > 0 {
		names := make([]string, 0, len(openWorld))
		for n := range openWorld {
			names = append(names, n)
		}
		sort.Strings(names)
		view.OpenWorldTools = names
	}
	return view
}

func payloadFindingCodes(raw json.RawMessage) []string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	arr, ok := m["findings"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		switch v := item.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			if c, ok := v["code"].(string); ok && c != "" {
				msg, _ := v["message"].(string)
				if msg != "" {
					out = append(out, c+": "+msg)
				} else {
					out = append(out, c)
				}
			}
		}
	}
	return out
}
