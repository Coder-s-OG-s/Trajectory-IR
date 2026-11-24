// Package hosthook is the Go port of integrations/hosthook/session.py: a
// cross-process seal-then-record session for an external agent host.
//
// Pre-tool: write PROJECT_CONTEXT and DECISION, then return. The host runs
// the tool after that. Post-tool: write TOOL_CALL and TOOL_RESULT for the
// result the host already produced. The function handed to the recorder only
// returns that result. It is not the real tool.
//
// A failed host tool writes TOOL_CALL, then ABORT with reason
// HOST_REPORTED_FAILURE, then COMMIT_STEP.
//
// The same hook owner asking again for a pending tool_use_id is a crash
// before the post hook. That is a replay, blocked when the effect is gated or
// unknown. A different owner is another registration of the same attempt.
// This is not exactly-once.
package hosthook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

const (
	toolSeq        = 2
	commitSeq      = 4
	errorTextLimit = 2000
	hostFailure    = "HOST_REPORTED_FAILURE"
)

// ErrNoSeal is returned when a post hook names a tool_use_id that no pre
// hook sealed.
var ErrNoSeal = errors.New("hosthook: no sealed call for tool_use_id")

// Session is one trajectory in one work dir. Owner names the hook
// registration. Classify picks the effect class. Nil means
// effects.ClassifyTool.
type Session struct {
	WorkDir, TenantID, TrajectoryID, Owner string
	Classify                               func(name string, annotations map[string]any) effects.EffectClass
}

// Record is one tool call in the hook state. Replay and Duplicate are
// return-only and never stored.
type Record struct {
	ToolUseID      string              `json:"tool_use_id"`
	StepN          int                 `json:"step_n"`
	ToolName       string              `json:"tool_name"`
	Args           map[string]any      `json:"args"`
	EffectClass    effects.EffectClass `json:"effect_class"`
	Owner          string              `json:"owner"`
	Sealed         bool                `json:"sealed"`
	Recorded       bool                `json:"recorded,omitempty"`
	Failed         bool                `json:"failed,omitempty"`
	AlreadyClaimed bool                `json:"already_claimed,omitempty"`
	Replay         bool                `json:"-"`
	Duplicate      bool                `json:"-"`
}

// ReplayBlocked reports whether a pre hook must refuse this call: it is a
// replay and its effect is gated or unknown.
func ReplayBlocked(r Record) bool {
	return r.Replay && retryBlocked(r)
}

func retryBlocked(r Record) bool {
	switch r.EffectClass {
	case effects.PURE, effects.READ_ONLY, effects.IDEMPOTENT_WRITE,
		effects.NON_IDEMPOTENT_WRITE, effects.AGENT_SPAWN, effects.SENSITIVE:
		return effects.RequiresBlockAndGate(r.EffectClass)
	default:
		return true
	}
}

func (s Session) classify(name string, annotations map[string]any) effects.EffectClass {
	if s.Classify != nil {
		return s.Classify(name, annotations)
	}
	return effects.ClassifyTool(name, annotations)
}

func (s Session) lock() (func(), error) {
	if err := os.MkdirAll(s.WorkDir, 0o700); err != nil {
		return nil, err
	}
	return acquireLock(s.lockPath(), lockWait)
}

func (s Session) openLog() (*nodelog.NodeLog, error) {
	return nodelog.Open(filepath.Join(s.WorkDir, "nodes.sqlite"))
}

