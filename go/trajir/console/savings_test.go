package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavingsEmpty(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.Savings()
	if err != nil {
		t.Fatal(err)
	}
	if got.TokensAvoidedEstimated != nil || got.LatestTokensAvoidedEstimated != nil {
		t.Fatalf("empty store must keep estimates null, got %+v", got)
	}
	if got.TrajectoryCount != 0 || got.ProjectionHits != 0 || got.TrajectoriesWithSavings != 0 {
		t.Fatalf("empty counts %+v", got)
	}
	if got.ByTrajectory == nil || len(got.ByTrajectory) != 0 {
		t.Fatalf("by_trajectory must be empty slice, got %#v", got.ByTrajectory)
	}
}

func TestSavingsSumsLifetimeAcrossTrajectories(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedEconomy(t, st, "econ-a")
	seedEconomy(t, st, "econ-b")
	plain := sampleEvent(KindNodeAppended)
	plain.TrajectoryID = "plain"
	plain.Payload = json.RawMessage(`{"node_id":"n1","kind":"TOOL_CALL"}`)
	if err := st.Append(plain); err != nil {
		t.Fatal(err)
	}

	got, err := st.Savings()
	if err != nil {
		t.Fatal(err)
	}
	if got.TrajectoryCount != 3 {
		t.Fatalf("trajectory_count=%d", got.TrajectoryCount)
	}
	if got.TrajectoriesWithSavings != 2 {
		t.Fatalf("with_savings=%d", got.TrajectoriesWithSavings)
	}
	if got.ProjectionHits != 4 {
		t.Fatalf("projection_hits=%d", got.ProjectionHits)
	}
	if got.TokensAvoidedEstimated == nil || *got.TokensAvoidedEstimated != 160 {
		t.Fatalf("lifetime total %+v", got.TokensAvoidedEstimated)
	}
	if got.LatestTokensAvoidedEstimated == nil || *got.LatestTokensAvoidedEstimated != 150 {
		t.Fatalf("latest total %+v", got.LatestTokensAvoidedEstimated)
	}
	if len(got.ByTrajectory) != 3 {
		t.Fatalf("rows %+v", got.ByTrajectory)
	}
	if got.ByTrajectory[0].TrajectoryID != "econ-a" || got.ByTrajectory[1].TrajectoryID != "econ-b" || got.ByTrajectory[2].TrajectoryID != "plain" {
		t.Fatalf("order %+v", got.ByTrajectory)
	}
	if got.ByTrajectory[0].TokensAvoidedEstimated == nil || *got.ByTrajectory[0].TokensAvoidedEstimated != 80 {
		t.Fatalf("econ-a lifetime %+v", got.ByTrajectory[0])
	}
	if got.ByTrajectory[0].LatestTokensAvoidedEstimated == nil || *got.ByTrajectory[0].LatestTokensAvoidedEstimated != 75 {
		t.Fatalf("econ-a latest %+v", got.ByTrajectory[0])
	}
	if got.ByTrajectory[2].TokensAvoidedEstimated != nil || got.ByTrajectory[2].ProjectionHits != 0 {
		t.Fatalf("plain row %+v", got.ByTrajectory[2])
	}
}

func TestSavingsHTTP(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedEconomy(t, st, "econ")
	srv := NewServer(st, "secret")

	req := httptest.NewRequest(http.MethodGet, "/v1/savings", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/savings", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var got SavingsView
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.TokensAvoidedEstimated == nil || *got.TokensAvoidedEstimated != 80 {
		t.Fatalf("savings %+v", got)
	}
	if strings.Contains(rr2.Body.String(), "billed") || strings.Contains(rr2.Body.String(), "invoice") || strings.Contains(rr2.Body.String(), "$") {
		t.Fatalf("savings JSON must not claim billed money: %s", rr2.Body.String())
	}
}

func TestSavingsHTTPBrokenFile(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Root(), trajectoriesDir, "broken.ndjson")
	if err := os.WriteFile(path, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/savings", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusInternalServerError {
		t.Fatalf("broken file status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSavingsUITrustsServerAggregates(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	html := rr.Body.String()
	for _, want := range []string{`id="savings-total"`, "Tokens saved (est.)"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in html", want)
		}
	}

	req2 := httptest.NewRequest(http.MethodGet, "/ui/app.js", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	js := rr2.Body.String()
	for _, want := range []string{"/v1/savings", "tokens_avoided_estimated", "Tokens saved (est.)"} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q in js", want)
		}
	}
	for _, banned := range []string{"billed tokens", "invoice tokens", "Math.ceil", "$ saved"} {
		if strings.Contains(js, banned) {
			t.Fatalf("UI must not invent billing claims: %s", banned)
		}
	}
}

func seedEconomy(t *testing.T, st *Store, id string) {
	t.Helper()
	for _, e := range loadEconomyFixture(t) {
		e.TrajectoryID = id
		if err := st.Append(e); err != nil {
			t.Fatal(err)
		}
	}
}
