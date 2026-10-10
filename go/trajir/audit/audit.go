// Package audit implements offline .tir evidence checks for trajir verify.
//
// These checks are observations over a verified package. They do not replace
// NodeLog or tir.Load.
package audit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

// Options control VerifyPackage.
type Options struct {
	// RequireSignature fails when the package has no SIGNATURE member.
	RequireSignature bool
}

// Finding is one audit failure.
type Finding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (f Finding) String() string {
	return f.Code + ": " + f.Message
}

// Result is the full audit outcome for one package.
type Result struct {
	Path     string    `json:"path"`
	OK       bool      `json:"ok"`
	Findings []Finding `json:"findings"`
}

// VerifyFile loads path with hash verification, then runs policy checks.
func VerifyFile(path string, opts Options) (*Result, error) {
	pkg, err := tir.Load(path)
	if err != nil {
		return &Result{
			Path: path,
			OK:   false,
			Findings: []Finding{{
				Code:    "LOAD_FAILED",
				Message: err.Error(),
			}},
		}, nil
	}
	return VerifyPackage(path, pkg, opts), nil
}

// VerifyPackage runs seal-before-execute and open-world checks on an already
// loaded package. Callers that need RequireSignature on unsigned packs pass
// opts; Load already verified any present SIGNATURE.
func VerifyPackage(path string, pkg *tir.Package, opts Options) *Result {
	res := &Result{Path: path, OK: true, Findings: nil}
	if pkg == nil {
		res.OK = false
		res.Findings = append(res.Findings, Finding{
			Code:    "NIL_PACKAGE",
			Message: "package is nil",
		})
		return res
	}

	if opts.RequireSignature && pkg.Signature == nil {
		res.Findings = append(res.Findings, Finding{
			Code:    "SIGNATURE_REQUIRED",
			Message: "package has no SIGNATURE member and --require-signature was set",
		})
	}

	res.Findings = append(res.Findings, checkSealBeforeExecute(pkg.Nodes)...)
	res.Findings = append(res.Findings, checkPlanMembership(pkg.Nodes)...)
	res.Findings = append(res.Findings, checkOpenWorldLies(pkg.Nodes)...)

	if len(res.Findings) > 0 {
		res.OK = false
		sort.Slice(res.Findings, func(i, j int) bool {
			if res.Findings[i].Code == res.Findings[j].Code {
				return res.Findings[i].Message < res.Findings[j].Message
			}
			return res.Findings[i].Code < res.Findings[j].Code
		})
	}
	return res
}

func checkSealBeforeExecute(nodes []map[string]any) []Finding {
	var out []Finding
	// Per step: earliest DECISION seq (if any).
	decisionSeq := map[int]int{}
	for _, n := range nodes {
		kind, _ := n["kind"].(string)
		if kind != "DECISION" {
			continue
		}
		step, ok := asInt(n["step_n"])
		if !ok {
			continue
		}
		seq, ok := asInt(n["seq"])
		if !ok {
			continue
		}
		if prev, exists := decisionSeq[step]; !exists || seq < prev {
			decisionSeq[step] = seq
		}
	}

	for _, n := range nodes {
		kind, _ := n["kind"].(string)
		if kind != "TOOL_CALL" {
			continue
		}
		step, ok := asInt(n["step_n"])
		if !ok {
			out = append(out, Finding{
				Code:    "SEAL_BEFORE_EXECUTE",
				Message: "TOOL_CALL missing step_n",
			})
			continue
		}
		seq, ok := asInt(n["seq"])
		if !ok {
			out = append(out, Finding{
				Code:    "SEAL_BEFORE_EXECUTE",
				Message: fmt.Sprintf("TOOL_CALL at step %d missing seq", step),
			})
			continue
		}
		payload, _ := n["payload"].(map[string]any)
		toolName := toolNameFromPayload(payload)
		needsSeal := true
		if eff, ok := effectFromPayload(payload); ok {
			// Only force seal for mutating / open-world / spawn / sensitive.
			switch effects.EffectClass(eff) {
			case effects.PURE, effects.READ_ONLY, effects.IDEMPOTENT_WRITE:
				if !effects.IsOpenWorldPrimitive(toolName) {
					needsSeal = false
				}
			}
		} else if toolName != "" && !effects.IsOpenWorldPrimitive(toolName) {
			// No effect recorded: still require a DECISION for every TOOL_CALL.
			// That is the Chain-of-Evidence rule (seal before execute_tool).
			needsSeal = true
		}

		if !needsSeal {
			continue
		}
		decSeq, has := decisionSeq[step]
		if !has || decSeq >= seq {
			name := toolName
			if name == "" {
				name = "?"
			}
			out = append(out, Finding{
				Code: "SEAL_BEFORE_EXECUTE",
				Message: fmt.Sprintf(
					"TOOL_CALL tool=%q step=%d seq=%d has no earlier DECISION for that step",
					name, step, seq,
				),
			})
		}
	}
	return out
}

