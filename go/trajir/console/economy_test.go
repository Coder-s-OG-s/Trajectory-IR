package console

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestEconomyMatchesSummaryFixture(t *testing.T) {
	t.Parallel()
	events := loadEconomyFixture(t)
	sum := Summarize("econ", events)
	ev := sum.Economy

	if ev.RawEstimatedTokens == nil || sum.RawEstimatedTokens == nil || *ev.RawEstimatedTokens != *sum.RawEstimatedTokens || *ev.RawEstimatedTokens != 100 {
		t.Fatalf("raw headline ev=%v sum=%v", ev.RawEstimatedTokens, sum.RawEstimatedTokens)
	}
	if ev.ProjectedEstimatedTokens == nil || *ev.ProjectedEstimatedTokens != *sum.ProjectedEstimatedTokens || *ev.ProjectedEstimatedTokens != 25 {
		t.Fatalf("projected headline %+v", ev.ProjectedEstimatedTokens)
	}
	if ev.TokensAvoidedEstimated == nil || *ev.TokensAvoidedEstimated != *sum.TokensAvoidedEstimated || *ev.TokensAvoidedEstimated != 75 {
		t.Fatalf("avoided headline %+v", ev.TokensAvoidedEstimated)
	}
	if ev.ProjectionHits != 2 || ev.RedactionCollapses != sum.RedactionCollapses || ev.RedactionCollapses != 2 {
		t.Fatalf("breakdown %+v sum collapses %d", ev, sum.RedactionCollapses)
	}
	if ev.SizeUnitsSaved == nil || *ev.SizeUnitsSaved != 300 {
		t.Fatalf("size saved %+v", ev.SizeUnitsSaved)
	}
	if ev.LifetimeTokensAvoidedEstimated == nil || *ev.LifetimeTokensAvoidedEstimated != 80 {
		t.Fatalf("lifetime avoided %+v", ev.LifetimeTokensAvoidedEstimated)
	}
	if len(ev.Steps) != 3 || ev.Steps[0].ID != "p1" || ev.Steps[1].Kind != KindRedactionApplied || ev.Steps[2].ID != "p2" {
		t.Fatalf("steps %+v", ev.Steps)
	}
	if len(ev.Largest) != 2 || ev.Largest[0].ID != "p2" || ev.Largest[1].ID != "p1" {
		t.Fatalf("largest %+v", ev.Largest)
	}
	if ev.Steps[1].ThoughtCollapses != 1 || ev.Steps[1].SecretFieldHits != 1 {
		t.Fatalf("redaction step %+v", ev.Steps[1])
	}

	csvText, err := EconomyTableCSV(ev)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csvText, "p2") || !strings.Contains(csvText, "75") {
		t.Fatalf("csv missing step figures:\n%s", csvText)
	}
	if strings.Contains(csvText, "{") {
		t.Fatal("csv leaked a JSON payload")
	}
	js, err := EconomyTableJSON(ev)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), `"tokens_avoided_estimated": 75`) {
		t.Fatalf("json export drifted:\n%s", js)
	}
}

func TestEconomyZeroProjections(t *testing.T) {
	t.Parallel()
	sum := Summarize("empty", nil)
	ev := sum.Economy
	if ev.ProjectionHits != 0 || ev.RawEstimatedTokens != nil || ev.TokensAvoidedEstimated != nil || ev.SizeUnitsSaved != nil {
		t.Fatalf("zero projection should stay null, got %+v", ev)
	}
	if ev.Steps == nil || len(ev.Steps) != 0 || ev.Largest == nil || ev.LifetimeTokensAvoidedEstimated != nil {
		t.Fatalf("empty slices/lifetime %+v", ev)
	}

	sum = Summarize("redact-only", []Event{{
		SchemaVersion: SchemaVersion, ID: "r", TS: "2026-09-22T15:00:00Z",
		Kind: KindRedactionApplied, Source: "go", TrajectoryID: "redact-only",
		Payload: []byte(`{"mode":"export","thought_collapses":4,"secret_field_hits":0}`),
	}})
	if sum.Economy.ProjectionHits != 0 || sum.Economy.RawEstimatedTokens != nil || sum.Economy.RedactionCollapses != 4 {
		t.Fatalf("redact only %+v", sum.Economy)
	}
	if len(sum.Economy.Steps) != 1 || len(sum.Economy.Largest) != 0 {
		t.Fatalf("redact steps %+v", sum.Economy)
	}
}

func TestEconomyUITrustsServerAggregates(t *testing.T) {
	t.Parallel()
	jsb, err := UI.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsb)
	start := strings.Index(js, "function renderEconomy")
	end := strings.Index(js, "function renderPanels")
	if start < 0 || end < start {
		t.Fatal("renderEconomy block missing")
	}
	body := js[start:end]
	for _, want := range []string{
		"s.economy",
		"economy.steps",
		"No projection events yet",
		"not a provider invoice",
		"text/csv",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("economy panel missing %q", want)
		}
	}
	for _, banned := range []string{"billed tokens", "invoice tokens", "Math.ceil", "/ 4", "NaN"} {
		if strings.Contains(body, banned) {
			t.Fatalf("economy panel must not invent estimates or billing claims: %s", banned)
		}
	}
}

func loadEconomyFixture(t *testing.T) []Event {
	t.Helper()
	f, err := os.Open("testdata/economy.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var events []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ev, err := ParseEvent([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}
