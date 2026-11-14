package nodelog_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/logerr"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/postgres"
)

// The SQLite NodeLog must not link the Postgres driver (#536).
func TestLogDoesNotImportPostgres(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not on PATH")
	}
	out, err := exec.Command(goBin, "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.Contains(dep, "jackc/pgx") || strings.HasSuffix(dep, "/trajir/postgres") {
			t.Fatalf("trajir/log depends on %s", dep)
		}
	}
}

func TestSlotErrorsAreShared(t *testing.T) {
	if !errors.Is(nodelog.ErrSlotConflict, postgres.ErrSlotConflict) || !errors.Is(nodelog.ErrSlotConflict, logerr.ErrSlotConflict) {
		t.Fatal("ErrSlotConflict differs between log, postgres and logerr")
	}
	if !errors.Is(nodelog.ErrNotStored, postgres.ErrNotStored) || !errors.Is(nodelog.ErrNotStored, logerr.ErrNotStored) {
		t.Fatal("ErrNotStored differs between log, postgres and logerr")
	}
}
