package console

import (
	"encoding/json"
	"time"
)

// Summary is the reader rollup for one trajectory (CONSOLE_EVENTS.md §3–§4).
type Summary struct {
	TrajectoryID string `json:"trajectory_id"`
	EventCount   int    `json:"event_count"`
	NodeCount    int    `json:"node_count"`
	LastTS       string `json:"last_ts,omitempty"`

	SealCreatedCount  int `json:"seal_created_count"`
	SealVerifiedOK    int `json:"seal_verified_ok"`
	SealVerifiedFail  int `json:"seal_verified_fail"`

	ProjectionSizeUnits int  `json:"projection_size_units,omitempty"`
	ProjectionBudget    int  `json:"projection_budget,omitempty"`
	NodesDropped        int  `json:"nodes_dropped,omitempty"`
	RawSizeUnits        *int `json:"raw_size_units,omitempty"`

	RawEstimatedTokens       *int `json:"raw_estimated_tokens"`
	ProjectedEstimatedTokens *int `json:"projected_estimated_tokens"`
	TokensAvoidedEstimated   *int `json:"tokens_avoided_estimated"`

	RedactionCollapses int `json:"redaction_collapses"`

	ExportsOK          int    `json:"exports_ok"`
	ImportsOK          int    `json:"imports_ok"`
	LastPackageMode    string `json:"last_package_mode,omitempty"`
	LastPackageBytes   int64  `json:"last_package_bytes,omitempty"`
	LastPackageMembers int    `json:"last_package_members,omitempty"`
	LastPackageNodes   int    `json:"last_package_nodes,omitempty"`
	LastPackageRedacted *bool `json:"last_package_redacted,omitempty"`
	TransferVerifyOK   *bool  `json:"transfer_verify_ok,omitempty"`
}

// Summarize builds aggregates from an ordered event list.
func Summarize(trajectoryID string, events []Event) Summary {
	s := Summary{
		TrajectoryID: trajectoryID,
		EventCount:   len(events),
	}
	var lastTS time.Time

	for _, e := range events {
		if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
			if t.After(lastTS) {
				lastTS = t
				s.LastTS = e.TS
			}
		}

		switch e.Kind {
		case KindNodeAppended:
			s.NodeCount++
		case KindSealCreated:
			s.SealCreatedCount++
		case KindSealVerified:
			ok, _ := payloadBool(e.Payload, "ok")
			if ok {
				s.SealVerifiedOK++
			} else {
				s.SealVerifiedFail++
			}
		case KindContextProjected:
			applyProjection(&s, e.Payload)
		case KindRedactionApplied:
			s.RedactionCollapses += payloadInt(e.Payload, "thought_collapses")
			s.RedactionCollapses += payloadInt(e.Payload, "secret_field_hits")
		case KindExportCompleted:
			ok, has := payloadBool(e.Payload, "ok")
			if has && ok {
				s.ExportsOK++
			}
			applyPackage(&s, e.Payload)
		case KindImportCompleted:
			ok, has := payloadBool(e.Payload, "verify_ok")
			if has {
				s.TransferVerifyOK = &ok
				if ok {
					s.ImportsOK++
				}
			}
			applyPackage(&s, e.Payload)
		}
	}
	return s
}

func applyProjection(s *Summary, raw json.RawMessage) {
	s.ProjectionSizeUnits = payloadInt(raw, "size_units")
	s.ProjectionBudget = payloadInt(raw, "budget")
	if ids, ok := payloadStringSlice(raw, "dropped_ids"); ok {
		s.NodesDropped = len(ids)
	}
	if v, ok := payloadIntOK(raw, "raw_size_units"); ok {
		s.RawSizeUnits = &v
	}

	rawLen, hasRaw := payloadIntOK(raw, "raw_char_len")
	projLen, hasProj := payloadIntOK(raw, "projected_char_len")
	if hasRaw {
		t := EstimatedTokens(rawLen)
		s.RawEstimatedTokens = &t
	} else {
		s.RawEstimatedTokens = nil
	}
	if hasProj {
		t := EstimatedTokens(projLen)
		s.ProjectedEstimatedTokens = &t
	} else {
		s.ProjectedEstimatedTokens = nil
	}
	if hasRaw && hasProj {
		avoided := EstimatedTokens(rawLen) - EstimatedTokens(projLen)
		if avoided < 0 {
			avoided = 0
		}
		s.TokensAvoidedEstimated = &avoided
	} else {
		s.TokensAvoidedEstimated = nil
	}
}

func applyPackage(s *Summary, raw json.RawMessage) {
	if mode, ok := payloadString(raw, "mode"); ok {
		s.LastPackageMode = mode
	}
	if v, ok := payloadInt64OK(raw, "bytes"); ok {
		s.LastPackageBytes = v
	}
	if v, ok := payloadIntOK(raw, "member_count"); ok {
		s.LastPackageMembers = v
	}
	if v, ok := payloadIntOK(raw, "node_count"); ok {
		s.LastPackageNodes = v
	}
	if v, ok := payloadBool(raw, "redacted"); ok {
		s.LastPackageRedacted = &v
	}
}

func payloadMap(raw json.RawMessage) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

func payloadInt(raw json.RawMessage, key string) int {
	v, _ := payloadIntOK(raw, key)
	return v
}

func payloadIntOK(raw json.RawMessage, key string) (int, bool) {
	m := payloadMap(raw)
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func payloadInt64OK(raw json.RawMessage, key string) (int64, bool) {
	m := payloadMap(raw)
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func payloadBool(raw json.RawMessage, key string) (bool, bool) {
	m := payloadMap(raw)
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func payloadString(raw json.RawMessage, key string) (string, bool) {
	m := payloadMap(raw)
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func payloadStringSlice(raw json.RawMessage, key string) ([]string, bool) {
	m := payloadMap(raw)
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
