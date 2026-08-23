// tests/e2e 端到端验收：CLI 级运行 examples/fullstack（计划 §42 Milestone 11）。
package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateExample(t *testing.T) {
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "fullstack"))
	if err := copyTree(t, src, filepath.Join(tmp, "fullstack")); err != nil {
		t.Fatal(err)
	}

	out, err := runInDir(t, filepath.Join(tmp, "fullstack"), "validate", "workflow.yaml")
	if err != nil {
		t.Fatalf("validate failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "valid (workflow/v1)") {
		t.Errorf("output = %q", out)
	}
}

// TestRunFullstackDemo 是计划 §42 的最终验收：
// 运行 fullstack Workflow，产出全部 6 个 Artifact，状态持久化正确。
func TestRunFullstackDemo(t *testing.T) {
	// 复制示例到临时目录运行（.workflow 落在临时目录，不污染仓库）。
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "fullstack"))
	if err := copyTree(t, src, filepath.Join(tmp, "fullstack")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "fullstack")

	out, err := runInDir(t, dir, "run", "workflow.yaml")
	if err != nil {
		t.Fatalf("run failed: %s\n%s", err, out)
	}

	// 输出包含 Succeeded 与全部 Artifact Kind（§42 验收清单）。
	for _, want := range []string{
		"Succeeded",
		"RequirementSpec",
		"ArchitectureSpec",
		"SourceCode",
		"OpenAPI",
		"FrontendSDK",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// .workflow 目录布局（§28）：execution + workflow.yaml 快照 + workspace + state.json。
	execDir := filepath.Join(dir, ".workflow", "executions", "execution-000001")
	for _, rel := range []string{
		"state.json",
		"workflow.yaml",
		"nodes/requirement/state.json",
		"nodes/backend/state.json",
		"artifacts",
		"workspace/project/README.md",
	} {
		if _, err := os.Stat(filepath.Join(execDir, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}

	// 记录第一次运行的 state.json 内容，验证不被第二次运行覆盖。
	firstState, err := os.ReadFile(filepath.Join(execDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	// 再次运行产生第二个 Execution（同一 Workflow 可运行多次）。
	out2, err := runInDir(t, dir, "run", "workflow.yaml")
	if err != nil {
		t.Fatalf("second run failed: %s\n%s", err, out2)
	}
	secondDir := filepath.Join(dir, ".workflow", "executions", "execution-000002")
	if _, err := os.Stat(secondDir); err != nil {
		t.Fatalf("second execution dir missing: %v", err)
	}

	// 回归（code review P0）：第二次运行必须把自己的状态写到 000002，
	// 不得覆盖 000001。firstState 在第二次运行之前读取，
	// 第二次运行后 000001 的内容必须保持不变。
	secondState, err := os.ReadFile(filepath.Join(secondDir, "state.json"))
	if err != nil {
		t.Fatalf("second run state.json missing: %v", err)
	}
	if len(secondState) == 0 {
		t.Error("second run state.json is empty")
	}
	afterSecond, err := os.ReadFile(filepath.Join(execDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstState) != string(afterSecond) {
		t.Errorf("execution-000001/state.json was overwritten by second run:\nbefore=%s\nafter=%s", firstState, afterSecond)
	}
	// 000002 有自己的完整状态目录。
	if _, err := os.Stat(filepath.Join(secondDir, "nodes", "frontend", "state.json")); err != nil {
		t.Errorf("second execution node state missing: %v", err)
	}
}

func absPath(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// runInDir 在指定目录执行 CLI run/validate。
func runInDir(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := execCommand(dir, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func copyTree(t *testing.T, src, dst string) error {
	t.Helper()
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
