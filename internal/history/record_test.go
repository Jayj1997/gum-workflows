package history

import (
	"context"
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
		RunID: uuid.NewString(), Workflow: "approval-loop", WorkflowFile: "workflow.yaml",
		Status: execution.StatusRunning, StartedAt: started,
		Nodes: map[string]*execution.NodeExecution{
			"review": {
				NodeID: "review", NodeDefinition: "human-approval", NodeExecutor: "v1",
				Current: execution.NodeRun{Status: execution.StatusPending},
			},
		},
	}

	if err := store.Record(context.Background(), exec); err != nil {
		t.Fatalf("Record(initial): %v", err)
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
	if err := store.Record(context.Background(), exec); err != nil {
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
	if err := store.Record(context.Background(), exec); err != nil {
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

func TestRecordRejectsMissingRunIdentity(t *testing.T) {
	store, _ := openTest(t)
	err := store.Record(context.Background(), &execution.WorkflowExecution{
		Workflow: "missing-identity", Status: execution.StatusRunning, StartedAt: fixedTime,
		Nodes: map[string]*execution.NodeExecution{},
	})
	if err == nil {
		t.Fatal("Record() = nil error, want missing Run ID rejection")
	}
}

func TestRecordPreservesConsumedCodeArtifactRef(t *testing.T) {
	store, _ := openTest(t)
	codeRef := artifact.ArtifactRef{
		ID: "project-code", Kind: artifact.KindSourceCode, Version: "1", URI: "/workspace/project",
	}
	exec := &execution.WorkflowExecution{
		RunID: uuid.NewString(), Workflow: "project-check", Status: execution.StatusStopped,
		StartedAt: fixedTime, FinishedAt: fixedTime.Add(time.Second),
		Nodes: map[string]*execution.NodeExecution{
			"check": {
				NodeID: "check", NodeDefinition: "code-check", NodeExecutor: "v1",
				Current: execution.NodeRun{
					RunID: uuid.NewString(), Round: 1, Status: execution.StatusSucceeded,
					Inputs: map[string]execution.InputSnapshot{
						"code": {From: "project.code", Ref: codeRef},
					},
					StartedAt: fixedTime, FinishedAt: fixedTime.Add(time.Second),
				},
			},
		},
	}

	if err := store.Record(context.Background(), exec); err != nil {
		t.Fatalf("Record(): %v", err)
	}
	detail, err := store.GetNodeRun(context.Background(), exec.RunID, "check")
	if err != nil {
		t.Fatalf("GetNodeRun(): %v", err)
	}
	if detail == nil || len(detail.Rounds) != 1 {
		t.Fatalf("GetNodeRun() = %+v, want one round", detail)
	}
	if got := detail.Rounds[0].Inputs["code"]; got.From != "project.code" || got.Ref != codeRef {
		t.Errorf("stored code input = %+v, want project.code and %+v", got, codeRef)
	}
}

func TestRecordPreservesStaticResultAndExecutionDiagnostics(t *testing.T) {
	store, _ := openTest(t)
	artifactStore := artifact.NewMemStore()
	resultRef, err := artifactStore.Put(artifact.Artifact{
		ID: "go-static-analysis-result", Kind: artifact.KindQualityCheckResult, Version: "1",
		Data: map[string]any{
			"effectiveConfig": map[string]any{"packageScope": "./..."},
			"toolchain":       map[string]any{"finalVersion": "go1.25.0"},
			"logs":            map[string]any{"stdout": "/data/stdout.log", "stderr": "/data/stderr.log"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := node.RunDiagnostics{
		BundleDigest: "sha256:abc", CWD: "/workspace/project",
		Arguments: []string{"/workspace/project", "/data/tool-output"},
		Launcher:  "/bin/sh", ResultAdapter: "go-static-analysis/v1",
		Executables: map[string]string{"go": "/usr/local/bin/go"},
		Toolchain:   map[string]string{"finalVersion": "go1.25.0", "goos": "darwin", "goarch": "arm64"},
		Logs: map[string]artifact.ArtifactRef{
			"stdout": {ID: "stdout", Kind: artifact.KindLog, URI: "/data/stdout.log"},
		},
	}
	exec := &execution.WorkflowExecution{
		RunID: uuid.NewString(), Workflow: "static-history", Status: execution.StatusStopped,
		StartedAt: fixedTime, FinishedAt: fixedTime.Add(time.Second),
		Nodes: map[string]*execution.NodeExecution{
			"check": {
				NodeID: "check", NodeDefinition: "go-static-analysis", NodeExecutor: "v1",
				Current: execution.NodeRun{
					RunID: uuid.NewString(), Round: 1, Status: execution.StatusSucceeded,
					Outputs: map[string]artifact.ArtifactRef{"result": resultRef}, Diagnostics: diagnostics,
					StartedAt: fixedTime, FinishedAt: fixedTime.Add(time.Second),
				},
			},
		},
	}
	if err := store.Record(context.Background(), exec); err != nil {
		t.Fatalf("Record(): %v", err)
	}
	detail, err := store.GetNodeRun(context.Background(), exec.RunID, "check")
	if err != nil {
		t.Fatalf("GetNodeRun(): %v", err)
	}
	round := detail.Rounds[0]
	if round.Outputs["result"] != resultRef {
		t.Errorf("result ref = %+v", round.Outputs["result"])
	}
	body, err := artifactStore.Get(round.Outputs["result"])
	if err != nil {
		t.Fatalf("Get(result from history ref): %v", err)
	}
	result := body.Data.(map[string]any)
	if result["effectiveConfig"] == nil || result["toolchain"] == nil || result["logs"] == nil {
		t.Errorf("result evidence = %+v", result)
	}
	if round.Diagnostics.BundleDigest != diagnostics.BundleDigest || round.Diagnostics.ResultAdapter != diagnostics.ResultAdapter || round.Diagnostics.Toolchain["finalVersion"] != "go1.25.0" {
		t.Errorf("diagnostics = %+v, want %+v", round.Diagnostics, diagnostics)
	}
}
