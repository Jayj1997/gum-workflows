package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// fakeFactory / fakeNode 提供语义校验测试所需的 Node Contract。
// Schema 与设计计划 §32 的第一批 MVP Node 一致。
type fakeFactory struct {
	nodeType       string
	inputs         map[string]artifact.Kind
	optionalInputs map[string]artifact.Kind
	outputs        map[string]artifact.Kind
}

func (f fakeFactory) Type() string { return f.nodeType }

func (f fakeFactory) Create(config node.Config) (node.Node, error) {
	return fakeNode{schema: node.Schema{
		Inputs:         f.inputs,
		OptionalInputs: f.optionalInputs,
		Outputs:        f.outputs,
	}}, nil
}

type fakeNode struct {
	schema node.Schema
}

func (n fakeNode) Type() string              { return "" }
func (n fakeNode) InputSchema() node.Schema  { return n.schema }
func (n fakeNode) OutputSchema() node.Schema { return n.schema }

func (n fakeNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, nil
}

// testValidator 返回带全部计划内 Node Type 的校验器。
func testValidator(t *testing.T) *SemanticValidator {
	t.Helper()

	kinds := artifact.NewRegistry()
	reg := node.NewRegistry()
	factories := []node.Factory{
		fakeFactory{
			nodeType: "requirement-analysis",
			outputs:  map[string]artifact.Kind{"requirement": artifact.KindRequirementSpec},
		},
		fakeFactory{
			nodeType: "architecture-design",
			inputs:   map[string]artifact.Kind{"requirement": artifact.KindRequirementSpec},
			outputs:  map[string]artifact.Kind{"architecture": artifact.KindArchitectureSpec},
		},
		fakeFactory{
			nodeType: "coding-agent",
			optionalInputs: map[string]artifact.Kind{
				"requirement":  artifact.KindRequirementSpec,
				"architecture": artifact.KindArchitectureSpec,
				"openapi":      artifact.KindOpenAPI,
				"frontend-sdk": artifact.KindFrontendSDK,
				"test-report":  artifact.KindTestReport,
				"approval":     artifact.KindApprovalResult,
			},
			outputs: map[string]artifact.Kind{
				"source-code": artifact.KindSourceCode,
				"openapi":     artifact.KindOpenAPI,
			},
		},
		fakeFactory{
			nodeType: "openapi-generator",
			inputs:   map[string]artifact.Kind{"openapi": artifact.KindOpenAPI},
			outputs:  map[string]artifact.Kind{"frontend-sdk": artifact.KindFrontendSDK},
		},
		fakeFactory{
			nodeType: "human-approval",
			outputs:  map[string]artifact.Kind{"approval": artifact.KindApprovalResult},
		},
		fakeFactory{
			nodeType: "cd",
		},
	}
	for _, f := range factories {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %q: %v", f.Type(), err)
		}
	}
	return NewSemanticValidator(reg, kinds)
}

// validateFixture 走完整管线：CUE -> Load -> Semantic。
func validateFixture(t *testing.T, path string) error {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := ValidateSchema(path, data); err != nil {
		t.Fatalf("fixture %s should pass CUE schema validation: %v", path, err)
	}
	def, err := workflow.Load(data)
	if err != nil {
		t.Fatalf("fixture %s should parse: %v", path, err)
	}
	return testValidator(t).Validate(def)
}

func TestSemanticValidFullstack(t *testing.T) {
	err := validateFixture(t, filepath.Join("testdata", "valid", "fullstack.yaml"))
	if err != nil {
		t.Fatalf("Validate() unexpected error:\n%v", err)
	}
}

func TestSemanticInvalidFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		wantErr string
	}{
		{
			fixture: filepath.Join("testdata", "invalid-node", "unknown-type.yaml"),
			wantErr: `node "requirement": unknown node type "nonexistent-node"`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-output", "unknown-output.yaml"),
			wantErr: `node "requirement" has no output "nonexistent-output"`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-type", "kind-mismatch.yaml"),
			wantErr: "artifact kind mismatch",
		},
		{
			fixture: filepath.Join("testdata", "invalid-cycle", "data-cycle.yaml"),
			wantErr: "dependency cycle detected",
		},
		{
			fixture: filepath.Join("testdata", "invalid-cycle", "control-cycle.yaml"),
			wantErr: "dependency cycle detected",
		},
	}
	for _, tt := range tests {
		t.Run(filepath.Base(filepath.Dir(tt.fixture))+"/"+filepath.Base(tt.fixture), func(t *testing.T) {
			err := validateFixture(t, tt.fixture)
			if err == nil {
				t.Fatalf("Validate() = nil error, want rejection containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error =\n%v\nwant containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSemanticProgrammaticChecks(t *testing.T) {
	base := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "test"},
		Nodes: map[string]workflow.NodeSpec{
			"requirement": {Type: "requirement-analysis"},
		},
	}

	t.Run("unknown dependsOn", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"requirement": {Type: "requirement-analysis"},
			"deploy":      {Type: "cd", DependsOn: []string{"nonexistent"}},
		}
		err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), `dependsOn unknown node "nonexistent"`) {
			t.Fatalf("Validate() error = %v, want dependsOn unknown node", err)
		}
	})

	t.Run("unbound required input", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"architecture": {Type: "architecture-design"},
		}
		err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), `required input "requirement" is not bound`) {
			t.Fatalf("Validate() error = %v, want unbound required input", err)
		}
	})

	t.Run("undeclared input name", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"requirement": {Type: "requirement-analysis"},
			"openapi": {
				Type:   "openapi-generator",
				Inputs: map[string]workflow.InputBinding{"surprise": {From: "requirement.requirement"}},
			},
		}
		err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), `input "surprise" is not declared`) {
			t.Fatalf("Validate() error = %v, want undeclared input", err)
		}
	})

	t.Run("unregistered artifact kind", func(t *testing.T) {
		reg := node.NewRegistry()
		if err := reg.Register(fakeFactory{
			nodeType: "figma-exporter",
			outputs:  map[string]artifact.Kind{"design": "FigmaDesign"},
		}); err != nil {
			t.Fatal(err)
		}
		v := NewSemanticValidator(reg, artifact.NewRegistry())

		def := base
		def.Nodes = map[string]workflow.NodeSpec{"designer": {Type: "figma-exporter"}}
		err := v.Validate(def)
		if err == nil || !strings.Contains(err.Error(), `unregistered artifact kind "FigmaDesign"`) {
			t.Fatalf("Validate() error = %v, want unregistered artifact kind", err)
		}
	})

	t.Run("multiple errors are aggregated", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"requirement": {Type: "nonexistent"},
			"architecture": {
				Type:   "architecture-design",
				Inputs: map[string]workflow.InputBinding{"requirement": {From: "requirement.requirement"}},
			},
		}
		err := testValidator(t).Validate(def)
		if err == nil {
			t.Fatal("Validate() = nil error, want rejection")
		}
		var verrs ValidationErrors
		if ok := asValidationErrors(err, &verrs); !ok || len(verrs) < 1 {
			t.Fatalf("Validate() error = %v, want aggregated ValidationErrors", err)
		}
	})
}

func asValidationErrors(err error, target *ValidationErrors) bool {
	if ve, ok := err.(ValidationErrors); ok {
		*target = ve
		return true
	}
	return false
}
