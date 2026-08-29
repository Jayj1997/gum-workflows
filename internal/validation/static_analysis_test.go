package validation

import (
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func TestValidateAcceptsStaticAnalysisProjectCodeContract(t *testing.T) {
	executors := node.NewExecutorRegistry()
	if err := builtins.RegisterAll(executors); err != nil {
		t.Fatal(err)
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	validator := NewSemanticValidator(executors, definitions, artifact.NewRegistry())
	definition := staticWorkflowDefinition()
	if warnings, err := validator.Validate(definition); err != nil {
		t.Fatalf("Validate() = warnings %v, error %v", warnings, err)
	}
}

func TestValidateRejectsStaticAnalysisConfigOverride(t *testing.T) {
	executors := node.NewExecutorRegistry()
	if err := builtins.RegisterAll(executors); err != nil {
		t.Fatal(err)
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definition := staticWorkflowDefinition()
	check := definition.Nodes["check"]
	check.Config = map[string]any{"script": "go vet ./pkg/..."}
	definition.Nodes["check"] = check
	_, err = NewSemanticValidator(executors, definitions, artifact.NewRegistry()).Validate(definition)
	if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), "no fields are supported") {
		t.Fatalf("Validate() error = %v, want node/field config rejection", err)
	}
}

func TestValidateDiagnosesMissingStaticAnalysisHostExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	executors := node.NewExecutorRegistry()
	if err := builtins.RegisterAll(executors); err != nil {
		t.Fatal(err)
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewSemanticValidator(executors, definitions, artifact.NewRegistry()).Validate(staticWorkflowDefinition())
	if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), "required executable") {
		t.Fatalf("Validate() error = %v, want node-scoped host executable diagnostic", err)
	}
}

func staticWorkflowDefinition() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "static-analysis"},
		Projects: []workflow.ProjectSpec{{Name: "project", Repository: "."}},
		Nodes: map[string]workflow.NodeSpec{
			"entry": {Node: "human-input"},
			"check": {
				Node: "go-static-analysis", DependsOn: []string{"entry"},
				Inputs: map[string]workflow.InputBinding{"code": {From: "project.code"}},
			},
		},
	}
}
