package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/product"
)

type applicationStub struct {
	view         product.WorkspaceView
	workflow     product.WorkflowView
	workflows    []product.WorkflowView
	createInput  product.CreateWorkflowInput
	err          error
	openCalled   bool
	createCalled bool
	listCalled   bool
}

func (s *applicationStub) OpenWorkspace(context.Context) (product.WorkspaceView, error) {
	s.openCalled = true
	return s.view, s.err
}

func (s *applicationStub) CreateWorkflow(_ context.Context, input product.CreateWorkflowInput) (product.WorkflowView, error) {
	s.createCalled = true
	s.createInput = input
	return s.workflow, s.err
}

func (s *applicationStub) ListWorkflows(context.Context) ([]product.WorkflowView, error) {
	s.listCalled = true
	return s.workflows, s.err
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
	if !application.openCalled {
		t.Fatal("workflow application was not called")
	}
	if view != application.view {
		t.Fatalf("view = %#v, want %#v", view, application.view)
	}
}

func TestDesktopAdapterCreatesAndListsThroughWorkflowApplication(t *testing.T) {
	t.Parallel()

	want := product.WorkflowView{ID: "0198fb41-43d2-7e2b-a4cd-2bc5f7889ff9", DisplayName: "Release checklist"}
	application := &applicationStub{workflow: want, workflows: []product.WorkflowView{want}}
	adapter := newDesktopAdapter(application)

	created, err := adapter.CreateWorkflow(product.CreateWorkflowInput{DisplayName: want.DisplayName})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	listed, err := adapter.ListWorkflows()
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if !application.createCalled || application.createInput.DisplayName != want.DisplayName {
		t.Fatalf("create call = %#v", application.createInput)
	}
	if !application.listCalled {
		t.Fatal("list workflows did not call WorkflowApplication")
	}
	if created != want || len(listed) != 1 || listed[0] != want {
		t.Fatalf("created/listed = %#v/%#v, want %#v", created, listed, want)
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
