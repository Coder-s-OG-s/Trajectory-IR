package temporal

import (
	"go.temporal.io/sdk/workflow"
)

// MemoWorkflowName is the registered workflow type for step memo storage.
const MemoWorkflowName = "TrajectoryIRMemoWorkflow"

// MemoWorkflow stores a step result as the workflow return value.
// Workflow id is MemoWorkflowID(workflowID, stepKey). Reusing the same id
// after completion is rejected so the first Save wins (Load returns that result).
func MemoWorkflow(ctx workflow.Context, value []byte) ([]byte, error) {
	return value, nil
}
