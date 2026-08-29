package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestValidateExample(t *testing.T) {
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "fullstack"))
	if err := copyTree(src, filepath.Join(tmp, "fullstack")); err != nil {
		t.Fatal(err)
	}

	out, err := runInDir(t, filepath.Join(tmp, "fullstack"), "validate", "workflow.yaml")
	if err != nil {
		t.Fatalf("validate failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "valid (workflow/v1)") {
		t.Errorf("output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "fullstack", ".workflow", "gum-workflows.db")); !os.IsNotExist(err) {
		t.Errorf("validate created database or returned unexpected stat error: %v", err)
	}
}

func TestHistoryListsSeededRuns(t *testing.T) {
	dir := t.TempDir()
	seedHistoryRun(t, dir)

	out, err := runInDir(t, dir, "history")
	if err != nil {
		t.Fatalf("history failed: %v\n%s", err, out)
	}
	for _, want := range []string{"RUN ID", "WORKFLOW", "STATUS", "STARTED", "DURATION", "NODES", "11223344", "history-demo", "Stopped", "1/2"} {
		if !strings.Contains(out, want) {
			t.Errorf("history output missing %q:\n%s", want, out)
		}
	}
}

func TestHistoryShowsRunDetailsByPrefix(t *testing.T) {
	dir := t.TempDir()
	runID := seedHistoryRun(t, dir)

	out, err := runInDir(t, dir, "history", runID[:8])
	if err != nil {
		t.Fatalf("history run detail failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Run " + runID, "Workflow:", "history-demo v1", "Status:", "Stopped",
		"Stopped reason:", "user_interrupt", "State dir:", ".workflow/executions/execution-000007",
		"Nodes:", "worker", "coding-agent", "rounds: 2", "inputs: 1", "outputs: 1",
		"Round 1 Failed", "error_kind: interaction", "Round 2 Succeeded",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("run detail missing %q:\n%s", want, out)
		}
	}
}

func TestHistoryShowsEveryNodeRoundAndArtifactReference(t *testing.T) {
	dir := t.TempDir()
	runID := seedHistoryRun(t, dir)

	out, err := runInDir(t, dir, "history", runID[:8], "worker")
	if err != nil {
		t.Fatalf("history node detail failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Node worker", "coding-agent v1", "Latest round: 2", "Round 1", "Failed",
		"error_kind: interaction", "invalid response", "Round 2", "Succeeded",
		"advise", "from: #advise-retry", "kind: markdown", "uri: artifacts/advise/2.json", "version: 2",
		"result", "uri: artifacts/result/3.json", "version: 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("node detail missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "artifact content") {
		t.Errorf("node detail unexpectedly inlined artifact content:\n%s", out)
	}

	missing, err := runInDir(t, dir, "history", runID[:8], "missing")
	if err != nil || !strings.Contains(missing, "not found") {
		t.Errorf("missing node output = %q, error = %v", missing, err)
	}
}

func seedHistoryRun(t *testing.T, dir string) string {
	t.Helper()
	store, err := history.Open(context.Background(), filepath.Join(dir, ".workflow", "gum-workflows.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	started := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	runID := "11223344-1111-4111-8111-111111111111"
	exec := &execution.WorkflowExecution{
		RunID: runID, ID: "execution-000007", Workflow: "history-demo", WorkflowVersion: "v1",
		WorkflowFile: "workflow.yaml", Status: execution.StatusStopped, StoppedReason: "user_interrupt",
		StartedAt: started, FinishedAt: started.Add(1500 * time.Millisecond),
		Nodes: map[string]*execution.NodeExecution{
			"worker": {
				NodeID: "worker", NodeDefinition: "coding-agent", NodeExecutor: "v1",
				History: []execution.NodeRun{{
					Round: 1, Status: execution.StatusFailed, Error: "invalid response", ErrorKind: node.ErrorKindInteraction,
					StartedAt: started, FinishedAt: started.Add(500 * time.Millisecond),
				}},
				Current: execution.NodeRun{
					Round: 2, Status: execution.StatusSucceeded,
					Inputs: map[string]execution.InputSnapshot{"advise": {
						From: "#advise-retry",
						Ref:  artifact.ArtifactRef{ID: "advise", Kind: "markdown", Version: "2", URI: "artifacts/advise/2.json"},
					}},
					Outputs: map[string]artifact.ArtifactRef{"result": {
						ID: "result", Kind: "markdown", Version: "3", URI: "artifacts/result/3.json",
					}},
					StartedAt: started.Add(time.Second), FinishedAt: started.Add(1400 * time.Millisecond),
				},
			},
			"review": {
				NodeID: "review", NodeDefinition: "human-approval", NodeExecutor: "v1",
				Current: execution.NodeRun{Status: execution.StatusPending},
			},
		},
	}
	if err := store.Record(context.Background(), exec); err != nil {
		t.Fatal(err)
	}
	return runID
}

func TestRunWithHumanNodeRejectsNonTTYStdinBeforeWritingState(t *testing.T) {
	dir := copyFullstackExample(t)

	cmd := execCommand(dir, "run", "workflow.yaml")
	configureCommand(t, cmd)
	cmd.Stdin = strings.NewReader("piped requirement\n\nfinish\n")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("run with piped stdin succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "interactive terminal") {
		t.Errorf("error does not explain TTY requirement:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".workflow")); !os.IsNotExist(statErr) {
		t.Errorf("non-TTY guard wrote runtime state: %v", statErr)
	}
}

func copyFullstackExample(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "fullstack")
	src := absPath(t, filepath.Join("..", "..", "examples", "fullstack"))
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func absPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func runInDir(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := execCommand(dir, args...)
	configureCommand(t, cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func configureCommand(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	xdg := t.TempDir()
	configDir := filepath.Join(xdg, "gum-workflows")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	llmYAML := `apiVersion: llm/v1
kind: llm
providers:
  - name: openai
    type: openai-compatible
    url: https://example.invalid/v1
    apikey: test-key
    default: true
    models:
      - name: gpt-4o
        default: true
`
	if err := os.WriteFile(filepath.Join(configDir, "llm.yaml"), []byte(llmYAML), 0o600); err != nil {
		t.Fatalf("write llm config: %v", err)
	}
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
