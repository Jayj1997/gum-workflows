package product_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
)

// saveModelBoundDraft stores a tracer Draft whose agent Node selects the given
// Gum Model UUID and returns the saved Draft view.
func saveModelBoundDraft(t *testing.T, ctx context.Context, application *product.Application, workflowID string, modelUUID string, lockVersion uint64) product.DraftUpdateView {
	t.Helper()
	saved, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflowID, ExpectedLockVersion: lockVersion,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes": []any{
				map[string]any{"id": "prompt", "definition": "human-chat", "executor": "v1", "displayName": "Prompt", "config": map[string]any{}},
				map[string]any{
					"id": "answer", "definition": "llm-chat", "executor": "v1", "displayName": "Answer", "config": map[string]any{},
					"inputs": map[string]any{"conversation": map[string]any{"from": "prompt.conversation"}},
					"llm":    map[string]any{"modelUuid": modelUUID},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("save model-bound Draft: %v", err)
	}
	return saved
}

func TestApplicationPreviewsAffectedWorkflowsBeforeModelDeletion(t *testing.T) {
	ctx := context.Background()
	store := openSettingsStore(t)
	application := newTestApplication(t, store)
	provider := createSettingsProvider(t, application, "Primary")
	model, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fast", ProviderModelID: "model-fast"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Release checklist"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Incident review"})
	if err != nil {
		t.Fatal(err)
	}
	saveModelBoundDraft(t, ctx, application, first.ID, model.ID, 1)
	saveModelBoundDraft(t, ctx, application, second.ID, model.ID, 1)

	impact, err := application.ListModelDeletionImpact(ctx, provider.ID, model.ID)
	if err != nil {
		t.Fatalf("list model deletion impact: %v", err)
	}
	if len(impact.Workflows) != 2 {
		t.Fatalf("affected workflows = %#v, want both referencing workflows", impact.Workflows)
	}
	// Workflow UUIDs are random, so match entries by identity rather than order.
	entries := map[string]product.AffectedWorkflowView{}
	for _, entry := range impact.Workflows {
		entries[entry.ID] = entry
	}
	if entry, ok := entries[first.ID]; !ok || entry.DisplayName != "Release checklist" || entry.NodeID != "answer" || entry.NodeDefinition != "llm-chat" || entry.ModelUUID != model.ID {
		t.Fatalf("first workflow entry = %#v", entries[first.ID])
	}
	if entry, ok := entries[second.ID]; !ok || entry.DisplayName != "Incident review" || entry.NodeID != "answer" || entry.NodeDefinition != "llm-chat" || entry.ModelUUID != model.ID {
		t.Fatalf("second workflow entry = %#v", entries[second.ID])
	}
	if len(impact.ModelSlots) != 0 {
		t.Fatalf("model deletion ModelSlots = %#v, want none (the slot is named by the arguments)", impact.ModelSlots)
	}
}

func TestApplicationPreviewsAffectedWorkflowsBeforeProviderDeletion(t *testing.T) {
	ctx := context.Background()
	store := openSettingsStore(t)
	application := newTestApplication(t, store)
	provider := createSettingsProvider(t, application, "Primary")
	first, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fast", ProviderModelID: "model-fast"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Strong", ProviderModelID: "model-strong"})
	if err != nil {
		t.Fatal(err)
	}
	firstWorkflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Release checklist"})
	if err != nil {
		t.Fatal(err)
	}
	secondWorkflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Incident review"})
	if err != nil {
		t.Fatal(err)
	}
	saveModelBoundDraft(t, ctx, application, firstWorkflow.ID, first.ID, 1)
	saveModelBoundDraft(t, ctx, application, secondWorkflow.ID, second.ID, 1)

	impact, err := application.ListProviderDeletionImpact(ctx, provider.ID)
	if err != nil {
		t.Fatalf("list provider deletion impact: %v", err)
	}
	if len(impact.ModelSlots) != 2 {
		t.Fatalf("provider deletion ModelSlots = %#v, want both slots listed", impact.ModelSlots)
	}
	slots := map[string]product.AffectedModelSlotView{}
	for _, slot := range impact.ModelSlots {
		slots[slot.ID] = slot
	}
	if slot, ok := slots[first.ID]; !ok || slot.DisplayName != "Fast" || slot.ProviderModelID != "model-fast" {
		t.Fatalf("first Model Slot entry = %#v", slots[first.ID])
	}
	if slot, ok := slots[second.ID]; !ok || slot.DisplayName != "Strong" || slot.ProviderModelID != "model-strong" {
		t.Fatalf("second Model Slot entry = %#v", slots[second.ID])
	}
	if len(impact.Workflows) != 2 {
		t.Fatalf("affected workflows = %#v, want one entry per referencing workflow", impact.Workflows)
	}
	seen := map[string]bool{}
	for _, entry := range impact.Workflows {
		if entry.NodeDefinition != "llm-chat" {
			t.Fatalf("affected workflow entry = %#v", entry)
		}
		if entry.ID != firstWorkflow.ID && entry.ID != secondWorkflow.ID {
			t.Fatalf("affected workflow ID = %q, want one of the two created workflows", entry.ID)
		}
		seen[entry.ModelUUID] = true
	}
	if !seen[first.ID] || !seen[second.ID] {
		t.Fatalf("provider impact missed Model UUIDs: %#v", impact.Workflows)
	}
}