// SealBeforeTool seals one tool call. It does not run the tool.
func (s Session) SealBeforeTool(name string, args map[string]any, toolUseID string, annotations map[string]any) (Record, error) {
	if toolUseID == "" {
		return Record{}, errors.New("hosthook: tool_use_id is required")
	}
	safeArgs, err := jsonableMap(args)
	if err != nil {
		return Record{}, err
	}
	effect := s.classify(name, annotations)
	release, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	st, err := s.loadState()
	if err != nil {
		return Record{}, err
	}
	if done, ok := st.Done[toolUseID]; ok {
		done.Replay = true
		return done, nil
	}
	if pending, ok := st.Pending[toolUseID]; ok {
		if pending.Owner != s.Owner {
			pending.Duplicate = true
		} else if retryBlocked(pending) {
			pending.Replay = true
		} else {
			pending.Duplicate = true
		}
		return pending, nil
	}
	step := st.NextStep
	if err := s.writeSeal(step, name, safeArgs); err != nil {
		return Record{}, err
	}
	rec := Record{
		ToolUseID:   toolUseID,
		StepN:       step,
		ToolName:    name,
		Args:        safeArgs,
		EffectClass: effect,
		Owner:       s.Owner,
		Sealed:      true,
	}
	st.Pending[toolUseID] = rec
	st.NextStep = step + 1
	if err := s.saveState(st); err != nil {
		return Record{}, err
	}
	return rec, nil
}

func (s Session) writeSeal(step int, name string, args map[string]any) error {
	log, err := s.openLog()
	if err != nil {
		return err
	}
	defer log.Close()
	if _, err := log.Append("PROJECT_CONTEXT", &step,
		map[string]any{"goal": "host-hook", "tool": name},
		s.TrajectoryID, s.TenantID, 0); err != nil {
		return err
	}
	plan := map[string]any{"tool_calls": []any{map[string]any{"name": name, "args": args}}}
	_, err = log.Append("DECISION", &step, resume.DecisionPayload(plan, nil), s.TrajectoryID, s.TenantID, 1)
	return err
}

// pendingFor loads state under the caller's lock and returns the pending
// record. A done record comes back with done=true.
func (s Session) pendingFor(toolUseID string) (*state, Record, bool, error) {
	if toolUseID == "" {
		return nil, Record{}, false, errors.New("hosthook: tool_use_id is required")
	}
	st, err := s.loadState()
	if err != nil {
		return nil, Record{}, false, err
	}
	if done, ok := st.Done[toolUseID]; ok {
		return st, done, true, nil
	}
	pending, ok := st.Pending[toolUseID]
	if !ok {
		return nil, Record{}, false, fmt.Errorf("%w %s", ErrNoSeal, toolUseID)
	}
	if err := effects.AssertOpenWorldEffect(pending.ToolName, pending.EffectClass, false); err != nil {
		return nil, Record{}, false, err
	}
	return st, pending, false, nil
}

func (s Session) closePending(st *state, toolUseID string, done Record) error {
	st.Done[toolUseID] = done
	delete(st.Pending, toolUseID)
	return s.saveState(st)
}

// RecordToolResult records the host's result for a call already sealed.
// The body handed to the recorder returns result. It does not call the tool.
func (s Session) RecordToolResult(toolUseID string, result any) (Record, error) {
	safeResult, err := jsonable(result)
	if err != nil {
		return Record{}, err
	}
	release, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	st, pending, done, err := s.pendingFor(toolUseID)
	if err != nil || done {
		return pending, err
	}
	if err := s.writeResult(pending, safeResult); err != nil {
		return Record{}, err
	}
	pending.Recorded = true
	if err := s.closePending(st, toolUseID, pending); err != nil {
		return Record{}, err
	}
	return pending, nil
}

func (s Session) writeResult(rec Record, result any) error {
	log, err := s.openLog()
	if err != nil {
		return err
	}
	defer log.Close()
	body := func(map[string]any) (any, error) { return result, nil }
	var call resume.ToolFunc
	if effects.RequiresBlockAndGate(rec.EffectClass) {
		call = resume.MakeGatedToolCall(log, s.TrajectoryID, s.TenantID, rec.StepN, toolSeq, rec.ToolName, body)
	} else {
		call = resume.MakePlainToolCall(log, s.TrajectoryID, s.TenantID, rec.StepN, toolSeq, rec.ToolName, body)
	}
	if _, err := call(rec.Args); err != nil {
		return err
	}
	step := rec.StepN
	_, err = log.Append("COMMIT_STEP", &step, map[string]any{}, s.TrajectoryID, s.TenantID, commitSeq)
	return err
}

