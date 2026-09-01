package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/chat"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/secret"
)

// findOpenAIError walks the error chain for the protocol adapter's
// Structural Error, which the Application wraps with context.
func findOpenAIError(err error) *chat.OpenAIError {
	for candidate := err; candidate != nil; candidate = errors.Unwrap(candidate) {
		if target, ok := candidate.(*chat.OpenAIError); ok {
			return target
		}
	}
	return nil
}

// openRealRunApplication returns an application whose Provider points at the
// given fixture server, with one Model slot and the tracer Draft saved.
func openRealRunApplication(t *testing.T, ctx context.Context, server *httptest.Server) (*product.Application, runtimepath.Paths, *history.Store, product.WorkflowView, uint64) {
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
	application := newTestApplicationWithRunsAt(t, store, paths, server)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "sk-real-run-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	return application, paths, store, workflow, saved.Draft.LockVersion
}

func TestApplicationRealRunPersistedHistoryShowsDiagnosticsAfterRestart(t *testing.T) {
	ctx := context.Background()
	server, _ := startFixtureLLMServer(t)
	application, paths, store, workflow, lockVersion := openRealRunApplication(t, ctx, server)

	run, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: lockVersion})
	if err != nil {
		t.Fatalf("start real Run: %v", err)
	}
	if run.Status != "succeeded" {
		t.Fatalf("run status = %q", run.Status)
	}
	agentRun := run.NodeRuns[1]
	if agentRun.Diagnostics["providerRequestId"] != "chatcmpl-fixture-1" || agentRun.Diagnostics["finishReason"] != "stop" {
		t.Fatalf("agent diagnostics = %#v", agentRun.Diagnostics)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// After restart the same Run, Node Run diagnostics and Conversation
	// Artifact stay queryable from SQLite and the filesystem.
	reopened, err := history.Open(ctx, paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	reopenedApp := newTestApplicationWithRunsAt(t, reopened, paths, server)
	detail, err := reopenedApp.GetRunHistory(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run history after restart: %v", err)
	}
	if detail.Status != "succeeded" || len(detail.NodeRuns) != 2 || len(detail.Artifacts) != 2 {
		t.Fatalf("run history after restart = %#v", detail)
	}
	if detail.NodeRuns[1].Diagnostics["providerRequestId"] != "chatcmpl-fixture-1" {
		t.Fatalf("persisted diagnostics after restart = %#v", detail.NodeRuns[1].Diagnostics)
	}
	if detail.Artifacts[1].Messages[1].Text != "Real model response." {
		t.Fatalf("persisted conversation = %#v", detail.Artifacts[1].Messages)
	}

	// The API key must not appear in the database, the Run view or any
	// persisted Artifact file.
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sk-real-run-secret") {
		t.Fatal("run history view contains plaintext API Key")
	}
	database, err := os.ReadFile(paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(database), "sk-real-run-secret") {
		t.Fatal("SQLite database contains plaintext API Key")
	}
	artifactsRoot := paths.ArtifactsDir(run.ID)
	walkErr := filepath.Walk(artifactsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "sk-real-run-secret") {
			t.Fatalf("Artifact %s contains plaintext API Key", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

func TestApplicationRealRunFailsWithoutPartialStateWhenProviderFails(t *testing.T) {
	ctx := context.Background()
	// The fixture rejects the call with a provider error body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream capacity"}}`))
	}))
	t.Cleanup(server.Close)
	application, paths, store, workflow, lockVersion := openRealRunApplication(t, ctx, server)

	_, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: lockVersion})
	if err == nil {
		t.Fatal("start Run against failing provider = nil error, want Structural Error")
	}
	openAIError := findOpenAIError(err)
	if openAIError == nil || openAIError.Kind != chat.ErrProvider {
		t.Fatalf("error = %v, want provider OpenAIError", err)
	}
	if strings.Contains(err.Error(), "sk-real-run-secret") {
		t.Fatalf("error leaks API key: %v", err)
	}

	// No Run, Revision materialization side effects or artifact directories.
	runs, listErr := application.ListRevisions(ctx, workflow.ID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(runs) != 0 {
		t.Fatalf("failed Run created revisions: %#v", runs)
	}
	if entries, readErr := os.ReadDir(paths.RunsDir()); readErr == nil && len(entries) > 0 {
		t.Fatalf("failed Run left run directories: %#v", entries)
	} else if !os.IsNotExist(readErr) {
		// An empty runs root is fine; a populated one is not.
		_ = store
	}
}

func TestApplicationRealRunAuthFailureIsStructuralAndSanitized(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided"}}`))
	}))
	t.Cleanup(server.Close)
	application, _, _, workflow, lockVersion := openRealRunApplication(t, ctx, server)

	_, err := application.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: lockVersion})
	openAIError := findOpenAIError(err)
	if openAIError == nil || openAIError.Kind != chat.ErrAuth {
		t.Fatalf("error = %v, want authentication OpenAIError", err)
	}
	if strings.Contains(err.Error(), "sk-real-run-secret") {
		t.Fatalf("auth error leaks API key: %v", err)
	}
}

func TestApplicationRealRunWithoutSecretResolutionFailsBeforeAnyWrite(t *testing.T) {
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
	// The Provider row persists a reference the Memory Adapter cannot resolve.
	application := newTestApplicationWithRunsAt(t, store, paths, server)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "sk-gone-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	// Wipe the secret so resolution fails before the model call.
	wiped := secret.NewMemoryAdapter()
	wipeApp := product.NewApplication(store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(wiped), product.WithChatAdapter(chat.NewOpenAIChatAdapter(server.Client())))
	_ = wipeApp
	// Simulate missing credential by removing it from the adapter the app uses.
	// Rebuild the application with a second memory adapter that never stored it.
	application2 := product.NewApplication(store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(chat.NewOpenAIChatAdapter(server.Client())))
	_, err = application2.StartRun(ctx, product.StartRunInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion})
	if err == nil {
		t.Fatal("start Run with unresolvable secret = nil error")
	}
	if strings.Contains(err.Error(), "sk-gone-secret") {
		t.Fatalf("secret error leaks value: %v", err)
	}
	if _, statErr := os.Stat(paths.RunsDir()); statErr == nil {
		entries, readErr := os.ReadDir(paths.RunsDir())
		if readErr == nil && len(entries) > 0 {
			t.Fatalf("failed Run left run directories: %#v", entries)
		}
	}
}
