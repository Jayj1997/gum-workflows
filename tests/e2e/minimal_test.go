// tests/e2e 端到端验收：CLI 级运行 examples/minimal（计划 §42 Milestone 11）。
// 旧 examples/fullstack 已退役（票 05）：新契约下 requirement-analysis 有必填输入，
// human-input 入口节点要到 T09 才存在；新 demo（含审批循环）在 T14 重写。
package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateExample(t *testing.T) {
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "minimal"))
	if err := copyTree(t, src, filepath.Join(tmp, "minimal")); err != nil {
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

// TestRunMinimalDemo 是当前 Schema 形态下的 CLI 级验收：
// 运行最小 human-free 链（coder -> sdk），产出全部 3 个 Artifact，
// 状态持久化正确。
func TestRunMinimalDemo(t *testing.T) {
	// 复制示例到临时目录运行（.workflow 落在临时目录，不污染仓库）。
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "minimal"))
	if err := copyTree(t, src, filepath.Join(tmp, "minimal")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "minimal")

	out, err := runInDir(t, dir, "run", "workflow.yaml")
	if err != nil {
		t.Fatalf("run failed: %s\n%s", err, out)
	}

	dbPath := filepath.Join(dir, ".workflow", "gum-workflows.db")
	if got := sqliteQuery(t, dbPath, `
SELECT
  (SELECT count(*) FROM node_type_definition),
  (SELECT count(*) FROM node_definition),
  (SELECT count(*) FROM node_executor),
  (SELECT count(*) FROM workflow),
  (SELECT count(*) FROM node_instance);`); got != "3|4|4|1|2" {
		t.Errorf("definition table counts = %q, want %q", got, "3|4|4|1|2")
	}
	if got := sqliteQuery(t, dbPath, `
SELECT name, requires_json FROM node_type_definition ORDER BY name;`); got != "agent|[\"llm\"]\nautomation|[]\nhuman|[]" {
		t.Errorf("node type definitions = %q", got)
	}
	if got := sqliteQuery(t, dbPath, `
SELECT name, type, requires_json,
       json_extract(inputs_json, '$.openapi.type'),
       json_extract(outputs_json, '$.source-code.type')
FROM node_definition WHERE name = 'coding-agent';`); got != `coding-agent|agent|["project"]|OpenAPI|SourceCode` {
		t.Errorf("coding-agent definition = %q", got)
	}
	if got := sqliteQuery(t, dbPath, `
SELECT d.name, e.version, e.name
FROM node_executor e JOIN node_definition d ON d.id = e.node_definition_id
ORDER BY d.name;`); got != "architecture-design|v1|architecture-design-v1\ncoding-agent|v1|coding-agent-v1\nopenapi-generator|v1|openapi-generator-v1\nrequirement-analysis|v1|requirement-analysis-v1" {
		t.Errorf("node executor definitions = %q", got)
	}
	if got := sqliteQuery(t, dbPath, `
SELECT name, version, json_extract(projects_json, '$[0].name'),
       json_extract(projects_json, '$[0].repository')
FROM workflow;`); got != "minimal-development|1.0|order-system|./project" {
		t.Errorf("workflow definition = %q", got)
	}
	if got := sqliteQuery(t, dbPath, `
SELECT ni.node_id, e.version, ni.llm_provider, ni.llm_model
FROM node_instance ni
JOIN node_executor e ON e.id = ni.node_executor_id
ORDER BY ni.node_id;`); got != "coder|v1|openai|gpt-4o\nsdk|v1||" {
		t.Errorf("resolved node instances = %q", got)
	}
	if dump := sqliteQuery(t, dbPath, ".dump"); strings.Contains(dump, "test-key") || strings.Contains(dump, "example.invalid") {
		t.Error("llm.yaml connection details leaked into the project database")
	}
	definitionIDs := sqliteQuery(t, dbPath, `
SELECT id FROM node_type_definition
UNION ALL SELECT id FROM node_definition
UNION ALL SELECT id FROM node_executor
UNION ALL SELECT id FROM workflow
UNION ALL SELECT id FROM node_instance
ORDER BY id;`)

	// 输出包含 Succeeded 与全部 Artifact 类型。
	for _, want := range []string{
		"Succeeded",
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
		"nodes/coder/state.json",
		"nodes/sdk/state.json",
		"artifacts",
		"workspace/project/README.md",
		"workspace/project/.mock-agent/task.md",
		"workspace/project/.mock-agent/openapi.yaml",
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
	if got := sqliteQuery(t, dbPath, `
SELECT id FROM node_type_definition
UNION ALL SELECT id FROM node_definition
UNION ALL SELECT id FROM node_executor
UNION ALL SELECT id FROM workflow
UNION ALL SELECT id FROM node_instance
ORDER BY id;`); got != definitionIDs {
		t.Errorf("definition UUIDs changed across repeated run:\nbefore=%s\nafter=%s", definitionIDs, got)
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
	if _, err := os.Stat(filepath.Join(secondDir, "nodes", "sdk", "state.json")); err != nil {
		t.Errorf("second execution node state missing: %v", err)
	}
}

func TestRunStopsWhenDefinitionDatabaseCannotOpen(t *testing.T) {
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "minimal"))
	if err := copyTree(t, src, filepath.Join(tmp, "minimal")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "minimal")
	if err := os.WriteFile(filepath.Join(dir, ".workflow"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("create invalid database parent: %v", err)
	}

	out, err := runInDir(t, dir, "run", "workflow.yaml")
	if err == nil {
		t.Fatalf("run succeeded with unusable database path:\n%s", out)
	}
	if !strings.Contains(out, "history database") {
		t.Errorf("error does not identify definition import startup failure:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".workflow", "executions")); err == nil {
		t.Error("engine created execution state after database failure")
	}
}

func TestRunRejectsNewerDatabaseExecutorWithoutBinaryImplementation(t *testing.T) {
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "minimal"))
	if err := copyTree(t, src, filepath.Join(tmp, "minimal")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "minimal")
	if out, err := runInDir(t, dir, "run", "workflow.yaml"); err != nil {
		t.Fatalf("initial run failed: %v\n%s", err, out)
	}
	dbPath := filepath.Join(dir, ".workflow", "gum-workflows.db")
	sqliteQuery(t, dbPath, `
INSERT INTO node_executor (id, node_definition_id, version, name, created_at)
SELECT '00000000-0000-0000-0000-000000000002', id, 'v2', 'coding-agent-v2', '2026-08-28T00:00:00Z'
FROM node_definition WHERE name = 'coding-agent';`)

	validateOut, validateErr := runInDir(t, dir, "validate", "workflow.yaml")
	if validateErr == nil {
		t.Fatalf("validate ignored database executor v2:\n%s", validateOut)
	}
	if !strings.Contains(validateOut, `executor "v2"`) || !strings.Contains(validateOut, "coding-agent") {
		t.Errorf("validate error does not identify unavailable executor:\n%s", validateOut)
	}

	out, err := runInDir(t, dir, "run", "workflow.yaml")
	if err == nil {
		t.Fatalf("run silently downgraded from database executor v2:\n%s", out)
	}
	if !strings.Contains(out, `executor "v2"`) || !strings.Contains(out, "coding-agent") {
		t.Errorf("error does not identify unavailable pinned executor:\n%s", out)
	}
}

