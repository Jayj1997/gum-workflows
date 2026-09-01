package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// startTestRun is a store-level helper that materializes a Draft and publishes a
// completed fake Run with two Node Runs and two Conversation Artifacts, mirroring
// the Application's StartRun write path without importing the product package.
func startTestRun(t *testing.T, ctx context.Context, s *Store, workflowID string, expectedLockVersion uint64, draftContent json.RawMessage, now time.Time) (productworkflow.StartRunResult, error) {
	t.Helper()
	revisionContent, semanticHash, err := productworkflow.RevisionContent(draftContent)
	if err != nil {
		return productworkflow.StartRunResult{}, err
	}
	revision := productworkflow.Revision{ID: "rev-" + workflowID + "-" + semanticHash[:8], WorkflowID: workflowID, SemanticHash: semanticHash, Content: revisionContent, CreatedAt: now}
	run := productworkflow.Run{ID: fmt.Sprintf("run-%s-%d", semanticHash[:8], expectedLockVersion), WorkflowID: workflowID, RevisionID: revision.ID, Status: "succeeded", Snapshot: productworkflow.RunSnapshot{RevisionID: revision.ID}, StartedAt: now, FinishedAt: now}
	nodeRuns := []productworkflow.NodeRun{
		{ID: "nr-h-" + run.ID, RunID: run.ID, NodeID: "prompt", NodeDefinition: "human-chat", NodeExecutor: "v1", Status: "succeeded", Inputs: map[string]artifact.ArtifactRef{}, Outputs: map[string]artifact.ArtifactRef{}, StartedAt: now, FinishedAt: now},
		{ID: "nr-a-" + run.ID, RunID: run.ID, NodeID: "answer", NodeDefinition: "llm-chat", NodeExecutor: "v1", Status: "succeeded", Inputs: map[string]artifact.ArtifactRef{}, Outputs: map[string]artifact.ArtifactRef{}, StartedAt: now, FinishedAt: now},
	}
	artifacts := []productworkflow.RunArtifact{
		{ID: "art-h-" + run.ID, RunID: run.ID, NodeRunID: nodeRuns[0].ID, NodeID: "prompt", Port: "conversation", Type: "Conversation", Version: "1", URI: "1.json", CreatedAt: now},
		{ID: "art-a-" + run.ID, RunID: run.ID, NodeRunID: nodeRuns[1].ID, NodeID: "answer", Port: "conversation", Type: "Conversation", Version: "2", URI: "2.json", CreatedAt: now.Add(time.Millisecond)},
	}
	// Use distinct run/node-run/artifact IDs per call by suffixing the lock version.
	return s.StartProductWorkflowRun(ctx, productworkflow.StartRunRequest{
		WorkflowID: workflowID, ExpectedLockVersion: expectedLockVersion, DraftContent: draftContent,
		Revision: revision, Run: run, NodeRuns: nodeRuns, Artifacts: artifacts,
	})
}

