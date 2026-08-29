package history

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestListRunsReturnsNewestTwentyWithDistinctNodeProgress(t *testing.T) {
	store, _ := openTest(t)
	base := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	for i := range 21 {
		exec := &execution.WorkflowExecution{
			RunID:    fmt.Sprintf("00000000-0000-4000-8000-%012d", i),
			Workflow: "history-demo", WorkflowVersion: "v1",
			Status: execution.StatusRunning, StartedAt: base.Add(time.Duration(i) * time.Minute),
			Nodes: map[string]*execution.NodeExecution{
				"done": {
					NodeID: "done", NodeDefinition: "coding-agent", NodeExecutor: "v1",
					History: []execution.NodeRun{{Round: 1, Status: execution.StatusSucceeded}},
					Current: execution.NodeRun{Round: 2, Status: execution.StatusSucceeded},
				},
				"waiting": {
					NodeID: "waiting", NodeDefinition: "human-approval", NodeExecutor: "v1",
					Current: execution.NodeRun{Status: execution.StatusPending},
				},
			},
		}
		if err := store.Record(context.Background(), exec); err != nil {
			t.Fatalf("Record(%d): %v", i, err)
		}
	}

	runs, err := store.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns(): %v", err)
	}
	if len(runs) != 20 {
		t.Fatalf("len(ListRuns()) = %d, want 20", len(runs))
	}
	if runs[0].ID != "00000000-0000-4000-8000-000000000020" || runs[19].ID != "00000000-0000-4000-8000-000000000001" {
		t.Errorf("run order = first %q last %q", runs[0].ID, runs[19].ID)
	}
	if runs[0].NodesCompleted != 1 || runs[0].NodesTotal != 2 {
		t.Errorf("node progress = %d/%d, want 1/2", runs[0].NodesCompleted, runs[0].NodesTotal)
	}
}

func TestListRunsCountsOnlyTheLatestRoundOfEachNode(t *testing.T) {
	store, _ := openTest(t)
	exec := &execution.WorkflowExecution{
		RunID: "99999999-1111-4111-8111-111111111111", Workflow: "queued-retry",
		Status: execution.StatusRunning, StartedAt: fixedTime,
		Nodes: map[string]*execution.NodeExecution{
			"worker": {
				NodeID: "worker", NodeDefinition: "coding-agent", NodeExecutor: "v1",
				History: []execution.NodeRun{{Round: 1, Status: execution.StatusSucceeded}},
				Current: execution.NodeRun{Round: 2, Status: execution.StatusPending},
			},
		},
	}
	if err := store.Record(context.Background(), exec); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].NodesCompleted != 0 || runs[0].NodesTotal != 1 {
		t.Errorf("latest-round progress = %d/%d, want 0/1", runs[0].NodesCompleted, runs[0].NodesTotal)
	}
}

func TestGetNodeRunReturnsAllRoundsAndArtifactReferences(t *testing.T) {
	store, _ := openTest(t)
	started := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	advise := artifact.ArtifactRef{ID: "advise", Kind: "markdown", Version: "2", URI: "artifacts/advise/2.json"}
	result := artifact.ArtifactRef{ID: "result", Kind: "markdown", Version: "3", URI: "artifacts/result/3.json"}
	exec := &execution.WorkflowExecution{
		RunID: "87654321-1111-4111-8111-111111111111", Workflow: "retry-demo",
		Status: execution.StatusRunning, StartedAt: started,
		Nodes: map[string]*execution.NodeExecution{
			"worker": {
				NodeID: "worker", NodeDefinition: "coding-agent", NodeExecutor: "v1",
				History: []execution.NodeRun{{
					Round: 1, Status: execution.StatusFailed, Error: "bad response", ErrorKind: node.ErrorKindInteraction,
					StartedAt: started, FinishedAt: started.Add(time.Second),
				}},
				Current: execution.NodeRun{
					Round: 2, Status: execution.StatusSucceeded,
					Inputs:    map[string]execution.InputSnapshot{"advise": {From: "#advise-retry", Ref: advise}},
					Outputs:   map[string]artifact.ArtifactRef{"result": result},
					StartedAt: started.Add(2 * time.Second), FinishedAt: started.Add(3 * time.Second),
				},
			},
		},
	}
	if err := store.Record(context.Background(), exec); err != nil {
		t.Fatal(err)
	}

	detail, err := store.GetNodeRun(context.Background(), "87654321", "worker")
	if err != nil {
		t.Fatalf("GetNodeRun(): %v", err)
	}
	if detail == nil || detail.NodeDefinition != "coding-agent" || len(detail.Rounds) != 2 {
		t.Fatalf("GetNodeRun() = %+v", detail)
	}
	if detail.Rounds[0].Round != 1 || detail.Rounds[0].ErrorKind != "interaction" || detail.Rounds[1].Round != 2 {
		t.Errorf("rounds = %+v", detail.Rounds)
	}
	if detail.Rounds[0].NodeRunID == "" || detail.Rounds[0].NodeRunID == detail.RunID {
		t.Errorf("node run id = %q, workflow run id = %q", detail.Rounds[0].NodeRunID, detail.RunID)
	}
	if got := detail.Rounds[1].Inputs["advise"]; got.From != "#advise-retry" || got.Ref != advise {
		t.Errorf("advise input = %+v", got)
	}
	if got := detail.Rounds[1].Outputs["result"]; got != result {
		t.Errorf("result output = %+v", got)
	}

	missing, err := store.GetNodeRun(context.Background(), "87654321", "missing")
	if err != nil || missing != nil {
		t.Errorf("GetNodeRun(missing) = %+v, %v; want nil, nil", missing, err)
	}
}

