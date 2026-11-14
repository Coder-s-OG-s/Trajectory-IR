package console

const (
	sealStatusCreated  = "created"
	sealStatusVerified = "verified"
	sealStatusFailed   = "failed"
)

// SealsView is the Locked decisions panel. Rows follow the log.
// Status is copied from the event. This does not hash the plan again.
type SealsView struct {
	Rows []SealRow `json:"rows"`
}

// SealRow is one seal.created or seal.verified.
type SealRow struct {
	ID          string   `json:"id"`
	TS          string   `json:"ts,omitempty"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	NodeID      string   `json:"node_id,omitempty"`
	StepN       int      `json:"step_n,omitempty"`
	ContentHash string   `json:"content_hash,omitempty"`
	ToolNames   []string `json:"tool_names,omitempty"`
	Error       string   `json:"error,omitempty"`
}

func deriveSeals(events []Event) SealsView {
	rows := make([]SealRow, 0)
	for _, e := range events {
		switch e.Kind {
		case KindSealCreated:
			rows = append(rows, sealFromCreated(e))
		case KindSealVerified:
			rows = append(rows, sealFromVerified(e))
		}
	}
	return SealsView{Rows: rows}
}

func sealFromCreated(e Event) SealRow {
	row := SealRow{
		ID:     e.ID,
		TS:     e.TS,
		Kind:   e.Kind,
		Status: sealStatusCreated,
	}
	fillSealRow(&row, e)
	return row
}

func sealFromVerified(e Event) SealRow {
	row := SealRow{
		ID:     e.ID,
		TS:     e.TS,
		Kind:   e.Kind,
		Status: sealStatusFailed,
	}
	ok, has := payloadBool(e.Payload, "ok")
	if has && ok {
		row.Status = sealStatusVerified
	}
	if msg, ok := payloadString(e.Payload, "error"); ok {
		row.Error = msg
	}
	fillSealRow(&row, e)
	return row
}

func fillSealRow(row *SealRow, e Event) {
	if id, ok := payloadString(e.Payload, "node_id"); ok {
		row.NodeID = id
	}
	if step, ok := payloadIntOK(e.Payload, "step_n"); ok {
		row.StepN = step
	}
	if hash, ok := payloadString(e.Payload, "content_hash"); ok {
		row.ContentHash = hash
	}
	if names, ok := payloadStringSlice(e.Payload, "tool_names"); ok {
		row.ToolNames = names
	}
}
