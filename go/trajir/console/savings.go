package console

// SavingsView is the cross-trajectory tokens-saved rollup.
// Headline tokens_avoided_estimated is the sum of each trajectory's
// lifetime_tokens_avoided_estimated (docs/CONSOLE_EVENTS.md §3.4).
// These are estimated_tokens (ceil chars/4), not billed tokens.
type SavingsView struct {
	TokensAvoidedEstimated       *int         `json:"tokens_avoided_estimated"`
	LatestTokensAvoidedEstimated *int         `json:"latest_tokens_avoided_estimated"`
	ProjectionHits               int          `json:"projection_hits"`
	TrajectoryCount              int          `json:"trajectory_count"`
	TrajectoriesWithSavings      int          `json:"trajectories_with_savings"`
	ByTrajectory                 []SavingsRow `json:"by_trajectory"`
}

// SavingsRow is one trajectory's contribution to SavingsView.
type SavingsRow struct {
	TrajectoryID                 string `json:"trajectory_id"`
	TokensAvoidedEstimated       *int   `json:"tokens_avoided_estimated"`
	LatestTokensAvoidedEstimated *int   `json:"latest_tokens_avoided_estimated"`
	ProjectionHits               int    `json:"projection_hits"`
}

// Savings sums economy lifetime avoided across every trajectory on disk.
func (s *Store) Savings() (SavingsView, error) {
	ids, err := s.ListTrajectories()
	if err != nil {
		return SavingsView{}, err
	}
	view := SavingsView{
		ByTrajectory:    make([]SavingsRow, 0, len(ids)),
		TrajectoryCount: len(ids),
	}
	var life, latest, hits int
	var sawLife, sawLatest bool
	for _, id := range ids {
		events, err := s.ReadEvents(id)
		if err != nil {
			return SavingsView{}, err
		}
		sum := Summarize(id, events)
		row := SavingsRow{
			TrajectoryID:                 id,
			TokensAvoidedEstimated:       copyInt(sum.Economy.LifetimeTokensAvoidedEstimated),
			LatestTokensAvoidedEstimated: copyInt(sum.Economy.TokensAvoidedEstimated),
			ProjectionHits:               sum.Economy.ProjectionHits,
		}
		view.ByTrajectory = append(view.ByTrajectory, row)
		hits += sum.Economy.ProjectionHits
		if row.TokensAvoidedEstimated != nil {
			life += *row.TokensAvoidedEstimated
			sawLife = true
			view.TrajectoriesWithSavings++
		}
		if row.LatestTokensAvoidedEstimated != nil {
			latest += *row.LatestTokensAvoidedEstimated
			sawLatest = true
		}
	}
	view.ProjectionHits = hits
	if sawLife {
		view.TokensAvoidedEstimated = copyInt(&life)
	}
	if sawLatest {
		view.LatestTokensAvoidedEstimated = copyInt(&latest)
	}
	return view, nil
}
