package e2e_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateExample(t *testing.T) {
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "minimal"))
	if err := copyTree(src, filepath.Join(tmp, "minimal")); err != nil {
		t.Fatal(err)
	}

	out, err := runInDir(t, filepath.Join(tmp, "minimal"), "validate", "workflow.yaml")
	if err != nil {
		t.Fatalf("validate failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "valid (workflow/v1)") {
		t.Errorf("output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "minimal", ".workflow", "gum-workflows.db")); !os.IsNotExist(err) {
		t.Errorf("validate created database or returned unexpected stat error: %v", err)
	}
}

func TestRunWithHumanNodeRejectsNonTTYStdinBeforeWritingState(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	workflowYAML := `apiVersion: workflow/v1
kind: workflow
metadata:
  name: human-entry
projects:
  - name: project
    repository: ./project
nodes:
  input:
    node: human-input
`
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(workflowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := execCommand(dir, "run", "workflow.yaml")
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

func TestInteractiveRunPersistsRepeatedExecutions(t *testing.T) {
	dir := copyMinimalExample(t)
	if out, err := runInteractiveInDir(t, dir, "run", "workflow.yaml"); err != nil {
		t.Fatalf("first interactive run: %v\n%s", err, out)
	}
	firstPath := filepath.Join(dir, ".workflow", "executions", "execution-000001", "state.json")
	firstState, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runInteractiveInDir(t, dir, "run", "workflow.yaml"); err != nil {
		t.Fatalf("second interactive run: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".workflow", "executions", "execution-000002", "nodes", "sdk", "state.json")); err != nil {
		t.Fatalf("second execution state missing: %v", err)
	}
	afterSecond, err := os.ReadFile(firstPath)
	if err != nil || string(afterSecond) != string(firstState) {
		t.Errorf("first execution state changed after second run: %v", err)
	}
	if got := sqliteQuery(t, filepath.Join(dir, ".workflow", "gum-workflows.db"), `SELECT count(*) FROM node_instance;`); got != "4" {
		t.Errorf("node instance count = %q, want 4", got)
	}
}

func TestInteractiveRunStopsWhenDefinitionDatabaseCannotOpen(t *testing.T) {
	dir := copyMinimalExample(t)
	if err := os.WriteFile(filepath.Join(dir, ".workflow"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runInteractiveInDir(t, dir, "run", "workflow.yaml")
	if err == nil || !strings.Contains(out, "history database") {
		t.Fatalf("run error = %v\n%s", err, out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".workflow", "executions")); statErr == nil {
		t.Error("engine state exists after database startup failure")
	}
}

func TestInteractiveRunRejectsNewerDatabaseExecutor(t *testing.T) {
	dir := copyMinimalExample(t)
	if out, err := runInteractiveInDir(t, dir, "run", "workflow.yaml"); err != nil {
		t.Fatalf("initial run: %v\n%s", err, out)
	}
	dbPath := filepath.Join(dir, ".workflow", "gum-workflows.db")
	sqliteQuery(t, dbPath, `
INSERT INTO node_executor (id, node_definition_id, version, name, created_at)
SELECT '00000000-0000-0000-0000-000000000002', id, 'v2', 'coding-agent-v2', '2026-08-28T00:00:00Z'
FROM node_definition WHERE name = 'coding-agent';`)
	out, err := runInteractiveInDir(t, dir, "run", "workflow.yaml")
	if err == nil || !strings.Contains(out, `executor "v2"`) {
		t.Fatalf("run did not reject unavailable v2: %v\n%s", err, out)
	}
}

func TestInteractiveRunMigratesVersionZeroDatabase(t *testing.T) {
	dir := copyMinimalExample(t)
	dbPath := filepath.Join(dir, ".workflow", "gum-workflows.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	sqliteQuery(t, dbPath, `PRAGMA user_version = 0;`)
	if out, err := runInteractiveInDir(t, dir, "run", "workflow.yaml"); err != nil {
		t.Fatalf("run did not migrate version-zero database: %v\n%s", err, out)
	}
	if got := sqliteQuery(t, dbPath, `PRAGMA user_version;`); got == "0" {
		t.Errorf("database remained at user_version 0")
	}
}

func copyMinimalExample(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "minimal")
	src := absPath(t, filepath.Join("..", "..", "examples", "minimal"))
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

func runInteractiveInDir(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := execInteractiveCommand(dir, args...)
	configureCommand(t, cmd)
	cmd.Stdin = strings.NewReader("build the order system\n\nfinish\n")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	before, _ := filepath.Glob(filepath.Join(dir, ".workflow", "executions", "execution-*"))
	if err := cmd.Start(); err != nil {
		return output.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return output.String(), err
		case <-deadline.C:
			_ = cmd.Process.Kill()
			<-done
			return output.String(), fmt.Errorf("interactive run did not settle")
		case <-ticker.C:
			executions, _ := filepath.Glob(filepath.Join(dir, ".workflow", "executions", "execution-*"))
			if len(executions) <= len(before) || !executionSettled(executions[len(executions)-1]) {
				continue
			}
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				return output.String(), err
			}
			return output.String(), <-done
		}
	}
}

func executionSettled(executionDir string) bool {
	states, _ := filepath.Glob(filepath.Join(executionDir, "nodes", "*", "state.json"))
	if len(states) == 0 {
		return false
	}
	for _, state := range states {
		data, err := os.ReadFile(state)
		if err != nil || !bytes.Contains(data, []byte(`"status": "Succeeded"`)) {
			return false
		}
	}
	return true
}

func sqliteQuery(t *testing.T, dbPath, query string) string {
	t.Helper()
	cmd := exec.Command("sqlite3", dbPath, query)
	result, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 query failed: %v\n%s", err, result)
	}
	return strings.TrimSpace(string(result))
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
