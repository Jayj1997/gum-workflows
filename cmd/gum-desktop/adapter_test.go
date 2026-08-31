package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/product"
)

type applicationStub struct {
	view   product.WorkspaceView
	err    error
	called bool
}

func (s *applicationStub) OpenWorkspace(context.Context) (product.WorkspaceView, error) {
	s.called = true
	return s.view, s.err
}

func TestDesktopAdapterUsesWorkflowApplication(t *testing.T) {
	t.Parallel()

	application := &applicationStub{view: product.WorkspaceView{
		Title:   "Gum Workflows",
		Message: "desktop result",
	}}
	adapter := newDesktopAdapter(application)

	view, err := adapter.OpenWorkspace()
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	if !application.called {
		t.Fatal("workflow application was not called")
	}
	if view != application.view {
		t.Fatalf("view = %#v, want %#v", view, application.view)
	}
}

func TestDesktopAdapterReturnsApplicationError(t *testing.T) {
	t.Parallel()

	want := errors.New("application unavailable")
	adapter := newDesktopAdapter(&applicationStub{err: want})

	_, err := adapter.OpenWorkspace()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
