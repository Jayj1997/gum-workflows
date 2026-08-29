package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/google/uuid"
)

// Record idempotently writes a full workflow execution snapshot and all node rounds.
func (s *Store) Record(ctx context.Context, exec *execution.WorkflowExecution) error {
	if exec == nil {
		return fmt.Errorf("record run: execution must not be nil")
	}
	if exec.RunID == "" {
		return fmt.Errorf("record run: run id must not be empty")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("record run %s: begin tx: %w", exec.RunID, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO workflow_run_history
  (id, workflow_name, workflow_version, status, workflow_file, error, stopped_reason, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  workflow_name = excluded.workflow_name,
  workflow_version = excluded.workflow_version,
  status = excluded.status,
  workflow_file = excluded.workflow_file,
  error = excluded.error,
  stopped_reason = excluded.stopped_reason,
  started_at = excluded.started_at,
  finished_at = excluded.finished_at`,
		exec.RunID, exec.Workflow, exec.WorkflowVersion, exec.Status, exec.WorkflowFile,
		exec.Error, exec.StoppedReason, formatTime(exec.StartedAt), nullableTime(exec.FinishedAt)); err != nil {
		return fmt.Errorf("record workflow run %s: %w", exec.RunID, err)
	}

	for _, nodeID := range sortedNodeIDs(exec.Nodes) {
		ne := exec.Nodes[nodeID]
		for i := range ne.History {
			if err := recordNodeRun(ctx, tx, exec.RunID, ne, &ne.History[i]); err != nil {
				return err
			}
		}
		if err := recordNodeRun(ctx, tx, exec.RunID, ne, &ne.Current); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("record run %s: commit: %w", exec.RunID, err)
	}
	return nil
}

func recordNodeRun(ctx context.Context, tx *sql.Tx, workflowRunID string, ne *execution.NodeExecution, run *execution.NodeRun) error {
	if run.RunID == "" {
		run.RunID = uuid.NewString()
	}
	round := run.Round
	inputs, err := marshalObject(run.Inputs)
	if err != nil {
		return fmt.Errorf("record node %q round %d inputs: %w", ne.NodeID, round, err)
	}
	outputs, err := marshalObject(run.Outputs)
	if err != nil {
		return fmt.Errorf("record node %q round %d outputs: %w", ne.NodeID, round, err)
	}
	diagnostics, err := marshalObject(run.Diagnostics)
	if err != nil {
		return fmt.Errorf("record node %q round %d diagnostics: %w", ne.NodeID, round, err)
	}
	args := []any{
		workflowRunID, ne.NodeID, ne.NodeDefinition, ne.NodeExecutor, round,
		run.Status, run.Error, run.ErrorKind, inputs, outputs, diagnostics,
		nullableTime(run.StartedAt), nullableTime(run.FinishedAt), run.RunID,
	}
	result, err := tx.ExecContext(ctx, `
UPDATE workflow_node_run_history SET
  run_id = ?,
  node_id = ?,
  node_definition = ?,
  node_executor = ?,
  round = ?,
  status = ?,
  error = ?,
  error_kind = ?,
  inputs_json = ?,
  outputs_json = ?,
  diagnostics_json = ?,
  started_at = ?,
  finished_at = ?
WHERE id = ?`, args...)
	if err != nil {
		return fmt.Errorf("record node %q round %d: update: %w", ne.NodeID, round, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record node %q round %d: rows affected: %w", ne.NodeID, round, err)
	}
	if updated == 1 {
		return nil
	}

	var storedID string
	if err := tx.QueryRowContext(ctx, `
INSERT INTO workflow_node_run_history
  (id, run_id, node_id, node_definition, node_executor, round, status, error, error_kind, inputs_json, outputs_json, diagnostics_json, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id, node_id, round) DO UPDATE SET
  node_definition = excluded.node_definition,
  node_executor = excluded.node_executor,
  status = excluded.status,
  error = excluded.error,
  error_kind = excluded.error_kind,
  inputs_json = excluded.inputs_json,
  outputs_json = excluded.outputs_json,
  diagnostics_json = excluded.diagnostics_json,
  started_at = excluded.started_at,
  finished_at = excluded.finished_at
RETURNING id`,
		run.RunID, workflowRunID, ne.NodeID, ne.NodeDefinition, ne.NodeExecutor, round,
		run.Status, run.Error, run.ErrorKind, inputs, outputs, diagnostics,
		nullableTime(run.StartedAt), nullableTime(run.FinishedAt)).Scan(&storedID); err != nil {
		return fmt.Errorf("record node %q round %d: %w", ne.NodeID, round, err)
	}
	run.RunID = storedID
	return nil
}

func marshalObject(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(data) == "null" {
		return "{}", nil
	}
	return string(data), nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return formatTime(value)
}

func sortedNodeIDs(nodes map[string]*execution.NodeExecution) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
