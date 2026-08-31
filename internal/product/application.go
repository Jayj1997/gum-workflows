// Package product defines the application boundary used by product-facing UI
// adapters. It is deliberately separate from the workflow/v1 CLI.
package product

import "context"

// WorkspaceView is the product shell state returned to a UI adapter.
type WorkspaceView struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// WorkflowApplication is the product use-case boundary consumed by UI adapters.
type WorkflowApplication interface {
	OpenWorkspace(ctx context.Context) (WorkspaceView, error)
}

// FakeApplication is the first product tracer implementation. Later tickets
// replace it behind WorkflowApplication without changing either UI adapter.
type FakeApplication struct{}

// NewFakeApplication creates the deterministic product tracer application.
func NewFakeApplication() *FakeApplication {
	return &FakeApplication{}
}

// OpenWorkspace completes the initial UI-to-application round-trip.
func (*FakeApplication) OpenWorkspace(context.Context) (WorkspaceView, error) {
	return WorkspaceView{
		Title:   "Gum Workflows",
		Message: "Product application round-trip complete",
	}, nil
}
