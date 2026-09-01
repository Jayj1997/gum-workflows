package product_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/secret"
)

// newTracerApplication opens a fresh SQLite store with a Provider, a Model and the
// tracer Draft so each history test starts from one runnable Workflow.
func newTracerApplication(t *testing.T, ctx context.Context) (*product.Application, runtimepath.Paths, *history.Store) {
	t.Helper()
	root := t.TempDir()
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := history.Open(ctx, paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplicationWithRuns(t, store, paths)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "test-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fake", ProviderModelID: "fake"}); err != nil {
		t.Fatal(err)
	}
	return application, paths, store
}

// assistantArtifact returns the agent Node's Artifact view from one Run.
func assistantArtifact(run product.RunView) product.ArtifactView {
	for _, view := range run.Artifacts {
		if view.NodeID == "answer" {
			return view
		}
	}
	return product.ArtifactView{}
}

func TestApplicationListsRevisionsRunsAndHistoryAfterRepeatedStartRun(t *testing.T) {
	ctx := context.Background()
	application, _, _ := newTracerApplication(t, ctx)
	workflow, saved := saveTracerDraft(t, ctx, application)
	first, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: first.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}

	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].ID != first.RevisionID || revisions[0].RunCount != 2 || revisions[0].SemanticHash == "" {
		t.Fatalf("revisions = %#v, want one reusing %q with runCount 2", revisions, first.RevisionID)
	}
	runs, err := application.ListRevisionRuns(ctx, revisions[0].ID)
	if err != nil {
		t.Fatalf("list revision runs: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != first.ID || runs[1].ID != second.ID {
		t.Fatalf("revision runs = %#v", runs)
	}
	detail, err := application.GetRunHistory(ctx, first.ID)
	if err != nil {
		t.Fatalf("get run history: %v", err)
	}
	if detail.ID != first.ID || detail.RevisionID != first.RevisionID || detail.Status != "succeeded" {
		t.Fatalf("run history = %#v", detail)
	}
	if len(detail.NodeRuns) != 2 || len(detail.Artifacts) != 2 {
		t.Fatalf("run history nodeRuns/artifacts = %#v", detail)
	}
	if len(first.Artifacts) != 2 {
		t.Fatalf("live run artifacts = %#v, want two", first.Artifacts)
	}
	// The historical Run reconstructs the same Conversation messages from disk.
	historicalAssistant := assistantArtifact(detail)
	if len(historicalAssistant.Messages) != 2 || historicalAssistant.Messages[0].Role != "user" || historicalAssistant.Messages[1].Role != "assistant" {
		t.Fatalf("historical conversation messages = %#v, want reconstructed user+assistant", historicalAssistant.Messages)
	}
}

func TestApplicationSemanticChangeCreatesNewRevision(t *testing.T) {
	ctx := context.Background()
	application, _, _ := newTracerApplication(t, ctx)
	workflow, saved := saveTracerDraft(t, ctx, application)
	first, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}

	// Semantic change: add a Control Dependency to the agent node.
	updated, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: first.Draft.LockVersion,
		Content: withAgentDependsOn(t, saved.Draft.Content),
	})
	if err != nil {
		t.Fatalf("semantic autosave: %v", err)
	}
	second, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: updated.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if second.RevisionID == first.RevisionID {
		t.Fatalf("semantic change reused Revision %q", first.RevisionID)
	}
	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("revisions = %d, want 2 after semantic change", len(revisions))
	}
}

func TestApplicationPresentationChangeDoesNotCreateRevision(t *testing.T) {
	ctx := context.Background()
	application, _, _ := newTracerApplication(t, ctx)
	workflow, saved := saveTracerDraft(t, ctx, application)
	first, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}

	// Non-semantic change: only display text, presentation hints and view state.
	updated, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: first.Draft.LockVersion,
		Content: withPresentationOnlyChanges(t, saved.Draft.Content),
	})
	if err != nil {
		t.Fatalf("presentation autosave: %v", err)
	}
	second, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: updated.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if second.RevisionID != first.RevisionID {
		t.Fatalf("presentation change created a new Revision: %q vs %q", second.RevisionID, first.RevisionID)
	}
	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("revisions = %d, want 1 (presentation must not create a Revision)", len(revisions))
	}
}

