package history

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// CreateProductWorkflow creates a Product Workflow with an immutable UUID.
func (s *Store) CreateProductWorkflow(ctx context.Context, displayName string) (productworkflow.Workflow, error) {
	workflow := productworkflow.Workflow{
		ID:          uuid.NewString(),
		DisplayName: displayName,
		CreatedAt:   time.Now().UTC(),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return productworkflow.Workflow{}, fmt.Errorf("begin create product workflow: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO product_workflow (id, display_name, created_at)
VALUES (?, ?, ?)`, workflow.ID, workflow.DisplayName, workflow.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return productworkflow.Workflow{}, fmt.Errorf("create product workflow: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO product_workflow_draft (workflow_id, content_json, lock_version, updated_at)
VALUES (?, ?, 1, ?)`, workflow.ID, string(productworkflow.InitialDraftContent()), workflow.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return productworkflow.Workflow{}, fmt.Errorf("create product workflow draft: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return productworkflow.Workflow{}, fmt.Errorf("commit create product workflow: %w", err)
	}
	return workflow, nil
}

// UpdateProductWorkflowDraft atomically applies one semantic autosave using CAS.
func (s *Store) UpdateProductWorkflowDraft(ctx context.Context, workflowID string, expectedLockVersion uint64, content json.RawMessage) (productworkflow.DraftUpdate, error) {
	normalized, err := productworkflow.NormalizeDraftContent(content)
	if err != nil {
		return productworkflow.DraftUpdate{}, fmt.Errorf("update product workflow draft %s: %w", workflowID, err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return productworkflow.DraftUpdate{}, fmt.Errorf("begin update product workflow draft %s: %w", workflowID, err)
	}
	defer func() { _ = tx.Rollback() }()

	current, err := scanProductWorkflowDraft(tx.QueryRowContext(ctx, `
SELECT workflow_id, content_json, lock_version, updated_at
FROM product_workflow_draft
WHERE workflow_id = ?`, workflowID), workflowID)
	if err != nil {
		return productworkflow.DraftUpdate{}, err
	}
	currentNormalized, err := productworkflow.NormalizeDraftContent(current.Content)
	if err != nil {
		return productworkflow.DraftUpdate{}, fmt.Errorf("normalize stored product workflow draft %s: %w", workflowID, err)
	}
	if bytes.Equal(currentNormalized, normalized) {
		if err := tx.Commit(); err != nil {
			return productworkflow.DraftUpdate{}, fmt.Errorf("commit product workflow draft no-op %s: %w", workflowID, err)
		}
		return productworkflow.DraftUpdate{Draft: current}, nil
	}
	if current.LockVersion != expectedLockVersion {
		if err := tx.Commit(); err != nil {
			return productworkflow.DraftUpdate{}, fmt.Errorf("commit product workflow draft conflict %s: %w", workflowID, err)
		}
		return productworkflow.DraftUpdate{Draft: current, Conflict: true}, nil
	}

	updatedAt := time.Now().UTC()
	nextVersion := current.LockVersion + 1
	result, err := tx.ExecContext(ctx, `
UPDATE product_workflow_draft
SET content_json = ?, lock_version = ?, updated_at = ?
WHERE workflow_id = ? AND lock_version = ?`, string(normalized), nextVersion, updatedAt.Format(time.RFC3339Nano), workflowID, expectedLockVersion)
	if err != nil {
		return productworkflow.DraftUpdate{}, fmt.Errorf("update product workflow draft %s: %w", workflowID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return productworkflow.DraftUpdate{}, fmt.Errorf("read updated product workflow draft rows %s: %w", workflowID, err)
	}
	if rowsAffected != 1 {
		latest, err := scanProductWorkflowDraft(tx.QueryRowContext(ctx, `
SELECT workflow_id, content_json, lock_version, updated_at
FROM product_workflow_draft
WHERE workflow_id = ?`, workflowID), workflowID)
		if err != nil {
			return productworkflow.DraftUpdate{}, err
		}
		if err := tx.Commit(); err != nil {
			return productworkflow.DraftUpdate{}, fmt.Errorf("commit product workflow draft conflict %s: %w", workflowID, err)
		}
		return productworkflow.DraftUpdate{Draft: latest, Conflict: true}, nil
	}
	if err := tx.Commit(); err != nil {
		return productworkflow.DraftUpdate{}, fmt.Errorf("commit update product workflow draft %s: %w", workflowID, err)
	}
	current.Content = normalized
	current.LockVersion = nextVersion
	current.UpdatedAt = updatedAt
	return productworkflow.DraftUpdate{Draft: current, Saved: true}, nil
}

// GetProductWorkflowDraft returns the single current Draft for a Product Workflow.
func (s *Store) GetProductWorkflowDraft(ctx context.Context, workflowID string) (productworkflow.Draft, error) {
	return scanProductWorkflowDraft(s.db.QueryRowContext(ctx, `
SELECT workflow_id, content_json, lock_version, updated_at
FROM product_workflow_draft
WHERE workflow_id = ?`, workflowID), workflowID)
}

// StartProductWorkflowRun atomically materializes the visible Draft, reuses or
// creates its immutable Revision, and persists one completed Product Run.
func (s *Store) StartProductWorkflowRun(ctx context.Context, request productworkflow.StartRunRequest) (productworkflow.StartRunResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return productworkflow.StartRunResult{}, fmt.Errorf("begin start product workflow Run %s: %w", request.Run.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	draft, err := scanProductWorkflowDraft(tx.QueryRowContext(ctx, `
SELECT workflow_id, content_json, lock_version, updated_at
FROM product_workflow_draft WHERE workflow_id = ?`, request.WorkflowID), request.WorkflowID)
	if err != nil {
		return productworkflow.StartRunResult{}, err
	}
	if draft.LockVersion != request.ExpectedLockVersion {
		return productworkflow.StartRunResult{}, fmt.Errorf("start product workflow Run: Draft lock version conflict: expected %d, current %d", request.ExpectedLockVersion, draft.LockVersion)
	}
	normalized, err := productworkflow.NormalizeDraftContent(request.DraftContent)
	if err != nil {
		return productworkflow.StartRunResult{}, fmt.Errorf("normalize start Run Draft: %w", err)
	}
	stored, err := productworkflow.NormalizeDraftContent(draft.Content)
	if err != nil {
		return productworkflow.StartRunResult{}, fmt.Errorf("normalize stored start Run Draft: %w", err)
	}
	if !bytes.Equal(stored, normalized) {
		draft.Content = normalized
		draft.LockVersion++
		draft.UpdatedAt = time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `
UPDATE product_workflow_draft
SET content_json = ?, lock_version = ?, updated_at = ?
WHERE workflow_id = ? AND lock_version = ?`, string(draft.Content), draft.LockVersion, draft.UpdatedAt.Format(time.RFC3339Nano), request.WorkflowID, request.ExpectedLockVersion); err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("materialize product workflow Draft %s: %w", request.WorkflowID, err)
		}
	}

	revision := request.Revision
	var createdAt, revisionContent string
	err = tx.QueryRowContext(ctx, `
SELECT id, content_json, created_at
FROM product_workflow_revision
WHERE workflow_id = ? AND semantic_hash = ?`, request.WorkflowID, revision.SemanticHash).Scan(&revision.ID, &revisionContent, &createdAt)
	if err == sql.ErrNoRows {
		_, err = tx.ExecContext(ctx, `
INSERT INTO product_workflow_revision (id, workflow_id, semantic_hash, content_json, created_at)
VALUES (?, ?, ?, ?, ?)`, revision.ID, request.WorkflowID, revision.SemanticHash, string(revision.Content), revision.CreatedAt.Format(time.RFC3339Nano))
		if err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("create product workflow Revision: %w", err)
		}
	} else if err != nil {
		return productworkflow.StartRunResult{}, fmt.Errorf("find product workflow Revision: %w", err)
	} else {
		revision.WorkflowID = request.WorkflowID
		revision.Content = []byte(revisionContent)
		revision.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("parse product workflow Revision created_at: %w", err)
		}
	}

	run := request.Run
	run.RevisionID = revision.ID
	run.Snapshot.RevisionID = revision.ID
	snapshotJSON, err := json.Marshal(run.Snapshot)
	if err != nil {
		return productworkflow.StartRunResult{}, fmt.Errorf("encode product workflow Run Snapshot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO product_workflow_run (id, workflow_id, revision_id, status, snapshot_json, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, run.ID, request.WorkflowID, run.RevisionID, run.Status, string(snapshotJSON), run.StartedAt.Format(time.RFC3339Nano), run.FinishedAt.Format(time.RFC3339Nano)); err != nil {
		return productworkflow.StartRunResult{}, fmt.Errorf("create product workflow Run: %w", err)
	}
	for _, nodeRun := range request.NodeRuns {
		inputsJSON, err := json.Marshal(nodeRun.Inputs)
		if err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("encode product Node Run %s inputs: %w", nodeRun.ID, err)
		}
		outputsJSON, err := json.Marshal(nodeRun.Outputs)
		if err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("encode product Node Run %s outputs: %w", nodeRun.ID, err)
		}
		diagnosticsJSON, err := json.Marshal(nodeRun.Diagnostics)
		if err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("encode product Node Run %s diagnostics: %w", nodeRun.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO product_workflow_node_run
  (id, run_id, node_id, node_definition, node_executor, status, inputs_json, outputs_json, diagnostics_json, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, nodeRun.ID, run.ID, nodeRun.NodeID, nodeRun.NodeDefinition, nodeRun.NodeExecutor, nodeRun.Status, string(inputsJSON), string(outputsJSON), string(diagnosticsJSON), nodeRun.StartedAt.Format(time.RFC3339Nano), nodeRun.FinishedAt.Format(time.RFC3339Nano)); err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("create product Node Run %s: %w", nodeRun.ID, err)
		}
	}
	for _, item := range request.Artifacts {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO product_workflow_artifact
  (id, run_id, node_run_id, node_id, port, artifact_type, version, uri, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, run.ID, item.NodeRunID, item.NodeID, item.Port, item.Type, item.Version, item.URI, item.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return productworkflow.StartRunResult{}, fmt.Errorf("create product Artifact %s: %w", item.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return productworkflow.StartRunResult{}, fmt.Errorf("commit product workflow Run %s: %w", run.ID, err)
	}
	return productworkflow.StartRunResult{Draft: draft, Revision: revision, Run: run}, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProductWorkflowDraft(row rowScanner, workflowID string) (productworkflow.Draft, error) {
	var draft productworkflow.Draft
	var contentJSON, updatedAt string
	var lockVersion int64
	err := row.Scan(&draft.WorkflowID, &contentJSON, &lockVersion, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return productworkflow.Draft{}, fmt.Errorf("get product workflow draft %s: not found", workflowID)
		}
		return productworkflow.Draft{}, fmt.Errorf("get product workflow draft %s: %w", workflowID, err)
	}
	draft.Content = []byte(contentJSON)
	draft.LockVersion = uint64(lockVersion)
	draft.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return productworkflow.Draft{}, fmt.Errorf("parse product workflow draft %s updated_at: %w", workflowID, err)
	}
	return draft, nil
}

// ListProductWorkflows returns Product Workflows in stable creation order.
func (s *Store) ListProductWorkflows(ctx context.Context) ([]productworkflow.Workflow, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, display_name, created_at
FROM product_workflow
ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list product workflows: %w", err)
	}
	defer rows.Close()

	workflows := make([]productworkflow.Workflow, 0)
	for rows.Next() {
		var workflow productworkflow.Workflow
		var createdAt string
		if err := rows.Scan(&workflow.ID, &workflow.DisplayName, &createdAt); err != nil {
			return nil, fmt.Errorf("scan product workflow: %w", err)
		}
		workflow.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse product workflow %s created_at: %w", workflow.ID, err)
		}
		workflows = append(workflows, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product workflows: %w", err)
	}
	return workflows, nil
}
