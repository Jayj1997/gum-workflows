package history

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixedTime 提供稳定时间戳，让测试不依赖墙钟。
var fixedTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func ctxWithNow() context.Context {
	return withNow(context.Background(), func() time.Time { return fixedTime })
}

// openTest 打开临时库，注入稳定时间戳。
func openTest(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "gum-workflows.db") // 验证父目录自动创建
	s, err := openAt(ctxWithNow(), dbPath)
	if err != nil {
		t.Fatalf("openAt: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dbPath
}

// TestOpenCreatesAndMigrates：首次 Open 建库与五表；user_version 推进到 1。
func TestOpenCreatesAndMigrates(t *testing.T) {
	s, dbPath := openTest(t)

	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v != latestUserVersion() {
		t.Fatalf("user_version = %d, want %d", v, latestUserVersion())
	}

	for _, table := range []string{
		"node_type_definition", "node_definition", "node_executor",
		"workflow", "node_instance",
	} {
		var name string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}

	// 库文件确实落盘。
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("db file not created: %v", err)
	}
}

// TestOpenIsIdempotent：重复 Open 同一文件不重放迁移、不报错。
func TestOpenIsIdempotent(t *testing.T) {
	_, dbPath := openTest(t)

	// 第二次 Open 同一文件：迁移应空操作。
	s2, err := openAt(ctxWithNow(), dbPath)
	if err != nil {
		t.Fatalf("second openAt: %v", err)
	}
	defer s2.Close()

	var v int
	if err := s2.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v != latestUserVersion() {
		t.Fatalf("user_version = %d, want %d", v, latestUserVersion())
	}
}

// TestForeignKeyCascade：删除 workflow 行应级联删除其 node_instance。
func TestForeignKeyCascade(t *testing.T) {
	s, _ := openTest(t)

	// 先导入一份 definition + executor 供 instance 引用。
	nt := NodeTypeDefRow{Name: "automation", Description: "d", Requires: nil}
	def := NodeDefRow{Name: "openapi-generator", Description: "d", Type: "automation"}
	if err := s.ImportDefinitions(ctxWithNow(), []NodeTypeDefRow{nt}, []NodeDefRow{def}, nil); err != nil {
		t.Fatalf("import defs: %v", err)
	}
	defID, err := s.selectID(context.Background(), "node_definition", "name", "openapi-generator")
	if err != nil || defID == "" {
		t.Fatalf("lookup def id: %v %q", err, defID)
	}
	exec := NodeExecRow{NodeDefinitionID: defID, Version: "v1", Name: "e", Updates: "u"}
	if err := s.ImportDefinitions(ctxWithNow(), nil, nil, []NodeExecRow{exec}); err != nil {
		t.Fatalf("import exec: %v", err)
	}
	execID, _ := s.selectID(context.Background(), "node_executor", "version", "v1")

	wf := WorkflowRow{Name: "w", Version: "1.0", Projects: []ProjectRow{{Name: "p", Repository: "./p"}}}
	inst := NodeInstanceRow{
		NodeID: "sdk", NodeDefinitionID: defID, NodeExecutorID: execID,
		DisplayName: "SDK", Config: map[string]any{"task": "x"},
	}
	if err := s.ImportWorkflow(ctxWithNow(), wf, []NodeInstanceRow{inst}); err != nil {
		t.Fatalf("import workflow: %v", err)
	}
	wfID, _ := s.selectID(context.Background(), "workflow", "name", "w")

	// 删除 workflow 行 → node_instance 应随之级联删除。
	if _, err := s.db.Exec(`DELETE FROM workflow WHERE id = ?`, wfID); err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM node_instance WHERE workflow_id = ?`, wfID).Scan(&n); err != nil {
		t.Fatalf("count instances: %v", err)
	}
	if n != 0 {
		t.Errorf("after deleting workflow, %d node_instance rows remain (want 0)", n)
	}
}

// selectID 是测试辅助：按单列自然键查 id（命中返回，否则 ""）。
func (s *Store) selectID(ctx context.Context, table, col, val string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		"SELECT id FROM "+table+" WHERE "+col+" = ?", val).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}
