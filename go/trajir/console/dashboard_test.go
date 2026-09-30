package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardHomeRollup(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedEconomy(t, st, "econ-a")
	plain := sampleEvent(KindNodeAppended)
	plain.TrajectoryID = "plain"
	plain.Payload = json.RawMessage(`{"node_id":"n1","kind":"TOOL_CALL"}`)
	if err := st.Append(plain); err != nil {
		t.Fatal(err)
	}

	got, err := st.Dashboard()
	if err != nil {
		t.Fatal(err)
	}
	if got.Savings.TokensAvoidedEstimated == nil || *got.Savings.TokensAvoidedEstimated != 80 {
		t.Fatalf("savings %+v", got.Savings.TokensAvoidedEstimated)
	}
	if len(got.Runs) != 2 {
		t.Fatalf("runs %+v", got.Runs)
	}
	if got.Runs[0].TrajectoryID != "econ-a" || got.Runs[1].TrajectoryID != "plain" {
		t.Fatalf("order %+v", got.Runs)
	}
	if got.Runs[0].EventCount != 3 || got.Runs[0].ProjectionHits != 2 {
		t.Fatalf("econ card %+v", got.Runs[0])
	}
	if got.Runs[1].TokensAvoidedEstimated != nil || got.Runs[1].EventCount != 1 {
		t.Fatalf("plain card %+v", got.Runs[1])
	}
}

func TestDashboardHTTP(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedEconomy(t, st, "econ")
	srv := NewServer(st, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got DashboardView
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Runs) != 1 || got.Runs[0].TrajectoryID != "econ" {
		t.Fatalf("runs %+v", got.Runs)
	}
	if got.Savings.TokensAvoidedEstimated == nil || *got.Savings.TokensAvoidedEstimated != 80 {
		t.Fatalf("savings %+v", got.Savings)
	}
}

func TestDashboardUIShell(t *testing.T) {
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
	for _, want := range []string{
		`id="home"`,
		`id="theme-toggle"`,
		`id="live-toggle"`,
		"Find a run",
		"All runs",
		"Tokens saved (est.)",
		"Show data folder",
		"Open PowerShell here",
		"Light mode",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in html", want)
		}
	}
	req2 := httptest.NewRequest(http.MethodGet, "/ui/app.js", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	js := rr2.Body.String()
	for _, want := range []string{"/v1/dashboard", "renderHome", "kindLabel", "applyTheme", "tickLive", `return "none"`} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q in js", want)
		}
	}
	for _, src := range []string{html, js} {
		if strings.Contains(src, "\u2014") || strings.Contains(src, "\u2013") {
			t.Fatal("dashboard copy must not use em or en dashes")
		}
		if strings.Contains(src, "\u00b7") {
			t.Fatal("dashboard copy must not use middle dots")
		}
	}
}