func TestApplicationDeletionImpactFailsForUnknownModelOrProvider(t *testing.T) {
	ctx := context.Background()
	application := newTestApplication(t, openSettingsStore(t))
	provider := createSettingsProvider(t, application, "Primary")
	if _, err := application.ListModelDeletionImpact(ctx, provider.ID, "missing-model"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown Model impact error = %v", err)
	}
	if _, err := application.ListProviderDeletionImpact(ctx, "missing-provider"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown Provider impact error = %v", err)
	}
}

func TestApplicationDanglingModelUuidBlocksPreviewAndStartRunUntilReselected(t *testing.T) {
	ctx := context.Background()
	application := newTestApplication(t, openSettingsStore(t))
	provider := createSettingsProvider(t, application, "Primary")
	first, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fast", ProviderModelID: "model-fast"})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Conversation"})
	if err != nil {
		t.Fatal(err)
	}
	saved := saveModelBoundDraft(t, ctx, application, workflow.ID, first.ID, 1)

	// Deleting the referenced Model leaves the Draft untouched.
	if err := application.DeleteLLMModel(ctx, provider.ID, first.ID); err != nil {
		t.Fatalf("delete referenced Model: %v", err)
	}
	afterDelete, err := application.GetDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterDelete.LockVersion != saved.Draft.LockVersion {
		t.Fatalf("delete changed Draft lock version to %d, want %d", afterDelete.LockVersion, saved.Draft.LockVersion)
	}

	// The dangling UUID produces a specific field diagnostic in Preview.
	var dangling *product.Diagnostic
	for _, diagnostic := range afterDelete.Preview.Diagnostics {
		if diagnostic.Code == "dangling-model-uuid" {
			found := diagnostic
			dangling = &found
		}
	}
	if dangling == nil {
		t.Fatalf("preview diagnostics = %#v, want dangling-model-uuid", afterDelete.Preview.Diagnostics)
	}
	if dangling.Path != "nodes[1].llm.modelUuid" || !strings.Contains(dangling.Message, first.ID) {
		t.Fatalf("dangling diagnostic = %#v", dangling)
	}

	// StartRun fails before creating any Run.
	if _, err := application.StartRun(ctx, singleTurnInput(workflow.ID, afterDelete.LockVersion)); err == nil {
		t.Fatal("start Run with dangling Model UUID = nil error")
	}
	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 0 {
		t.Fatalf("blocked StartRun persisted %d Revision(s), want zero", len(revisions))
	}

	// Re-selecting a live Model clears the diagnostic and saves normally.
	second, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Strong", ProviderModelID: "model-strong"})
	if err != nil {
		t.Fatal(err)
	}
	reselected := saveModelBoundDraft(t, ctx, application, workflow.ID, second.ID, afterDelete.LockVersion)
	for _, diagnostic := range reselected.Preview.Diagnostics {
		if diagnostic.Code == "dangling-model-uuid" || diagnostic.Code == "missing-model-uuid" {
			t.Fatalf("re-selected Draft still reports %q", diagnostic.Code)
		}
	}
	if reselected.Draft.LockVersion != afterDelete.LockVersion+1 {
		t.Fatalf("re-selected lock version = %d, want %d", reselected.Draft.LockVersion, afterDelete.LockVersion+1)
	}
}

