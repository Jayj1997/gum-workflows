package history

import (
	"context"
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	stmts   []string // 按顺序执行的 DDL 语句
}

// DefinitionSchemaVersion 是包含五张定义侧表的首个 SQLite schema 版本。
const DefinitionSchemaVersion = 1

// RunHistorySchemaVersion adds workflow and node-run history tables.
const RunHistorySchemaVersion = 2

// StableRunIdentitySchemaVersion removes the legacy filesystem execution ID;
// the Workflow Run UUID is now the only Run and directory identity.
const StableRunIdentitySchemaVersion = 3

// NodeRunDiagnosticsSchemaVersion preserves immutable script and host execution facts.
const NodeRunDiagnosticsSchemaVersion = 4

// ProductWorkflowSchemaVersion adds SQLite-only Product Workflow identities.
const ProductWorkflowSchemaVersion = 5

// ProductWorkflowDraftSchemaVersion adds one mutable Draft per Product Workflow.
const ProductWorkflowDraftSchemaVersion = 6

// ProductLLMSettingsSchemaVersion adds SQLite-backed Provider and Model Slots.
const ProductLLMSettingsSchemaVersion = 7

// ProductWorkflowRunSchemaVersion adds immutable Revisions and P9 fake Run history.
const ProductWorkflowRunSchemaVersion = 8

