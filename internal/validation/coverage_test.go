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

func TestValidateAcceptsCoverageDefaultAndConfiguredThreshold(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
	}{
		{name: "default threshold"},
		{name: "integer threshold", config: map[string]any{"minimumStatementCoverage": 80}},
		{name: "fractional threshold", config: map[string]any{"minimumStatementCoverage": 74.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := newCoverageValidator(t)
			definition := coverageWorkflowDefinition()
			check := definition.Nodes["check"]
			check.Config = tt.config
			definition.Nodes["check"] = check
			if warnings, err := validator.Validate(definition); err != nil {
				t.Fatalf("Validate() = warnings %v, error %v", warnings, err)
			}
		})
	}
}

func TestValidateRejectsInvalidCoverageThreshold(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "string", value: "80"},
		{name: "negative", value: -0.1},
		{name: "over one hundred", value: 100.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := coverageWorkflowDefinition()
			check := definition.Nodes["check"]
			check.Config = map[string]any{"minimumStatementCoverage": tt.value}
			definition.Nodes["check"] = check
			_, err := newCoverageValidator(t).Validate(definition)
			if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), "minimumStatementCoverage") {
				t.Fatalf("Validate() error = %v, want node/field threshold rejection", err)
			}
		})
	}
}

func TestValidateRejectsCoverageConfigOutsideClosedContract(t *testing.T) {
	definition := coverageWorkflowDefinition()
	check := definition.Nodes["check"]
	check.Config = map[string]any{"packageScope": "./pkg/..."}
	definition.Nodes["check"] = check
	_, err := newCoverageValidator(t).Validate(definition)
	if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), "packageScope") {
		t.Fatalf("Validate() error = %v, want closed config rejection", err)
	}
}

func TestValidateDiagnosesMissingCoverageHostExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := newCoverageValidator(t).Validate(coverageWorkflowDefinition())
	if err == nil || !strings.Contains(err.Error(), `node "check"`) || !strings.Contains(err.Error(), "required executable") {
		t.Fatalf("Validate() error = %v, want node-scoped host executable diagnostic", err)
	}
}

func TestCoverageThresholdUsesCompleteValidationPipeline(t *testing.T) {
	tests := []struct {
		name      string
		threshold string
		wantError bool
	}{
		{name: "fractional", threshold: "74.5"},
		{name: "non numeric", threshold: `"80"`, wantError: true},
		{name: "out of range", threshold: "101", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: coverage\nprojects:\n  - name: project\n    repository: .\nnodes:\n  entry:\n    node: human-input\n  check:\n    node: go-coverage-check\n    dependsOn: [entry]\n    inputs:\n      code:\n        from: project.code\n    config:\n      minimumStatementCoverage: " + tt.threshold + "\n")
			if err := ValidateSchema("coverage.yaml", data); err != nil {
				t.Fatalf("CUE validation: %v", err)
			}
			definition, err := workflow.Load(data)
			if err != nil {
				t.Fatalf("workflow.Load(): %v", err)
			}
			_, err = newCoverageValidator(t).Validate(definition)
			if tt.wantError && err == nil {
				t.Fatal("complete validation = nil error, want threshold rejection")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("complete validation error: %v", err)
			}
		})
	}
}

func newCoverageValidator(t *testing.T) *SemanticValidator {
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

func coverageWorkflowDefinition() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "coverage"},
		Projects: []workflow.ProjectSpec{{Name: "project", Repository: "."}},
		Nodes: map[string]workflow.NodeSpec{
			"entry": {Node: "human-input"},
			"check": {
				Node: "go-coverage-check", DependsOn: []string{"entry"},
				Inputs: map[string]workflow.InputBinding{"code": {From: "project.code"}},
			},
		},
	}
}
