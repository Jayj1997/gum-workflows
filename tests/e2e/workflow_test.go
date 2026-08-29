package e2e_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestValidateExample(t *testing.T) {
	dir := copyFullstackExample(t)
	dataRoot := filepath.Join(t.TempDir(), "local-data")
	out, err := runInDirWithDataRoot(t, dir, dataRoot, "validate", "workflow.yaml")
	if err != nil {
		t.Fatalf("validate failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "valid (workflow/v1)") {
		t.Errorf("output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".workflow", "gum-workflows.db")); !os.IsNotExist(err) {
		t.Errorf("validate created database or returned unexpected stat error: %v", err)
	}
	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Errorf("validate created Local Data Root or returned unexpected stat error: %v", err)
	}
}

func TestHistoryListsSeededRuns(t *testing.T) {
	dataRoot := t.TempDir()
	seedHistoryRun(t, dataRoot)

	out, err := runInDirWithDataRoot(t, t.TempDir(), dataRoot, "history")
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
	dataRoot := t.TempDir()
	runID := seedHistoryRun(t, dataRoot)

	out, err := runInDirWithDataRoot(t, t.TempDir(), dataRoot, "history", runID[:8])
	if err != nil {
		t.Fatalf("history run detail failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Run " + runID, "Workflow:", "history-demo v1", "Status:", "Stopped",
		"Stopped reason:", "user_interrupt", "State dir:", filepath.Join(dataRoot, "runs", "execution-000007"),
		"Nodes:", "worker", "coding-agent", "rounds: 2", "inputs: 1", "outputs: 1",
		"Round 1 Failed", "error_kind: interaction", "Round 2 Succeeded",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("run detail missing %q:\n%s", want, out)
		}
	}
}

func TestHistoryShowsEveryNodeRoundAndArtifactReference(t *testing.T) {
	dataRoot := t.TempDir()
	runID := seedHistoryRun(t, dataRoot)

	out, err := runInDirWithDataRoot(t, t.TempDir(), dataRoot, "history", runID[:8], "worker")
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

	missing, err := runInDirWithDataRoot(t, t.TempDir(), dataRoot, "history", runID[:8], "missing")
	if err != nil || !strings.Contains(missing, "not found") {
		t.Errorf("missing node output = %q, error = %v", missing, err)
	}
}

func seedHistoryRun(t *testing.T, dataRoot string) string {
	t.Helper()
	store, err := history.Open(context.Background(), filepath.Join(dataRoot, "product.db"))
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
	dataRoot := filepath.Join(t.TempDir(), "local-data")

	cmd := execCommand(dir, "run", "workflow.yaml")
	configureCommand(t, cmd, dataRoot)
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
	if _, statErr := os.Stat(dataRoot); !os.IsNotExist(statErr) {
		t.Errorf("non-TTY guard wrote Local Data Root: %v", statErr)
	}
}

func TestRunWritesOnlyLocalDataRootAndIsVisibleToHistory(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the CLI lifecycle e2e uses the macOS script utility to provide a PTY")
	}

	dir := copyFullstackExample(t)
	dataRoot := filepath.Join(t.TempDir(), "local-data")
	cmd := exec.Command("/usr/bin/script", "-q", "/dev/null", binPath, "run", "workflow.yaml")
	cmd.Dir = dir
	configureCommand(t, cmd, dataRoot)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output lockedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start CLI run: %v", err)
	}
	waitForOutput(t, &output, "[requirement] Enter requirement", cmd)
	if _, err := io.WriteString(stdin, "implement the requested change\n\nf\n\n"); err != nil {
		t.Fatalf("write CLI input: %v", err)
	}

	stateFile := waitForReviewSuccess(t, dataRoot, cmd, &output)
	if _, err := stdin.Write([]byte{3}); err != nil {
		t.Fatalf("send Ctrl-C to CLI run after %s: %v", stateFile, err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatalf("CLI run did not stop cleanly: %v\n%s", err, output.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("CLI run did not stop after interrupt:\n%s", output.String())
	}

	if _, err := os.Stat(filepath.Join(dir, ".workflow")); !os.IsNotExist(err) {
		t.Fatalf("CLI run wrote project-local Gum state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "product.db")); err != nil {
		t.Fatalf("CLI run did not create the global product database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "project", ".mock-agent", "task.md")); err != nil {
		t.Fatalf("Agent change is not visible in the in-place project: %v", err)
	}
	workspaceCopies, err := filepath.Glob(filepath.Join(dataRoot, "runs", "*", "workspace", "project"))
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceCopies) != 0 {
		t.Fatalf("CLI run copied the project into Local Data Root: %v", workspaceCopies)
	}

	historyOutput, err := runInDirWithDataRoot(t, t.TempDir(), dataRoot, "history")
	if err != nil {
		t.Fatalf("history after CLI run failed: %v\n%s", err, historyOutput)
	}
	for _, want := range []string{"fullstack-development", "Stopped"} {
		if !strings.Contains(historyOutput, want) {
			t.Errorf("history output missing %q:\n%s", want, historyOutput)
		}
	}
}

func waitForReviewSuccess(t *testing.T, dataRoot string, cmd *exec.Cmd, output *lockedBuffer) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	pattern := filepath.Join(dataRoot, "runs", "*", "nodes", "review", "state.json")
	for time.Now().Before(deadline) {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range matches {
			data, err := os.ReadFile(path)
			if err == nil && strings.Contains(string(data), `"status": "Succeeded"`) {
				return path
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	t.Fatalf("review did not succeed before timeout:\n%s", output.String())
	return ""
}

func waitForOutput(t *testing.T, output *lockedBuffer, want string, cmd *exec.Cmd) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	t.Fatalf("CLI output did not contain %q:\n%s", want, output.String())
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
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

func runInDirWithDataRoot(t *testing.T, dir, dataRoot string, args ...string) (string, error) {
	t.Helper()
	cmd := execCommand(dir, args...)
	configureCommand(t, cmd, dataRoot)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func configureCommand(t *testing.T, cmd *exec.Cmd, dataRoot string) {
	t.Helper()
	xdg := t.TempDir()
	configDir := filepath.Join(xdg, "gum-workflows")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	llmYAML, err := os.ReadFile(absPath(t, filepath.Join("..", "..", "examples", "fullstack", "llm.example.yaml")))
	if err != nil {
		t.Fatalf("read demo llm config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "llm.yaml"), llmYAML, 0o600); err != nil {
		t.Fatalf("write llm config: %v", err)
	}
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "GUM_WORKFLOWS_DATA_ROOT="+dataRoot)
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
