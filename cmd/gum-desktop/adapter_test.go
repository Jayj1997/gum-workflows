package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/product"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
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
	draft        product.DraftView
	update       product.DraftUpdateView
	updateInput  product.UpdateDraftInput
	catalog      []nodecatalog.Entry
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

func (s *applicationStub) GetDraft(context.Context, string) (product.DraftView, error) {
	return s.draft, s.err
}

func (s *applicationStub) UpdateDraft(_ context.Context, input product.UpdateDraftInput) (product.DraftUpdateView, error) {
	s.updateInput = input
	return s.update, s.err
}

func (s *applicationStub) ListNodeCatalog(context.Context) ([]nodecatalog.Entry, error) {
	return s.catalog, s.err
}

func (s *applicationStub) GetLLMSettings(context.Context) (product.LLMSettingsView, error) {
	return product.LLMSettingsView{}, s.err
}
func (s *applicationStub) CreateLLMProvider(context.Context, product.CreateLLMProviderInput) (product.LLMProviderView, error) {
	return product.LLMProviderView{}, s.err
}
func (s *applicationStub) UpdateLLMProvider(context.Context, product.UpdateLLMProviderInput) (product.LLMProviderView, error) {
	return product.LLMProviderView{}, s.err
}
func (s *applicationStub) DeleteLLMProvider(context.Context, string) error { return s.err }
func (s *applicationStub) SetDefaultLLMProvider(context.Context, string) (product.LLMSettingsView, error) {
	return product.LLMSettingsView{}, s.err
}
func (s *applicationStub) CreateLLMModel(context.Context, product.CreateLLMModelInput) (product.LLMModelView, error) {
	return product.LLMModelView{}, s.err
}
func (s *applicationStub) UpdateLLMModel(context.Context, product.UpdateLLMModelInput) (product.LLMModelView, error) {
	return product.LLMModelView{}, s.err
}
func (s *applicationStub) DeleteLLMModel(context.Context, string, string) error { return s.err }
func (s *applicationStub) SetDefaultLLMModel(context.Context, string, string) (product.LLMSettingsView, error) {
	return product.LLMSettingsView{}, s.err
}
func (s *applicationStub) ResolveDefaultLLMModel(context.Context) (product.ResolvedLLMModelView, error) {
	return product.ResolvedLLMModelView{}, s.err
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

func TestDesktopAdapterListsRegisteredNodeCatalog(t *testing.T) {
	t.Parallel()

	want := []nodecatalog.Entry{{
		Definition: nodecatalog.Definition{ID: "llm-chat", DisplayName: "LLM chat"},
		Executor:   nodecatalog.Executor{DefinitionID: "llm-chat", Version: "v1"},
	}}
	adapter := newDesktopAdapter(&applicationStub{catalog: want})
	got, err := adapter.ListNodeCatalog()
	if err != nil {
		t.Fatalf("list Node Catalog: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Node Catalog = %#v, want %#v", got, want)
	}
}

func TestDesktopAdapterLoadsAndAutosavesDraftThroughWorkflowApplication(t *testing.T) {
	t.Parallel()

	draft := product.DraftView{WorkflowID: "workflow-id", LockVersion: 7, Content: map[string]any{"nodes": []any{}}}
	update := product.DraftUpdateView{Draft: product.DraftView{WorkflowID: "workflow-id", LockVersion: 8}, Saved: true}
	application := &applicationStub{draft: draft, update: update}
	adapter := newDesktopAdapter(application)

	loaded, err := adapter.GetDraft("workflow-id")
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	input := product.UpdateDraftInput{WorkflowID: "workflow-id", ExpectedLockVersion: 7, Content: map[string]any{"nodes": []any{}}}
	saved, err := adapter.UpdateDraft(input)
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if !reflect.DeepEqual(loaded, draft) || !reflect.DeepEqual(saved, update) || !reflect.DeepEqual(application.updateInput, input) {
		t.Fatalf("loaded/saved/input = %#v/%#v/%#v", loaded, saved, application.updateInput)
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