func TestApplicationFirstModelUuidMaterializationCreatesNewRevision(t *testing.T) {
	ctx := context.Background()
	application, _, _ := newTracerApplication(t, ctx)
	workflow, saved := saveTracerDraft(t, ctx, application)

	// First StartRun materializes the default Model UUID into the agent node.
	first, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	// Re-running the same materialized content reuses the Revision.
	second, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: first.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if second.RevisionID != first.RevisionID {
		t.Fatalf("materialized re-run created a new Revision: %q vs %q", second.RevisionID, first.RevisionID)
	}

	// Selecting a different Model UUID changes the execution semantics, so the
	// next StartRun must create a new immutable Revision.
	settings, err := application.GetLLMSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Providers) != 1 || len(settings.Providers[0].Models) != 1 {
		t.Fatalf("settings = %#v, want one Provider with one Model", settings)
	}
	other, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: settings.Providers[0].ID, DisplayName: "Other", ProviderModelID: "other"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: second.Draft.LockVersion,
		Content: withAgentModelUUID(t, second.Draft.Content, other.ID),
	})
	if err != nil {
		t.Fatalf("model selection autosave: %v", err)
	}
	third, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: updated.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if third.RevisionID == first.RevisionID {
		t.Fatalf("model UUID change reused Revision %q", first.RevisionID)
	}
	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("revisions = %d, want 2 (materialized default and explicit model UUID)", len(revisions))
	}
}

func TestApplicationRunHistorySurvivesRestart(t *testing.T) {
	ctx := context.Background()
	application, paths, store := newTracerApplication(t, ctx)
	workflow, saved := saveTracerDraft(t, ctx, application)
	first, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the same database and runs directory: history must remain queryable.
	reopened, err := history.Open(ctx, paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	reopenedApp := product.NewApplication(reopened, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()))

	revisions, err := reopenedApp.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("list revisions after restart: %v", err)
	}
	if len(revisions) != 1 || revisions[0].ID != first.RevisionID {
		t.Fatalf("revisions after restart = %#v", revisions)
	}
	runs, err := reopenedApp.ListRevisionRuns(ctx, first.RevisionID)
	if err != nil {
		t.Fatalf("list revision runs after restart: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != first.ID {
		t.Fatalf("runs after restart = %#v", runs)
	}
	detail, err := reopenedApp.GetRunHistory(ctx, first.ID)
	if err != nil {
		t.Fatalf("get run history after restart: %v", err)
	}
	if detail.Status != "succeeded" || len(detail.NodeRuns) != 2 || len(detail.Artifacts) != 2 {
		t.Fatalf("run history after restart = %#v", detail)
	}
	historicalAssistant := assistantArtifact(detail)
	if len(historicalAssistant.Messages) != 2 {
		t.Fatalf("conversation messages after restart = %#v, want reconstructed", historicalAssistant.Messages)
	}
}

func mustCatalog(t *testing.T) *nodecatalog.Registry {
	t.Helper()
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	return registry
}

// withAgentDependsOn returns a copy of the tracer Draft content with a Control
// Dependency from the agent node to the human node (a semantic change).
func withAgentDependsOn(t *testing.T, content map[string]any) map[string]any {
	t.Helper()
	clone := cloneMap(t, content)
	nodes := clone["nodes"].([]any)
	for _, value := range nodes {
		node := value.(map[string]any)
		if node["id"] == "answer" {
			node["dependsOn"] = []any{"prompt"}
		}
	}
	return clone
}

// withAgentModelUUID returns a copy of the tracer Draft content with the agent
// node's LLM preference pointed at the given Gum Model UUID.
func withAgentModelUUID(t *testing.T, content map[string]any, modelUUID string) map[string]any {
	t.Helper()
	clone := cloneMap(t, content)
	nodes := clone["nodes"].([]any)
	for _, value := range nodes {
		node := value.(map[string]any)
		if node["id"] != "answer" {
			continue
		}
		preference, _ := node["llm"].(map[string]any)
		if preference == nil {
			preference = map[string]any{}
			node["llm"] = preference
		}
		preference["modelUuid"] = modelUUID
	}
	return clone
}

// withPresentationOnlyChanges returns a copy of the tracer Draft content with only
// non-semantic fields changed (display text, presentation hints, view state).
func withPresentationOnlyChanges(t *testing.T, content map[string]any) map[string]any {
	t.Helper()
	clone := cloneMap(t, content)
	clone["displayName"] = "Renamed workflow"
	clone["view"] = map[string]any{"zoom": 2}
	nodes := clone["nodes"].([]any)
	for _, value := range nodes {
		node := value.(map[string]any)
		node["displayName"] = "Renamed " + node["id"].(string)
		node["presentation"] = map[string]any{"x": 9, "y": 9}
	}
	return clone
}
