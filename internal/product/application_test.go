package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Jayj1997/gum-workflows/internal/chat"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/secret"
)

func TestApplicationStoresProviderAPIKeyOutsideSQLiteAndReturnsNoPlaintext(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "product.db")
	store, err := history.Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	secrets := secret.NewMemoryAdapter()
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	application := product.NewApplication(store, registry, product.WithSecretAdapter(secrets))

	created, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "sk-super-secret",
	})
	if err != nil {
		t.Fatalf("create Provider: %v", err)
	}
	if !created.HasAPIKey {
		t.Fatal("created Provider does not report a configured API Key")
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("encode Provider view: %v", err)
	}
	if strings.Contains(string(encoded), "sk-super-secret") {
		t.Fatalf("Provider ViewModel contains plaintext API Key: %s", encoded)
	}
	database, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read SQLite database: %v", err)
	}
	if strings.Contains(string(database), "sk-super-secret") {
		t.Fatal("SQLite database contains plaintext API Key")
	}
}

func TestApplicationDeletesProviderCredentialOnlyAfterConfirmation(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	secrets := secret.NewMemoryAdapter()
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	application := product.NewApplication(store, registry, product.WithSecretAdapter(secrets))
	created, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "sk-delete-me",
	})
	if err != nil {
		t.Fatalf("create Provider: %v", err)
	}
	settings, err := store.GetLLMSettings(ctx)
	if err != nil {
		t.Fatalf("get persisted Provider: %v", err)
	}
	reference := settings.Providers[0].APIKeyRef

	if err := application.DeleteLLMProvider(ctx, product.DeleteLLMProviderInput{ProviderID: created.ID}); err == nil {
		t.Fatal("delete Provider succeeded without confirmation")
	}
	if _, err := secrets.Resolve(ctx, reference); err != nil {
		t.Fatalf("unconfirmed delete removed credential: %v", err)
	}
	if err := application.DeleteLLMProvider(ctx, product.DeleteLLMProviderInput{ProviderID: created.ID, Confirmed: true}); err != nil {
		t.Fatalf("delete confirmed Provider: %v", err)
	}
	if _, err := secrets.Resolve(ctx, reference); err == nil {
		t.Fatal("confirmed Provider delete left credential in Secret Adapter")
	}
}

func TestApplicationUpdatesProviderWithoutReturningOrPersistingAPIKey(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	secrets := secret.NewMemoryAdapter()
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	application := product.NewApplication(store, registry, product.WithSecretAdapter(secrets))
	created, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "sk-original",
	})
	if err != nil {
		t.Fatalf("create Provider: %v", err)
	}
	settings, err := store.GetLLMSettings(ctx)
	if err != nil {
		t.Fatalf("get persisted Provider: %v", err)
	}
	reference := settings.Providers[0].APIKeyRef

	if _, err := application.UpdateLLMProvider(ctx, product.UpdateLLMProviderInput{
		ID: created.ID, Name: "Renamed", Protocol: created.Protocol, BaseURL: created.BaseURL,
	}); err != nil {
		t.Fatalf("update Provider without rotating Key: %v", err)
	}
	if value, err := secrets.Resolve(ctx, reference); err != nil || value != "sk-original" {
		t.Fatalf("preserved API Key = %q, %v", value, err)
	}
	updated, err := application.UpdateLLMProvider(ctx, product.UpdateLLMProviderInput{
		ID: created.ID, Name: "Renamed", Protocol: created.Protocol, BaseURL: created.BaseURL, APIKey: "sk-rotated",
	})
	if err != nil {
		t.Fatalf("rotate Provider Key: %v", err)
	}
	if value, err := secrets.Resolve(ctx, reference); err != nil || value != "sk-rotated" {
		t.Fatalf("rotated API Key = %q, %v", value, err)
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("encode updated Provider: %v", err)
	}
	if strings.Contains(string(encoded), "sk-original") || strings.Contains(string(encoded), "sk-rotated") {
		t.Fatalf("updated Provider ViewModel contains plaintext API Key: %s", encoded)
	}
}

func TestApplicationFailsWhenSecretAdapterIsUnavailable(t *testing.T) {
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
	application := product.NewApplication(store, registry)
	_, err = application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "sk-must-not-leak",
	})
	if err == nil || !strings.Contains(err.Error(), "secret adapter is not configured") {
		t.Fatalf("create Provider error = %v", err)
	}
	if strings.Contains(err.Error(), "sk-must-not-leak") {
		t.Fatalf("create Provider error contains API Key: %v", err)
	}
	settings, err := store.GetLLMSettings(ctx)
	if err != nil {
		t.Fatalf("get settings after failed create: %v", err)
	}
	if len(settings.Providers) != 0 {
		t.Fatalf("failed create persisted Providers: %#v", settings.Providers)
	}
}

