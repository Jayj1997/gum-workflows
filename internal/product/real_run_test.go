package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type failingChatAdapter struct{ err error }

func (a failingChatAdapter) Generate(context.Context, chat.Connection, chat.GenerateRequest) (chat.GenerateResult, error) {
	return chat.GenerateResult{}, a.err
}

type blockingChatAdapter struct {
	entered chan struct{}
	release chan struct{}
}

func (a blockingChatAdapter) Generate(context.Context, chat.Connection, chat.GenerateRequest) (chat.GenerateResult, error) {
	close(a.entered)
	<-a.release
	return chat.GenerateResult{Assistant: chat.AssistantTextMessage("Completed after release."), FinishReason: "stop", ProviderRequestID: "chatcmpl-blocked"}, nil
}

type cancelingChatAdapter struct{ entered chan struct{} }

func (a cancelingChatAdapter) Generate(ctx context.Context, _ chat.Connection, _ chat.GenerateRequest) (chat.GenerateResult, error) {
	close(a.entered)
	<-ctx.Done()
	return chat.GenerateResult{}, ctx.Err()
}

type successfulCancelingChatAdapter struct{ cancel context.CancelFunc }

func (a successfulCancelingChatAdapter) Generate(context.Context, chat.Connection, chat.GenerateRequest) (chat.GenerateResult, error) {
	a.cancel()
	return chat.GenerateResult{Assistant: chat.AssistantTextMessage("Provider result is known."), FinishReason: "stop", ProviderRequestID: "chatcmpl-known-result"}, nil
}

type finalArtifactWriteFailingChatAdapter struct {
	runsDir       string
	artifactsDir  string
	artifactsSave string
}

func (a *finalArtifactWriteFailingChatAdapter) Generate(context.Context, chat.Connection, chat.GenerateRequest) (chat.GenerateResult, error) {
	matches, err := filepath.Glob(filepath.Join(a.runsDir, "*", "artifacts"))
	if err != nil || len(matches) != 1 {
		return chat.GenerateResult{}, fmt.Errorf("locate in-flight Artifact directory: matches=%v error=%w", matches, err)
	}
	a.artifactsDir = matches[0]
	a.artifactsSave = matches[0] + ".saved"
	if err := os.Rename(a.artifactsDir, a.artifactsSave); err != nil {
		return chat.GenerateResult{}, fmt.Errorf("move Artifact directory: %w", err)
	}
	if err := os.WriteFile(a.artifactsDir, []byte("block final Artifact write"), 0o600); err != nil {
		return chat.GenerateResult{}, fmt.Errorf("block Artifact directory: %w", err)
	}
	return chat.GenerateResult{Assistant: chat.AssistantTextMessage("Provider completed."), FinishReason: "stop", ProviderRequestID: "chatcmpl-before-artifact-failure"}, nil
}

