package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	out, err := cmd.CombinedOutput()
	return string(out), err
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
