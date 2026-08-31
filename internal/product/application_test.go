package product_test

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"

	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

func TestApplicationManagesProviderModelSettingsAndResolvesDefaults(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "product.db")
	store, err := history.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	application := newTestApplication(t, store)

	first, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://primary.example/v1", APIKeyRef: "keychain://primary",
	})
	if err != nil {
		t.Fatalf("create first Provider: %v", err)
	}
	if !first.EffectiveDefault || first.ExplicitDefault {
		t.Fatalf("first Provider defaults = effective %t explicit %t", first.EffectiveDefault, first.ExplicitDefault)
	}
	second, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Secondary", Protocol: "openai-chat-completions", BaseURL: "https://secondary.example/v1", APIKeyRef: "keychain://secondary",
	})
	if err != nil {
		t.Fatalf("create second Provider: %v", err)
	}
	temperature := 0.3
	maxOutputTokens := 1024
	firstModel, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{
		ProviderID: first.ID, DisplayName: "Fast", ProviderModelID: "model-fast",
		GenerationDefaults: productworkflow.GenerationDefaults{Temperature: &temperature, MaxOutputTokens: &maxOutputTokens},
	})
	if err != nil {
		t.Fatalf("create first Model: %v", err)
	}
	if !firstModel.EffectiveDefault || firstModel.ExplicitDefault {
		t.Fatalf("first Model defaults = effective %t explicit %t", firstModel.EffectiveDefault, firstModel.ExplicitDefault)
	}
	secondModel, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{
		ProviderID: first.ID, DisplayName: "Strong", ProviderModelID: "model-strong",
	})
	if err != nil {
		t.Fatalf("create second Model: %v", err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{
		ProviderID: second.ID, DisplayName: "Backup", ProviderModelID: "backup-model",
	}); err != nil {
		t.Fatalf("create backup Model: %v", err)
	}

	resolved, err := application.ResolveDefaultLLMModel(ctx)
	if err != nil {
		t.Fatalf("resolve fallback default: %v", err)
	}
	if resolved.Provider.ID != first.ID || resolved.Model.ID != firstModel.ID {
		t.Fatalf("fallback selection = %#v, want first Provider and Model", resolved)
	}

	if _, err := application.SetDefaultLLMProvider(ctx, second.ID); err != nil {
		t.Fatalf("set Provider default: %v", err)
	}
	if _, err := application.SetDefaultLLMModel(ctx, first.ID, secondModel.ID); err != nil {
		t.Fatalf("set Model default: %v", err)
	}
	resolved, err = application.ResolveDefaultLLMModel(ctx)
	if err != nil {
		t.Fatalf("resolve explicit default: %v", err)
	}
	if resolved.Provider.ID != second.ID || resolved.Model.ProviderID != second.ID {
		t.Fatalf("explicit selection = %#v, want second Provider and its Model", resolved)
	}

	updatedProvider, err := application.UpdateLLMProvider(ctx, product.UpdateLLMProviderInput{
		ID: first.ID, Name: "Primary renamed", Protocol: "openai-chat-completions", BaseURL: "https://new.example/v1", APIKeyRef: "keychain://rotated",
	})
	if err != nil {
		t.Fatalf("update Provider: %v", err)
	}
	updatedModel, err := application.UpdateLLMModel(ctx, product.UpdateLLMModelInput{
		ID: firstModel.ID, ProviderID: first.ID, DisplayName: "Fast renamed", ProviderModelID: "model-fast-v2",
		GenerationDefaults: productworkflow.GenerationDefaults{MaxOutputTokens: &maxOutputTokens},
	})
	if err != nil {
		t.Fatalf("update Model: %v", err)
	}
	if updatedProvider.ID != first.ID || updatedModel.ID != firstModel.ID {
		t.Fatalf("editing changed stable UUIDs: Provider %q -> %q, Model %q -> %q", first.ID, updatedProvider.ID, firstModel.ID, updatedModel.ID)
	}
	if updatedModel.GenerationDefaults.Temperature != nil || updatedModel.GenerationDefaults.MaxOutputTokens == nil || *updatedModel.GenerationDefaults.MaxOutputTokens != 1024 {
		t.Fatalf("updated generation defaults = %#v", updatedModel.GenerationDefaults)
	}

	if err := application.DeleteLLMProvider(ctx, second.ID); err != nil {
		t.Fatalf("delete explicit default Provider: %v", err)
	}
	resolved, err = application.ResolveDefaultLLMModel(ctx)
	if err != nil {
		t.Fatalf("resolve after deleting Provider default: %v", err)
	}
	if resolved.Provider.ID != first.ID || resolved.Model.ID != secondModel.ID {
		t.Fatalf("selection after deleting Provider default = %#v", resolved)
	}
	if err := application.DeleteLLMModel(ctx, first.ID, secondModel.ID); err != nil {
		t.Fatalf("delete explicit default Model: %v", err)
	}
	resolved, err = application.ResolveDefaultLLMModel(ctx)
	if err != nil {
		t.Fatalf("resolve after deleting Model default: %v", err)
	}
	if resolved.Model.ID != firstModel.ID {
		t.Fatalf("Model after deleting explicit default = %q, want %q", resolved.Model.ID, firstModel.ID)
	}

	settingsBeforeRestart, err := application.GetLLMSettings(ctx)
	if err != nil {
		t.Fatalf("get LLM settings: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close product database: %v", err)
	}
	reopened, err := history.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen product database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	settingsAfterRestart, err := newTestApplication(t, reopened).GetLLMSettings(ctx)
	if err != nil {
		t.Fatalf("get LLM settings after restart: %v", err)
	}
	if !reflect.DeepEqual(settingsAfterRestart, settingsBeforeRestart) {
		t.Fatalf("settings after restart = %#v, want %#v", settingsAfterRestart, settingsBeforeRestart)
	}
}

