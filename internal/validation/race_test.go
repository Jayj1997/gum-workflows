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

func TestValidateRejectsRaceConfigOutsideClosedContract(t *testing.T) {
	definition := raceWorkflowDefinition()
	check := definition.Nodes["check"]
	check.Config = map[string]any{"packageScope": "./pkg/..."}
	definition.Nodes["check"] = check
	_, err := newRaceValidator(t).Validate(definition)
	if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), "no fields are supported") {
		t.Fatalf("Validate() error = %v, want node-scoped closed config rejection", err)
	}
}

func TestValidateDiagnosesMissingRaceHostExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := newRaceValidator(t).Validate(raceWorkflowDefinition())
	if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), "required executable") {
		t.Fatalf("Validate() error = %v, want node-scoped host executable diagnostic", err)
	}
}

func newRaceValidator(t *testing.T) *SemanticValidator {
	t.Helper()
	executors := node.NewExecutorRegistry()
	if err := builtins.RegisterAll(executors); err != nil {
		t.Fatal(err)
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return NewSemanticValidator(executors, definitions, artifact.NewRegistry())
}

func raceWorkflowDefinition() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "race"},
		Projects: []workflow.ProjectSpec{{Name: "project", Repository: "."}},
		Nodes: map[string]workflow.NodeSpec{
			"entry": {Node: "human-input"},
			"check": {
				Node: "go-race-check", DependsOn: []string{"entry"},
				Inputs: map[string]workflow.InputBinding{"code": {From: "project.code"}},
			},
		},
	}
}