func TestRunMigratesExistingVersionZeroDatabaseBeforeResolution(t *testing.T) {
	tmp := t.TempDir()
	src := absPath(t, filepath.Join("..", "..", "examples", "minimal"))
	if err := copyTree(t, src, filepath.Join(tmp, "minimal")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "minimal")
	dbPath := filepath.Join(dir, ".workflow", "gum-workflows.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("create workflow dir: %v", err)
	}
	sqliteQuery(t, dbPath, `PRAGMA user_version = 0;`)

	validateOut, err := runInDir(t, dir, "validate", "workflow.yaml")
	if err != nil {
		t.Fatalf("validate rejected version-zero database: %v\n%s", err, validateOut)
	}
	if got := sqliteQuery(t, dbPath, `PRAGMA user_version;`); got != "0" {
		t.Errorf("validate changed user_version to %q, want 0", got)
	}
	if got := sqliteQuery(t, dbPath, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'node_executor';`); got != "0" {
		t.Errorf("validate created node_executor table, count = %q, want 0", got)
	}

	out, err := runInDir(t, dir, "run", "workflow.yaml")
	if err != nil {
		t.Fatalf("run did not migrate version-zero database: %v\n%s", err, out)
	}
	if got := sqliteQuery(t, dbPath, `SELECT count(*) FROM node_instance;`); got != "2" {
		t.Errorf("node instance count after migration = %q, want 2", got)
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

func sqliteQuery(t *testing.T, dbPath, query string) string {
	t.Helper()
	cmd := exec.Command("sqlite3", dbPath, query)
	result, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 query failed: %v\n%s", err, result)
	}
	return strings.TrimSpace(string(result))
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
