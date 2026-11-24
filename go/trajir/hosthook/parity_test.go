package hosthook_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

var update = flag.Bool("update", false, "rewrite testdata/parity golden files")

// step is one op in a shared sequence. conformance/hosthook_parity_test.py
// runs the same files through integrations/hosthook/session.py and checks
// the same golden node ids, so both sessions must write identical nodes.
type step struct {
	Op     string         `json:"op"`
	ID     string         `json:"id"`
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Owner  string         `json:"owner"`
	Result any            `json:"result"`
	Error  any            `json:"error"`
}

type goldenRow struct {
	Kind string `json:"kind"`
	Step int    `json:"step_n"`
	Seq  int    `json:"seq"`
	ID   string `json:"id"`
}

func TestParityGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "sequences", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no sequences: %v", err)
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var steps []step
			if err := json.Unmarshal(raw, &steps); err != nil {
				t.Fatal(err)
			}
			s := hosthook.Session{WorkDir: t.TempDir(), TenantID: "local", TrajectoryID: "parity"}
			for _, st := range steps {
				s.Owner = st.Owner
				switch st.Op {
				case "seal":
					_, err = s.SealBeforeTool(st.Tool, st.Args, st.ID, nil)
				case "result":
					_, err = s.RecordToolResult(st.ID, st.Result)
				case "failure":
					_, err = s.RecordToolFailure(st.ID, st.Error)
				default:
					t.Fatalf("unknown op %q", st.Op)
				}
				if err != nil {
					t.Fatalf("%s %s: %v", st.Op, st.ID, err)
				}
			}
			nl, err := nodelog.Open(filepath.Join(s.WorkDir, "nodes.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer nl.Close()
			list, err := nl.ListNodes("parity", "local")
			if err != nil {
				t.Fatal(err)
			}
			got := make([]goldenRow, 0, len(list))
			for _, n := range list {
				got = append(got, goldenRow{Kind: n["kind"].(string), Step: n["step_n"].(int), Seq: n["seq"].(int), ID: n["id"].(string)})
			}
			golden := filepath.Join("testdata", "parity", name+".golden.json")
			if *update {
				out, _ := json.MarshalIndent(got, "", "  ")
				if err := os.WriteFile(golden, append(out, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			raw, err = os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to create)", err)
			}
			var want []goldenRow
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("nodes differ from %s\ngot  %v\nwant %v", golden, got, want)
			}
		})
	}
}
