package console

import (
	"bufio"
	"bytes"
	"os"
	"testing"
)

func TestConsoleDemoFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/console_demo.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		e, err := ParseEvent(line)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	sum := Summarize("console-demo", events)
	if sum.SealCreatedCount != 1 || sum.SealVerifiedOK != 1 || sum.SealVerifiedFail != 0 {
		t.Fatalf("seals %+v", sum)
	}
	if sum.TokensAvoidedEstimated == nil || *sum.TokensAvoidedEstimated != 75 {
		t.Fatalf("tokens avoided %+v", sum.TokensAvoidedEstimated)
	}
	if sum.RawEstimatedTokens == nil || *sum.RawEstimatedTokens != 100 {
		t.Fatalf("raw tokens %+v", sum.RawEstimatedTokens)
	}
	if sum.ExportsOK != 1 || sum.ImportsOK != 1 || sum.LastPackageMode != "thin" {
		t.Fatalf("transfer %+v", sum)
	}
	if sum.LastPackageRedacted == nil || !*sum.LastPackageRedacted {
		t.Fatalf("redacted %+v", sum.LastPackageRedacted)
	}
	if sum.RedactionCollapses != 3 {
		t.Fatalf("redaction %d", sum.RedactionCollapses)
	}
}