func (a *finalArtifactWriteFailingChatAdapter) restore(t *testing.T) {
	t.Helper()
	if a.artifactsDir == "" {
		return
	}
	if err := os.Remove(a.artifactsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(a.artifactsSave, a.artifactsDir); err != nil {
		t.Fatal(err)
	}
}

// findOpenAIError returns the protocol adapter's Structural Error from the
// Application-wrapped chain, if any.
func findOpenAIError(err error) *chat.OpenAIError {
	var openAIError *chat.OpenAIError
	if errors.As(err, &openAIError) {
		return openAIError
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

	run, err := application.StartRun(ctx, singleTurnInput(workflow.ID, lockVersion))
	if err != nil {
		t.Fatalf("start real Run: %v", err)
	}
	if run.Status != "succeeded" {
		t.Fatalf("run status = %q", run.Status)
	}
	agentRun := run.NodeRuns[1]
	if agentRun.Diagnostics == nil || agentRun.Diagnostics.ProviderRequestID != "chatcmpl-fixture-1" || agentRun.Diagnostics.FinishReason != "stop" {
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
	if detail.NodeRuns[1].Diagnostics == nil || detail.NodeRuns[1].Diagnostics.ProviderRequestID != "chatcmpl-fixture-1" {
		t.Fatalf("persisted diagnostics after restart = %#v", detail.NodeRuns[1].Diagnostics)
	}
	if len(detail.NodeRuns[0].Outputs) != 1 || len(detail.NodeRuns[1].Inputs) != 1 || len(detail.NodeRuns[1].Outputs) != 1 {
		t.Fatalf("persisted Node Run inputs/outputs = %#v", detail.NodeRuns)
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

func TestApplicationUsesTheProviderDialectForInstructions(t *testing.T) {
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
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{
		Name: "System dialect", Protocol: "openai-chat-completions", Dialect: "system",
		BaseURL: server.URL + "/v1", APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	content := cloneMap(t, saved.Draft.Content)
	nodes := content["nodes"].([]any)
	nodes[1].(map[string]any)["config"] = map[string]any{"instructions": "Answer tersely."}
	saved, err = application.UpdateDraft(ctx, product.UpdateDraftInput{WorkflowID: workflow.ID, ExpectedLockVersion: saved.Draft.LockVersion, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 {
		t.Fatalf("fixture requests = %d, want one", len(*requests))
	}
	messages := (*requests)[0].Body["messages"].([]any)
	if got := messages[0].(map[string]any)["role"]; got != "system" {
		t.Fatalf("instructions role = %q, want system", got)
	}
	if len(run.Snapshot.LLMSelections) != 1 || run.Snapshot.LLMSelections[0].Dialect != "system" {
		t.Fatalf("Run Snapshot LLM selection = %#v, want system dialect", run.Snapshot.LLMSelections)
	}
}

func TestApplicationRunsTheAuthoredHumanSourceWithSubmittedText(t *testing.T) {
	ctx := context.Background()
	server, requests := startFixtureLLMServer(t)
	application, _, _, workflow, lockVersion := openRealRunApplication(t, ctx, server)
	run, err := application.StartRun(ctx, product.StartRunInput{
		WorkflowID: workflow.ID, ExpectedLockVersion: lockVersion,
		HumanInput: product.HumanRunInput{NodeID: "prompt", Text: "  Explain the application seam.\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	messages := (*requests)[0].Body["messages"].([]any)
	if got := messages[len(messages)-1].(map[string]any)["content"]; got != "  Explain the application seam.\n" {
		t.Fatalf("submitted user message = %q", got)
	}
	if got := run.Artifacts[0].Messages[0]; got.Role != "user" || got.Text != "  Explain the application seam.\n" {
		t.Fatalf("source Conversation = %#v", run.Artifacts[0].Messages)
	}
}

func TestApplicationPersistsStructuralFailureWhenProviderFails(t *testing.T) {
	ctx := context.Background()
	// The fixture rejects the call with a provider error body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream capacity"}}`))
	}))
	t.Cleanup(server.Close)
	application, _, _, workflow, lockVersion := openRealRunApplication(t, ctx, server)

	_, err := application.StartRun(ctx, singleTurnInput(workflow.ID, lockVersion))
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
	if !strings.Contains(err.Error(), "start a new Run") {
		t.Fatalf("error has no user action: %v", err)
	}

	// The real execution boundary was crossed, so the failed attempt remains
	// traceable as one Failed Run instead of disappearing from history.
	revisions, listErr := application.ListRevisions(ctx, workflow.ID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(revisions) != 1 {
		t.Fatalf("failed Run revisions = %#v, want one", revisions)
	}
	runs, listErr := application.ListRevisionRuns(ctx, revisions[0].ID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("failed Run history = %#v, want one failed Run", runs)
	}
	detail, historyErr := application.GetRunHistory(ctx, runs[0].ID)
	if historyErr != nil {
		t.Fatal(historyErr)
	}
	if detail.Status != "failed" || len(detail.NodeRuns) != 2 || detail.NodeRuns[0].Status != "succeeded" || detail.NodeRuns[1].Status != "failed" {
		t.Fatalf("failed Run detail = %#v", detail)
	}
	if detail.Error == nil || detail.Error.Kind != "structural" || detail.Error.Code != "provider" || !strings.Contains(detail.Error.Message, "upstream capacity") || detail.Error.UserAction == "" {
		t.Fatalf("failed Run error = %#v", detail.Error)
	}
	agentRun := detail.NodeRuns[1]
	if agentRun.Diagnostics == nil || agentRun.Diagnostics.Error == nil || agentRun.Diagnostics.Error.Code != "provider" || agentRun.StartedAt.IsZero() || agentRun.FinishedAt.IsZero() {
		t.Fatalf("failed agent Node Run = %#v", agentRun)
	}
}

func TestApplicationFinalArtifactWriteFailureTerminatesTheAgentNodeRun(t *testing.T) {
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
	adapter := &finalArtifactWriteFailingChatAdapter{runsDir: paths.RunsDir()}
	application := product.NewApplication(store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(adapter))
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "opaque-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)

	_, runErr := application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if runErr == nil || !strings.Contains(runErr.Error(), "write assistant Conversation Artifact") {
		t.Fatalf("StartRun error = %v, want final Artifact write failure", runErr)
	}
	adapter.restore(t)

	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("revisions = %#v, error = %v", revisions, err)
	}
	runs, err := application.ListRevisionRuns(ctx, revisions[0].ID)
	if err != nil || len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("Runs = %#v, error = %v", runs, err)
	}
	detail, err := application.GetRunHistory(ctx, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.NodeRuns) != 2 || detail.NodeRuns[0].Status != "succeeded" || detail.NodeRuns[1].Status != "failed" {
		t.Fatalf("terminal Node Runs = %#v", detail.NodeRuns)
	}
	if detail.NodeRuns[1].Diagnostics == nil || detail.NodeRuns[1].Diagnostics.Error == nil || detail.NodeRuns[1].Diagnostics.Error.Code != "runtime" {
		t.Fatalf("failed agent diagnostics = %#v", detail.NodeRuns[1].Diagnostics)
	}
	if len(detail.Artifacts) != 1 || detail.Artifacts[0].Messages[0].Text != "Hello from the product UI." {
		t.Fatalf("preserved source Artifact = %#v", detail.Artifacts)
	}
}

func TestApplicationPersistsRunningRunBeforeCallingProvider(t *testing.T) {
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
	adapter := blockingChatAdapter{entered: make(chan struct{}), release: make(chan struct{})}
	application := product.NewApplication(store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(adapter))
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "opaque-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	done := make(chan error, 1)
	go func() {
		_, runErr := application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
		done <- runErr
	}()
	<-adapter.entered
	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("in-flight revisions = %#v, want one", revisions)
	}
	runs, err := application.ListRevisionRuns(ctx, revisions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "running" {
		t.Fatalf("in-flight Runs = %#v, want one running Run", runs)
	}
	reopened := newTestApplicationWithRuns(t, store, paths)
	if _, err := reopened.OpenWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	interrupted, err := reopened.GetRunHistory(ctx, runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != "interrupted" || interrupted.Error == nil || interrupted.Error.Kind != "unknown-outcome" || !strings.Contains(interrupted.Error.UserAction, "cannot Resume") {
		t.Fatalf("interrupted Run = %#v", interrupted)
	}
	if len(interrupted.NodeRuns) != 2 || interrupted.NodeRuns[0].NodeID != "prompt" || interrupted.NodeRuns[0].Status != "succeeded" || interrupted.NodeRuns[1].Status != "unknown-outcome" || interrupted.NodeRuns[1].Diagnostics == nil || interrupted.NodeRuns[1].Diagnostics.Error == nil || len(interrupted.Artifacts) != 1 || interrupted.Artifacts[0].Messages[0].Text != "Hello from the product UI." {
		t.Fatalf("interrupted progress = Node Runs %#v Artifacts %#v", interrupted.NodeRuns, interrupted.Artifacts)
	}
	close(adapter.release)
	if err := <-done; err == nil {
		t.Fatal("provider completion overwrote Interrupted Run")
	}
}

func TestApplicationRecordsCanceledProviderCallAsUnknownOutcome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	root := t.TempDir()
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := history.Open(context.Background(), paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	adapter := cancelingChatAdapter{entered: make(chan struct{})}
	application := product.NewApplication(store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(adapter))
	provider, err := application.CreateLLMProvider(context.Background(), product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "opaque-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(context.Background(), product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, context.Background(), application)
	done := make(chan error, 1)
	go func() {
		_, runErr := application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
		done <- runErr
	}()
	<-adapter.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled StartRun error = %v", err)
	}
	revisions, err := application.ListRevisions(context.Background(), workflow.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("canceled revisions = %#v, error = %v", revisions, err)
	}
	runs, err := application.ListRevisionRuns(context.Background(), revisions[0].ID)
	if err != nil || len(runs) != 1 || runs[0].Status != "interrupted" || runs[0].Error == nil || runs[0].Error.Kind != "unknown-outcome" {
		t.Fatalf("canceled Runs = %#v, error = %v", runs, err)
	}
	detail, err := application.GetRunHistory(context.Background(), runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.NodeRuns) != 2 || detail.NodeRuns[1].Status != "unknown-outcome" || len(detail.Artifacts) != 1 {
		t.Fatalf("canceled Run detail = %#v", detail)
	}
}

func TestApplicationFinalizesAKnownProviderResultAfterCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	root := t.TempDir()
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := history.Open(context.Background(), paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := product.NewApplication(store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(successfulCancelingChatAdapter{cancel: cancel}))
	provider, err := application.CreateLLMProvider(context.Background(), product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "opaque-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(context.Background(), product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, context.Background(), application)
	run, err := application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if err != nil {
		t.Fatalf("known Provider result must finish locally after caller cancellation: %v", err)
	}
	if run.Status != "succeeded" || len(run.NodeRuns) != 2 || run.NodeRuns[1].Status != "succeeded" || len(run.Artifacts) != 2 {
		t.Fatalf("finalized Run = %#v", run)
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

	_, err := application.StartRun(ctx, singleTurnInput(workflow.ID, lockVersion))
	openAIError := findOpenAIError(err)
	if openAIError == nil || openAIError.Kind != chat.ErrAuth {
		t.Fatalf("error = %v, want authentication OpenAIError", err)
	}
	if strings.Contains(err.Error(), "sk-real-run-secret") {
		t.Fatalf("auth error leaks API key: %v", err)
	}
}

func TestApplicationRedactsAnArbitraryAPIKeyEchoedByTheProvider(t *testing.T) {
	ctx := context.Background()
	const apiKey = "gum-secret-canary-8291"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"credential %s was rejected"}}`, apiKey)
	}))
	t.Cleanup(server.Close)
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
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: apiKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	_, err = application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	var openAIError *chat.OpenAIError
	if !errors.As(err, &openAIError) || openAIError.Kind != chat.ErrAuth {
		t.Fatalf("error = %v, want authentication OpenAIError", err)
	}
	if strings.Contains(err.Error(), apiKey) || strings.Contains(openAIError.ProviderMessage, apiKey) {
		t.Fatalf("provider error leaked API Key: %v", err)
	}
}

func TestApplicationPreservesProgrammableAdapterErrorIdentity(t *testing.T) {
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
	sentinel := errors.New("fixture transport failure")
	application := product.NewApplication(
		store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()),
		product.WithChatAdapter(failingChatAdapter{err: fmt.Errorf("safe transport context: %w", sentinel)}),
	)
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://example.test/v1", APIKey: "opaque-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	_, err = application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if !errors.Is(err, sentinel) {
		t.Fatalf("error chain lost sentinel: %v", err)
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
	// A second application over the same database uses a fresh Secret Adapter
	// that never stored the Provider credential, so resolution fails before
	// the model call and before any Run state is created.
	withoutCredential := product.NewApplication(store, mustCatalog(t), product.WithRunPaths(paths), product.WithSecretAdapter(secret.NewMemoryAdapter()), product.WithChatAdapter(chat.NewOpenAIChatAdapter(server.Client())))
	_, err = withoutCredential.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if err == nil {
		t.Fatal("start Run with unresolvable secret = nil error")
	}
	if strings.Contains(err.Error(), "sk-gone-secret") {
		t.Fatalf("secret error leaks value: %v", err)
	}
	if entries, readErr := os.ReadDir(paths.RunsDir()); readErr == nil && len(entries) > 0 {
		t.Fatalf("failed Run left run directories: %#v", entries)
	}
}