// checkPlanMembership fails a TOOL_CALL that is not in the sealed plan for
// its own step. An earlier DECISION on that step is not enough: the call's
// name and args have to match an unused plan entry. Plans do not leak across
// steps, so a later turn cannot reuse the previous turn's tool list.
func checkPlanMembership(nodes []map[string]any) []Finding {
	type callKey struct {
		name string
		args string
	}
	type planInfo struct {
		seq    int
		counts map[callKey]int
	}
	plans := map[int]*planInfo{}
	for _, n := range nodes {
		kind, _ := n["kind"].(string)
		if kind != "DECISION" {
			continue
		}
		step, ok := asInt(n["step_n"])
		if !ok {
			continue
		}
		seq, ok := asInt(n["seq"])
		if !ok {
			continue
		}
		payload, _ := n["payload"].(map[string]any)
		plan, _ := payload["plan"].(map[string]any)
		if plan == nil {
			continue
		}
		if prev, exists := plans[step]; exists && prev.seq <= seq {
			continue
		}
		counts := map[callKey]int{}
		calls, _ := plan["tool_calls"].([]any)
		for _, raw := range calls {
			call, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := call["name"].(string)
			if name == "" {
				name, _ = call["tool"].(string)
			}
			if name == "" {
				continue
			}
			counts[callKey{name: name, args: canonicalArgs(call["args"])}]++
		}
		plans[step] = &planInfo{seq: seq, counts: counts}
	}

	var out []Finding
	for _, n := range nodes {
		kind, _ := n["kind"].(string)
		if kind != "TOOL_CALL" {
			continue
		}
		step, ok := asInt(n["step_n"])
		if !ok {
			continue
		}
		seq, ok := asInt(n["seq"])
		if !ok {
			continue
		}
		info, has := plans[step]
		if !has || seq <= info.seq {
			continue
		}
		payload, _ := n["payload"].(map[string]any)
		name := toolNameFromPayload(payload)
		args := canonicalArgs(nil)
		if payload != nil {
			args = canonicalArgs(payload["args"])
		}
		key := callKey{name: name, args: args}
		if info.counts[key] > 0 {
			info.counts[key]--
			continue
		}
		label := name
		if label == "" {
			label = "?"
		}
		out = append(out, Finding{
			Code: "PLAN_MISMATCH",
			Message: fmt.Sprintf(
				"TOOL_CALL tool=%q step=%d seq=%d is not in the sealed DECISION plan for that step",
				label, step, seq,
			),
		})
	}
	return out
}

func canonicalArgs(v any) string {
	if v == nil {
		v = map[string]any{}
	}
	buf, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(buf)
}

func checkOpenWorldLies(nodes []map[string]any) []Finding {
	// AllowOpenWorldOverride is a live execution choice. DECISION and TOOL_CALL
	// payloads do not record it, so verify always passes false. A safer tag on
	// an open-world primitive is an OPEN_WORLD_LIE offline even if the process
	// that ran the tool set the override. See docs/EVIDENCE_SIDECAR.md.
	const allowOverride = false
	var out []Finding
	for _, n := range nodes {
		kind, _ := n["kind"].(string)
		payload, _ := n["payload"].(map[string]any)
		switch kind {
		case "TOOL_CALL":
			name := toolNameFromPayload(payload)
			eff, ok := effectFromPayload(payload)
			if !ok || name == "" {
				continue
			}
			if err := effects.AssertOpenWorldEffect(name, effects.EffectClass(eff), allowOverride); err != nil {
				out = append(out, Finding{
					Code:    "OPEN_WORLD_LIE",
					Message: err.Error(),
				})
			}
		case "DECISION":
			plan, _ := payload["plan"].(map[string]any)
			if plan == nil {
				continue
			}
			calls, _ := plan["tool_calls"].([]any)
			for _, raw := range calls {
				call, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				name, _ := call["name"].(string)
				eff, ok := effectFromCall(call)
				if !ok || name == "" {
					continue
				}
				if err := effects.AssertOpenWorldEffect(name, effects.EffectClass(eff), allowOverride); err != nil {
					out = append(out, Finding{
						Code:    "OPEN_WORLD_LIE",
						Message: err.Error(),
					})
				}
			}
		}
	}
	return out
}

func toolNameFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if s, ok := payload["tool"].(string); ok {
		return s
	}
	if s, ok := payload["tool_name"].(string); ok {
		return s
	}
	return ""
}

func effectFromPayload(payload map[string]any) (string, bool) {
	if payload == nil {
		return "", false
	}
	for _, key := range []string{"effect_class", "effect"} {
		if s, ok := payload[key].(string); ok && s != "" {
			return strings.ToUpper(s), true
		}
	}
	return "", false
}

func effectFromCall(call map[string]any) (string, bool) {
	for _, key := range []string{"effect_class", "effect"} {
		if s, ok := call[key].(string); ok && s != "" {
			return strings.ToUpper(s), true
		}
	}
	return "", false
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
