package product_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/product"
)

func TestApplicationCreatesAndListsSQLiteWorkflowsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "product.db")

	store, err := history.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	application := product.NewApplication(store)

	first, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Release checklist"})
	if err != nil {
		t.Fatalf("create first workflow: %v", err)
	}
	second, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "Incident review"})
	if err != nil {
		t.Fatalf("create second workflow: %v", err)
	}
	for _, workflow := range []product.WorkflowView{first, second} {
		if _, err := uuid.Parse(workflow.ID); err != nil {
			t.Errorf("workflow ID %q is not a UUID: %v", workflow.ID, err)
		}
	}

	want, err := application.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close product database: %v", err)
	}

	reopened, err := history.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen product database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := product.NewApplication(reopened).ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list workflows after restart: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workflows after restart = %#v, want %#v", got, want)
	}
	if len(got) != 2 {
		t.Fatalf("workflow count = %d, want 2", len(got))
	}
	if got[0].DisplayName != "Release checklist" || got[1].DisplayName != "Incident review" {
		t.Fatalf("workflow order = %#v, want creation order", got)
	}
}

func TestApplicationDoesNotListWorkflowV1Imports(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ImportWorkflow(ctx, history.WorkflowRow{Name: "yaml-workflow", Version: "v1"}, nil); err != nil {
		t.Fatalf("import workflow/v1 definition: %v", err)
	}

	workflows, err := product.NewApplication(store).ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list Product Workflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("Product Workflows = %#v, want workflow/v1 import excluded", workflows)
	}
}

func TestApplicationRejectsBlankWorkflowNameWithoutCreatingWorkflow(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "product.db"))
	if err != nil {
		t.Fatalf("open product database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	application := product.NewApplication(store)

	if _, err := application.CreateWorkflow(ctx, product.CreateWorkflowInput{DisplayName: "  "}); err == nil {
		t.Fatal("create blank workflow = nil error, want rejection")
	}
	workflows, err := application.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("workflow count = %d, want zero", len(workflows))
	}
}
