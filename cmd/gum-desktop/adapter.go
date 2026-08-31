package main

import (
	"context"

	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
)

// DesktopAdapter is the only object bound into the desktop WebView. Keeping
// the binding narrow prevents UI code from reaching storage or runtime APIs.
type DesktopAdapter struct {
	application product.WorkflowApplication
	ctx         context.Context
}

func newDesktopAdapter(application product.WorkflowApplication) *DesktopAdapter {
	return &DesktopAdapter{
		application: application,
		ctx:         context.Background(),
	}
}

func (a *DesktopAdapter) startup(ctx context.Context) {
	a.ctx = ctx
}

// OpenWorkspace forwards the product-shell action through WorkflowApplication.
func (a *DesktopAdapter) OpenWorkspace() (product.WorkspaceView, error) {
	return a.application.OpenWorkspace(a.ctx)
}

// CreateWorkflow forwards Product Workflow creation through WorkflowApplication.
func (a *DesktopAdapter) CreateWorkflow(input product.CreateWorkflowInput) (product.WorkflowView, error) {
	return a.application.CreateWorkflow(a.ctx, input)
}

// ListWorkflows forwards Product Workflow listing through WorkflowApplication.
func (a *DesktopAdapter) ListWorkflows() ([]product.WorkflowView, error) {
	return a.application.ListWorkflows(a.ctx)
}

// GetDraft forwards Product Workflow Draft loading through WorkflowApplication.
func (a *DesktopAdapter) GetDraft(workflowID string) (product.DraftView, error) {
	return a.application.GetDraft(a.ctx, workflowID)
}

// UpdateDraft forwards an autosave request through WorkflowApplication.
func (a *DesktopAdapter) UpdateDraft(input product.UpdateDraftInput) (product.DraftUpdateView, error) {
	return a.application.UpdateDraft(a.ctx, input)
}

// ListNodeCatalog forwards the registered product Node Catalog.
func (a *DesktopAdapter) ListNodeCatalog() ([]nodecatalog.Entry, error) {
	return a.application.ListNodeCatalog(a.ctx)
}
