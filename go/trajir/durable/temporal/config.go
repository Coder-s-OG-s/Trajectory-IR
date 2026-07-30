package temporal

import (
	"fmt"
	"os"
	"strings"
)

// Config holds Temporal client and worker settings.
// Env defaults match a local Temporal dev server.
type Config struct {
	HostPort  string // e.g. localhost:7233
	Namespace string // e.g. default
	TaskQueue string // e.g. trajectory-ir
}

// ConfigFromEnv reads TEMPORAL_HOSTPORT, TEMPORAL_NAMESPACE, TEMPORAL_TASK_QUEUE.
func ConfigFromEnv() Config {
	c := Config{
		HostPort:  "localhost:7233",
		Namespace: "default",
		TaskQueue: "trajectory-ir",
	}
	if v := strings.TrimSpace(os.Getenv("TEMPORAL_HOSTPORT")); v != "" {
		c.HostPort = v
	}
	if v := strings.TrimSpace(os.Getenv("TEMPORAL_NAMESPACE")); v != "" {
		c.Namespace = v
	}
	if v := strings.TrimSpace(os.Getenv("TEMPORAL_TASK_QUEUE")); v != "" {
		c.TaskQueue = v
	}
	return c
}

// Validate checks required fields.
func (c Config) Validate() error {
	if c.HostPort == "" {
		return fmt.Errorf("temporal: HostPort is required")
	}
	if c.Namespace == "" {
		return fmt.Errorf("temporal: Namespace is required")
	}
	if c.TaskQueue == "" {
		return fmt.Errorf("temporal: TaskQueue is required")
	}
	return nil
}

// MemoWorkflowID builds a stable Temporal workflow id for one durable step memo.
func MemoWorkflowID(workflowID, stepKey string) string {
	// Temporal workflow ids must be unique; slash is fine for readability.
	return "trajir-memo/" + workflowID + "/" + stepKey
}
