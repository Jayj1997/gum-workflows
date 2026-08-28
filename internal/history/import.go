package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type nowFunc func() time.Time

// ImportDefinitions 幂等导入内嵌种子三类定义（设计文档 §8.2 步骤 1）：
// node types / definitions / executors，按自然键 upsert。
// 导入顺序即依赖顺序：types -> definitions -> executors
// （executor 按 node_definition_id 引用 definition，definition 按 type 引用 node type）。
// 重复导入同键覆盖而非新建行：已有行的 id 复用（保证跨 run UUID 稳定）。
func (s *Store) ImportDefinitions(ctx context.Context, nodeTypes []NodeTypeDefRow, defs []NodeDefRow, executors []NodeExecRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, t := range nodeTypes {
		if err := upsertNodeType(ctx, tx, t); err != nil {
			return fmt.Errorf("import node type %q: %w", t.Name, err)
		}
	}
	for _, d := range defs {
		if err := upsertNodeDef(ctx, tx, d); err != nil {
			return fmt.Errorf("import node definition %q: %w", d.Name, err)
		}
	}
	for _, e := range executors {
		if err := upsertNodeExec(ctx, tx, e); err != nil {
			return fmt.Errorf("import node executor (%s, %s): %w", e.Node, e.Version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit definition import: %w", err)
	}
	return nil
}

// DefinitionID 按 Node Definition 名查已导入的 node_definition.id（UUID）。
// 供 cmd 层在导入 node_instance 前固定 definition 引用。
func (s *Store) DefinitionID(ctx context.Context, name string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM node_definition WHERE name = ?`, name).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("node definition %q not imported", name)
	}
	if err != nil {
		return "", fmt.Errorf("lookup node definition %q: %w", name, err)
	}
	return id, nil
}

// ExecutorID 按 (definition name, version) 查已导入的 node_executor.id。
// 供 cmd 层固定 executor 引用（与 engine 实际使用的版本一致）。
func (s *Store) ExecutorID(ctx context.Context, definitionName, version string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
SELECT e.id FROM node_executor e
JOIN node_definition d ON d.id = e.node_definition_id
WHERE d.name = ? AND e.version = ?`, definitionName, version).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("node executor (%s, %s) not imported", definitionName, version)
	}
	if err != nil {
		return "", fmt.Errorf("lookup node executor (%s, %s): %w", definitionName, version, err)
	}
	return id, nil
}

// ExecutorVersions 返回数据库中某 Node Definition 已导入的全部执行器版本。
func (s *Store) ExecutorVersions(ctx context.Context, definitionName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.version FROM node_executor e
JOIN node_definition d ON d.id = e.node_definition_id
WHERE d.name = ?`, definitionName)
	if err != nil {
		return nil, fmt.Errorf("list node executor versions for %q: %w", definitionName, err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan node executor version for %q: %w", definitionName, err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node executor versions for %q: %w", definitionName, err)
	}
	return versions, nil
}

// ImportWorkflow 幂等导入本次 workflow 及其 node instances
// （设计文档 §8.2 步骤 2-3）。workflow 按 (name, version) 覆盖式 upsert；
// node instances 按 (workflow_id, node_id) upsert。projects 写入 projects_json。
// 已有行的 id 复用，保证重复 run UUID 稳定。
func (s *Store) ImportWorkflow(ctx context.Context, wf WorkflowRow, instances []NodeInstanceRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	wfID, err := upsertWorkflow(ctx, tx, wf)
	if err != nil {
		return fmt.Errorf("import workflow (%s, %s): %w", wf.Name, wf.Version, err)
	}
	for _, inst := range instances {
		inst.WorkflowID = wfID
		if err := upsertNodeInstance(ctx, tx, inst); err != nil {
			return fmt.Errorf("import node instance %q: %w", inst.NodeID, err)
		}
	}
	if err := deleteStaleNodeInstances(ctx, tx, wfID, instances); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow import: %w", err)
	}
	return nil
}

func deleteStaleNodeInstances(ctx context.Context, tx *sql.Tx, workflowID string, instances []NodeInstanceRow) error {
	keep := make(map[string]struct{}, len(instances))
	for _, inst := range instances {
		keep[inst.NodeID] = struct{}{}
	}

	rows, err := tx.QueryContext(ctx, `SELECT node_id FROM node_instance WHERE workflow_id = ?`, workflowID)
	if err != nil {
		return fmt.Errorf("list existing node instances: %w", err)
	}
	var stale []string
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan existing node instance: %w", err)
		}
		if _, ok := keep[nodeID]; !ok {
			stale = append(stale, nodeID)
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close existing node instances: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate existing node instances: %w", err)
	}

	for _, nodeID := range stale {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM node_instance WHERE workflow_id = ? AND node_id = ?`, workflowID, nodeID); err != nil {
			return fmt.Errorf("delete stale node instance %q: %w", nodeID, err)
		}
	}
	return nil
}