func TestApplicationReselectedModelUuidSavesAndRunsAsNewRevision(t *testing.T) {
	ctx := context.Background()
	server, _ := startFixtureLLMServer(t)
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
	application := newTestApplicationWithRunsAt(t, store, paths, server)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "test-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fast", ProviderModelID: "model-fast"})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Conversation"})
	if err != nil {
		t.Fatal(err)
	}
	saved := saveModelBoundDraft(t, ctx, application, workflow.ID, first.ID, 1)
	blocked, err := application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if err != nil {
		t.Fatal(err)
	}

	// Re-select a different live Model, then run: the Draft keeps saving and
	// the model change forms a new immutable Revision rather than mutating the
	// one the blocked Run already created.
	second, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Strong", ProviderModelID: "model-strong"})
	if err != nil {
		t.Fatal(err)
	}
	reselected := saveModelBoundDraft(t, ctx, application, workflow.ID, second.ID, blocked.Draft.LockVersion)
	if reselected.Draft.LockVersion != blocked.Draft.LockVersion+1 {
		t.Fatalf("re-selected lock version = %d, want %d", reselected.Draft.LockVersion, blocked.Draft.LockVersion+1)
	}
	for _, diagnostic := range reselected.Preview.Diagnostics {
		if diagnostic.Code == "dangling-model-uuid" || diagnostic.Code == "missing-model-uuid" {
			t.Fatalf("re-selected Draft still reports %q", diagnostic.Code)
		}
	}
	run, err := application.StartRun(ctx, singleTurnInput(workflow.ID, reselected.Draft.LockVersion))
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" || run.RevisionID == blocked.RevisionID {
		t.Fatalf("re-selected Run = status %q revision %q, want succeeded with a new Revision (blocked %q)", run.Status, run.RevisionID, blocked.RevisionID)
	}
	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("revisions = %d, want 2 (deleted-UUID revision and re-selected UUID revision)", len(revisions))
	}
}

func TestApplicationHistoricalRunSnapshotKeepsDeletedModelSelection(t *testing.T) {
	ctx := context.Background()
	server, requests := startFixtureLLMServer(t)
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
	application := newTestApplicationWithRunsAt(t, store, paths, server)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "test-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Conversation"})
	if err != nil {
		t.Fatal(err)
	}
	saved := saveModelBoundDraft(t, ctx, application, workflow.ID, model.ID, 1)
	run, err := application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || (*requests)[0].Body["model"] != "fixture-model" {
		t.Fatalf("fixture request model = %#v", (*requests)[0].Body)
	}

	// Delete the whole Provider: the historical Run keeps showing the Provider
	// name and Provider Model ID resolved at Run time.
	if err := application.DeleteLLMProvider(ctx, product.DeleteLLMProviderInput{ProviderID: provider.ID, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	detail, err := application.GetRunHistory(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Snapshot.LLMSelections) != 1 {
		t.Fatalf("historical LLM selections = %#v", detail.Snapshot.LLMSelections)
	}
	selection := detail.Snapshot.LLMSelections[0]
	if selection.ProviderName != "Primary" || selection.ProviderModelID != "fixture-model" || selection.ModelUUID != model.ID {
		t.Fatalf("historical selection = %#v", selection)
	}
	for _, diagnostic := range detail.Draft.Preview.Diagnostics {
		if diagnostic.Code == "dangling-model-uuid" {
			t.Fatal("historical Preview flagged the deleted Model UUID")
		}
	}
}

func openSettingsStore(t *testing.T) *history.Store {
	t.Helper()
	store, err := history.Open(context.Background(), filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createSettingsProvider(t *testing.T, application *product.Application, name string) product.LLMProviderView {
	t.Helper()
	provider, err := application.CreateLLMProvider(context.Background(), product.CreateLLMProviderInput{
		Name: name, Protocol: "openai-chat-completions", BaseURL: "https://api.example/v1", APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