func TestApplicationStartsRealConversationRunFromVisibleDraft(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatalf("create runtime paths: %v", err)
	}
	store, err := history.Open(ctx, paths.Database())
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	server, requests := startFixtureLLMServer(t)
	application := product.NewApplication(store, registry, product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(chat.NewOpenAIChatAdapter(server.Client())))

	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("create Provider: %v", err)
	}
	model, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{
		ProviderID: provider.ID, DisplayName: "Fixture model", ProviderModelID: "fake-model",
		GenerationDefaults: productworkflow.GenerationDefaults{Temperature: float64Pointer(0.2), MaxOutputTokens: intPointer(32)},
	})
	if err != nil {
		t.Fatalf("create Model: %v", err)
	}
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Conversation"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	saved, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: 1,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"project":               map[string]any{"repository": "/workspace/example"},
			"nodes": []any{
				map[string]any{"id": "prompt", "definition": "human-chat", "executor": "v1", "displayName": "Prompt", "config": map[string]any{}},
				map[string]any{
					"id": "answer", "definition": "llm-chat", "executor": "v1", "displayName": "Answer", "config": map[string]any{"temperature": 0.8, "max_output_tokens": 64, "instructions": "Answer tersely."},
					"inputs": map[string]any{"conversation": map[string]any{"from": "prompt.conversation"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("save tracer Draft: %v", err)
	}

	run, err := application.StartRun(ctx, product.StartRunInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion,
	})
	if err != nil {
		t.Fatalf("start real Run: %v", err)
	}

	if run.Status != "succeeded" || run.ID == "" || run.RevisionID == "" {
		t.Fatalf("Run = %#v, want successful identities", run)
	}
	if run.Draft.LockVersion != saved.Draft.LockVersion+1 {
		t.Fatalf("materialized lock version = %d, want %d", run.Draft.LockVersion, saved.Draft.LockVersion+1)
	}
	nodes := run.Draft.Content["nodes"].([]any)
	preference := nodes[1].(map[string]any)["llm"].(map[string]any)
	if preference["modelUuid"] != model.ID {
		t.Fatalf("materialized Model UUID = %#v, want %q", preference["modelUuid"], model.ID)
	}
	if len(run.NodeRuns) != 2 || run.NodeRuns[0].Status != "succeeded" || run.NodeRuns[1].Status != "succeeded" {
		t.Fatalf("Node Runs = %#v, want two successful rounds", run.NodeRuns)
	}
	// The real call's usage, finish reason and Provider request ID are visible
	// on the agent Node Run.
	agentRun := run.NodeRuns[1]
	if agentRun.Diagnostics["providerRequestId"] != "chatcmpl-fixture-1" || agentRun.Diagnostics["finishReason"] != "stop" {
		t.Fatalf("agent Node Run diagnostics = %#v", agentRun.Diagnostics)
	}
	usage, ok := agentRun.Diagnostics["usage"].(*chat.Usage)
	if !ok {
		t.Fatalf("agent Node Run usage = %#v", agentRun.Diagnostics["usage"])
	}
	if usage.InputTokens != 12 || usage.OutputTokens != 7 || usage.TotalTokens != 19 {
		t.Fatalf("agent Node Run usage = %#v", usage)
	}
	if len(run.Snapshot.Executors) != 2 || run.Snapshot.Executors[1].NodeID != "answer" || run.Snapshot.Executors[1].Version != "v1" {
		t.Fatalf("Run Snapshot Executors = %#v", run.Snapshot.Executors)
	}
	if len(run.Snapshot.LLMSelections) != 1 || run.Snapshot.LLMSelections[0].Temperature == nil || *run.Snapshot.LLMSelections[0].Temperature != 0.8 || run.Snapshot.LLMSelections[0].MaxOutputTokens == nil || *run.Snapshot.LLMSelections[0].MaxOutputTokens != 64 {
		t.Fatalf("Run Snapshot LLM selections = %#v, want effective Node config", run.Snapshot.LLMSelections)
	}
	if run.Snapshot.Project["repository"] != "/workspace/example" {
		t.Fatalf("Run Snapshot Project = %#v", run.Snapshot.Project)
	}
	if len(run.Artifacts) != 2 {
		t.Fatalf("Artifacts = %#v, want source and assistant Conversation versions", run.Artifacts)
	}
	conversation := run.Artifacts[1]
	if conversation.Type != "Conversation" || len(conversation.Messages) != 2 || conversation.Messages[0].Role != "user" || conversation.Messages[1].Role != "assistant" || conversation.Messages[1].Text != "Real model response." {
		t.Fatalf("Conversation Artifact = %#v", conversation)
	}
	if _, err := os.Stat(filepath.Join(paths.ArtifactsDir(run.ID), conversation.URI)); err != nil {
		t.Fatalf("persisted Conversation Artifact: %v", err)
	}

	// The fixture saw exactly one authenticated call with the mapped single-turn
	// body: developer instructions, one user message, the Provider Model ID and
	// the effective generation parameters.
	if len(*requests) != 1 {
		t.Fatalf("fixture requests = %d, want one", len(*requests))
	}
	got := (*requests)[0]
	if got.Auth != "Bearer test-api-key" {
		t.Fatalf("fixture authorization = %q", got.Auth)
	}
	messages := got.Body["messages"].([]any)
	wantMessages := []any{
		map[string]any{"role": "developer", "content": "Answer tersely."},
		map[string]any{"role": "user", "content": "Hello from the product UI."},
	}
	if fmt.Sprint(messages) != fmt.Sprint(wantMessages) {
		t.Fatalf("fixture messages = %#v, want %#v", messages, wantMessages)
	}
	if got.Body["model"] != "fake-model" || got.Body["temperature"] != 0.8 || got.Body["max_tokens"] != float64(64) {
		t.Fatalf("fixture model/params = %#v", got.Body)
	}
}

func TestApplicationRealRunConsumesTheAuthoredConversationDataEdge(t *testing.T) {
	ctx := context.Background()
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
	server, _ := startFixtureLLMServer(t)
	application := newTestApplicationWithRunsAt(t, store, paths, server)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "test-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fake"}); err != nil {
		t.Fatal(err)
	}
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Conversation"})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := application.UpdateDraft(ctx, product.UpdateDraftInput{WorkflowID: workflow.ID, ExpectedLockVersion: 1, Content: map[string]any{
		"semanticSchemaVersion": "productWorkflow/v1",
		"nodes": []any{
			map[string]any{"id": "unused", "definition": "human-chat", "executor": "v1", "displayName": "Unused", "config": map[string]any{}},
			map[string]any{"id": "prompt", "definition": "human-chat", "executor": "v1", "displayName": "Prompt", "config": map[string]any{}},
			map[string]any{"id": "answer", "definition": "llm-chat", "executor": "v1", "displayName": "Answer", "config": map[string]any{}, "inputs": map[string]any{"conversation": map[string]any{"from": "prompt.conversation"}}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if run.NodeRuns[0].NodeID != "prompt" || run.Artifacts[0].NodeID != "prompt" {
		t.Fatalf("real source = Node Run %q Artifact %q, want authored source prompt", run.NodeRuns[0].NodeID, run.Artifacts[0].NodeID)
	}
}

func float64Pointer(value float64) *float64 { return &value }
func intPointer(value int) *int             { return &value }

func TestApplicationStartRunRejectsStaleDraftWithoutMaterializingModel(t *testing.T) {
	ctx := context.Background()
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
	workflow, draft := saveTracerDraft(t, ctx, application)
	newerContent := cloneMap(t, draft.Draft.Content)
	newerContent["project"] = map[string]any{"repository": "/workspace/newer"}
	newer, err := application.UpdateDraft(ctx, product.UpdateDraftInput{WorkflowID: workflow.ID, ExpectedLockVersion: draft.Draft.LockVersion, Content: newerContent})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: draft.Draft.LockVersion}); err == nil {
		t.Fatal("start stale Draft = nil error, want conflict")
	}
	latest, err := application.GetDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.LockVersion != newer.Draft.LockVersion {
		t.Fatalf("Draft lock version = %d, want %d", latest.LockVersion, newer.Draft.LockVersion)
	}
	nodes := latest.Content["nodes"].([]any)
	if _, materialized := nodes[1].(map[string]any)["llm"]; materialized {
		t.Fatalf("stale StartRun materialized Model UUID: %#v", nodes[1])
	}
	if _, err := os.Stat(paths.RunsDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale StartRun runs directory error = %v, want not exist", err)
	}
}

