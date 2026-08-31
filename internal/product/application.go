// Package product defines the application boundary used by product-facing UI
// adapters. It is deliberately separate from the workflow/v1 CLI.
package product

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
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
	GetDraft(ctx context.Context, workflowID string) (DraftView, error)
	UpdateDraft(ctx context.Context, input UpdateDraftInput) (DraftUpdateView, error)
	ListNodeCatalog(ctx context.Context) ([]nodecatalog.Entry, error)
}

// DraftView is the current mutable Product Workflow definition returned to UI adapters.
type DraftView struct {
	WorkflowID  string           `json:"workflowId"`
	Content     map[string]any   `json:"content"`
	LockVersion uint64           `json:"lockVersion"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	Preview     *WorkflowPreview `json:"preview,omitempty"`
}

// UpdateDraftInput is an autosave request against the UI's current lock token.
type UpdateDraftInput struct {
	WorkflowID          string         `json:"workflowId"`
	ExpectedLockVersion uint64         `json:"expectedLockVersion"`
	Content             map[string]any `json:"content"`
}

// Diagnostic identifies one problem in an incomplete Product Workflow Draft.
type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// WorkflowPreview is the renderer-independent structure derived from a Draft.
type WorkflowPreview struct {
	Nodes       []any        `json:"nodes"`
	Edges       []any        `json:"edges"`
	Groups      []any        `json:"groups"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// DraftUpdateView returns autosave state and the complete latest Draft projection.
type DraftUpdateView struct {
	Draft           DraftView       `json:"draft"`
	Preview         WorkflowPreview `json:"preview"`
	Saved           bool            `json:"saved"`
	Conflict        bool            `json:"conflict"`
	RefreshRequired bool            `json:"refreshRequired"`
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
	catalog    *nodecatalog.Registry
}

// NewApplication creates the Product Application with injected persistence and
// the explicitly assembled product Node Definition/Executor registry.
func NewApplication(repository productworkflow.Repository, catalog *nodecatalog.Registry) *Application {
	return &Application{repository: repository, catalog: catalog}
}

// ListNodeCatalog returns addable Nodes from the registered Definitions and Executors.
func (a *Application) ListNodeCatalog(context.Context) ([]nodecatalog.Entry, error) {
	if a.catalog == nil {
		return nil, fmt.Errorf("list node catalog: registry is not configured")
	}
	return a.catalog.Catalog(), nil
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

// GetDraft returns the current mutable definition for a Product Workflow.
func (a *Application) GetDraft(ctx context.Context, workflowID string) (DraftView, error) {
	draft, err := a.repository.GetProductWorkflowDraft(ctx, workflowID)
	if err != nil {
		return DraftView{}, fmt.Errorf("get draft: %w", err)
	}
	view, err := draftView(draft)
	if err != nil {
		return DraftView{}, err
	}
	preview := a.previewDraft(view.Content)
	view.Preview = &preview
	return view, nil
}

// UpdateDraft autosaves semantic content and returns the latest Preview and Diagnostics.
func (a *Application) UpdateDraft(ctx context.Context, input UpdateDraftInput) (DraftUpdateView, error) {
	if strings.TrimSpace(input.WorkflowID) == "" {
		return DraftUpdateView{}, fmt.Errorf("update draft: workflow ID must not be empty")
	}
	if input.ExpectedLockVersion == 0 {
		return DraftUpdateView{}, fmt.Errorf("update draft: expected lock version must be positive")
	}
	content, err := json.Marshal(input.Content)
	if err != nil {
		return DraftUpdateView{}, fmt.Errorf("update draft: encode content: %w", err)
	}
	update, err := a.repository.UpdateProductWorkflowDraft(ctx, input.WorkflowID, input.ExpectedLockVersion, content)
	if err != nil {
		return DraftUpdateView{}, fmt.Errorf("update draft: %w", err)
	}
	view, err := draftView(update.Draft)
	if err != nil {
		return DraftUpdateView{}, fmt.Errorf("update draft: %w", err)
	}
	return DraftUpdateView{
		Draft:           view,
		Preview:         a.previewDraft(view.Content),
		Saved:           update.Saved,
		Conflict:        update.Conflict,
		RefreshRequired: update.Conflict,
	}, nil
}

func draftView(draft productworkflow.Draft) (DraftView, error) {
	var content map[string]any
	if err := json.Unmarshal(draft.Content, &content); err != nil {
		return DraftView{}, fmt.Errorf("decode draft content: %w", err)
	}
	return DraftView{WorkflowID: draft.WorkflowID, Content: content, LockVersion: draft.LockVersion, UpdatedAt: draft.UpdatedAt}, nil
}

func (a *Application) previewDraft(content map[string]any) WorkflowPreview {
	preview := WorkflowPreview{Nodes: []any{}, Edges: []any{}, Groups: []any{}, Diagnostics: []Diagnostic{}}
	if content["semanticSchemaVersion"] != "productWorkflow/v1" {
		preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
			Code: "invalid-semantic-schema-version", Severity: "error", Path: "semanticSchemaVersion",
			Message: "semantic schema version must be productWorkflow/v1",
		})
	}
	nodes, ok := content["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
			Code: "workflow-needs-node", Severity: "error", Path: "nodes",
			Message: "workflow must contain at least one node",
		})
		return preview
	}
	preview.Nodes = append(preview.Nodes, nodes...)
	for index, value := range nodes {
		node, ok := value.(map[string]any)
		if !ok {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "invalid-node", Severity: "error", Path: fmt.Sprintf("nodes[%d]", index), Message: "node must be an object",
			})
			continue
		}
		definitionID, _ := node["definition"].(string)
		definition, found := a.catalog.Definition(definitionID)
		if !found {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "unknown-node-definition", Severity: "error", Path: fmt.Sprintf("nodes[%d].definition", index),
				Message: fmt.Sprintf("node definition %q is not in the Catalog", definitionID),
			})
			continue
		}
		config, ok := node["config"].(map[string]any)
		if !ok {
			if node["config"] == nil {
				config = map[string]any{}
			} else {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "invalid-node-config", Severity: "error", Path: fmt.Sprintf("nodes[%d].config", index), Message: "config must be an object",
				})
				continue
			}
		}
		for _, issue := range definition.Config.Validate(config) {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "invalid-node-config-" + issue.Code, Severity: "error",
				Path: fmt.Sprintf("nodes[%d].config.%s", index, issue.Field), Message: issue.Message,
			})
		}
	}
	return preview
}

func workflowView(workflow productworkflow.Workflow) WorkflowView {
	return WorkflowView{
		ID:          workflow.ID,
		DisplayName: workflow.DisplayName,
		CreatedAt:   workflow.CreatedAt,
	}
}
