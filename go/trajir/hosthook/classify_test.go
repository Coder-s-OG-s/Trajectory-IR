package hosthook_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
)

func classifier(t *testing.T, host, proj string) func(string, map[string]any) effects.EffectClass {
	t.Helper()
	c, err := hosthook.NewClassifier(host, proj)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeEffects(t *testing.T, proj, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(proj, ".trajir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".trajir", "effects.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeReadIsReadOnly(t *testing.T) {
	c := classifier(t, "claude", t.TempDir())
	for _, name := range []string{"Read", "Grep", "Glob", "WebFetch"} {
		if got := c(name, nil); got != effects.READ_ONLY {
			t.Fatalf("%s = %s", name, got)
		}
	}
}

func TestGeminiReadIsReadOnly(t *testing.T) {
	if got := classifier(t, "gemini", t.TempDir())("read_file", nil); got != effects.READ_ONLY {
		t.Fatalf("read_file = %s", got)
	}
}

func TestBashAndWritesStayNonIdempotent(t *testing.T) {
	c := classifier(t, "claude", t.TempDir())
	for _, name := range []string{"Bash", "Write", "Edit"} {
		if got := c(name, nil); got != effects.NON_IDEMPOTENT_WRITE {
			t.Fatalf("%s = %s", name, got)
		}
	}
}

func TestUnknownToolFailsClosed(t *testing.T) {
	if got := classifier(t, "cursor", t.TempDir())("mystery", nil); got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("mystery = %s", got)
	}
}

func TestMCPAnnotationsUsedWhenNoMapEntry(t *testing.T) {
	c := classifier(t, "claude", t.TempDir())
	if got := c("mcp__x__get", map[string]any{"readOnlyHint": true}); got != effects.READ_ONLY {
		t.Fatalf("annotated = %s", got)
	}
}

func TestProjectOverrideWins(t *testing.T) {
	proj := t.TempDir()
	writeEffects(t, proj, `{"tools": {"Read": "NON_IDEMPOTENT_WRITE", "deploy_preview": "IDEMPOTENT_WRITE"}}`)
	c := classifier(t, "claude", proj)
	if got := c("Read", nil); got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("Read = %s", got)
	}
	if got := c("deploy_preview", nil); got != effects.IDEMPOTENT_WRITE {
		t.Fatalf("deploy_preview = %s", got)
	}
}

func TestBadClassInOverrideErrors(t *testing.T) {
	proj := t.TempDir()
	writeEffects(t, proj, `{"tools": {"x": "SAFE"}}`)
	_, err := hosthook.NewClassifier("claude", proj)
	if err == nil || !strings.Contains(err.Error(), "effects.json") {
		t.Fatalf("want error naming effects.json, got %v", err)
	}
}

func TestOverrideCannotSoftenOpenWorldTool(t *testing.T) {
	proj := t.TempDir()
	writeEffects(t, proj, `{"tools": {"Bash": "READ_ONLY"}}`)
	_, err := hosthook.NewClassifier("claude", proj)
	if err == nil || !strings.Contains(err.Error(), "Bash") {
		t.Fatalf("want error naming Bash, got %v", err)
	}
}

func TestUnknownHostHasNoDefaults(t *testing.T) {
	if got := classifier(t, "other", t.TempDir())("Read", nil); got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("Read on unknown host = %s", got)
	}
}
