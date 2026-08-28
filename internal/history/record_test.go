package history

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/google/uuid"
)

func TestRecordUpsertsWorkflowAndOneRowPerNodeRound(t *testing.T) {
	store, _ := openTest(t)
	started := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	exec := &execution.WorkflowExecution{
		ID: "execution-000007", Workflow: "approval-loop", WorkflowFile: "workflow.yaml",
		Status: execution.StatusRunning, StartedAt: started,
		Nodes: map[string]*execution.NodeExecution{
			"review": {
				NodeID: "review", NodeDefinition: "human-approval", NodeExecutor: "v1",
				Current: execution.NodeRun{Status: execution.StatusPending},
			},
		},
	}

	if err := store.Record(exec); err != nil {
		t.Fatalf("Record(initial): %v", err)
	}
	if _, err := uuid.Parse(exec.RunID); err != nil {
		t.Fatalf("RunID = %q, want UUID: %v", exec.RunID, err)
	}
	firstRunID := exec.RunID
	var firstNodeRunID string
	if err := store.db.QueryRow(`SELECT id FROM workflow_node_run_history WHERE run_id = ? AND node_id = ? AND round = 0`, exec.RunID, "review").Scan(&firstNodeRunID); err != nil {
		t.Fatalf("query initial node row: %v", err)
	}

	ref := artifact.ArtifactRef{ID: "advise", Kind: "markdown", Version: "2", URI: "2.json"}
	exec.Nodes["review"].Current = execution.NodeRun{
		RunID: firstNodeRunID, Round: 1, Status: execution.StatusFailed,
		Inputs: map[string]execution.InputSnapshot{"advise": {From: "#advise-retry", Ref: ref}},
		Error:  "invalid response", ErrorKind: node.ErrorKindInteraction,
		StartedAt: started.Add(time.Second), FinishedAt: started.Add(2 * time.Second),
	}
	exec.Status = execution.StatusFailed
	exec.Error = "node failed"
	exec.FinishedAt = started.Add(3 * time.Second)
	if err := store.Record(exec); err != nil {
		t.Fatalf("Record(update): %v", err)
	}
	if exec.RunID != firstRunID {
		t.Fatalf("RunID changed from %q to %q", firstRunID, exec.RunID)
	}

	var (
		nodeRunID, status, errorKind, inputsJSON string
		count                                    int
	)
	if err := store.db.QueryRow(`SELECT id, status, error_kind, inputs_json FROM workflow_node_run_history WHERE run_id = ? AND node_id = ? AND round = 1`, exec.RunID, "review").Scan(&nodeRunID, &status, &errorKind, &inputsJSON); err != nil {
		t.Fatalf("query updated node row: %v", err)
	}
	if nodeRunID != firstNodeRunID {
		t.Errorf("node run id changed from %q to %q", firstNodeRunID, nodeRunID)
	}
	if status != "Failed" || errorKind != "interaction" {
		t.Errorf("node row status/error_kind = %q/%q, want Failed/interaction", status, errorKind)
	}
	var inputs map[string]execution.InputSnapshot
	if err := json.Unmarshal([]byte(inputsJSON), &inputs); err != nil {
		t.Fatalf("decode inputs_json: %v", err)
	}
	if got := inputs["advise"]; got.From != "#advise-retry" || got.Ref != ref {
		t.Errorf("inputs_json advise = %+v, want marker and ref %+v", got, ref)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM workflow_node_run_history WHERE run_id = ?`, exec.RunID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("node run rows = %d, want 1 after initial Pending upsert", count)
	}

	firstRound := exec.Nodes["review"].Current
	exec.Nodes["review"].History = []execution.NodeRun{firstRound}
	exec.Nodes["review"].Current = execution.NodeRun{
		RunID: uuid.NewString(), Round: 2, Status: execution.StatusSucceeded,
		Outputs:   map[string]artifact.ArtifactRef{"approve": {ID: "approve", Kind: "bool", Version: "2", URI: "3.json"}},
		StartedAt: started.Add(4 * time.Second), FinishedAt: started.Add(5 * time.Second),
	}
	exec.Status = execution.StatusStopped
	exec.StoppedReason = "user_interrupt"
	exec.Error = ""
	exec.FinishedAt = started.Add(6 * time.Second)
	if err := store.Record(exec); err != nil {
		t.Fatalf("Record(second round): %v", err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM workflow_node_run_history WHERE run_id = ?`, exec.RunID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("node run rows = %d, want 2 rounds", count)
	}
	rows, err := store.db.Query(`SELECT round FROM workflow_node_run_history WHERE run_id = ? ORDER BY round`, exec.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var rounds []int
	for rows.Next() {
		var round int
		if err := rows.Scan(&round); err != nil {
			t.Fatal(err)
		}
		rounds = append(rounds, round)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 2 || rounds[0] != 1 || rounds[1] != 2 {
		t.Errorf("stored rounds = %v, want [1 2]", rounds)
	}
	var workflowStatus, stoppedReason string
	if err := store.db.QueryRow(`SELECT status, stopped_reason FROM workflow_run_history WHERE id = ?`, exec.RunID).Scan(&workflowStatus, &stoppedReason); err != nil {
		t.Fatal(err)
	}
	if workflowStatus != "Stopped" || stoppedReason != "user_interrupt" {
		t.Errorf("workflow status/reason = %q/%q, want Stopped/user_interrupt", workflowStatus, stoppedReason)
	}

	if _, err := store.db.Exec(`DELETE FROM workflow_run_history WHERE id = ?`, exec.RunID); err != nil {
		t.Fatalf("delete workflow run: %v", err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM workflow_node_run_history WHERE run_id = ?`, exec.RunID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("node rows after workflow run delete = %d, want 0", count)
	}
}
