package history

import (
	"context"
	"testing"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

func TestStoreListsDraftModelReferencesAcrossWorkflows(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t)
	first, err := s.CreateProductWorkflow(ctx, "Release checklist")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateProductWorkflow(ctx, "Incident review")
	if err != nil {
		t.Fatal(err)
	}
	// Only the agent node with a non-empty llm.modelUuid is reported; the
	// human node, empty preference and missing llm key are skipped.
	firstContent := `{"nodes":[{"definition":"human-chat","id":"prompt"},{"definition":"llm-chat","id":"answer","llm":{"modelUuid":"model-a"}}],"semanticSchemaVersion":"productWorkflow/v1"}`
	draft, err := s.GetProductWorkflowDraft(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateProductWorkflowDraft(ctx, first.ID, draft.LockVersion, []byte(firstContent)); err != nil {
		t.Fatal(err)
	}
	secondContent := `{"nodes":[{"definition":"llm-chat","id":"answer","llm":{"modelUuid":"model-b"}},{"definition":"llm-chat","id":"writer"}],"semanticSchemaVersion":"productWorkflow/v1"}`
	secondDraft, err := s.GetProductWorkflowDraft(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateProductWorkflowDraft(ctx, second.ID, secondDraft.LockVersion, []byte(secondContent)); err != nil {
		t.Fatal(err)
	}

	references, err := s.ListProductWorkflowDraftModelReferences(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 2 {
		t.Fatalf("references = %#v, want one per bound agent node", references)
	}
	// References are ordered by (workflow ID, definition, model UUID); both
	// created workflows are random UUIDs, so match by identity, not position.
	got := map[string]productworkflow.WorkflowModelReference{}
	for _, reference := range references {
		got[reference.WorkflowID] = reference
	}
	if reference, ok := got[first.ID]; !ok || reference.NodeDefinition != "llm-chat" || reference.ModelUUID != "model-a" {
		t.Fatalf("first workflow reference = %#v", got[first.ID])
	}
	if reference, ok := got[second.ID]; !ok || reference.NodeDefinition != "llm-chat" || reference.ModelUUID != "model-b" {
		t.Fatalf("second workflow reference = %#v", got[second.ID])
	}
}