// RecordToolFailure records a host-reported failure and clears the pending
// row. It writes TOOL_CALL, ABORT HOST_REPORTED_FAILURE, then COMMIT_STEP.
// When an outcome is already stored it writes nothing more.
func (s Session) RecordToolFailure(toolUseID string, errVal any) (Record, error) {
	text, err := errorText(errVal)
	if err != nil {
		return Record{}, err
	}
	release, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	st, pending, done, err := s.pendingFor(toolUseID)
	if err != nil || done {
		return pending, err
	}
	claimed, err := s.writeFailure(pending, text)
	if err != nil {
		return Record{}, err
	}
	pending.Failed = true
	pending.Recorded = !claimed
	pending.AlreadyClaimed = claimed
	if err := s.closePending(st, toolUseID, pending); err != nil {
		return Record{}, err
	}
	return pending, nil
}

// writeFailure returns alreadyClaimed=true when an outcome was already
// stored and nothing was written.
func (s Session) writeFailure(rec Record, text string) (bool, error) {
	log, err := s.openLog()
	if err != nil {
		return false, err
	}
	defer log.Close()
	step := rec.StepN
	outcomeSeq := toolSeq + 1
	for _, kind := range []string{"TOOL_RESULT", "ABORT"} {
		has, err := log.Has(s.TrajectoryID, s.TenantID, step, kind, &outcomeSeq)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	args := rec.Args
	if args == nil {
		args = map[string]any{}
	}
	payload := map[string]any{
		"tool":            rec.ToolName,
		"args":            args,
		"idempotency_key": resume.IdempotencyKey(s.TenantID, s.TrajectoryID, step, toolSeq),
	}
	if effects.RequiresBlockAndGate(rec.EffectClass) {
		_, err = log.ClaimToolCall(step, payload, s.TrajectoryID, s.TenantID, toolSeq)
	} else {
		_, err = log.Append("TOOL_CALL", &step, payload, s.TrajectoryID, s.TenantID, toolSeq)
	}
	if err != nil {
		return false, err
	}
	abort := map[string]any{"reason": hostFailure, "tool": rec.ToolName, "error": text}
	if _, err := log.Append("ABORT", &step, abort, s.TrajectoryID, s.TenantID, outcomeSeq); err != nil {
		return false, err
	}
	_, err = log.Append("COMMIT_STEP", &step, map[string]any{}, s.TrajectoryID, s.TenantID, commitSeq)
	return false, err
}

// jsonable round-trips v through JSON so it holds only JSON types. Numbers
// stay json.Number so integers above 2^53 keep their exact value.
func jsonable(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("hosthook: value is not JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func jsonableMap(args map[string]any) (map[string]any, error) {
	if args == nil {
		return map[string]any{}, nil
	}
	v, err := jsonable(args)
	if err != nil {
		return nil, err
	}
	return v.(map[string]any), nil
}

// errorText matches session.py _error_text: a string as is, anything else as
// json.dumps(sort_keys=True), capped at errorTextLimit characters.
func errorText(errVal any) (string, error) {
	if errVal == nil || errVal == "" {
		return "host reported a tool failure", nil
	}
	text, ok := errVal.(string)
	if !ok {
		v, err := jsonable(errVal)
		if err != nil {
			return "", err
		}
		text = pyDumps(v)
	}
	if utf8.RuneCountInString(text) > errorTextLimit {
		text = string([]rune(text)[:errorTextLimit])
	}
	return text, nil
}

// pyDumps writes v the way Python json.dumps(v, sort_keys=True) does:
// ", " and ": " separators, sorted keys, non-ASCII escaped.
func pyDumps(v any) string {
	var b strings.Builder
	writePy(&b, v)
	return b.String()
}

func writePy(b *strings.Builder, v any) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			writePy(b, k)
			b.WriteString(": ")
			writePy(b, t[k])
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteString(", ")
			}
			writePy(b, item)
		}
		b.WriteByte(']')
	case nil:
		b.WriteString("null")
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(t)
		b.WriteString(asciiEscape(strings.TrimSuffix(buf.String(), "\n")))
	}
}

func asciiEscape(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r < utf8.RuneSelf:
			b.WriteRune(r)
		case r < 0x10000:
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&b, "\\u%04x\\u%04x", r1, r2)
		}
	}
	return b.String()
}
