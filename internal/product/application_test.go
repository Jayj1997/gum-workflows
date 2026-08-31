package product_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
)

func newTestApplication(t *testing.T, store *history.Store) *product.Application {
	t.Helper()
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	return product.NewApplication(store, registry)
}

func TestApplicationCreatesAndListsSQLiteWorkflowsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "product.db")

	store, err := history.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	application := newTestApplication(t, store)

	first, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Release checklist"})
	if err != nil {
		t.Fatalf("create first workflow: %v", err)
	}
	second, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Incident review"})
	if err != nil {
		t.Fatalf("create second workflow: %v", err)
	}
	for _, workflow := range []product.WorkflowView{first, second} {
		if _, err := uuid.Parse(workflow.ID); err != nil {
			t.Errorf("workflow ID %q is not a UUID: %v", workflow.ID, err)
		}
	}

	want, err := application.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close product database: %v", err)
	}

	reopened, err := history.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen product database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := newTestApplication(t, reopened).ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list workflows after restart: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workflows after restart = %#v, want %#v", got, want)
	}
	if len(got) != 2 {
		t.Fatalf("workflow count = %d, want 2", len(got))
	}
	if got[0].DisplayName != "Release checklist" || got[1].DisplayName != "Incident review" {
		t.Fatalf("workflow order = %#v, want creation order", got)
	}
}

func TestApplicationCreatesAndLoadsOneDraftPerWorkflow(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)

	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Release checklist"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	draft, err := application.GetDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}

	if draft.WorkflowID != workflow.ID {
		t.Fatalf("draft workflow ID = %q, want %q", draft.WorkflowID, workflow.ID)
	}
	if draft.LockVersion != 1 {
		t.Fatalf("draft lock version = %d, want 1", draft.LockVersion)
	}
	if got := draft.Content["semanticSchemaVersion"]; got != "productWorkflow/v1" {
		t.Fatalf("semantic schema version = %#v", got)
	}
	if got, ok := draft.Content["nodes"].([]any); !ok || len(got) != 0 {
		t.Fatalf("initial nodes = %#v, want empty list", draft.Content["nodes"])
	}
}

func TestApplicationAutosavesOnlySemanticChangesAndRejectsStaleUpdates(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Release checklist"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	initial, err := application.GetDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("get initial draft: %v", err)
	}

	changed, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID:          workflow.ID,
		ExpectedLockVersion: initial.LockVersion,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes":                 []any{},
			"project":               map[string]any{"repository": "/workspace/example"},
		},
	})
	if err != nil {
		t.Fatalf("save changed draft: %v", err)
	}
	if !changed.Saved || changed.Conflict || changed.Draft.LockVersion != 2 {
		t.Fatalf("changed result = %#v", changed)
	}

	noOp, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID:          workflow.ID,
		ExpectedLockVersion: changed.Draft.LockVersion,
		Content: map[string]any{
			"project":               map[string]any{"repository": "/workspace/example"},
			"nodes":                 []any{},
			"semanticSchemaVersion": "productWorkflow/v1",
		},
	})
	if err != nil {
		t.Fatalf("save semantically unchanged draft: %v", err)
	}
	if noOp.Saved || noOp.Conflict || noOp.Draft.LockVersion != changed.Draft.LockVersion {
		t.Fatalf("no-op result = %#v", noOp)
	}
	if !noOp.Draft.UpdatedAt.Equal(changed.Draft.UpdatedAt) {
		t.Fatalf("no-op updated_at = %s, want %s", noOp.Draft.UpdatedAt, changed.Draft.UpdatedAt)
	}

	newer, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID:          workflow.ID,
		ExpectedLockVersion: changed.Draft.LockVersion,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes":                 []any{},
			"project":               map[string]any{"repository": "/workspace/newer"},
		},
	})
	if err != nil {
		t.Fatalf("save newer draft: %v", err)
	}
	conflict, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID:          workflow.ID,
		ExpectedLockVersion: changed.Draft.LockVersion,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes":                 []any{},
			"project":               map[string]any{"repository": "/workspace/stale"},
		},
	})
	if err != nil {
		t.Fatalf("save stale draft: %v", err)
	}
	if !conflict.Conflict || !conflict.RefreshRequired || conflict.Saved {
		t.Fatalf("conflict result = %#v", conflict)
	}
	if !reflect.DeepEqual(conflict.Draft, newer.Draft) {
		t.Fatalf("conflict draft = %#v, want latest %#v", conflict.Draft, newer.Draft)
	}
	if got := conflict.Draft.Content["project"].(map[string]any)["repository"]; got != "/workspace/newer" {
		t.Fatalf("stored repository = %#v, want newer value", got)
	}
}

