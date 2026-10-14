// Package audit implements offline .tir evidence checks for trajir verify.
//
// These checks are observations over a verified package. They do not replace
// NodeLog or tir.Load.
package audit

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

// Options control VerifyFile. A nil trust store means the store is not
// configured. An empty non-nil store rejects both unsigned and signed packs.
type Options struct {
	// RequireSignature fails when the package has no SIGNATURE member.
	RequireSignature bool
	// TrustedKeys, when non-nil, requires the package public key to match one entry.
	TrustedKeys []ed25519.PublicKey
	// TrustedKeyIDs, when non-nil, requires document signer.key_id to be listed.
	TrustedKeyIDs []string
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

// VerifyFile loads path with hash verification and the trust store, then runs
// policy checks. --require-signature and a configured trust store are enforced
// inside LoadWithOptions, so a pack signed by an untrusted key does not pass.
func VerifyFile(path string, opts Options) (*Result, error) {
	pkg, err := tir.LoadWithOptions(path, tir.VerifyOptions{
		RequireSignature: opts.RequireSignature,
		TrustedKeys:      opts.TrustedKeys,
		TrustedKeyIDs:    opts.TrustedKeyIDs,
	})
	if err != nil {
		code := "LOAD_FAILED"
		if errors.Is(err, tir.ErrSignature) {
			code = "SIGNATURE_REQUIRED"
		}
		return &Result{
			Path: path,
			OK:   false,
			Findings: []Finding{{
				Code:    code,
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
		// The payload effect_class is the agent's own label. A write tagged
		// READ_ONLY or IDEMPOTENT_WRITE still needs a DECISION sealed first.
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
			name := callName(call)
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
				name := callName(call)
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

func callName(call map[string]any) string {
	if call == nil {
		return ""
	}
	if name, ok := call["name"].(string); ok && name != "" {
		return name
	}
	if name, ok := call["tool"].(string); ok {
		return name
	}
	return ""
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
