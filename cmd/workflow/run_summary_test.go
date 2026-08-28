package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestRunSummaryShowsWaitingRoundsAndErrorKind(t *testing.T) {
	exec := &execution.WorkflowExecution{
		ID: "execution-000001", Workflow: "review-loop", Status: execution.StatusRunning,
		Nodes: map[string]*execution.NodeExecution{
			"review": {
				NodeID: "review", NodeDefinition: "human-approval",
				History: []execution.NodeRun{{Round: 1, Status: execution.StatusFailed}},
				Current: execution.NodeRun{
					Round: 2, Status: execution.StatusWaitingHuman,
					Error: "invalid response", ErrorKind: node.ErrorKindInteraction,
				},
			},
		},
	}
	var output bytes.Buffer
	printExecutionSummaryTo(&output, exec)
	for _, want := range []string{"WaitingHuman", "rounds: 2", "error_kind: interaction"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("summary %q missing %q", output.String(), want)
		}
	}
}