func upsertNodeType(ctx context.Context, tx *sql.Tx, t NodeTypeDefRow) error {
	id, err := stableID(ctx, tx, t.ID, "node_type_definition", "name", t.Name, "", "")
	if err != nil {
		return err
	}
	reqJSON, err := marshalSlice(t.Requires, "requires_json")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO node_type_definition (id, name, description, requires_json, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
  description   = excluded.description,
  requires_json = excluded.requires_json`,
		id, t.Name, t.Description, string(reqJSON), nowStamp(ctx))
	return err
}

func upsertNodeDef(ctx context.Context, tx *sql.Tx, d NodeDefRow) error {
	id, err := stableID(ctx, tx, d.ID, "node_definition", "name", d.Name, "", "")
	if err != nil {
		return err
	}
	reqJSON, err := marshalSlice(d.Requires, "requires_json")
	if err != nil {
		return err
	}
	inJSON, err := marshalMap(d.Inputs, "inputs_json")
	if err != nil {
		return err
	}
	outJSON, err := marshalMap(d.Outputs, "outputs_json")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO node_definition (id, name, description, type, requires_json, inputs_json, outputs_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
  description   = excluded.description,
  type          = excluded.type,
  requires_json = excluded.requires_json,
  inputs_json   = excluded.inputs_json,
  outputs_json  = excluded.outputs_json`,
		id, d.Name, d.Description, d.Type, string(reqJSON), string(inJSON), string(outJSON), nowStamp(ctx))
	return err
}

func upsertNodeExec(ctx context.Context, tx *sql.Tx, e NodeExecRow) error {
	defID := e.NodeDefinitionID
	if defID == "" {
		if e.Node == "" {
			return fmt.Errorf("node executor: Node (definition name) must not be empty when NodeDefinitionID unset")
		}
		var existing string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM node_definition WHERE name = ?`, e.Node).Scan(&existing)
		if err == sql.ErrNoRows {
			return fmt.Errorf("node executor %q: unknown node definition %q (import definitions before executors)",
				e.Name, e.Node)
		}
		if err != nil {
			return fmt.Errorf("lookup node definition %q: %w", e.Node, err)
		}
		defID = existing
	}

	id, err := stableID(ctx, tx, e.ID, "node_executor", "node_definition_id", defID, "version", e.Version)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO node_executor (id, node_definition_id, version, name, description, updates, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(node_definition_id, version) DO UPDATE SET
  name        = excluded.name,
  description = excluded.description,
  updates     = excluded.updates`,
		id, defID, e.Version, e.Name, e.Description, e.Updates, nowStamp(ctx))
	return err
}

func upsertWorkflow(ctx context.Context, tx *sql.Tx, wf WorkflowRow) (string, error) {
	id, err := stableID(ctx, tx, wf.ID, "workflow", "name", wf.Name, "version", wf.Version)
	if err != nil {
		return "", err
	}
	projJSON, err := marshalSlice(wf.Projects, "projects_json")
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO workflow (id, name, version, description, projects_json, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(name, version) DO UPDATE SET
  description   = excluded.description,
  projects_json = excluded.projects_json`,
		id, wf.Name, wf.Version, wf.Description, string(projJSON), nowStamp(ctx))
	return id, err
}

func upsertNodeInstance(ctx context.Context, tx *sql.Tx, inst NodeInstanceRow) error {
	id, err := stableID(ctx, tx, inst.ID, "node_instance", "workflow_id", inst.WorkflowID, "node_id", inst.NodeID)
	if err != nil {
		return err
	}
	inJSON, err := marshalMap(inst.Inputs, "inputs_json")
	if err != nil {
		return err
	}
	depJSON, err := marshalSlice(inst.DependsOn, "depends_on_json")
	if err != nil {
		return err
	}
	cfgJSON, err := marshalMap(inst.Config, "config_json")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO node_instance (id, workflow_id, node_id, node_definition_id, node_executor_id,
  display_name, description, llm_provider, llm_model, inputs_json, depends_on_json, config_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workflow_id, node_id) DO UPDATE SET
  node_definition_id = excluded.node_definition_id,
  node_executor_id   = excluded.node_executor_id,
  display_name       = excluded.display_name,
  description         = excluded.description,
  llm_provider        = excluded.llm_provider,
  llm_model           = excluded.llm_model,
  inputs_json         = excluded.inputs_json,
  depends_on_json     = excluded.depends_on_json,
  config_json         = excluded.config_json`,
		id, inst.WorkflowID, inst.NodeID, inst.NodeDefinitionID, inst.NodeExecutorID,
		inst.DisplayName, inst.Description, inst.LLMProvider, inst.LLMModel,
		string(inJSON), string(depJSON), string(cfgJSON))
	return err
}

func marshalSlice[T any](value []T, field string) ([]byte, error) {
	if value == nil {
		value = []T{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", field, err)
	}
	return data, nil
}

func marshalMap[V any](value map[string]V, field string) ([]byte, error) {
	if value == nil {
		value = map[string]V{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", field, err)
	}
	return data, nil
}

func stableID(ctx context.Context, tx *sql.Tx, provided, table, keyCol1, keyVal1, keyCol2, keyVal2 string) (string, error) {
	if provided != "" {
		return provided, nil
	}
	existing, err := selectID(ctx, tx, table, keyCol1, keyVal1, keyCol2, keyVal2)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}
	return uuid.NewString(), nil
}

func selectID(ctx context.Context, tx *sql.Tx, table, keyCol1, keyVal1, keyCol2, keyVal2 string) (string, error) {
	q := fmt.Sprintf(`SELECT id FROM %s WHERE %s = ?`, table, keyCol1)
	args := []any{keyVal1}
	if keyCol2 != "" {
		q = fmt.Sprintf(`SELECT id FROM %s WHERE %s = ? AND %s = ?`, table, keyCol1, keyCol2)
		args = append(args, keyVal2)
	}
	var id string
	err := tx.QueryRowContext(ctx, q, args...).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookup existing id in %s: %w", table, err)
	}
	return id, nil
}

func nowStamp(ctx context.Context) string {
	if fn, ok := ctx.Value(nowKey{}).(nowFunc); ok && fn != nil {
		return fn().UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

type nowKey struct{}

func withNow(ctx context.Context, fn nowFunc) context.Context {
	return context.WithValue(ctx, nowKey{}, fn)
}
