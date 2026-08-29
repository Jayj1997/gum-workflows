package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/google/uuid"
)

// validFixtureEnv 满足语义校验的运行前提（票 06 起 validate 全链生效）：
// 临时 XDG_CONFIG_HOME 提供 llm.yaml（agent 节点的默认链解析），
// fixture 的相对 projects 路径已就位于 testdata/examples/。
// 不触真实 $HOME（DEVELOPMENT.md §6）。
func validFixtureEnv(t *testing.T) {
	t.Helper()

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv(runtimepath.DataRootEnv, filepath.Join(t.TempDir(), "local-data"))
	dir := filepath.Join(xdg, "gum-workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir xdg config: %v", err)
	}
	llmYAML := `
apiVersion: llm/v1
kind: llm
providers:
  - name: openai
    type: openai-compatible
    url: https://api.openai.com/v1
    apikey: plain-test-key
    default: true
    models:
      - name: gpt-4o
        default: true
      - name: gpt-4o-mini
`
	if err := os.WriteFile(filepath.Join(dir, "llm.yaml"), []byte(llmYAML), 0o644); err != nil {
		t.Fatalf("write llm.yaml: %v", err)
	}
}

func TestRunUsage(t *testing.T) {
	t.Setenv(runtimepath.DataRootEnv, filepath.Join(t.TempDir(), "local-data"))
	for _, args := range [][]string{{}, {"validate"}, {"run", "x.yaml"}, {"validate", "a", "b"}} {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) = nil error, want usage error", args)
		}
	}
	if err := run([]string{"bogus", "x.yaml"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run(bogus) error = %v, want unknown command", err)
	}
}

func TestValidateCmd(t *testing.T) {
	validFixtureEnv(t)
	resolve := testRuntimePaths(t)

	t.Run("valid workflow", func(t *testing.T) {
		err := validateCmd(filepath.Join("..", "..", "internal", "workflow", "testdata", "valid.yaml"), resolve)
		if err != nil {
			t.Fatalf("validateCmd() unexpected error: %v", err)
		}
	})

	t.Run("example workflow passes full pipeline", func(t *testing.T) {
		if err := validateCmd(filepath.Join("..", "..", "examples", "fullstack", "workflow.yaml"), resolve); err != nil {
			t.Fatalf("validateCmd(example) unexpected error: %v", err)
		}
	})

	t.Run("semantic violation is rejected", func(t *testing.T) {
		// unknown-type 结构合法（CUE 通过），语义层必须拦截。
		path := filepath.Join("..", "..", "internal", "validation", "testdata", "invalid-node", "unknown-type.yaml")
		err := validateCmd(path, resolve)
		if err == nil {
			t.Fatal("validateCmd(unknown-type) = nil error, want semantic rejection")
		}
		if !strings.Contains(err.Error(), "unknown node definition") {
			t.Errorf("error %q should mention unknown node definition", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if err := validateCmd("nonexistent.yaml", resolve); err == nil {
			t.Fatal("validateCmd(nonexistent) = nil error, want read failure")
		}
	})
}

func TestValidateResolvesInjectedPathsWithoutCreatingOrMigrating(t *testing.T) {
	validFixtureEnv(t)
	root := filepath.Join(t.TempDir(), "runtime")
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	resolved := 0
	resolve := func() (runtimepath.Paths, error) {
		resolved++
		return paths, nil
	}

	workflowFile := filepath.Join("..", "..", "internal", "workflow", "testdata", "valid.yaml")
	if err := runWithRuntimePaths([]string{"validate", workflowFile}, resolve); err != nil {
		t.Fatalf("validate unexpected error: %v", err)
	}
	if resolved != 1 {
		t.Fatalf("validate resolved runtime paths %d times, want 1", resolved)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("validate created runtime root or returned unexpected stat error: %v", err)
	}
}

func testRuntimePaths(t *testing.T) func() (runtimepath.Paths, error) {
	t.Helper()
	paths, err := runtimepath.Resolve(filepath.Join(t.TempDir(), "local-data"))
	if err != nil {
		t.Fatal(err)
	}
	return func() (runtimepath.Paths, error) { return paths, nil }
}

func TestRunWorkflowUsesInjectedRuntimePaths(t *testing.T) {
	validFixtureEnv(t)
	environmentRoot := filepath.Join(t.TempDir(), "must-not-be-used")
	t.Setenv(runtimepath.DataRootEnv, environmentRoot)
	root := filepath.Join(t.TempDir(), "runtime")
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workflowFile := copyFullstackWorkflow(t)

	if err := runWorkflow(ctx, workflowFile, true, cancelHumanGateway{cancel: cancel}, paths); err != nil {
		t.Fatalf("runWorkflow() unexpected error: %v", err)
	}
	if _, err := os.Stat(paths.Database()); err != nil {
		t.Fatalf("injected database path: %v", err)
	}
	entries, err := os.ReadDir(paths.RunsDir())
	if err != nil {
		t.Fatalf("read injected Runs path: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("execution entries = %d, want 1", len(entries))
	}
	runID := entries[0].Name()
	if _, err := uuid.Parse(runID); err != nil {
		t.Fatalf("Run directory %q is not a stable UUID: %v", runID, err)
	}
	for _, want := range []string{
		filepath.Join(paths.RunDir(runID), "state.json"),
		paths.ArtifactsDir(runID),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("injected runtime path %q: %v", want, err)
		}
	}
	if _, err := os.Stat(environmentRoot); !os.IsNotExist(err) {
		t.Fatalf("injected paths did not take priority over environment override: %v", err)
	}
	projectStateDir := filepath.Join(filepath.Dir(workflowFile), "project", ".workflow")
	if _, err := os.Stat(projectStateDir); !os.IsNotExist(err) {
		t.Fatalf("run wrote Gum state into the user project: %v", err)
	}
	historyStore, err := history.OpenReadOnly(context.Background(), paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer historyStore.Close()
	runs, err := historyStore.ListRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != runID {
		t.Fatalf("History Run identity = %+v, want directory UUID %q", runs, runID)
	}
}

func copyFullstackWorkflow(t *testing.T) string {
	t.Helper()
	source := filepath.Join("..", "..", "examples", "fullstack", "workflow.yaml")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	workflowFile := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(workflowFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return workflowFile
}

func TestHistoryUsesInjectedRuntimePaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime")
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := history.Open(context.Background(), paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	runID := "11223344-1111-4111-8111-111111111111"
	if err := store.Record(context.Background(), &execution.WorkflowExecution{
		RunID: runID, Workflow: "injected-history",
		Status: execution.StatusStopped, StartedAt: started, FinishedAt: started.Add(time.Second),
		Nodes: map[string]*execution.NodeExecution{},
	}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := historyCmd(context.Background(), []string{runID[:8]}, paths, &out); err != nil {
		t.Fatalf("historyCmd() unexpected error: %v", err)
	}
	for _, want := range []string{"injected-history", paths.RunDir(runID)} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("history output missing %q:\n%s", want, out.String())
		}
	}
}

type cancelHumanGateway struct {
	cancel context.CancelFunc
}

func (g cancelHumanGateway) RequestRound(ctx context.Context, _ execution.RoundRequest) (execution.RoundResponse, error) {
	g.cancel()
	<-ctx.Done()
	return execution.RoundResponse{}, ctx.Err()
}
