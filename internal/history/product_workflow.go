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
