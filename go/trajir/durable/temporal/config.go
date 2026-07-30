package temporal

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"go.temporal.io/sdk/client"
)

// Config holds Temporal client and worker settings.
// Env defaults match a local Temporal dev server (plaintext).
// For production, set TEMPORAL_TLS=true and optionally TEMPORAL_API_KEY.
type Config struct {
	HostPort  string // e.g. localhost:7233
	Namespace string // e.g. default
	TaskQueue string // e.g. trajectory-ir
	// TLS enables TLS on the Temporal frontend connection.
	TLS bool
	// APIKey is an optional Temporal Cloud API key (requires TLS).
	APIKey string
}

// ConfigFromEnv reads TEMPORAL_HOSTPORT, TEMPORAL_NAMESPACE, TEMPORAL_TASK_QUEUE,
// TEMPORAL_TLS, and TEMPORAL_API_KEY.
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
	if v := strings.TrimSpace(os.Getenv("TEMPORAL_TLS")); v != "" {
		c.TLS = strings.EqualFold(v, "1") || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	if v := strings.TrimSpace(os.Getenv("TEMPORAL_API_KEY")); v != "" {
		c.APIKey = v
		// API keys imply TLS for Temporal Cloud.
		c.TLS = true
	}
	return c
}

// Validate checks required fields and TLS/API key consistency.
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
	if c.APIKey != "" && !c.TLS {
		return fmt.Errorf("temporal: APIKey requires TLS")
	}
	return nil
}

// ClientOptions builds SDK client options from this config.
// Local defaults remain plaintext; production should enable TLS.
func (c Config) ClientOptions() client.Options {
	opts := client.Options{
		HostPort:  c.HostPort,
		Namespace: c.Namespace,
	}
	if c.TLS {
		opts.ConnectionOptions = client.ConnectionOptions{
			TLS: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
	}
	if c.APIKey != "" {
		opts.Credentials = client.NewAPIKeyStaticCredentials(c.APIKey)
	}
	return opts
}

// MemoWorkflowID builds a stable Temporal workflow id for one durable step memo.
func MemoWorkflowID(workflowID, stepKey string) string {
	// Temporal workflow ids must be unique; slash is fine for readability.
	return "trajir-memo/" + workflowID + "/" + stepKey
}
