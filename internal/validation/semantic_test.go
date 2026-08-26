package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// fakeFactory 提供语义校验测试所需的 Executor 与端口契约
// （与种子 Node Definition 同构）。
type fakeFactory struct {
	definition string
	inputs     map[string]definition.InputPort
	outputs    map[string]definition.OutputPort
}

func (f fakeFactory) Definition() string { return f.definition }
func (f fakeFactory) Version() string    { return "v1" }

// Contract 声明端口契约（testValidator 注册进内存 definition.Registry）。
func (f fakeFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return f.inputs, f.outputs
}

func (f fakeFactory) Create(config node.Config) (node.Node, error) {
	return fakeNode{}, nil
}

type fakeNode struct{}

func (n fakeNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, nil
}

// testValidator 返回带全部计划内 Node 定义的校验器
// （契约与种子 Node Definition 同构；见 fakeFactory）。
func testValidator(t *testing.T) *SemanticValidator {
	t.Helper()

	kinds := artifact.NewRegistry()
	factories := []fakeFactory{
		{
			definition: "requirement-analysis",
			outputs: map[string]definition.OutputPort{
				"rationality":     {Type: "int"},
				"analysis-output": {Type: "markdown"},
			},
		},
		{
			definition: "architecture-design",
			inputs:     map[string]definition.InputPort{"analysis-output": {Type: "markdown"}},
			outputs:    map[string]definition.OutputPort{"architecture": {Type: "ArchitectureSpec"}},
		},
		{
			definition: "coding-agent",
			inputs: map[string]definition.InputPort{
				"analysis-output": {Type: "markdown", Optional: true},
				"architecture":    {Type: "ArchitectureSpec", Optional: true},
				"openapi":         {Type: "OpenAPI", Optional: true},
				"frontend-sdk":    {Type: "FrontendSDK", Optional: true},
				"test-report":     {Type: "TestReport", Optional: true},
				"approval":        {Type: "ApprovalResult", Optional: true},
			},
			outputs: map[string]definition.OutputPort{
				"source-code": {Type: "SourceCode"},
				"openapi":     {Type: "OpenAPI"},
			},
		},
		{
			definition: "openapi-generator",
			inputs:     map[string]definition.InputPort{"openapi": {Type: "OpenAPI"}},
			outputs:    map[string]definition.OutputPort{"frontend-sdk": {Type: "FrontendSDK"}},
		},
		{
			definition: "human-approval",
			outputs: map[string]definition.OutputPort{
				"approval": {Type: "ApprovalResult"},
			},
		},
		{definition: "cd"},
	}

	defs := definition.NewRegistry()
	for _, nt := range []definition.NodeType{
		definition.TypeAgent, definition.TypeAutomation, definition.TypeHuman,
	} {
		if err := defs.RegisterNodeType(definition.NodeTypeDefinition{
			APIVersion: definition.NodeTypeAPIVersionV1,
			Kind:       definition.NodeTypeDefinitionKind,
			Metadata:   definition.Metadata{Name: string(nt), Description: "test"},
		}); err != nil {
			t.Fatalf("register node type %s: %v", nt, err)
		}
	}
	executors := node.NewExecutorRegistry()
	for _, f := range factories {
		d := definition.NodeDefinition{
			APIVersion: definition.NodeDefinitionAPIVersionV1,
			Kind:       definition.NodeDefinitionKind,
			Metadata:   definition.Metadata{Name: f.definition, Description: "test"},
			Type:       definition.TypeAgent,
			Inputs:     f.inputs,
			Outputs:    f.outputs,
		}
		if err := defs.RegisterDefinition(d); err != nil {
			t.Fatalf("register definition %q: %v", f.definition, err)
		}
		if err := executors.Register(f); err != nil {
			t.Fatalf("register executor %q: %v", f.definition, err)
		}
	}
	return NewSemanticValidator(executors, defs, kinds)
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
			wantErr: `node "requirement": unknown node definition "nonexistent-node"`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-output", "unknown-output.yaml"),
			wantErr: `node "analysis" has no output "nonexistent-output"`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-type", "kind-mismatch.yaml"),
			wantErr: "artifact type mismatch",
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
			"requirement": {Node: "requirement-analysis"},
		},
	}

	t.Run("unknown dependsOn", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"requirement": {Node: "requirement-analysis"},
			"deploy":      {Node: "cd", DependsOn: []string{"nonexistent"}},
		}
		err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), `dependsOn unknown node "nonexistent"`) {
			t.Fatalf("Validate() error = %v, want dependsOn unknown node", err)
		}
	})

	t.Run("unbound required input", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"architecture": {Node: "architecture-design"},
		}
		err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), `required input "analysis-output" is not bound`) {
			t.Fatalf("Validate() error = %v, want unbound required input", err)
		}
	})

	t.Run("undeclared input name", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"requirement": {Node: "requirement-analysis"},
			"openapi": {
				Node:   "openapi-generator",
				Inputs: map[string]workflow.InputBinding{"surprise": {From: "requirement.analysis-output"}},
			},
		}
		err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), `input "surprise" is not declared`) {
			t.Fatalf("Validate() error = %v, want undeclared input", err)
		}
	})

	t.Run("unregistered artifact kind", func(t *testing.T) {
		defs := definition.NewRegistry()
		if err := defs.RegisterNodeType(definition.NodeTypeDefinition{
			APIVersion: definition.NodeTypeAPIVersionV1,
			Kind:       definition.NodeTypeDefinitionKind,
			Metadata:   definition.Metadata{Name: string(definition.TypeAgent), Description: "test"},
		}); err != nil {
			t.Fatal(err)
		}
		if err := defs.RegisterDefinition(definition.NodeDefinition{
			APIVersion: definition.NodeDefinitionAPIVersionV1,
			Kind:       definition.NodeDefinitionKind,
			Metadata:   definition.Metadata{Name: "figma-exporter", Description: "test"},
			Type:       definition.TypeAgent,
			Outputs:    map[string]definition.OutputPort{"design": {Type: "FigmaDesign"}},
		}); err != nil {
			t.Fatal(err)
		}
		executors := node.NewExecutorRegistry()
		if err := executors.Register(fakeFactory{
			definition: "figma-exporter",
			outputs:    map[string]definition.OutputPort{"design": {Type: "FigmaDesign"}},
		}); err != nil {
			t.Fatal(err)
		}
		v := NewSemanticValidator(executors, defs, artifact.NewRegistry())

		def := base
		def.Nodes = map[string]workflow.NodeSpec{"designer": {Node: "figma-exporter"}}
		err := v.Validate(def)
		if err == nil || !strings.Contains(err.Error(), `unregistered artifact kind "FigmaDesign"`) {
			t.Fatalf("Validate() error = %v, want unregistered artifact kind", err)
		}
	})

	t.Run("multiple errors are aggregated", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"requirement": {Node: "nonexistent"},
			"architecture": {
				Node:   "architecture-design",
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
