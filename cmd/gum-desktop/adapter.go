package main

import (
	"context"

	"github.com/Jayj1997/gum-workflows/internal/product"
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
