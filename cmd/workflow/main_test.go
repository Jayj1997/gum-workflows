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
)

// validFixtureEnv 满足语义校验的运行前提（票 06 起 validate 全链生效）：
// 临时 XDG_CONFIG_HOME 提供 llm.yaml（agent 节点的默认链解析），
// fixture 的相对 projects 路径已就位于 testdata/examples/。
// 不触真实 $HOME（DEVELOPMENT.md §6）。
func validFixtureEnv(t *testing.T) {
	t.Helper()

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
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

	t.Run("valid workflow", func(t *testing.T) {
		err := validateCmd(filepath.Join("..", "..", "internal", "workflow", "testdata", "valid.yaml"))
		if err != nil {
			t.Fatalf("validateCmd() unexpected error: %v", err)
		}
	})

	t.Run("example workflow passes full pipeline", func(t *testing.T) {
		if err := validateCmd(filepath.Join("..", "..", "examples", "fullstack", "workflow.yaml")); err != nil {
			t.Fatalf("validateCmd(example) unexpected error: %v", err)
		}
	})

	t.Run("semantic violation is rejected", func(t *testing.T) {
		// unknown-type 结构合法（CUE 通过），语义层必须拦截。
		path := filepath.Join("..", "..", "internal", "validation", "testdata", "invalid-node", "unknown-type.yaml")
		err := validateCmd(path)
		if err == nil {
			t.Fatal("validateCmd(unknown-type) = nil error, want semantic rejection")
		}
		if !strings.Contains(err.Error(), "unknown node definition") {
			t.Errorf("error %q should mention unknown node definition", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if err := validateCmd("nonexistent.yaml"); err == nil {
			t.Fatal("validateCmd(nonexistent) = nil error, want read failure")
		}
	})
}

func TestValidateDoesNotResolveOrCreateRuntimePaths(t *testing.T) {
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
	if resolved != 0 {
		t.Fatalf("validate resolved runtime paths %d times, want 0", resolved)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("validate created runtime root or returned unexpected stat error: %v", err)
	}
}

func TestRunWorkflowUsesInjectedRuntimePaths(t *testing.T) {
	validFixtureEnv(t)
	root := filepath.Join(t.TempDir(), "runtime")
	paths, err := runtimepath.New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workflowFile := filepath.Join("..", "..", "examples", "fullstack", "workflow.yaml")

	if err := runWorkflow(ctx, workflowFile, true, cancelHumanGateway{cancel: cancel}, paths); err != nil {
		t.Fatalf("runWorkflow() unexpected error: %v", err)
	}
	if _, err := os.Stat(paths.Database()); err != nil {
		t.Fatalf("injected database path: %v", err)
	}
	entries, err := os.ReadDir(paths.ExecutionsDir())
	if err != nil {
		t.Fatalf("read injected executions path: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("execution entries = %d, want 1", len(entries))
	}
	executionID := entries[0].Name()
	for _, want := range []string{
		filepath.Join(paths.ExecutionDir(executionID), "state.json"),
		paths.ArtifactsDir(executionID),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("injected runtime path %q: %v", want, err)
		}
	}
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
		RunID: runID, ID: "execution-000007", Workflow: "injected-history",
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
	for _, want := range []string{"injected-history", paths.ExecutionDir("execution-000007")} {
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
