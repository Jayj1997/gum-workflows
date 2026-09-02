package history

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

func TestStartProductWorkflowRunRollsBackEveryVisibleWriteOnLateFailure(t *testing.T) {
	ctx := context.Background()
	store, _ := openTest(t)
	workflow, err := store.CreateProductWorkflow(ctx, "Conversation")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.GetProductWorkflowDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	materialized := json.RawMessage(`{"nodes":[{"definition":"llm-chat","id":"answer","llm":{"modelUuid":"model-id"}}],"semanticSchemaVersion":"productWorkflow/v1"}`)
	revisionContent, semanticHash, err := productworkflow.RevisionContent(materialized)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	revision := productworkflow.Revision{ID: "revision-id", WorkflowID: workflow.ID, SemanticHash: semanticHash, Content: revisionContent, CreatedAt: now}
	run := productworkflow.Run{ID: "run-id", WorkflowID: workflow.ID, RevisionID: revision.ID, Status: "succeeded", Snapshot: productworkflow.RunSnapshot{RevisionID: revision.ID}, StartedAt: now, FinishedAt: now}
	failed := productworkflow.StartRunRequest{
		WorkflowID: workflow.ID, ExpectedLockVersion: initial.LockVersion, DraftContent: materialized,
		Revision: revision, Run: run,
		Artifacts: []productworkflow.RunArtifact{{ID: "artifact-id", RunID: run.ID, NodeRunID: "missing-node-run", NodeID: "answer", Port: "conversation", Type: "Conversation", Version: "1", URI: "1.json", CreatedAt: now}},
	}
	if _, err := store.StartProductWorkflowRun(ctx, failed); err == nil {
		t.Fatal("late Artifact insert failure = nil error")
	}
	afterFailure, err := store.GetProductWorkflowDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.LockVersion != initial.LockVersion || string(afterFailure.Content) != string(initial.Content) {
		t.Fatalf("failed StartRun changed Draft = %#v, want %#v", afterFailure, initial)
	}

	failed.Artifacts = nil
	result, err := store.StartProductWorkflowRun(ctx, failed)
	if err != nil {
		t.Fatalf("retry same Revision and Run identities after rollback: %v", err)
	}
	if result.Revision.ID != revision.ID || result.Run.ID != run.ID || result.Draft.LockVersion != initial.LockVersion+1 {
		t.Fatalf("retry result = %#v", result)
	}
}

func TestRecordProductWorkflowRunProgressOnlyTreatsTheSameArtifactAsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, _ := openTest(t)
	workflow, err := store.CreateProductWorkflow(ctx, "Conversation")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := store.GetProductWorkflowDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	content := json.RawMessage(`{"nodes":[{"definition":"human-chat","id":"prompt"}],"semanticSchemaVersion":"productWorkflow/v1"}`)
	revisionContent, semanticHash, err := productworkflow.RevisionContent(content)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	revision := productworkflow.Revision{ID: "revision-idempotency", WorkflowID: workflow.ID, SemanticHash: semanticHash, Content: revisionContent, CreatedAt: now}
	run := productworkflow.Run{ID: "run-idempotency", WorkflowID: workflow.ID, RevisionID: revision.ID, Status: "running", Snapshot: productworkflow.RunSnapshot{RevisionID: revision.ID}, StartedAt: now, FinishedAt: now}
	if _, err := store.BeginProductWorkflowRun(ctx, productworkflow.StartRunRequest{WorkflowID: workflow.ID, ExpectedLockVersion: draft.LockVersion, DraftContent: content, Revision: revision, Run: run}); err != nil {
		t.Fatal(err)
	}
	ref := artifact.ArtifactRef{ID: "artifact-id", Kind: artifact.KindConversation, Version: "1", URI: "1.json"}
	nodeRun := productworkflow.NodeRun{ID: "node-run-id", RunID: run.ID, NodeID: "prompt", NodeDefinition: "human-chat", NodeExecutor: "v1", Status: "succeeded", Inputs: map[string]artifact.ArtifactRef{}, Outputs: map[string]artifact.ArtifactRef{"conversation": ref}, StartedAt: now, FinishedAt: now}
	item := productworkflow.RunArtifact{ID: ref.ID, RunID: run.ID, NodeRunID: nodeRun.ID, NodeID: "prompt", Port: "conversation", Type: "Conversation", Version: "1", URI: ref.URI, CreatedAt: now}
	if err := store.RecordProductWorkflowRunProgress(ctx, run.ID, []productworkflow.NodeRun{nodeRun}, []productworkflow.RunArtifact{item}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordProductWorkflowRunProgress(ctx, run.ID, []productworkflow.NodeRun{nodeRun}, []productworkflow.RunArtifact{item}); err != nil {
		t.Fatalf("repeat identical Artifact: %v", err)
	}

	mismatched := item
	mismatched.URI = "other.json"
	if err := store.RecordProductWorkflowRunProgress(ctx, run.ID, nil, []productworkflow.RunArtifact{mismatched}); err == nil {
		t.Fatal("same Artifact ID with different metadata = nil error")
	}
	colliding := item
	colliding.ID = "different-artifact-id"
	if err := store.RecordProductWorkflowRunProgress(ctx, run.ID, nil, []productworkflow.RunArtifact{colliding}); err == nil {
		t.Fatal("different Artifact ID with the same Run/Node/port/version = nil error")
	}
}