func TestApplicationReturnsSettingsDiagnosticsWhenDefaultCannotResolve(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)
	if _, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Unsafe", Protocol: "openai-chat-completions", BaseURL: "https://api.example/v1", APIKeyRef: "plaintext-secret",
	}); err == nil {
		t.Fatal("create Provider accepted a plaintext API Key instead of a Secret reference")
	}

	resolved, err := application.ResolveDefaultLLMModel(ctx)
	if err != nil {
		t.Fatalf("resolve without Provider: %v", err)
	}
	if len(resolved.Diagnostics) != 1 || resolved.Diagnostics[0].Code != "llm-provider-required" {
		t.Fatalf("diagnostics without Provider = %#v", resolved.Diagnostics)
	}
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://api.example/v1", APIKeyRef: "keychain://primary",
	})
	if err != nil {
		t.Fatalf("create Provider: %v", err)
	}
	resolved, err = application.ResolveDefaultLLMModel(ctx)
	if err != nil {
		t.Fatalf("resolve without Model: %v", err)
	}
	if len(resolved.Diagnostics) != 1 || resolved.Diagnostics[0].Code != "llm-model-required" || resolved.Diagnostics[0].Path != "llm.providers."+provider.ID+".models" {
		t.Fatalf("diagnostics without Model = %#v", resolved.Diagnostics)
	}
}

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