func TestStoreProductHistoryListsRevisionsRunsAndRunDetail(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t)
	workflow, err := s.CreateProductWorkflow(ctx, "Conversation")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.GetProductWorkflowDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	content := json.RawMessage(`{"nodes":[{"definition":"llm-chat","id":"answer","llm":{"modelUuid":"model-id"}},{"definition":"human-chat","id":"prompt"}],"semanticSchemaVersion":"productWorkflow/v1"}`)

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	first, err := startTestRun(t, ctx, s, workflow.ID, draft.LockVersion, content, now)
	if err != nil {
		t.Fatalf("first start run: %v", err)
	}
	secondDraft, err := s.GetProductWorkflowDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := startTestRun(t, ctx, s, workflow.ID, secondDraft.LockVersion, content, now.Add(time.Second))
	if err != nil {
		t.Fatalf("second start run: %v", err)
	}

	// Same semantic content reuses the Revision but creates a new Run.
	revisions, err := s.ListProductWorkflowRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].ID != first.Revision.ID || revisions[0].SemanticHash != first.Revision.SemanticHash {
		t.Fatalf("revisions = %#v, want one reusing %q", revisions, first.Revision.ID)
	}
	runs, err := s.ListProductWorkflowRevisionRuns(ctx, first.Revision.ID)
	if err != nil {
		t.Fatalf("list revision runs: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != first.Run.ID || runs[1].ID != second.Run.ID {
		t.Fatalf("revision runs = %#v, want %q and %q", runs, first.Run.ID, second.Run.ID)
	}

	// Run detail returns the Run, its Node Runs and Artifact references.
	run, nodeRuns, artifacts, err := s.GetProductRun(ctx, first.Run.ID)
	if err != nil {
		t.Fatalf("get product run: %v", err)
	}
	if run.ID != first.Run.ID || run.Status != "succeeded" {
		t.Fatalf("run = %#v", run)
	}
	if len(nodeRuns) != 2 {
		t.Fatalf("node runs = %#v, want two", nodeRuns)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %#v, want two", artifacts)
	}
	artifactByNode := map[string]productworkflow.RunArtifact{}
	for _, item := range artifacts {
		artifactByNode[item.NodeID] = item
	}
	if artifactByNode["prompt"].URI != "1.json" || artifactByNode["answer"].URI != "2.json" {
		t.Fatalf("artifacts = %#v, want prompt=1.json and answer=2.json", artifacts)
	}
}

func TestStoreProductRunNotFound(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t)
	_, _, _, err := s.GetProductRun(ctx, "missing-run")
	if !errors.Is(err, productworkflow.ErrRunNotFound) {
		t.Fatalf("get missing run err = %v, want ErrRunNotFound", err)
	}
}

func TestStoreSemanticChangeCreatesNewRevisionButPresentationChangeDoesNot(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t)
	workflow, err := s.CreateProductWorkflow(ctx, "Conversation")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.GetProductWorkflowDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	base := json.RawMessage(`{"nodes":[{"definition":"llm-chat","id":"answer","llm":{"modelUuid":"model-id"},"config":{"temperature":0.5}},{"definition":"human-chat","id":"prompt"}],"semanticSchemaVersion":"productWorkflow/v1"}`)
	if _, err := startTestRun(t, ctx, s, workflow.ID, draft.LockVersion, base, now); err != nil {
		t.Fatalf("base run: %v", err)
	}
	d1, _ := s.GetProductWorkflowDraft(ctx, workflow.ID)

	// Semantic change: different temperature config -> new Revision.
	semantic := json.RawMessage(`{"nodes":[{"definition":"llm-chat","id":"answer","llm":{"modelUuid":"model-id"},"config":{"temperature":0.9}},{"definition":"human-chat","id":"prompt"}],"semanticSchemaVersion":"productWorkflow/v1"}`)
	if _, err := startTestRun(t, ctx, s, workflow.ID, d1.LockVersion, semantic, now.Add(time.Second)); err != nil {
		t.Fatalf("semantic-change run: %v", err)
	}
	d2, _ := s.GetProductWorkflowDraft(ctx, workflow.ID)
	revisions, err := s.ListProductWorkflowRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("after semantic change revisions = %d, want 2", len(revisions))
	}

	// Non-semantic change: only displayName/view/presentation -> same Revision count.
	presentation := json.RawMessage(`{"displayName":"Renamed","view":{"zoom":2},"nodes":[{"definition":"llm-chat","id":"answer","displayName":"Writer","presentation":{"x":9},"llm":{"modelUuid":"model-id"},"config":{"temperature":0.9}},{"definition":"human-chat","id":"prompt","displayName":"Renamed prompt"}],"semanticSchemaVersion":"productWorkflow/v1"}`)
	if _, err := startTestRun(t, ctx, s, workflow.ID, d2.LockVersion, presentation, now.Add(2*time.Second)); err != nil {
		t.Fatalf("presentation-change run: %v", err)
	}
	revisions, err = s.ListProductWorkflowRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("after presentation change revisions = %d, want 2 (no new Revision)", len(revisions))
	}
}
