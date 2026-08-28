package history

import (
	"context"
	"database/sql"
	"fmt"
)

// migration 是一条顺序迁移脚本（运行历史设计 §9.3）。
// 每条对应一个 user_version：v1 迁移把库从 version 0 推进到 version 1。
// Open 时若当前 user_version < len(migrations)，则逐个在事务内执行
// 并推进 user_version；幂等可重入——重复 Open 不重放已完成的迁移。
type migration struct {
	version int
	stmts   []string // 按顺序执行的 DDL 语句
}

// migrations 是顺序迁移脚本数组（index 0 = version 1 的迁移）。
// 新增迁移只允许 append（不改写历史迁移），保证已迁移的库向前兼容。
var migrations = []migration{
	{
		version: 1,
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
}

// latestUserVersion 返回 migrations 推进后的最终 user_version。
func latestUserVersion() int {
	return len(migrations)
}

// migrate 在事务内把库从当前 user_version 顺序推进到最新。
// 已是最新则空操作（幂等可重入）。单条迁移失败即整体回滚并报错，
// user_version 仅在事务提交后推进，保证不会出现「version 前进但 DDL 半完成」。
func migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	for current < latestUserVersion() {
		m := migrations[current] // index = current 对应「从 current 推进到 current+1」的迁移
		if m.version != current+1 {
			return fmt.Errorf("migration index %d expects version %d, got %d",
				current, current+1, m.version)
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("migrate to version %d: %w", m.version, err)
		}
		current++
	}
	return nil
}

// applyMigration 在单个事务内执行一条迁移的全部 DDL，并把 user_version
// 推进到 m.version；DDL 或版本推进任一步失败都会整体回滚。
func applyMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	for _, stmt := range m.stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec DDL: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, m.version)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("set user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}