func TestApplicationStartRunWithoutDefaultLeavesDraftAndRunsUntouched(t *testing.T) {
	ctx := context.Background()
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
	workflow, saved := saveTracerDraft(t, ctx, application)

	if _, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion}); err == nil {
		t.Fatal("start Run without default = nil error, want settings diagnostic")
	}
	latest, err := application.GetDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.LockVersion != saved.Draft.LockVersion || !reflect.DeepEqual(latest.Content, saved.Draft.Content) {
		t.Fatalf("failed StartRun changed Draft = %#v, want %#v", latest, saved.Draft)
	}
	if _, err := os.Stat(paths.RunsDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed StartRun runs directory error = %v, want not exist", err)
	}
}

func TestApplicationRepeatedStartRunReusesRevisionAndCreatesNewRun(t *testing.T) {
	ctx := context.Background()
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
	server, _ := startFixtureLLMServer(t)
	application := newTestApplicationWithRunsAt(t, store, paths, server)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "test-api-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fake"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	first, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: first.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if second.RevisionID != first.RevisionID {
		t.Fatalf("Revision IDs = %q and %q, want reuse", first.RevisionID, second.RevisionID)
	}
	if second.ID == first.ID {
		t.Fatalf("Run ID = %q reused", second.ID)
	}
	if second.Draft.LockVersion != first.Draft.LockVersion {
		t.Fatalf("second Run changed Draft lock version from %d to %d", first.Draft.LockVersion, second.Draft.LockVersion)
	}
	for _, runID := range []string{first.ID, second.ID} {
		if _, err := os.Stat(paths.ArtifactsDir(runID)); err != nil {
			t.Fatalf("Run %s Artifact directory: %v", runID, err)
		}
	}
}

