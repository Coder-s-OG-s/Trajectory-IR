package console

// DashboardView is the operator home page: savings plus one card per run.
type DashboardView struct {
	Savings SavingsView `json:"savings"`
	Runs    []RunCard   `json:"runs"`
}

// RunCard is a short, human-facing summary of one trajectory.
type RunCard struct {
	TrajectoryID                 string `json:"trajectory_id"`
	EventCount                   int    `json:"event_count"`
	NodeCount                    int    `json:"node_count"`
	LastTS                       string `json:"last_ts,omitempty"`
	SealCreatedCount             int    `json:"seal_created_count"`
	SealVerifiedOK               int    `json:"seal_verified_ok"`
	SealVerifiedFail             int    `json:"seal_verified_fail"`
	ExportsOK                    int    `json:"exports_ok"`
	ImportsOK                    int    `json:"imports_ok"`
	TokensAvoidedEstimated       *int   `json:"tokens_avoided_estimated"`
	LatestTokensAvoidedEstimated *int   `json:"latest_tokens_avoided_estimated"`
	ProjectionHits               int    `json:"projection_hits"`
}

// Dashboard scans every trajectory once for the home screen.
func (s *Store) Dashboard() (DashboardView, error) {
	sums, err := s.summarizeAll()
	if err != nil {
		return DashboardView{}, err
	}
	runs := make([]RunCard, 0, len(sums))
	for _, sum := range sums {
		runs = append(runs, runCardFrom(sum))
	}
	return DashboardView{
		Savings: savingsFromSummaries(sums),
		Runs:    runs,
	}, nil
}

func runCardFrom(sum Summary) RunCard {
	return RunCard{
		TrajectoryID:                 sum.TrajectoryID,
		EventCount:                   sum.EventCount,
		NodeCount:                    sum.NodeCount,
		LastTS:                       sum.LastTS,
		SealCreatedCount:             sum.SealCreatedCount,
		SealVerifiedOK:               sum.SealVerifiedOK,
		SealVerifiedFail:             sum.SealVerifiedFail,
		ExportsOK:                    sum.ExportsOK,
		ImportsOK:                    sum.ImportsOK,
		TokensAvoidedEstimated:       copyInt(sum.Economy.LifetimeTokensAvoidedEstimated),
		LatestTokensAvoidedEstimated: copyInt(sum.Economy.TokensAvoidedEstimated),
		ProjectionHits:               sum.Economy.ProjectionHits,
	}
}
