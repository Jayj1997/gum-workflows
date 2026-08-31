package history

import (
	"context"
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
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO product_workflow (id, display_name, created_at)
VALUES (?, ?, ?)`, workflow.ID, workflow.DisplayName, workflow.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return productworkflow.Workflow{}, fmt.Errorf("create product workflow: %w", err)
	}
	return workflow, nil
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