func TestApplicationPreviewShowsConversationDataBinding(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Conversation"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	result, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID:          workflow.ID,
		ExpectedLockVersion: 1,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes": []any{
				map[string]any{"id": "prompt", "definition": "human-chat", "executor": "v1", "displayName": "Prompt", "config": map[string]any{}},
				map[string]any{
					"id": "answer", "definition": "llm-chat", "executor": "v1", "displayName": "Answer", "config": map[string]any{},
					"inputs": map[string]any{"conversation": map[string]any{"from": "prompt.conversation"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("save bound draft: %v", err)
	}

	if len(result.Preview.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Preview.Diagnostics)
	}
	if len(result.Preview.Edges) != 1 {
		t.Fatalf("edges = %#v, want one Data Edge", result.Preview.Edges)
	}
	want := product.PreviewEdge{
		Kind: "data", SourceNodeID: "prompt", SourcePort: "conversation",
		TargetNodeID: "answer", TargetPort: "conversation", ArtifactType: "Conversation",
	}
	if !reflect.DeepEqual(result.Preview.Edges[0], want) {
		t.Fatalf("Data Edge = %#v, want %#v", result.Preview.Edges[0], want)
	}
}

func TestApplicationPreviewAggregatesInputBindingDiagnosticsWithoutHidingGraph(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	if err := registry.RegisterDefinition(nodecatalog.Definition{
		ID: "text-source", DisplayName: "Text source", Kind: nodecatalog.NodeHuman,
		Inputs: map[string]nodecatalog.Port{}, Outputs: map[string]nodecatalog.Port{"text": {Type: "Text"}},
		Config: nodecatalog.ConfigSchema{Fields: []nodecatalog.ConfigField{}},
	}); err != nil {
		t.Fatalf("register text source: %v", err)
	}
	if err := registry.RegisterExecutor(nodecatalog.Executor{DefinitionID: "text-source", Version: "v1"}); err != nil {
		t.Fatalf("register text source executor: %v", err)
	}
	application := product.NewApplication(store, registry)
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Incomplete bindings"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	node := func(id, definition string, inputs map[string]any) map[string]any {
		return map[string]any{
			"id": id, "definition": definition, "executor": "v1", "displayName": id,
			"config": map[string]any{}, "inputs": inputs,
		}
	}
	result, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: 1,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes": []any{
				node("prompt", "human-chat", nil),
				node("text", "text-source", nil),
				node("missing", "llm-chat", nil),
				node("unknown-output", "llm-chat", map[string]any{"conversation": map[string]any{"from": "prompt.missing"}}),
				node("unknown-input", "llm-chat", map[string]any{
					"conversation": map[string]any{"from": "prompt.conversation"},
					"prompt":       map[string]any{"from": "prompt.conversation"},
				}),
				node("incompatible", "llm-chat", map[string]any{"conversation": map[string]any{"from": "text.text"}}),
				node("future", "future-node", map[string]any{"conversation": map[string]any{"from": "prompt.conversation"}}),
			},
		},
	})
	if err != nil {
		t.Fatalf("save incomplete bindings: %v", err)
	}

	wantCodes := []string{"incompatible-input-type", "missing-input-binding", "unknown-input-port", "unknown-node-definition", "unknown-output-port"}
	gotCodes := make([]string, 0, len(result.Preview.Diagnostics))
	for _, diagnostic := range result.Preview.Diagnostics {
		gotCodes = append(gotCodes, diagnostic.Code)
	}
	sort.Strings(gotCodes)
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Fatalf("diagnostic codes = %#v, want %#v; diagnostics = %#v", gotCodes, wantCodes, result.Preview.Diagnostics)
	}
	if len(result.Preview.Nodes) != 7 {
		t.Fatalf("preview nodes = %#v, want all seven recognizable Nodes", result.Preview.Nodes)
	}
	if len(result.Preview.Edges) != 5 {
		t.Fatalf("preview edges = %#v, want every recognizable binding", result.Preview.Edges)
	}
}

func TestApplicationPreviewSeparatesControlDependenciesAndCycleGroups(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := newTestApplication(t, store)
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Review loop"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	result, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: 1,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes": []any{
				map[string]any{
					"id": "prompt", "definition": "human-chat", "executor": "v1", "displayName": "Prompt", "config": map[string]any{},
					"dependsOn": []any{"answer"},
				},
				map[string]any{
					"id": "answer", "definition": "llm-chat", "executor": "v1", "displayName": "Answer", "config": map[string]any{},
					"inputs": map[string]any{"conversation": map[string]any{"from": "prompt.conversation"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("save cyclic draft: %v", err)
	}

	wantEdges := []product.PreviewEdge{
		{Kind: "data", SourceNodeID: "prompt", SourcePort: "conversation", TargetNodeID: "answer", TargetPort: "conversation", ArtifactType: "Conversation"},
		{Kind: "control", SourceNodeID: "answer", TargetNodeID: "prompt"},
	}
	if !reflect.DeepEqual(result.Preview.Edges, wantEdges) {
		t.Fatalf("edges = %#v, want %#v", result.Preview.Edges, wantEdges)
	}
	if !reflect.DeepEqual(result.Preview.Groups, []product.PreviewGroup{{NodeIDs: []string{"answer", "prompt"}}}) {
		t.Fatalf("cycle groups = %#v", result.Preview.Groups)
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
		"nodes[0].inputs.conversation",
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
