package console

const (
	statusCreated   = "created"
	statusVerified  = "verified"
	statusFailed    = "failed"
	chainContinuous = "continuous"
	chainBreak      = "break"
)

// SealView is one seal.* event as the reader projects it for the Seals panel.
// Status comes from kind and payload.ok. Hashes are not recomputed.
type SealView struct {
	ID           string   `json:"id"`
	TS           string   `json:"ts"`
	Kind         string   `json:"kind"`
	Status       string   `json:"status"`
	Chain        string   `json:"chain"`
	NodeID       string   `json:"node_id,omitempty"`
	StepN        *int     `json:"step_n,omitempty"`
	ContentHash  string   `json:"content_hash,omitempty"`
	ToolNames    []string `json:"tool_names,omitempty"`
	Error        string   `json:"error,omitempty"`
	FromNodeID   string   `json:"from_node_id,omitempty"`
	ToNodeID     string   `json:"to_node_id,omitempty"`
	FromSeq      *int     `json:"from_seq,omitempty"`
	ToSeq        *int     `json:"to_seq,omitempty"`
	CoveredNodes int      `json:"covered_nodes"`
}

type nodeSpan struct {
	ids  []string
	seqs []*int
}

func (sp *nodeSpan) add(id string, seq *int) {
	sp.ids = append(sp.ids, id)
	sp.seqs = append(sp.seqs, seq)
}

func (sp nodeSpan) apply(v *SealView) {
	v.CoveredNodes = len(sp.ids)
	if len(sp.ids) == 0 {
		return
	}
	v.FromNodeID = sp.ids[0]
	v.ToNodeID = sp.ids[len(sp.ids)-1]
	if sp.seqs[0] != nil && sp.seqs[len(sp.seqs)-1] != nil {
		v.FromSeq = cloneInt(*sp.seqs[0])
		v.ToSeq = cloneInt(*sp.seqs[len(sp.seqs)-1])
	}
}

func deriveSeals(events []Event) []SealView {
	out := make([]SealView, 0)
	var span nodeSpan
	var prevStep *int
	createdRange := map[string]nodeSpan{}
	createdStep := map[string]int{}

	for _, e := range events {
		switch e.Kind {
		case KindNodeAppended:
			id, _ := payloadString(e.Payload, "node_id")
			var seq *int
			if n, ok := payloadIntOK(e.Payload, "seq"); ok {
				seq = cloneInt(n)
			}
			span.add(id, seq)
		case KindSealCreated:
			view := sealViewFrom(e, statusCreated)
			span.apply(&view)
			if view.NodeID != "" {
				createdRange[view.NodeID] = span
				if view.StepN != nil {
					createdStep[view.NodeID] = *view.StepN
				}
			}
			span = nodeSpan{}
			view.Chain = chainContinuous
			if view.StepN == nil {
				if prevStep != nil {
					view.Chain = chainBreak
				}
			} else if prevStep != nil && *view.StepN <= *prevStep {
				view.Chain = chainBreak
			}
			if view.StepN != nil {
				prevStep = cloneInt(*view.StepN)
			}
			if names, ok := payloadStringSlice(e.Payload, "tool_names"); ok {
				view.ToolNames = names
			}
			out = append(out, view)
		case KindSealVerified:
			ok, _ := payloadBool(e.Payload, "ok")
			status := statusVerified
			if !ok {
				status = statusFailed
			}
			view := sealViewFrom(e, status)
			if msg, has := payloadString(e.Payload, "error"); has {
				view.Error = msg
			}
			view.Chain = chainBreak
			if view.NodeID != "" {
				if sp, found := createdRange[view.NodeID]; found {
					sp.apply(&view)
					view.Chain = chainContinuous
				}
				if view.StepN == nil {
					if step, found := createdStep[view.NodeID]; found {
						view.StepN = cloneInt(step)
					}
				}
			}
			out = append(out, view)
		}
	}
	return out
}

func sealViewFrom(e Event, status string) SealView {
	nodeID, _ := payloadString(e.Payload, "node_id")
	hash, _ := payloadString(e.Payload, "content_hash")
	view := SealView{
		ID:          e.ID,
		TS:          e.TS,
		Kind:        e.Kind,
		Status:      status,
		NodeID:      nodeID,
		ContentHash: hash,
	}
	if step, ok := payloadIntOK(e.Payload, "step_n"); ok {
		view.StepN = cloneInt(step)
	}
	return view
}

func cloneInt(v int) *int {
	n := v
	return &n
}
