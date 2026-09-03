package product_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
)

// generateFailedRunWithCanary returns an application, its runtime paths and a
// failed Run ID that used the canary API key, so leak tests can scan every
// sink: database, logs, artifacts and bundle.
func generateFailedRunWithCanary(t *testing.T, canary string) (*product.Application, runtimepath.Paths, string) {
	t.Helper()
	ctx := context.Background()
	// The fixture echoes the canary inside the provider error message, which
	// is exactly the path redaction must survive.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"credential ` + canary + ` was rejected"}}`))
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
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: canary})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	_, err = application.StartRun(ctx, singleTurnInput(workflow.ID, saved.Draft.LockVersion))
	if err == nil {
		t.Fatal("canary run unexpectedly succeeded")
	}
	revisions, err := application.ListRevisions(ctx, workflow.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("canary revisions = %#v, error = %v", revisions, err)
	}
	runs, err := application.ListRevisionRuns(ctx, revisions[0].ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("canary runs = %#v, error = %v", runs, err)
	}
	return application, paths, runs[0].ID
}

// assertNoCanary walks one directory tree and fails when the canary appears
// in any file.
func assertNoCanary(t *testing.T, canary, root, label string) {
	t.Helper()
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), canary) {
			t.Fatalf("%s %s contains the canary secret", label, path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

func TestDiagnosticsBundleContainsIdentityAndSanitizedLogWithoutBodies(t *testing.T) {
	ctx := context.Background()
	const canary = "gum-secret-canary-bundle-3312"
	application, paths, runID := generateFailedRunWithCanary(t, canary)

	bundle, err := application.GenerateDiagnosticsBundle(ctx, runID)
	if err != nil {
		t.Fatalf("generate diagnostics bundle: %v", err)
	}
	if bundle.RunID != runID || bundle.Path == "" || bundle.SchemaVersion != "productDiagnosticsBundle/v1" || bundle.AppVersion == "" {
		t.Fatalf("bundle view = %#v", bundle)
	}

	manifestData, err := os.ReadFile(filepath.Join(bundle.Path, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SchemaVersion     string `json:"schemaVersion"`
		AppVersion        string `json:"appVersion"`
		ProductSchemaHint string `json:"productSchemaVersion"`
		Run               struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			NodeRuns []struct {
				NodeRunID   string            `json:"nodeRunId"`
				NodeID      string            `json:"nodeId"`
				Status      string            `json:"status"`
				LatencyMs   int64             `json:"latencyMs"`
				Inputs      map[string]string `json:"inputs"`
				Outputs     map[string]string `json:"outputs"`
				Diagnostics *struct {
					Error *struct {
						Kind string `json:"kind"`
						Code string `json:"code"`
					} `json:"error"`
				} `json:"diagnostics"`
			} `json:"nodeRuns"`
		} `json:"run"`
		Includes []struct {
			Name string `json:"name"`
		} `json:"includes"`
		Excluded []string `json:"excluded"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode manifest: %v\n%s", err, manifestData)
	}
	if manifest.SchemaVersion != "productDiagnosticsBundle/v1" || manifest.AppVersion == "" || manifest.ProductSchemaHint == "" {
		t.Fatalf("manifest header = %#v", manifest)
	}
	if manifest.Run.ID != runID || manifest.Run.Status != "failed" {
		t.Fatalf("manifest run summary = %#v", manifest.Run)
	}
	if len(manifest.Run.NodeRuns) != 2 {
		t.Fatalf("manifest node runs = %#v", manifest.Run.NodeRuns)
	}
	agent := manifest.Run.NodeRuns[1]
	if agent.NodeID != "answer" || agent.Status != "failed" || agent.Diagnostics == nil || agent.Diagnostics.Error == nil || agent.Diagnostics.Error.Code != "authentication" {
		t.Fatalf("agent node run summary = %#v", agent)
	}
	if agent.LatencyMs < 0 || len(agent.Inputs) != 1 || len(agent.Outputs) != 0 {
		t.Fatalf("agent node run telemetry = %#v", agent)
	}
	if manifest.Run.NodeRuns[0].NodeID != "prompt" || manifest.Run.NodeRuns[0].Status != "succeeded" {
		t.Fatalf("human node run summary = %#v", manifest.Run.NodeRuns[0])
	}

	// The run log copy exists and carries identity, phase and latency lines.
	logData, err := os.ReadFile(filepath.Join(bundle.Path, "run.log"))
	if err != nil {
		t.Fatalf("read bundle run log: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, `"runId":"`+runID+`"`) {
		t.Fatalf("bundle run log has no run identity:\n%s", logText)
	}
	for _, phase := range []string{"node run started", "node run finished", "run-failed"} {
		if !strings.Contains(logText, phase) {
			t.Fatalf("bundle run log missing phase %q:\n%s", phase, logText)
		}
	}
	if !strings.Contains(logText, "latencyMs") {
		t.Fatalf("bundle run log has no latency:\n%s", logText)
	}
	if strings.Contains(logText, canary) {
		t.Fatalf("bundle run log contains the canary secret:\n%s", logText)
	}
	// The same boundary holds for the live run log below the Local Data Root.
	assertNoCanary(t, canary, paths.RunDir(runID), "run directory")

	// The bundle declares its content boundary.
	names := make([]string, 0, len(manifest.Includes))
	for _, item := range manifest.Includes {
		names = append(names, item.Name)
	}
	if len(names) != 2 || names[0] != "manifest.json" || names[1] != "run.log" {
		t.Fatalf("bundle includes = %#v", names)
	}
	joined := strings.Join(manifest.Excluded, ",")
	for _, forbidden := range []string{"prompt", "conversation", "api key"} {
		if !strings.Contains(joined, forbidden) {
			t.Fatalf("bundle exclusions do not mention %q: %v", forbidden, manifest.Excluded)
		}
	}
}

func TestDiagnosticsBundleExcludesPromptsConversationsAndSecrets(t *testing.T) {
	ctx := context.Background()
	const canary = "gum-secret-canary-bodies-7741"
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
	provider, err := application.CreateLLMProvider(ctx, product.CreateLLMProviderInput{Name: "Primary", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: canary})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateLLMModel(ctx, product.CreateLLMModelInput{ProviderID: provider.ID, DisplayName: "Fixture", ProviderModelID: "fixture-model"}); err != nil {
		t.Fatal(err)
	}
	workflow, saved := saveTracerDraft(t, ctx, application)
	input := singleTurnInput(workflow.ID, saved.Draft.LockVersion)
	input.HumanInput.Text = "secret-free prompt body marker qzx-381"
	run, err := application.StartRun(ctx, input)
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	bundle, err := application.GenerateDiagnosticsBundle(ctx, run.ID)
	if err != nil {
		t.Fatalf("generate diagnostics bundle: %v", err)
	}

	walkErr := filepath.Walk(bundle.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(data)
		if strings.Contains(text, input.HumanInput.Text) {
			t.Fatalf("bundle file %s contains the prompt body", path)
		}
		if strings.Contains(text, "Real model response.") {
			t.Fatalf("bundle file %s contains the conversation body", path)
		}
		if strings.Contains(text, canary) {
			t.Fatalf("bundle file %s contains the API key", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

func TestDiagnosticsBundleRejectsUnknownRun(t *testing.T) {
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
	if _, err := application.GenerateDiagnosticsBundle(ctx, "00000000-0000-4000-8000-000000000000"); err == nil {
		t.Fatal("unknown run bundle generation = nil error")
	}
}
