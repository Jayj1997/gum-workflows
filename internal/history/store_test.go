package history

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// fixedTime 提供稳定时间戳，让测试不依赖墙钟。
var fixedTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func ctxWithNow() context.Context {
	return withNow(context.Background(), func() time.Time { return fixedTime })
}

func openTest(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "gum-workflows.db") // 验证父目录自动创建
	s, err := Open(ctxWithNow(), dbPath)
	if err != nil {
		t.Fatalf("openAt: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dbPath
}

func TestOpenCreatesAndMigrates(t *testing.T) {
	s, dbPath := openTest(t)

	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v != latestUserVersion() {
		t.Fatalf("user_version = %d, want %d", v, latestUserVersion())
	}
	for _, tc := range []struct {
		pragma string
		want   string
	}{
		{pragma: "journal_mode", want: "wal"},
		{pragma: "busy_timeout", want: "5000"},
		{pragma: "foreign_keys", want: "1"},
	} {
		var got string
		if err := s.db.QueryRow(`PRAGMA ` + tc.pragma).Scan(&got); err != nil {
			t.Errorf("read PRAGMA %s: %v", tc.pragma, err)
		} else if got != tc.want {
			t.Errorf("PRAGMA %s = %q, want %q", tc.pragma, got, tc.want)
		}
	}

	for _, table := range []string{
		"node_type_definition", "node_definition", "node_executor",
		"workflow", "node_instance", "workflow_run_history", "workflow_node_run_history",
		"product_workflow", "product_workflow_draft", "product_llm_provider", "product_llm_model",
		"product_workflow_revision", "product_workflow_run", "product_workflow_node_run", "product_workflow_artifact",
	} {
		var name string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
	rows, err := s.db.Query(`PRAGMA table_info(workflow_run_history)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "execution_id" {
			t.Fatal("workflow_run_history still contains legacy execution_id")
		}
	}

	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("db file not created: %v", err)
	}
}

func TestOpenRequiresInjectedPath(t *testing.T) {
	for _, tt := range []struct {
		name string
		open func(context.Context, string) (*Store, error)
	}{
		{name: "read-write", open: Open},
		{name: "read-only", open: OpenReadOnly},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.open(context.Background(), ""); err == nil {
				t.Fatal("open with empty path = nil error, want rejection")
			}
		})
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	_, dbPath := openTest(t)

	// 第二次 Open 同一文件：迁移应空操作。
	s2, err := Open(ctxWithNow(), dbPath)
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

func TestOpenUpgradesRunHistoryToSingleRunIdentity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "product.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:RunHistorySchemaVersion] {
		if err := applyMigration(context.Background(), tx, migration); err != nil {
			t.Fatal(err)
		}
	}
	runID := "11111111-1111-4111-8111-111111111111"
	if _, err := tx.Exec(`
INSERT INTO workflow_run_history
  (id, workflow_name, workflow_version, status, workflow_file, execution_id, error, stopped_reason, started_at)
VALUES (?, 'legacy-local-data', 'v1', 'Stopped', 'workflow.yaml', 'execution-000007', '', 'user_interrupt', '2026-08-29T12:00:00Z')`, runID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.ID != runID || run.Workflow != "legacy-local-data" {
		t.Fatalf("upgraded Run = %+v", run)
	}
}

func TestProductWorkflowMigrationPreservesExistingDefinitionsAndRunHistory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "product.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:NodeRunDiagnosticsSchemaVersion] {
		if err := applyMigration(context.Background(), tx, migration); err != nil {
			t.Fatal(err)
		}
	}
	workflowID := "11111111-1111-4111-8111-111111111111"
	runID := "22222222-2222-4222-8222-222222222222"
	if _, err := tx.Exec(`
INSERT INTO workflow (id, name, version, description, projects_json, created_at)
VALUES (?, 'existing-yaml', 'v1', '', '[]', '2026-08-30T12:00:00Z')`, workflowID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`
INSERT INTO workflow_run_history
  (id, workflow_name, workflow_version, status, workflow_file, error, stopped_reason, started_at)
VALUES (?, 'existing-yaml', 'v1', 'Stopped', 'workflow.yaml', '', 'user_interrupt', '2026-08-30T12:00:00Z')`, runID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	gotWorkflowID, err := store.selectID(context.Background(), "workflow", "name", "existing-yaml")
	if err != nil || gotWorkflowID != workflowID {
		t.Fatalf("existing workflow ID = %q, error = %v", gotWorkflowID, err)
	}
	run, err := store.GetRun(context.Background(), runID)
	if err != nil || run == nil || run.Workflow != "existing-yaml" {
		t.Fatalf("existing run = %#v, error = %v", run, err)
	}
	created, err := store.CreateProductWorkflow(context.Background(), "New product workflow")
	if err != nil || created.ID == "" {
		t.Fatalf("create Product Workflow = %#v, error = %v", created, err)
	}
}

func TestProductWorkflowDraftMigrationBackfillsExistingProductWorkflows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "product.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:ProductWorkflowSchemaVersion] {
		if err := applyMigration(context.Background(), tx, migration); err != nil {
			t.Fatal(err)
		}
	}
	workflowID := "11111111-1111-4111-8111-111111111111"
	if _, err := tx.Exec(`
INSERT INTO product_workflow (id, display_name, created_at)
VALUES (?, 'Existing product workflow', '2026-08-31T09:00:00Z')`, workflowID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	draft, err := store.GetProductWorkflowDraft(context.Background(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	if draft.LockVersion != 1 || string(draft.Content) != string(productworkflow.InitialDraftContent()) {
		t.Fatalf("backfilled Draft = %#v", draft)
	}
}

func TestProductLLMSettingsMigrationAndUUIDTieBreak(t *testing.T) {
	s, _ := openTest(t)
	for _, table := range []string{"product_llm_provider", "product_llm_model"} {
		var name string
		if err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
	}
	providerTime := "2026-09-01T00:00:00Z"
	for _, row := range []struct{ id, name string }{
		{id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", name: "Later UUID"},
		{id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", name: "Earlier UUID"},
	} {
		if _, err := s.db.Exec(`INSERT INTO product_llm_provider (id, name, protocol, base_url, api_key_ref, created_at) VALUES (?, ?, 'openai-chat-completions', 'https://api.example/v1', 'keychain://test', ?)`, row.id, row.name, providerTime); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"dddddddd-dddd-4ddd-8ddd-dddddddddddd", "cccccccc-cccc-4ccc-8ccc-cccccccccccc"} {
		if _, err := s.db.Exec(`INSERT INTO product_llm_model (id, provider_id, display_name, provider_model_id, created_at) VALUES (?, 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'Model', ?, ?)`, id, id, providerTime); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := s.ResolveDefaultLLMModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Provider.ID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || resolved.Model.ID != "cccccccc-cccc-4ccc-8ccc-cccccccccccc" {
		t.Fatalf("UUID tie-break = Provider %s Model %s", resolved.Provider.ID, resolved.Model.ID)
	}
	rows, err := s.db.Query(`PRAGMA table_info(product_llm_provider)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if !columns["api_key_ref"] || columns["api_key"] || columns["secret"] {
		t.Fatalf("Provider credential columns = %#v", columns)
	}
}

func TestConcurrentOpenIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gum-workflows.db")
	start := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			store, err := Open(context.Background(), dbPath)
			if err == nil {
				err = store.Close()
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Open: %v", err)
		}
	}
}

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

func (s *Store) selectID(ctx context.Context, table, col, val string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		"SELECT id FROM "+table+" WHERE "+col+" = ?", val).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}
