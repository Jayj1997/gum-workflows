// Package workflow defines the SQLite-backed Product Workflow identity model.
// It is separate from the YAML workflow/v1 definition package.
package workflow

import (
	"context"
	"time"
)

// Workflow is the stable identity and display metadata of a Product Workflow.
type Workflow struct {
	ID          string
	DisplayName string
	CreatedAt   time.Time
}

// Repository persists Product Workflows independently from workflow/v1 imports.
type Repository interface {
	CreateProductWorkflow(ctx context.Context, displayName string) (Workflow, error)
	ListProductWorkflows(ctx context.Context) ([]Workflow, error)
}
