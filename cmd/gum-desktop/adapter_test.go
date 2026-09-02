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
	view                product.WorkspaceView
	workflow            product.WorkflowView
	workflows           []product.WorkflowView
	createInput         product.CreateWorkflowInput
	err                 error
	openCalled          bool
	openCalls           int
	createCalled        bool
	listCalled          bool
	draft               product.DraftView
	update              product.DraftUpdateView
	updateInput         product.UpdateDraftInput
	startInput          product.StartRunInput
	run                 product.RunView
	revisions           []product.RevisionView
	runSummaries        []product.RunSummaryView
	catalog             []nodecatalog.Entry
	providerInput       product.CreateLLMProviderInput
	deleteProviderInput product.DeleteLLMProviderInput
}

func (s *applicationStub) OpenWorkspace(context.Context) (product.WorkspaceView, error) {
	s.openCalled = true
	s.openCalls++
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

func (s *applicationStub) StartRun(_ context.Context, input product.StartRunInput) (product.RunView, error) {
	s.startInput = input
	return s.run, s.err
}

func (s *applicationStub) ListRevisions(context.Context, string) ([]product.RevisionView, error) {
	return s.revisions, s.err
}

func (s *applicationStub) ListRevisionRuns(context.Context, string) ([]product.RunSummaryView, error) {
	return s.runSummaries, s.err
}

func (s *applicationStub) GetRunHistory(context.Context, string) (product.RunView, error) {
	return s.run, s.err
}

func (s *applicationStub) ListNodeCatalog(context.Context) ([]nodecatalog.Entry, error) {
	return s.catalog, s.err
}

func (s *applicationStub) GetLLMSettings(context.Context) (product.LLMSettingsView, error) {
	return product.LLMSettingsView{}, s.err
}
func (s *applicationStub) CreateLLMProvider(_ context.Context, input product.CreateLLMProviderInput) (product.LLMProviderView, error) {
	s.providerInput = input
	return product.LLMProviderView{}, s.err
}
func (s *applicationStub) UpdateLLMProvider(context.Context, product.UpdateLLMProviderInput) (product.LLMProviderView, error) {
	return product.LLMProviderView{}, s.err
}
func (s *applicationStub) DeleteLLMProvider(_ context.Context, input product.DeleteLLMProviderInput) error {
	s.deleteProviderInput = input
	return s.err
}
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
func (s *applicationStub) ListModelDeletionImpact(context.Context, string, string) (product.AffectedWorkflowsView, error) {
	return product.AffectedWorkflowsView{}, s.err
}
func (s *applicationStub) ListProviderDeletionImpact(context.Context, string) (product.AffectedWorkflowsView, error) {
	return product.AffectedWorkflowsView{}, s.err
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

func TestDesktopAdapterInitializesRunRecoveryOnStartup(t *testing.T) {
	t.Parallel()
	application := &applicationStub{view: product.WorkspaceView{Title: "Gum Workflows", Message: "recovered"}}
	adapter := newDesktopAdapter(application)
	adapter.startup(context.Background())
	if application.openCalls != 1 {
		t.Fatalf("startup recovery calls = %d, want one", application.openCalls)
	}
	view, err := adapter.OpenWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if application.openCalls != 1 || view != application.view {
		t.Fatalf("startup recovery calls/view = %d/%#v", application.openCalls, view)
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

func TestDesktopAdapterForwardsProviderSecretInputsThroughApplication(t *testing.T) {
	t.Parallel()

	application := &applicationStub{}
	adapter := newDesktopAdapter(application)
	createInput := product.CreateLLMProviderInput{
		Name: "Primary", Protocol: "openai-chat-completions", BaseURL: "https://api.example/v1", APIKey: "sk-desktop-secret",
	}
	if _, err := adapter.CreateLLMProvider(createInput); err != nil {
		t.Fatalf("create LLM Provider: %v", err)
	}
	deleteInput := product.DeleteLLMProviderInput{ProviderID: "provider-id", Confirmed: true}
	if err := adapter.DeleteLLMProvider(deleteInput); err != nil {
		t.Fatalf("delete LLM Provider: %v", err)
	}
	if !reflect.DeepEqual(application.providerInput, createInput) || !reflect.DeepEqual(application.deleteProviderInput, deleteInput) {
		t.Fatalf("Provider inputs = %#v/%#v, want %#v/%#v", application.providerInput, application.deleteProviderInput, createInput, deleteInput)
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

func TestDesktopAdapterStartsRunThroughWorkflowApplication(t *testing.T) {
	t.Parallel()

	want := product.RunView{ID: "run-id", RevisionID: "revision-id", Status: "succeeded"}
	application := &applicationStub{run: want}
	adapter := newDesktopAdapter(application)
	input := product.StartRunInput{WorkflowID: "workflow-id", ExpectedLockVersion: 7, HumanInput: product.HumanRunInput{NodeID: "prompt", Text: "Hello"}}
	got, err := adapter.StartRun(input)
	if err != nil {
		t.Fatalf("start Run: %v", err)
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(application.startInput, input) {
		t.Fatalf("Run/input = %#v/%#v, want %#v/%#v", got, application.startInput, want, input)
	}
}

func TestDesktopAdapterListsRevisionsRunsAndGetsRunThroughWorkflowApplication(t *testing.T) {
	t.Parallel()

	revisions := []product.RevisionView{{ID: "revision-id", SemanticHash: "abc", RunCount: 2}}
	runSummaries := []product.RunSummaryView{{ID: "run-id", RevisionID: "revision-id", Status: "succeeded"}}
	run := product.RunView{ID: "run-id", RevisionID: "revision-id", Status: "succeeded"}
	adapter := newDesktopAdapter(&applicationStub{revisions: revisions, runSummaries: runSummaries, run: run})

	gotRevisions, err := adapter.ListRevisions("workflow-id")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if !reflect.DeepEqual(gotRevisions, revisions) {
		t.Fatalf("revisions = %#v, want %#v", gotRevisions, revisions)
	}
	gotRuns, err := adapter.ListRevisionRuns("revision-id")
	if err != nil {
		t.Fatalf("list revision runs: %v", err)
	}
	if !reflect.DeepEqual(gotRuns, runSummaries) {
		t.Fatalf("revision runs = %#v, want %#v", gotRuns, runSummaries)
	}
	gotRun, err := adapter.GetRunHistory("run-id")
	if err != nil {
		t.Fatalf("get run history: %v", err)
	}
	if !reflect.DeepEqual(gotRun, run) {
		t.Fatalf("run history = %#v, want %#v", gotRun, run)
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
