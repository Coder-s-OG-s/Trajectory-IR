package console

import "encoding/json"

const (
	handoffConnected  = "connected"
	handoffExportOnly = "export_only"
	handoffImportOnly = "import_only"
	handoffFailed     = "failed"
)

// TransfersView is the Transfers panel (docs/CONSOLE_EVENTS.md §4).
// Handoffs pair an export.completed with a later import.completed.
type TransfersView struct {
	ExportsOK int       `json:"exports_ok"`
	ImportsOK int       `json:"imports_ok"`
	Handoffs  []Handoff `json:"handoffs"`
}

// Handoff is one package move. Connected means an import attached to an
// export and verify_ok is true. Same path wins; otherwise the oldest open
// export in this trajectory is used.
type Handoff struct {
	Status       string `json:"status"`
	Mode         string `json:"mode,omitempty"`
	Redacted     *bool  `json:"redacted"`
	Bytes        int64  `json:"bytes"`
	MemberCount  int    `json:"member_count"`
	NodeCount    int    `json:"node_count"`
	ExportID     string `json:"export_id,omitempty"`
	ImportID     string `json:"import_id,omitempty"`
	ExportTS     string `json:"export_ts,omitempty"`
	ImportTS     string `json:"import_ts,omitempty"`
	ExportPath   string `json:"export_path,omitempty"`
	ImportPath   string `json:"import_path,omitempty"`
	ConsolePath  string `json:"console_path,omitempty"`
	ExportSource string `json:"export_source,omitempty"`
	ImportSource string `json:"import_source,omitempty"`
	Runtime      string `json:"runtime,omitempty"`
	VerifyOK     *bool  `json:"verify_ok"`
	Error        string `json:"error,omitempty"`
}

func deriveTransfers(events []Event, s Summary) TransfersView {
	view := TransfersView{
		ExportsOK: s.ExportsOK,
		ImportsOK: s.ImportsOK,
		Handoffs:  []Handoff{},
	}
	// Indices into view.Handoffs. Pointers would dangle once append grows the slice.
	pending := []int{}
	for _, e := range events {
		switch e.Kind {
		case KindExportCompleted:
			view.Handoffs = append(view.Handoffs, handoffFromExport(e))
			pending = append(pending, len(view.Handoffs)-1)
		case KindImportCompleted:
			idx := matchPending(view.Handoffs, pending, e)
			if idx < 0 {
				view.Handoffs = append(view.Handoffs, handoffFromImport(e))
				continue
			}
			applyImport(&view.Handoffs[idx], e)
			pending = dropPending(pending, idx)
		}
	}
	return view
}

func handoffFromExport(e Event) Handoff {
	h := Handoff{
		Status:       handoffExportOnly,
		ExportID:     e.ID,
		ExportTS:     e.TS,
		ExportSource: e.Source,
		Runtime:      e.Runtime,
	}
	fillPackage(&h, e.Payload, true)
	if ok, has := payloadBool(e.Payload, "ok"); has && !ok {
		h.Status = handoffFailed
		if msg, ok := payloadString(e.Payload, "error"); ok {
			h.Error = msg
		}
	}
	return h
}

func handoffFromImport(e Event) Handoff {
	h := Handoff{
		Status:       handoffImportOnly,
		ImportID:     e.ID,
		ImportTS:     e.TS,
		ImportSource: e.Source,
		Runtime:      e.Runtime,
	}
	fillPackage(&h, e.Payload, false)
	applyVerify(&h, e.Payload, false)
	return h
}

func applyImport(h *Handoff, e Event) {
	h.ImportID = e.ID
	h.ImportTS = e.TS
	h.ImportSource = e.Source
	if h.Runtime == "" {
		h.Runtime = e.Runtime
	}
	if path, ok := payloadString(e.Payload, "path"); ok {
		h.ImportPath = path
	}
	if h.Mode == "" {
		if mode, ok := payloadString(e.Payload, "mode"); ok {
			h.Mode = mode
		}
	}
	if h.Redacted == nil {
		if v, ok := payloadBool(e.Payload, "redacted"); ok {
			h.Redacted = &v
		}
	}
	if h.Bytes == 0 {
		if v, ok := payloadInt64OK(e.Payload, "bytes"); ok {
			h.Bytes = v
		}
	}
	if h.MemberCount == 0 {
		if v, ok := payloadIntOK(e.Payload, "member_count"); ok {
			h.MemberCount = v
		}
	}
	if h.NodeCount == 0 {
		if v, ok := payloadIntOK(e.Payload, "node_count"); ok {
			h.NodeCount = v
		}
	}
	applyVerify(h, e.Payload, true)
}

func applyVerify(h *Handoff, raw json.RawMessage, paired bool) {
	ok, has := payloadBool(raw, "verify_ok")
	if !has {
		return
	}
	h.VerifyOK = &ok
	if msg, hasMsg := payloadString(raw, "error"); hasMsg {
		h.Error = msg
	}
	if !ok {
		h.Status = handoffFailed
		return
	}
	if paired && h.Status != handoffFailed {
		h.Status = handoffConnected
	}
}

func matchPending(handoffs []Handoff, pending []int, e Event) int {
	path, _ := payloadString(e.Payload, "path")
	if path != "" {
		for _, idx := range pending {
			if handoffs[idx].ExportPath == path {
				return idx
			}
		}
	}
	if len(pending) == 0 {
		return -1
	}
	return pending[0]
}

func dropPending(pending []int, handoffIdx int) []int {
	for i, p := range pending {
		if p == handoffIdx {
			return append(pending[:i], pending[i+1:]...)
		}
	}
	return pending
}

func fillPackage(h *Handoff, raw json.RawMessage, export bool) {
	if mode, ok := payloadString(raw, "mode"); ok {
		h.Mode = mode
	}
	if v, ok := payloadBool(raw, "redacted"); ok {
		h.Redacted = &v
	}
	if v, ok := payloadInt64OK(raw, "bytes"); ok {
		h.Bytes = v
	}
	if v, ok := payloadIntOK(raw, "member_count"); ok {
		h.MemberCount = v
	}
	if v, ok := payloadIntOK(raw, "node_count"); ok {
		h.NodeCount = v
	}
	if path, ok := payloadString(raw, "path"); ok {
		if export {
			h.ExportPath = path
		} else {
			h.ImportPath = path
		}
	}
	if rel, ok := payloadString(raw, "console_path"); ok && h.ConsolePath == "" {
		h.ConsolePath = rel
	}
}
