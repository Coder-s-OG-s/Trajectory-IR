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
	if !strings.Contains(csvText, "raw_char_len") || !strings.Contains(csvText, "projected_char_len") {
		t.Fatalf("csv header drifted:\n%s", csvText)
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

func TestProjectionExposesCharLengthsWithoutInventedUsage(t *testing.T) {
	t.Parallel()
	sum := Summarize("econ", loadEconomyFixture(t))
	if sum.RawCharLen == nil || *sum.RawCharLen != 400 || sum.ProjectedCharLen == nil || *sum.ProjectedCharLen != 100 {
		t.Fatalf("latest chars raw=%v projected=%v", sum.RawCharLen, sum.ProjectedCharLen)
	}
	if sum.PromptTokens != nil || sum.CompletionTokens != nil || sum.Model != "" {
		t.Fatalf("usage invented prompt=%v completion=%v model=%q", sum.PromptTokens, sum.CompletionTokens, sum.Model)
	}
	if sum.Economy.RawCharLen == nil || *sum.Economy.RawCharLen != 400 || sum.Economy.ProjectedCharLen == nil || *sum.Economy.ProjectedCharLen != 100 {
		t.Fatalf("economy chars %+v %+v", sum.Economy.RawCharLen, sum.Economy.ProjectedCharLen)
	}
	first := sum.Economy.Steps[0]
	if first.RawCharLen == nil || *first.RawCharLen != 40 || first.ProjectedCharLen == nil || *first.ProjectedCharLen != 20 {
		t.Fatalf("first step chars %+v", first)
	}
	if first.PromptTokens != nil || sum.Economy.Steps[1].RawCharLen != nil {
		t.Fatalf("step usage or redaction chars %+v %+v", first.PromptTokens, sum.Economy.Steps[1].RawCharLen)
	}
}

func TestProviderUsageSticksWhenLaterEventOmitsIt(t *testing.T) {
	t.Parallel()
	base := []Event{
		{
			SchemaVersion: SchemaVersion, ID: "u1", TS: "2026-09-22T15:00:00Z",
			Kind: KindContextProjected, Source: "go", TrajectoryID: "usage",
			Payload: []byte(`{"raw_char_len":40,"projected_char_len":20,"prompt_tokens":1200,"completion_tokens":30,"model":"demo-model"}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "n1", TS: "2026-09-22T15:01:00Z",
			Kind: KindNodeAppended, Source: "go", TrajectoryID: "usage",
			Payload: []byte(`{"node_id":"n1","kind":"TOOL_CALL"}`),
		},
	}
	sum := Summarize("usage", base)
	if sum.PromptTokens == nil || *sum.PromptTokens != 1200 || sum.CompletionTokens == nil || *sum.CompletionTokens != 30 || sum.Model != "demo-model" {
		t.Fatalf("usage dropped prompt=%v completion=%v model=%q", sum.PromptTokens, sum.CompletionTokens, sum.Model)
	}
	if sum.Economy.PromptTokens == nil || *sum.Economy.PromptTokens != 1200 || sum.Economy.Model != "demo-model" {
		t.Fatalf("economy usage %+v %q", sum.Economy.PromptTokens, sum.Economy.Model)
	}
	later := append(append([]Event{}, base...), Event{
		SchemaVersion: SchemaVersion, ID: "u2", TS: "2026-09-22T15:02:00Z",
		Kind: KindNodeAppended, Source: "go", TrajectoryID: "usage",
		Payload: []byte(`{"model":"other-model"}`),
	})
	sum = Summarize("usage", later)
	if sum.Model != "other-model" || sum.PromptTokens == nil || *sum.PromptTokens != 1200 || sum.CompletionTokens == nil || *sum.CompletionTokens != 30 {
		t.Fatalf("later omit cleared usage prompt=%v completion=%v model=%q", sum.PromptTokens, sum.CompletionTokens, sum.Model)
	}
	nested := Summarize("nested", []Event{{
		SchemaVersion: SchemaVersion, ID: "n", TS: "2026-09-22T15:00:00Z",
		Kind: KindNodeAppended, Source: "go", TrajectoryID: "nested",
		Payload: []byte(`{"usage":{"prompt_tokens":9,"completion_tokens":9,"model":"hidden"}}`),
	}})
	if nested.PromptTokens != nil || nested.CompletionTokens != nil || nested.Model != "" {
		t.Fatalf("nested usage was guessed prompt=%v model=%q", nested.PromptTokens, nested.Model)
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
