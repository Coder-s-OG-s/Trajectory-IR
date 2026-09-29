package console

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"sort"
	"strconv"
)

// EconomyView is the Context economy panel (docs/CONSOLE_EVENTS.md §3).
// Headline token fields are copied from Summary so the panel and the rollup
// stay on the same latest context.projected.
type EconomyView struct {
	ProjectionHits           int  `json:"projection_hits"`
	RedactionCollapses       int  `json:"redaction_collapses"`
	RawEstimatedTokens       *int `json:"raw_estimated_tokens"`
	ProjectedEstimatedTokens *int `json:"projected_estimated_tokens"`
	TokensAvoidedEstimated   *int `json:"tokens_avoided_estimated"`
	SizeUnitsSaved           *int `json:"size_units_saved"`

	LifetimeRawEstimatedTokens       *int `json:"lifetime_raw_estimated_tokens"`
	LifetimeProjectedEstimatedTokens *int `json:"lifetime_projected_estimated_tokens"`
	LifetimeTokensAvoidedEstimated   *int `json:"lifetime_tokens_avoided_estimated"`

	Steps   []EconomyStep `json:"steps"`
	Largest []EconomyStep `json:"largest"`
}

// EconomyStep is one context.projected or redaction.applied event.
type EconomyStep struct {
	ID     string `json:"id"`
	TS     string `json:"ts"`
	Kind   string `json:"kind"`
	StepN  *int   `json:"step_n,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Metric string `json:"metric,omitempty"`

	Budget       *int `json:"budget,omitempty"`
	SizeUnits    *int `json:"size_units,omitempty"`
	RawSizeUnits *int `json:"raw_size_units,omitempty"`
	Dropped      int  `json:"dropped"`
	Included     int  `json:"included"`

	ThoughtCollapses int `json:"thought_collapses"`
	SecretFieldHits  int `json:"secret_field_hits"`

	RawEstimatedTokens       *int `json:"raw_estimated_tokens"`
	ProjectedEstimatedTokens *int `json:"projected_estimated_tokens"`
	TokensAvoidedEstimated   *int `json:"tokens_avoided_estimated"`
}

func deriveEconomy(events []Event, s Summary) EconomyView {
	view := EconomyView{
		RedactionCollapses:       s.RedactionCollapses,
		RawEstimatedTokens:       copyInt(s.RawEstimatedTokens),
		ProjectedEstimatedTokens: copyInt(s.ProjectedEstimatedTokens),
		TokensAvoidedEstimated:   copyInt(s.TokensAvoidedEstimated),
		Steps:                    []EconomyStep{},
		Largest:                  []EconomyStep{},
	}
	if s.RawSizeUnits != nil {
		saved := *s.RawSizeUnits - s.ProjectionSizeUnits
		if saved < 0 {
			saved = 0
		}
		view.SizeUnitsSaved = copyInt(&saved)
	}

	var lifeRaw, lifeProj, lifeAvoid int
	var sawRaw, sawProj, sawAvoid bool

	for _, e := range events {
		switch e.Kind {
		case KindContextProjected:
			view.ProjectionHits++
			step := projectionStep(e)
			view.Steps = append(view.Steps, step)
			if step.RawEstimatedTokens != nil {
				lifeRaw += *step.RawEstimatedTokens
				sawRaw = true
			}
			if step.ProjectedEstimatedTokens != nil {
				lifeProj += *step.ProjectedEstimatedTokens
				sawProj = true
			}
			if step.TokensAvoidedEstimated != nil {
				lifeAvoid += *step.TokensAvoidedEstimated
				sawAvoid = true
			}
		case KindRedactionApplied:
			view.Steps = append(view.Steps, redactionStep(e))
		}
	}
	if sawRaw {
		view.LifetimeRawEstimatedTokens = copyInt(&lifeRaw)
	}
	if sawProj {
		view.LifetimeProjectedEstimatedTokens = copyInt(&lifeProj)
	}
	if sawAvoid {
		view.LifetimeTokensAvoidedEstimated = copyInt(&lifeAvoid)
	}

	view.Largest = append([]EconomyStep{}, projectionSteps(view.Steps)...)
	sort.SliceStable(view.Largest, func(i, j int) bool {
		return largerPayload(view.Largest[i], view.Largest[j])
	})
	return view
}

func projectionSteps(steps []EconomyStep) []EconomyStep {
	out := make([]EconomyStep, 0)
	for _, step := range steps {
		if step.Kind == KindContextProjected {
			out = append(out, step)
		}
	}
	return out
}

func largerPayload(a, b EconomyStep) bool {
	if a.RawEstimatedTokens != nil || b.RawEstimatedTokens != nil {
		if a.RawEstimatedTokens == nil {
			return false
		}
		if b.RawEstimatedTokens == nil {
			return true
		}
		if *a.RawEstimatedTokens != *b.RawEstimatedTokens {
			return *a.RawEstimatedTokens > *b.RawEstimatedTokens
		}
	}
	if a.SizeUnits != nil || b.SizeUnits != nil {
		if a.SizeUnits == nil {
			return false
		}
		if b.SizeUnits == nil {
			return true
		}
		return *a.SizeUnits > *b.SizeUnits
	}
	return false
}

func projectionStep(e Event) EconomyStep {
	step := EconomyStep{
		ID:       e.ID,
		TS:       e.TS,
		Kind:     e.Kind,
		Dropped:  0,
		Included: 0,
	}
	if n, ok := payloadIntOK(e.Payload, "step_n"); ok {
		step.StepN = copyInt(&n)
	}
	if metric, ok := payloadString(e.Payload, "metric"); ok {
		step.Metric = metric
	}
	if n, ok := payloadIntOK(e.Payload, "budget"); ok {
		step.Budget = copyInt(&n)
	}
	if n, ok := payloadIntOK(e.Payload, "size_units"); ok {
		step.SizeUnits = copyInt(&n)
	}
	if n, ok := payloadIntOK(e.Payload, "raw_size_units"); ok {
		step.RawSizeUnits = copyInt(&n)
	}
	if ids, ok := payloadStringSlice(e.Payload, "dropped_ids"); ok {
		step.Dropped = len(ids)
	}
	if ids, ok := payloadStringSlice(e.Payload, "included_ids"); ok {
		step.Included = len(ids)
	}

	rawLen, hasRaw := payloadIntOK(e.Payload, "raw_char_len")
	projLen, hasProj := payloadIntOK(e.Payload, "projected_char_len")
	if hasRaw {
		t := EstimatedTokens(rawLen)
		step.RawEstimatedTokens = copyInt(&t)
	}
	if hasProj {
		t := EstimatedTokens(projLen)
		step.ProjectedEstimatedTokens = copyInt(&t)
	}
	if hasRaw && hasProj {
		avoided := EstimatedTokens(rawLen) - EstimatedTokens(projLen)
		if avoided < 0 {
			avoided = 0
		}
		step.TokensAvoidedEstimated = copyInt(&avoided)
	}
	return step
}

func redactionStep(e Event) EconomyStep {
	step := EconomyStep{
		ID:   e.ID,
		TS:   e.TS,
		Kind: e.Kind,
	}
	if mode, ok := payloadString(e.Payload, "mode"); ok {
		step.Mode = mode
	}
	if n, ok := payloadIntOK(e.Payload, "step_n"); ok {
		step.StepN = copyInt(&n)
	}
	if n, ok := payloadIntOK(e.Payload, "thought_collapses"); ok {
		step.ThoughtCollapses = n
	}
	if n, ok := payloadIntOK(e.Payload, "secret_field_hits"); ok {
		step.SecretFieldHits = n
	}
	return step
}

func copyInt(v *int) *int {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}

// EconomyTableJSON is the partner export of the economy object.
func EconomyTableJSON(v EconomyView) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// EconomyTableCSV is the partner export of economy steps.
// Null estimates are empty cells. The header matches EconomyStep names.
func EconomyTableCSV(v EconomyView) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{
		"id", "ts", "kind", "step_n", "mode",
		"raw_estimated_tokens", "projected_estimated_tokens", "tokens_avoided_estimated",
		"dropped", "thought_collapses", "secret_field_hits",
	}
	if err := w.Write(header); err != nil {
		return "", err
	}
	for _, step := range v.Steps {
		row := []string{
			step.ID,
			step.TS,
			step.Kind,
			csvInt(step.StepN),
			step.Mode,
			csvInt(step.RawEstimatedTokens),
			csvInt(step.ProjectedEstimatedTokens),
			csvInt(step.TokensAvoidedEstimated),
			strconv.Itoa(step.Dropped),
			strconv.Itoa(step.ThoughtCollapses),
			strconv.Itoa(step.SecretFieldHits),
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	return buf.String(), w.Error()
}

func csvInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