// 新增迁移只允许 append（不改写历史迁移），保证已迁移的库向前兼容。
var migrations = []migration{
	{
		version: DefinitionSchemaVersion,
		stmts: []string{
			// 定义侧五表（设计文档 §8.3）。UUID 主键；YAML 不含 id，
			// 导入时生成。FK + 级联删除在 executor/instance 上。
			`CREATE TABLE node_type_definition (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  description   TEXT NOT NULL DEFAULT '',
  requires_json TEXT NOT NULL DEFAULT '[]',
  created_at    TEXT NOT NULL
)`,
			`CREATE TABLE node_definition (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  description   TEXT NOT NULL DEFAULT '',
  type          TEXT NOT NULL REFERENCES node_type_definition(name),
  requires_json TEXT NOT NULL DEFAULT '[]',
  inputs_json   TEXT NOT NULL DEFAULT '{}',
  outputs_json  TEXT NOT NULL DEFAULT '{}',
  created_at    TEXT NOT NULL
)`,
			`CREATE TABLE node_executor (
  id                  TEXT PRIMARY KEY,
  node_definition_id  TEXT NOT NULL REFERENCES node_definition(id) ON DELETE CASCADE,
  version             TEXT NOT NULL,
  name                TEXT NOT NULL DEFAULT '',
  description         TEXT NOT NULL DEFAULT '',
  updates             TEXT NOT NULL DEFAULT '',
  created_at          TEXT NOT NULL,
  UNIQUE (node_definition_id, version)
)`,
			`CREATE TABLE workflow (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  version       TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  projects_json TEXT NOT NULL DEFAULT '[]',
  created_at    TEXT NOT NULL,
  UNIQUE (name, version)
)`,
			`CREATE TABLE node_instance (
  id                  TEXT PRIMARY KEY,
  workflow_id         TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
  node_id             TEXT NOT NULL,
  node_definition_id  TEXT NOT NULL REFERENCES node_definition(id),
  node_executor_id    TEXT NOT NULL REFERENCES node_executor(id),
  display_name        TEXT NOT NULL DEFAULT '',
  description         TEXT NOT NULL DEFAULT '',
  llm_provider        TEXT NOT NULL DEFAULT '',
  llm_model           TEXT NOT NULL DEFAULT '',
  inputs_json         TEXT NOT NULL DEFAULT '{}',
  depends_on_json     TEXT NOT NULL DEFAULT '[]',
  config_json         TEXT NOT NULL DEFAULT '{}',
  UNIQUE (workflow_id, node_id)
)`,
		},
	},
	{
		version: RunHistorySchemaVersion,
		stmts: []string{
			`CREATE TABLE workflow_run_history (
  id               TEXT PRIMARY KEY,
  workflow_name    TEXT NOT NULL,
  workflow_version TEXT NOT NULL DEFAULT '',
  status           TEXT NOT NULL,
  workflow_file    TEXT NOT NULL DEFAULT '',
  execution_id     TEXT NOT NULL,
  error            TEXT NOT NULL DEFAULT '',
  stopped_reason   TEXT NOT NULL DEFAULT '',
  started_at       TEXT NOT NULL,
  finished_at      TEXT
)`,
			`CREATE INDEX idx_run_history_started_at ON workflow_run_history (started_at DESC)`,
			`CREATE INDEX idx_run_history_workflow ON workflow_run_history (workflow_name)`,
			`CREATE TABLE workflow_node_run_history (
  id              TEXT PRIMARY KEY,
  run_id          TEXT NOT NULL REFERENCES workflow_run_history(id) ON DELETE CASCADE,
  node_id         TEXT NOT NULL,
  node_definition TEXT NOT NULL,
  node_executor   TEXT NOT NULL,
  round           INTEGER NOT NULL,
  status          TEXT NOT NULL,
  error           TEXT NOT NULL DEFAULT '',
  error_kind      TEXT NOT NULL DEFAULT '',
  inputs_json     TEXT NOT NULL DEFAULT '{}',
  outputs_json    TEXT NOT NULL DEFAULT '{}',
  started_at      TEXT,
  finished_at     TEXT,
  UNIQUE (run_id, node_id, round)
)`,
			`CREATE INDEX idx_node_run_history_run ON workflow_node_run_history (run_id)`,
		},
	},
	{
		version: StableRunIdentitySchemaVersion,
		stmts: []string{
			`ALTER TABLE workflow_run_history DROP COLUMN execution_id`,
		},
	},
	{
		version: NodeRunDiagnosticsSchemaVersion,
		stmts: []string{
			`ALTER TABLE workflow_node_run_history ADD COLUMN diagnostics_json TEXT NOT NULL DEFAULT '{}'`,
		},
	},
	{
		version: ProductWorkflowSchemaVersion,
		stmts: []string{
			`CREATE TABLE product_workflow (
  id           TEXT PRIMARY KEY,
  display_name TEXT NOT NULL CHECK (length(trim(display_name)) > 0),
  created_at   TEXT NOT NULL
)`,
			`CREATE INDEX idx_product_workflow_created_at ON product_workflow (created_at ASC, id ASC)`,
		},
	},
	{
		version: ProductWorkflowDraftSchemaVersion,
		stmts: []string{
			`CREATE TABLE product_workflow_draft (
  workflow_id  TEXT PRIMARY KEY REFERENCES product_workflow(id) ON DELETE CASCADE,
  content_json TEXT NOT NULL,
  lock_version INTEGER NOT NULL CHECK (lock_version >= 1),
  updated_at   TEXT NOT NULL
)`,
			`INSERT INTO product_workflow_draft (workflow_id, content_json, lock_version, updated_at)
SELECT id, '{"nodes":[],"semanticSchemaVersion":"productWorkflow/v1"}', 1, created_at
FROM product_workflow`,
		},
	},
	{
		version: ProductLLMSettingsSchemaVersion,
		stmts: []string{
			`CREATE TABLE product_llm_provider (
  id                  TEXT PRIMARY KEY,
  name                TEXT NOT NULL CHECK (length(trim(name)) > 0),
  protocol            TEXT NOT NULL CHECK (length(trim(protocol)) > 0),
  base_url            TEXT NOT NULL CHECK (length(trim(base_url)) > 0),
  api_key_ref         TEXT NOT NULL CHECK (length(trim(api_key_ref)) > 0),
  is_explicit_default INTEGER NOT NULL DEFAULT 0 CHECK (is_explicit_default IN (0, 1)),
  created_at          TEXT NOT NULL,
  deleted_at          TEXT
)`,
			`CREATE UNIQUE INDEX idx_product_llm_provider_default
ON product_llm_provider (is_explicit_default)
WHERE is_explicit_default = 1 AND deleted_at IS NULL`,
			`CREATE INDEX idx_product_llm_provider_order
ON product_llm_provider (created_at ASC, id ASC)`,
			`CREATE TABLE product_llm_model (
  id                  TEXT PRIMARY KEY,
  provider_id         TEXT NOT NULL REFERENCES product_llm_provider(id),
  display_name        TEXT NOT NULL CHECK (length(trim(display_name)) > 0),
  provider_model_id   TEXT NOT NULL CHECK (length(trim(provider_model_id)) > 0),
  generation_defaults_json TEXT NOT NULL DEFAULT '{}',
  is_explicit_default INTEGER NOT NULL DEFAULT 0 CHECK (is_explicit_default IN (0, 1)),
  created_at          TEXT NOT NULL,
  deleted_at          TEXT
)`,
			`CREATE UNIQUE INDEX idx_product_llm_model_default
ON product_llm_model (provider_id, is_explicit_default)
WHERE is_explicit_default = 1 AND deleted_at IS NULL`,
			`CREATE INDEX idx_product_llm_model_order
ON product_llm_model (provider_id, created_at ASC, id ASC)`,
		},
	},
	{
		version: ProductWorkflowRunSchemaVersion,
		stmts: []string{
			`CREATE TABLE product_workflow_revision (
  id            TEXT PRIMARY KEY,
  workflow_id   TEXT NOT NULL REFERENCES product_workflow(id) ON DELETE CASCADE,
  semantic_hash TEXT NOT NULL,
  content_json  TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  UNIQUE (workflow_id, semantic_hash)
)`,
			`CREATE TABLE product_workflow_run (
  id            TEXT PRIMARY KEY,
  workflow_id   TEXT NOT NULL REFERENCES product_workflow(id),
  revision_id   TEXT NOT NULL REFERENCES product_workflow_revision(id),
  status        TEXT NOT NULL,
  snapshot_json TEXT NOT NULL,
  started_at    TEXT NOT NULL,
  finished_at   TEXT NOT NULL
)`,
			`CREATE INDEX idx_product_workflow_run_revision
ON product_workflow_run (revision_id, started_at ASC, id ASC)`,
			`CREATE TABLE product_workflow_node_run (
  id              TEXT PRIMARY KEY,
  run_id          TEXT NOT NULL REFERENCES product_workflow_run(id) ON DELETE CASCADE,
  node_id         TEXT NOT NULL,
  node_definition TEXT NOT NULL,
  node_executor   TEXT NOT NULL,
  status          TEXT NOT NULL,
  inputs_json     TEXT NOT NULL,
  outputs_json    TEXT NOT NULL,
  started_at      TEXT NOT NULL,
  finished_at     TEXT NOT NULL,
  UNIQUE (run_id, node_id)
)`,
			`CREATE TABLE product_workflow_artifact (
  id          TEXT PRIMARY KEY,
  run_id      TEXT NOT NULL REFERENCES product_workflow_run(id) ON DELETE CASCADE,
  node_run_id TEXT NOT NULL REFERENCES product_workflow_node_run(id) ON DELETE CASCADE,
  node_id     TEXT NOT NULL,
  port        TEXT NOT NULL,
  artifact_type TEXT NOT NULL,
  version     TEXT NOT NULL,
  uri         TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  UNIQUE (run_id, node_id, port, version)
)`,
		},
	},
}

func latestUserVersion() int {
	return len(migrations)
}

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	for current < latestUserVersion() {
		m := migrations[current] // index = current 对应「从 current 推进到 current+1」的迁移
		if m.version != current+1 {
			return fmt.Errorf("migration index %d expects version %d, got %d",
				current, current+1, m.version)
		}
		if err := applyMigration(ctx, tx, m); err != nil {
			return fmt.Errorf("migrate to version %d: %w", m.version, err)
		}
		current++
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func applyMigration(ctx context.Context, tx *sql.Tx, m migration) error {
	for _, stmt := range m.stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec DDL: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, m.version)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}
	return nil
}