func TestGetRunResolvesUUIDPrefixAndReturnsLatestNodeSummaries(t *testing.T) {
	store, _ := openTest(t)
	started := time.Date(2026, 8, 29, 9, 30, 0, 0, time.UTC)
	exec := &execution.WorkflowExecution{
		RunID:    "12345678-1111-4111-8111-111111111111",
		Workflow: "approval-loop", WorkflowVersion: "v2", WorkflowFile: "workflow.yaml",
		Status: execution.StatusStopped, StoppedReason: "user_interrupt", StartedAt: started, FinishedAt: started.Add(3 * time.Second),
		Nodes: map[string]*execution.NodeExecution{
			"review": {
				NodeID: "review", NodeDefinition: "human-approval", NodeExecutor: "v1",
				History: []execution.NodeRun{{Round: 1, Status: execution.StatusFailed}},
				Current: execution.NodeRun{
					Round: 2, Status: execution.StatusSucceeded,
					Inputs: map[string]execution.InputSnapshot{"request": {}},
				},
			},
		},
	}
	if err := store.Record(context.Background(), exec); err != nil {
		t.Fatal(err)
	}

	run, err := store.GetRun(context.Background(), "12345678")
	if err != nil {
		t.Fatalf("GetRun(): %v", err)
	}
	if run == nil || run.ID != exec.RunID || run.StoppedReason != "user_interrupt" {
		t.Fatalf("GetRun() = %+v", run)
	}
	if len(run.Nodes) != 1 {
		t.Fatalf("len(run.Nodes) = %d, want 1", len(run.Nodes))
	}
	node := run.Nodes[0]
	if node.NodeID != "review" || node.Status != "Succeeded" || node.Rounds != 2 || node.Inputs != 1 || node.Outputs != 0 {
		t.Errorf("node summary = %+v", node)
	}
}

func TestGetRunPrefixResolutionEmptyAndErrors(t *testing.T) {
	store, _ := openTest(t)
	for i, id := range []string{
		"abcdef12-1111-4111-8111-111111111111",
		"abcdef12-2222-4222-8222-222222222222",
	} {
		exec := &execution.WorkflowExecution{
			RunID: id, Workflow: "demo",
			Status: execution.StatusRunning, StartedAt: fixedTime.Add(time.Duration(i) * time.Second),
			Nodes: map[string]*execution.NodeExecution{},
		}
		if err := store.Record(context.Background(), exec); err != nil {
			t.Fatal(err)
		}
	}

	if run, err := store.GetRun(context.Background(), "deadbeef"); err != nil || run != nil {
		t.Errorf("GetRun(no match) = %+v, %v; want nil, nil", run, err)
	}
	if _, err := store.GetRun(context.Background(), "abcdef12"); err == nil ||
		!strings.Contains(err.Error(), "ambiguous") ||
		!strings.Contains(err.Error(), "abcdef12-1111") ||
		!strings.Contains(err.Error(), "abcdef12-2222") {
		t.Errorf("ambiguous prefix error = %v", err)
	}
	if _, err := store.GetRun(context.Background(), "abcdef1"); err == nil || !strings.Contains(err.Error(), "at least 8") {
		t.Errorf("short prefix error = %v", err)
	}
}