func TestApplicationSavesInvalidDraftWithCompletePreviewDiagnostics(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Incomplete workflow"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	result, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID:          workflow.ID,
		ExpectedLockVersion: 1,
		Content:             map[string]any{},
	})
	if err != nil {
		t.Fatalf("save invalid draft: %v", err)
	}
	if !result.Saved || result.Draft.LockVersion != 2 {
		t.Fatalf("save invalid result = %#v", result)
	}
	if result.Preview.Nodes == nil || result.Preview.Edges == nil || result.Preview.Groups == nil {
		t.Fatalf("preview collections must be present: %#v", result.Preview)
	}
	if len(result.Preview.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want schema version and nodes errors", result.Preview.Diagnostics)
	}
	loaded, err := application.GetDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("reload invalid draft: %v", err)
	}
	if len(loaded.Content) != 0 || loaded.LockVersion != 2 {
		t.Fatalf("loaded invalid draft = %#v", loaded)
	}
}

func TestApplicationDoesNotListWorkflowV1Imports(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ImportWorkflow(ctx, history.WorkflowRow{Name: "yaml-workflow", Version: "v1"}, nil); err != nil {
		t.Fatalf("import workflow/v1 definition: %v", err)
	}

	workflows, err := newTestApplication(t, store).ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list Product Workflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("Product Workflows = %#v, want workflow/v1 import excluded", workflows)
	}
}

func TestApplicationRejectsBlankWorkflowNameWithoutCreatingWorkflow(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)

	if _, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "  "}); err == nil {
		t.Fatal("create blank workflow = nil error, want rejection")
	}
	workflows, err := application.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("workflow count = %d, want zero", len(workflows))
	}
}

func TestApplicationCatalogAndConfigDiagnosticsComeFromRegisteredDefinitions(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)

	catalog, err := application.ListNodeCatalog(ctx)
	if err != nil {
		t.Fatalf("list Node Catalog: %v", err)
	}
	if len(catalog) != 2 || catalog[0].Definition.ID != "human-chat" || catalog[1].Definition.ID != "llm-chat" {
		t.Fatalf("Node Catalog = %#v", catalog)
	}

	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Chat"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	result, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: 1,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes": []any{map[string]any{
				"id": "writer", "definition": "llm-chat", "executor": "v1", "displayName": "Writer",
				"config": map[string]any{"temperature": 2.5, "max_output_tokens": 0, "unknown": true},
			}},
		},
	})
	if err != nil {
		t.Fatalf("save invalid Node config: %v", err)
	}
	wantPaths := []string{
		"nodes[0].config.temperature",
		"nodes[0].config.max_output_tokens",
		"nodes[0].config.unknown",
	}
	if len(result.Preview.Diagnostics) != len(wantPaths) {
		t.Fatalf("diagnostics = %#v", result.Preview.Diagnostics)
	}
	for i, want := range wantPaths {
		if result.Preview.Diagnostics[i].Path != want {
			t.Fatalf("diagnostic %d path = %q, want %q", i, result.Preview.Diagnostics[i].Path, want)
		}
	}
	if len(result.Preview.Nodes) != 1 {
		t.Fatalf("preview nodes = %#v, want invalid Node retained", result.Preview.Nodes)
	}
}
