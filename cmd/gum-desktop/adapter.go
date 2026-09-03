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
	startupView product.WorkspaceView
	startupErr  error
	startupDone bool
}

func newDesktopAdapter(application product.WorkflowApplication) *DesktopAdapter {
	return &DesktopAdapter{
		application: application,
		ctx:         context.Background(),
	}
}

func (a *DesktopAdapter) startup(ctx context.Context) {
	a.ctx = ctx
	a.startupView, a.startupErr = a.application.OpenWorkspace(ctx)
	a.startupDone = true
}

// OpenWorkspace forwards the product-shell action through WorkflowApplication.
func (a *DesktopAdapter) OpenWorkspace() (product.WorkspaceView, error) {
	if a.startupDone {
		return a.startupView, a.startupErr
	}
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

// StartRun forwards the visible Draft token through WorkflowApplication.
func (a *DesktopAdapter) StartRun(input product.StartRunInput) (product.RunView, error) {
	return a.application.StartRun(a.ctx, input)
}

// ListRevisions forwards the Product Workflow Revision history list.
func (a *DesktopAdapter) ListRevisions(workflowID string) ([]product.RevisionView, error) {
	return a.application.ListRevisions(a.ctx, workflowID)
}

// ListRevisionRuns forwards the Run list for one Revision.
func (a *DesktopAdapter) ListRevisionRuns(revisionID string) ([]product.RunSummaryView, error) {
	return a.application.ListRevisionRuns(a.ctx, revisionID)
}

// GetRunHistory forwards one historical Run detail query.
func (a *DesktopAdapter) GetRunHistory(runID string) (product.RunView, error) {
	return a.application.GetRunHistory(a.ctx, runID)
}

// GenerateDiagnosticsBundle forwards an explicit crash bundle generation.
func (a *DesktopAdapter) GenerateDiagnosticsBundle(runID string) (product.DiagnosticsBundleView, error) {
	return a.application.GenerateDiagnosticsBundle(a.ctx, runID)
}

// ListNodeCatalog forwards the registered product Node Catalog.
func (a *DesktopAdapter) ListNodeCatalog() ([]nodecatalog.Entry, error) {
	return a.application.ListNodeCatalog(a.ctx)
}

// GetLLMSettings forwards the active Provider -> Models settings query.
func (a *DesktopAdapter) GetLLMSettings() (product.LLMSettingsView, error) {
	return a.application.GetLLMSettings(a.ctx)
}

// CreateLLMProvider forwards Provider creation through WorkflowApplication.
func (a *DesktopAdapter) CreateLLMProvider(input product.CreateLLMProviderInput) (product.LLMProviderView, error) {
	return a.application.CreateLLMProvider(a.ctx, input)
}

// UpdateLLMProvider forwards Provider editing through WorkflowApplication.
func (a *DesktopAdapter) UpdateLLMProvider(input product.UpdateLLMProviderInput) (product.LLMProviderView, error) {
	return a.application.UpdateLLMProvider(a.ctx, input)
}

// DeleteLLMProvider forwards Provider deletion through WorkflowApplication.
func (a *DesktopAdapter) DeleteLLMProvider(input product.DeleteLLMProviderInput) error {
	return a.application.DeleteLLMProvider(a.ctx, input)
}

// SetDefaultLLMProvider forwards explicit Provider default selection.
func (a *DesktopAdapter) SetDefaultLLMProvider(providerID string) (product.LLMSettingsView, error) {
	return a.application.SetDefaultLLMProvider(a.ctx, providerID)
}

// CreateLLMModel forwards Model Slot creation through WorkflowApplication.
func (a *DesktopAdapter) CreateLLMModel(input product.CreateLLMModelInput) (product.LLMModelView, error) {
	return a.application.CreateLLMModel(a.ctx, input)
}

// UpdateLLMModel forwards Model Slot editing through WorkflowApplication.
func (a *DesktopAdapter) UpdateLLMModel(input product.UpdateLLMModelInput) (product.LLMModelView, error) {
	return a.application.UpdateLLMModel(a.ctx, input)
}

// DeleteLLMModel forwards Model Slot deletion through WorkflowApplication.
func (a *DesktopAdapter) DeleteLLMModel(providerID, modelID string) error {
	return a.application.DeleteLLMModel(a.ctx, providerID, modelID)
}

// ListModelDeletionImpact previews the Workflows referencing one Model Slot.
func (a *DesktopAdapter) ListModelDeletionImpact(providerID, modelID string) (product.AffectedWorkflowsView, error) {
	return a.application.ListModelDeletionImpact(a.ctx, providerID, modelID)
}

// ListProviderDeletionImpact previews the Model Slots and Workflows affected
// by removing one Provider.
func (a *DesktopAdapter) ListProviderDeletionImpact(providerID string) (product.AffectedWorkflowsView, error) {
	return a.application.ListProviderDeletionImpact(a.ctx, providerID)
}

// SetDefaultLLMModel forwards explicit Model default selection.
func (a *DesktopAdapter) SetDefaultLLMModel(providerID, modelID string) (product.LLMSettingsView, error) {
	return a.application.SetDefaultLLMModel(a.ctx, providerID, modelID)
}
