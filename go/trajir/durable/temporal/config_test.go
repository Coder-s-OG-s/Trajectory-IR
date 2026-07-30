package temporal_test

import (
	"os"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable/temporal"
)

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("TEMPORAL_HOSTPORT", "")
	t.Setenv("TEMPORAL_NAMESPACE", "")
	t.Setenv("TEMPORAL_TASK_QUEUE", "")
	// Clear may not unset if empty string; set then clear via custom.
	_ = os.Unsetenv("TEMPORAL_HOSTPORT")
	_ = os.Unsetenv("TEMPORAL_NAMESPACE")
	_ = os.Unsetenv("TEMPORAL_TASK_QUEUE")

	c := temporal.ConfigFromEnv()
	if c.HostPort != "localhost:7233" {
		t.Fatalf("HostPort=%q", c.HostPort)
	}
	if c.Namespace != "default" {
		t.Fatalf("Namespace=%q", c.Namespace)
	}
	if c.TaskQueue != "trajectory-ir" {
		t.Fatalf("TaskQueue=%q", c.TaskQueue)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("TEMPORAL_HOSTPORT", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "prod")
	t.Setenv("TEMPORAL_TASK_QUEUE", "trajir-workers")

	c := temporal.ConfigFromEnv()
	if c.HostPort != "temporal:7233" || c.Namespace != "prod" || c.TaskQueue != "trajir-workers" {
		t.Fatalf("%+v", c)
	}
}

func TestMemoWorkflowID(t *testing.T) {
	id := temporal.MemoWorkflowID("wf1", "infer:step1")
	if id != "trajir-memo/wf1/infer:step1" {
		t.Fatalf("id=%q", id)
	}
}

func TestValidateRejectsEmpty(t *testing.T) {
	err := temporal.Config{}.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
}
