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

func TestValidateAcceptsComplexityDefaultsAndConfiguredPolicy(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
	}{
		{name: "defaults"},
		{name: "configured", config: map[string]any{
			"maximumCyclomaticComplexity": 20,
			"includeTests":                true,
			"excludeGeneratedFiles":       false,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := complexityWorkflowDefinition()
			check := definition.Nodes["check"]
			check.Config = tt.config
			definition.Nodes["check"] = check
			if warnings, err := newComplexityValidator(t).Validate(definition); err != nil {
				t.Fatalf("Validate() = warnings %v, error %v", warnings, err)
			}
		})
	}
}

func TestValidateRejectsInvalidComplexityPolicy(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		field  string
	}{
		{name: "zero maximum", config: map[string]any{"maximumCyclomaticComplexity": 0}, field: "maximumCyclomaticComplexity"},
		{name: "fractional maximum", config: map[string]any{"maximumCyclomaticComplexity": 15.5}, field: "maximumCyclomaticComplexity"},
		{name: "string maximum", config: map[string]any{"maximumCyclomaticComplexity": "15"}, field: "maximumCyclomaticComplexity"},
		{name: "non boolean tests", config: map[string]any{"includeTests": "true"}, field: "includeTests"},
		{name: "non boolean generated", config: map[string]any{"excludeGeneratedFiles": 1}, field: "excludeGeneratedFiles"},
		{name: "unknown field", config: map[string]any{"packageScope": "./pkg/..."}, field: "packageScope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := complexityWorkflowDefinition()
			check := definition.Nodes["check"]
			check.Config = tt.config
			definition.Nodes["check"] = check
			_, err := newComplexityValidator(t).Validate(definition)
			if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate() error = %v, want node/field policy rejection", err)
			}
		})
	}
}

func newComplexityValidator(t *testing.T) *SemanticValidator {
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

func complexityWorkflowDefinition() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "complexity"},
		Projects: []workflow.ProjectSpec{{Name: "project", Repository: "."}},
		Nodes: map[string]workflow.NodeSpec{
			"entry": {Node: "human-input"},
			"check": {
				Node: "go-complexity-check", DependsOn: []string{"entry"},
				Inputs: map[string]workflow.InputBinding{"code": {From: "project.code"}},
			},
		},
	}
}
