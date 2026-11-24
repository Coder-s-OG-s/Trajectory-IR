package console

import (
	"encoding/json"
	"testing"
)

func runtimeEvent(kind, runtime string) Event {
	return Event{Kind: kind, Runtime: runtime, TrajectoryID: "r", TS: "2026-11-24T10:00:00Z", Payload: json.RawMessage(`{"kind":"DECISION","node_id":"n","step_n":1,"content_hash":"h"}`)}
}

func TestRunCardCarriesRuntime(t *testing.T) {
	sum := Summarize("r", []Event{
		runtimeEvent(KindNodeAppended, "claude-code"),
		runtimeEvent(KindSealCreated, "claude-code"),
	})
	if sum.Runtime != "claude-code" || runCardFrom(sum).Runtime != "claude-code" {
		t.Fatalf("summary runtime = %q", sum.Runtime)
	}
}

func TestExportAndVerifyDoNotRelabelRun(t *testing.T) {
	sum := Summarize("r", []Event{
		runtimeEvent(KindNodeAppended, "codex"),
		runtimeEvent(KindExportCompleted, "trajir-export"),
		runtimeEvent(KindAuditCompleted, "trajir-verify"),
	})
	if sum.Runtime != "codex" {
		t.Fatalf("runtime = %q", sum.Runtime)
	}
}

func TestRunCardWithoutRuntimeUnchanged(t *testing.T) {
	sum := Summarize("r", []Event{runtimeEvent(KindNodeAppended, "")})
	raw, err := json.Marshal(runCardFrom(sum))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["runtime"]; ok {
		t.Fatalf("empty runtime serialized: %s", raw)
	}
}
