package nodelog_test

import "testing"

func TestGetNodePayloadReadsOneSlot(t *testing.T) {
	nl := openTemp(t)
	step := 1
	for seq, kind := range map[int]string{2: "TOOL_CALL", 3: "TOOL_RESULT"} {
		if _, err := nl.Append(kind, &step, map[string]any{"seq": seq}, "t1", "demo", seq); err != nil {
			t.Fatal(err)
		}
	}

	got, found, err := nl.GetNodePayload("t1", "demo", &step, 3, "TOOL_RESULT")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got["seq"].(interface{ String() string }).String() != "3" {
		t.Fatalf("payload=%#v", got)
	}

	for _, miss := range []struct {
		tenant string
		seq    int
		kind   string
	}{
		{"demo", 3, "TOOL_CALL"},
		{"demo", 5, "TOOL_RESULT"},
		{"other", 3, "TOOL_RESULT"},
	} {
		_, found, err := nl.GetNodePayload("t1", miss.tenant, &step, miss.seq, miss.kind)
		if err != nil || found {
			t.Fatalf("%+v: found=%v err=%v, want miss", miss, found, err)
		}
	}

	if _, _, err := nl.GetNodePayload("t1", "", &step, 3, "TOOL_RESULT"); err == nil {
		t.Fatal("empty tenant must be rejected")
	}
}

func TestGetNodePayloadReadsTrajectoryLevelSlot(t *testing.T) {
	nl := openTemp(t)
	if _, err := nl.Append("STATE_SET", nil, map[string]any{"k": "v"}, "t1", "demo", 0); err != nil {
		t.Fatal(err)
	}
	got, found, err := nl.GetNodePayload("t1", "demo", nil, 0, "STATE_SET")
	if err != nil || !found || got["k"] != "v" {
		t.Fatalf("got=%#v found=%v err=%v", got, found, err)
	}
	step := 1
	if _, found, err := nl.GetNodePayload("t1", "demo", &step, 0, "STATE_SET"); err != nil || found {
		t.Fatalf("step 1: found=%v err=%v, want miss", found, err)
	}
}
