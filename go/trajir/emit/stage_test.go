package emit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

func TestStagePackageCopiesTir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "run.tir")
	if err := os.WriteFile(src, tirBytes("PK-demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := emit.StagePackage(dir, "t1", src, "evt1")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "packages/t1/run.tir" {
		t.Fatalf("rel=%s", rel)
	}
	got, err := os.ReadFile(filepath.Join(dir, "packages", "t1", "run.tir"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(tirBytes("PK-demo")) {
		t.Fatalf("copied %q", got)
	}
	rel2, err := emit.StagePackage(dir, "t1", src, "evt1")
	if err != nil || rel2 != rel {
		t.Fatalf("idempotent copy rel=%s err=%v", rel2, err)
	}
}

func TestStagePackageRejectsBadIDAndMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := emit.StagePackage(dir, "../x", "no.tir", ""); err == nil {
		t.Fatal("expected bad id")
	}
	if _, err := emit.StagePackage(dir, "t1", filepath.Join(dir, "missing.tir"), ""); err == nil {
		t.Fatal("expected missing")
	}
	if _, err := emit.StagePackage("", "t1", filepath.Join(dir, "x.tir"), ""); err == nil {
		t.Fatal("expected empty data dir")
	}
	if _, err := emit.StagePackage(dir, "", filepath.Join(dir, "x.tir"), ""); err == nil {
		t.Fatal("expected empty trajectory id")
	}
	if _, err := emit.StagePackage(dir, "t1", "", "evt"); err == nil {
		t.Fatal("expected empty src")
	}
}

func TestStagePackageRejectsTraversalAndDir(t *testing.T) {
	dir := t.TempDir()
	traversal := dir + string(filepath.Separator) + ".." + string(filepath.Separator) + "secret.tir"
	if _, err := emit.StagePackage(dir, "t1", traversal, "e"); err == nil {
		t.Fatal("expected .. reject")
	}
	if _, err := emit.StagePackage(dir, "t1", dir, "e"); err == nil {
		t.Fatal("expected directory reject")
	}
	if _, err := emit.StagePackage(dir, "t1", "foo\x00bar.tir", "e"); err == nil {
		t.Fatal("expected NUL reject")
	}
}

func TestStagePackageRejectsBadName(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bad name.tir")
	if err := os.WriteFile(src, tirBytes("pkg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := emit.StagePackage(dir, "t1", src, "evt99"); err == nil {
		t.Fatal("expected bad name reject")
	}
}

func TestStagePackageRejectsNonTir(t *testing.T) {
	dir := t.TempDir()
	// A file that is not named .tir must never be staged, even with an event id.
	secret := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(secret, tirBytes("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := emit.StagePackage(dir, "t1", secret, "evt1"); err == nil {
		t.Fatal("expected non-.tir name reject")
	}
	// A .tir name without the zip header is not a package either.
	fake := filepath.Join(dir, "fake.tir")
	if err := os.WriteFile(fake, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := emit.StagePackage(dir, "t1", fake, "evt1"); err == nil {
		t.Fatal("expected magic reject")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "packages", "t1")); len(entries) != 0 {
		t.Fatalf("staged %d files", len(entries))
	}
}

// tirBytes prefixes body with the zip header so StagePackage accepts it.
func tirBytes(body string) []byte {
	return append([]byte{0x50, 0x4b, 0x03, 0x04}, body...)
}

func TestStagePackageDuplicateDifferentContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "run.tir")
	if err := os.WriteFile(src, tirBytes("first!!"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := emit.StagePackage(dir, "t1", src, "evt1")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "packages/t1/run.tir" {
		t.Fatalf("rel=%s", rel)
	}
	if err := os.WriteFile(src, tirBytes("second!"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel2, err := emit.StagePackage(dir, "t1", src, "evt2-extra")
	if err != nil {
		t.Fatal(err)
	}
	if rel2 != "packages/t1/run-evt2extra.tir" {
		t.Fatalf("rel2=%s", rel2)
	}
	got, err := os.ReadFile(filepath.Join(dir, "packages", "t1", "run-evt2extra.tir"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(tirBytes("second!")) {
		t.Fatalf("copied %q", got)
	}
}

func TestStagePackageDuplicateNoEventID(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "run.tir")
	if err := os.WriteFile(src, tirBytes("aaaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := emit.StagePackage(dir, "t1", src, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, tirBytes("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := emit.StagePackage(dir, "t1", src, "")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "packages/t1/run-dup.tir" {
		t.Fatalf("rel=%s", rel)
	}
}

func TestSafeTirName(t *testing.T) {
	if _, err := emit.SafeTirName("ok.tir"); err != nil {
		t.Fatal(err)
	}
	if got, err := emit.SafeTirName("ok.TIR"); err != nil || got != "ok.TIR" {
		t.Fatalf("case: %s %v", got, err)
	}
	if _, err := emit.SafeTirName("../x.tir"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := emit.SafeTirName("nope.zip"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := emit.SafeTirName(""); err == nil {
		t.Fatal("expected empty")
	}
	if _, err := emit.SafeTirName("a/b.tir"); err == nil {
		t.Fatal("expected slash")
	}
	if _, err := emit.SafeTirName("bad name.tir"); err == nil {
		t.Fatal("expected space")
	}
	long := strings.Repeat("a", 177) + ".tir"
	if _, err := emit.SafeTirName(long); err == nil {
		t.Fatal("expected too long")
	}
	okLen := strings.Repeat("a", 176) + ".tir"
	if _, err := emit.SafeTirName(okLen); err != nil {
		t.Fatal(err)
	}
}

func TestRelConsolePath(t *testing.T) {
	if got := emit.RelConsolePath("t1", "run.tir"); got != "packages/t1/run.tir" {
		t.Fatalf("got %s", got)
	}
}

func TestFileSinkStagesExport(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "out.tir")
	if err := os.WriteFile(src, tirBytes("tir-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sink := &emit.FileSink{Dir: dir}
	if err := sink.Emit(emit.Event{
		Kind:         emit.KindExportCompleted,
		TrajectoryID: "pack",
		Payload: map[string]any{
			"path": src, "mode": "thin", "redacted": true,
			"bytes": 9, "member_count": 5, "node_count": 1, "ok": true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "trajectories", "pack.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "packages/pack/out.tir") {
		t.Fatalf("missing console_path: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "packages", "pack", "out.tir")); err != nil {
		t.Fatal(err)
	}
}
