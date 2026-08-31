// Package product defines the application boundary used by product-facing UI
// adapters. It is deliberately separate from the workflow/v1 CLI.
package product

import (
	"context"
	"fmt"
	"strings"
	"time"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// WorkspaceView is the product shell state returned to a UI adapter.
type WorkspaceView struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// WorkflowApplication is the product use-case boundary consumed by UI adapters.
type WorkflowApplication interface {
	OpenWorkspace(ctx context.Context) (WorkspaceView, error)
	CreateWorkflow(ctx context.Context, input CreateWorkflowInput) (WorkflowView, error)
	ListWorkflows(ctx context.Context) ([]WorkflowView, error)
}

// CreateWorkflowInput is the user-authored metadata for a new Product Workflow.
type CreateWorkflowInput struct {
	DisplayName string `json:"displayName"`
}

// WorkflowView is Product Workflow identity and display metadata returned to UI adapters.
type WorkflowView struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Application coordinates Product Workflow use cases for UI adapters.
type Application struct {
	repository productworkflow.Repository
}

// NewApplication creates the Product Application with an injected repository.
func NewApplication(repository productworkflow.Repository) *Application {
	return &Application{repository: repository}
}

// OpenWorkspace returns the product shell state.
func (*Application) OpenWorkspace(context.Context) (WorkspaceView, error) {
	return WorkspaceView{Title: "Gum Workflows", Message: "Product workspace ready"}, nil
}

// CreateWorkflow creates a SQLite Product Workflow through the repository seam.
func (a *Application) CreateWorkflow(ctx context.Context, input CreateWorkflowInput) (WorkflowView, error) {
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		return WorkflowView{}, fmt.Errorf("create workflow: display name must not be empty")
	}
	workflow, err := a.repository.CreateProductWorkflow(ctx, displayName)
	if err != nil {
		return WorkflowView{}, fmt.Errorf("create workflow: %w", err)
	}
	return workflowView(workflow), nil
}

// ListWorkflows lists SQLite Product Workflows in repository-defined stable order.
func (a *Application) ListWorkflows(ctx context.Context) ([]WorkflowView, error) {
	workflows, err := a.repository.ListProductWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	views := make([]WorkflowView, 0, len(workflows))
	for _, workflow := range workflows {
		views = append(views, workflowView(workflow))
	}
	return views, nil
}

func workflowView(workflow productworkflow.Workflow) WorkflowView {
	return WorkflowView{
		ID:          workflow.ID,
		DisplayName: workflow.DisplayName,
		CreatedAt:   workflow.CreatedAt,
	}
}