func TestApplicationArtifactFailureDoesNotMaterializeDraft(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runsPath := filepath.Join(root, "runs-is-a-file")
	if err := os.WriteFile(runsPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), runsPath)
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
	workflow, saved := saveTracerDraft(t, ctx, application)
	if _, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion}); err == nil {
		t.Fatal("start Run with invalid Artifact root = nil error")
	}
	latest, err := application.GetDraft(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.LockVersion != saved.Draft.LockVersion || !reflect.DeepEqual(latest.Content, saved.Draft.Content) {
		t.Fatalf("Artifact failure changed Draft = %#v, want %#v", latest, saved.Draft)
	}
}

func newTestApplicationWithRuns(t *testing.T, store *history.Store, paths runtimepath.Paths) *product.Application {
	t.Helper()
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	return product.NewApplication(store, registry, product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()))
}

// newTestApplicationWithRunsAt additionally points the chat protocol Adapter
// at a local fixture server so StartRun exercises a real HTTP call.
func newTestApplicationWithRunsAt(t *testing.T, store *history.Store, paths runtimepath.Paths, server *httptest.Server) *product.Application {
	t.Helper()
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load product Node Catalog: %v", err)
	}
	return product.NewApplication(store, registry, product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(chat.NewOpenAIChatAdapter(server.Client())))
}

func saveTracerDraft(t *testing.T, ctx context.Context, application *product.Application) (product.WorkflowView, product.DraftUpdateView) {
	t.Helper()
	workflow, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Conversation"})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	saved, err := application.UpdateDraft(ctx, product.UpdateDraftInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: 1,
		Content: map[string]any{
			"semanticSchemaVersion": "productWorkflow/v1",
			"nodes": []any{
				map[string]any{"id": "prompt", "definition": "human-chat", "executor": "v1", "displayName": "Prompt", "config": map[string]any{}},
				map[string]any{"id": "answer", "definition": "llm-chat", "executor": "v1", "displayName": "Answer", "config": map[string]any{}, "inputs": map[string]any{"conversation": map[string]any{"from": "prompt.conversation"}}},
			},
		},
	})
	if err != nil {
		t.Fatalf("save tracer Draft: %v", err)
	}
	return workflow, saved
}

func cloneMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestApplicationManagesProviderModelSettingsAndResolvesDefaults(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "product.db")
	store, err := history.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	application := newTestApplication(t, store)

	first, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://primary.example/v1", APIKey: "primary-secret",
	})
	if err != nil {
		t.Fatalf("create first Provider: %v", err)
	}
	if !first.EffectiveDefault || first.ExplicitDefault {
		t.Fatalf("first Provider defaults = effective %t explicit %t", first.EffectiveDefault, first.ExplicitDefault)
	}
	second, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "Secondary", Protocol: "openai-chat-completions", BaseURL: "https://secondary.example/v1", APIKey: "secondary-secret",
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
		ID: first.ID, Name: "Primary renamed", Protocol: "openai-chat-completions", BaseURL: "https://new.example/v1", APIKey: "rotated-secret",
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

	if err := application.DeleteLLMProvider(ctx, product.DeleteLLMProviderInput{ProviderID: second.ID, Confirmed: true}); err != nil {
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
		Name: "Unsafe", Protocol: "openai-chat-completions", BaseURL: "https://api.example/v1",
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
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://api.example/v1", APIKey: "primary-secret",
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
	return product.NewApplication(store, registry, product.WithSecretAdapter(secret.NewMemoryAdapter()))
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
