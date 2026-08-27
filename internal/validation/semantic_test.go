package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/llm"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// fakeFactory 提供语义校验测试所需的 Executor 与端口契约
// （与种子 Node Definition 同构）。
type fakeFactory struct {
	definition string
	nodeType   definition.NodeType
	inputs     map[string]definition.InputPort
	outputs    map[string]definition.OutputPort
}

func (f fakeFactory) Definition() string { return f.definition }
func (f fakeFactory) Version() string    { return "v1" }

// NodeType 声明该定义的类别（缺省 agent，与 tests/dag 的 fakeFactory 同构）。
func (f fakeFactory) NodeType() definition.NodeType {
	if f.nodeType == "" {
		return definition.TypeAgent
	}
	return f.nodeType
}

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

// testFactories 是语义校验测试的 Node 定义集（契约与种子 Node Definition 同构）。
// cd/human-approval 两个额外定义承载 agent 以外的类别（llm 字段合法性）；
// human-approval 为 human 类（环豁免判定用）。
func testFactories() []fakeFactory {
	return []fakeFactory{
		{
			definition: "requirement-analysis",
			inputs:     map[string]definition.InputPort{"requirement": {Type: "markdown"}},
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
				"requirement":     {Type: "markdown", Optional: true},
				"analysis-output": {Type: "markdown", Optional: true},
				"architecture":    {Type: "ArchitectureSpec", Optional: true},
				"openapi":         {Type: "OpenAPI", Optional: true},
				"frontend-sdk":    {Type: "FrontendSDK", Optional: true},
				"test-report":     {Type: "TestReport", Optional: true},
				"approval":        {Type: "ApprovalResult", Optional: true},
				"advise":          {Type: "markdown", Optional: true},
			},
			outputs: map[string]definition.OutputPort{
				"source-code": {Type: "SourceCode"},
				"openapi":     {Type: "OpenAPI"},
			},
		},
		{
			definition: "openapi-generator",
			nodeType:   definition.TypeAutomation,
			inputs:     map[string]definition.InputPort{"openapi": {Type: "OpenAPI"}},
			outputs:    map[string]definition.OutputPort{"frontend-sdk": {Type: "FrontendSDK"}},
		},
		{
			// union 消费端：验证 consumer ⊇ producer 的正向兼容
			//（设计文档 §4：要宽就显式写 union）。
			definition: "doc-writer",
			nodeType:   definition.TypeAutomation,
			inputs:     map[string]definition.InputPort{"content": {Type: "markdown|OpenAPI"}},
			outputs:    map[string]definition.OutputPort{"doc": {Type: "markdown"}},
		},
		{
			definition: "human-approval",
			nodeType:   definition.TypeHuman,
			outputs: map[string]definition.OutputPort{
				"approve": {Type: "bool"},
				"advise":  {Type: "markdown"},
			},
		},
		{definition: "cd", nodeType: definition.TypeAutomation},
	}
}

// testLLMYAML 是语义校验测试注入的 llm.yaml 内容（openai 为默认 provider，
// gpt-4o 为默认 model）。
const testLLMYAML = `
apiVersion: llm/v1
kind: llm
providers:
  - name: openai
    type: openai-compatible
    url: https://api.openai.com/v1
    apikey: plain-test-key
    default: true
    models:
      - name: gpt-4o
        default: true
      - name: gpt-4o-mini
  - name: anthropic
    type: anthropic
    url: https://anthropic
    apikey: plain-test-key
    models:
      - name: claude-sonnet-5
`

// testValidator 返回带全部计划内 Node 定义的校验器
// （契约与种子 Node Definition 同构；见 testFactories）。
// llm 配置注入（agent 节点解析链）与 workflow 文件锚点（projects
// 相对路径）由各用例按需叠加 Option。
func testValidator(t *testing.T, opts ...Option) *SemanticValidator {
	t.Helper()

	kinds := artifact.NewRegistry()
	factories := testFactories()

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
			Type:       f.NodeType(),
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
	return NewSemanticValidator(executors, defs, kinds, opts...)
}

// validateFixture 走完整管线：CUE -> Load -> Semantic（含 llm 注入与
// projects 路径锚点，fixture 的 projects.repository 一律指向
// testdata/examples/order-system）。
func validateFixture(t *testing.T, path string) ([]Warning, error) {
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

	c, err := llm.Load([]byte(testLLMYAML))
	if err != nil {
		t.Fatalf("load test llm config: %v", err)
	}
	return testValidator(t, WithLLMConfig(&c), WithWorkflowFile(path)).Validate(def)
}

func TestSemanticValidFullstack(t *testing.T) {
	warnings, err := validateFixture(t, filepath.Join("testdata", "valid", "minimal.yaml"))
	if err != nil {
		t.Fatalf("Validate() unexpected error:\n%v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Validate() unexpected warnings:\n%v", warnings)
	}
}

// TestSemanticValidUnionPort 验证 TypeExpr 正向兼容（设计文档 §4）：
// union 消费端（markdown|OpenAPI）接受 markdown 生产者。
func TestSemanticValidUnionPort(t *testing.T) {
	warnings, err := validateFixture(t, filepath.Join("testdata", "valid", "union-port.yaml"))
	if err != nil {
		t.Fatalf("Validate() unexpected error:\n%v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Validate() unexpected warnings:\n%v", warnings)
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
			// optional 端口同样做类型兼容检查（票 06：optional 端口同样校验）。
			fixture: filepath.Join("testdata", "invalid-type", "optional-port-mismatch.yaml"),
			wantErr: `node "coder" input "openapi": artifact type mismatch`,
		},
		{
			// human 类节点以输入挂接流程但未声明 dependsOn（设计文档 §10 检查 #8）。
			fixture: filepath.Join("testdata", "invalid-human", "input-without-depends-on.yaml"),
			wantErr: `node "review": human node with inputs must declare dependsOn`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-executor", "unknown-version.yaml"),
			wantErr: `node "coder" executor: executor not found: executor "v9" of node definition "coding-agent" (versions: ["v1"])`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-llm", "non-agent-node.yaml"),
			wantErr: `node "sdk": target_model is only valid on agent nodes (definition "openapi-generator" has type "automation")`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-llm", "unknown-provider.yaml"),
			wantErr: `node "coder": llm: unknown provider "azure"`,
		},
		{
			// Q3 象限：只填 target_model、属默认 provider 之外的模型
			// -> resolver 提示补 llm 字段。
			fixture: filepath.Join("testdata", "invalid-llm", "unknown-model.yaml"),
			wantErr: `node "coder": target_model: default provider "openai" has no model "claude-sonnet-5" (models: gpt-4o, gpt-4o-mini); add "llm" to select another provider`,
		},
		{
			// Q1 象限：llm + target_model 但 model 不归属该 provider。
			fixture: filepath.Join("testdata", "invalid-llm", "cross-provider-model.yaml"),
			wantErr: `node "coder": target_model: provider "openai" has no model "claude-sonnet-5"`,
		},
		{
			fixture: filepath.Join("testdata", "invalid-projects", "zero-entries.yaml"),
			wantErr: "projects: must contain exactly 1 entry, got 0",
		},
		{
			fixture: filepath.Join("testdata", "invalid-projects", "missing-dir.yaml"),
			wantErr: `projects[0] "order-system": repository`,
		},
	}
	for _, tt := range tests {
		t.Run(filepath.Base(filepath.Dir(tt.fixture))+"/"+filepath.Base(tt.fixture), func(t *testing.T) {
			_, err := validateFixture(t, tt.fixture)
			if err == nil {
				t.Fatalf("Validate() = nil error, want rejection containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error =\n%v\nwant containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestSemanticCycleDowngradedToWarning 验证环降为提示（票 06 核心变更）：
// 不含 human 节点的环 -> warning（非 error，validate 放行）；
// 含 human 节点的环 -> 合法迭代路径，不提示。
func TestSemanticCycleDowngradedToWarning(t *testing.T) {
	t.Run("data cycle without human warns but passes", func(t *testing.T) {
		warnings, err := validateFixture(t, filepath.Join("testdata", "warning-cycle", "data-cycle.yaml"))
		if err != nil {
			t.Fatalf("Validate() unexpected error (cycle must be a warning):\n%v", err)
		}
		if len(warnings) != 1 {
			t.Fatalf("Validate() warnings = %v, want exactly 1", warnings)
		}
		if !strings.Contains(warnings[0].Message, "dependency cycle") {
			t.Errorf("warning %q should describe the cycle", warnings[0].Message)
		}
		if !strings.Contains(warnings[0].Message, "convergence guard") {
			t.Errorf("warning %q should mention the convergence guard safety net", warnings[0].Message)
		}
	})

	t.Run("control cycle without human warns but passes", func(t *testing.T) {
		warnings, err := validateFixture(t, filepath.Join("testdata", "warning-cycle", "control-cycle.yaml"))
		if err != nil {
			t.Fatalf("Validate() unexpected error (cycle must be a warning):\n%v", err)
		}
		if len(warnings) != 1 {
			t.Fatalf("Validate() warnings = %v, want exactly 1", warnings)
		}
	})

	t.Run("cycle containing human node is silent", func(t *testing.T) {
		warnings, err := validateFixture(t, filepath.Join("testdata", "valid", "human-cycle.yaml"))
		if err != nil {
			t.Fatalf("Validate() unexpected error:\n%v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("Validate() warnings = %v, want none (human cycle is a legal iteration path)", warnings)
		}
	})
}

func TestSemanticProgrammaticChecks(t *testing.T) {
	base := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "test"},
		Projects:   []workflow.ProjectSpec{{Name: "p", Repository: "./examples/order-system"}},
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
		_, err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), `dependsOn unknown node "nonexistent"`) {
			t.Fatalf("Validate() error = %v, want dependsOn unknown node", err)
		}
	})

	t.Run("unbound required input", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"architecture": {Node: "architecture-design"},
		}
		_, err := testValidator(t).Validate(def)
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
		_, err := testValidator(t).Validate(def)
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
		_, err := v.Validate(def)
		if err == nil || !strings.Contains(err.Error(), `unregistered artifact kind "FigmaDesign"`) {
			t.Fatalf("Validate() error = %v, want unregistered artifact kind", err)
		}
	})

	t.Run("llm absent without agent nodes is fine", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{
			"sdk": {Node: "openapi-generator"},
		}
		// openapi-generator 有必填输入，改用无输入的 cd（automation）。
		def.Nodes = map[string]workflow.NodeSpec{"deploy": {Node: "cd"}}
		_, err := testValidator(t).Validate(def)
		if err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("llm absent with agent nodes is rejected", func(t *testing.T) {
		def := base
		def.Nodes = map[string]workflow.NodeSpec{"coder": {Node: "coding-agent"}}
		_, err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), "no llm.yaml was found") {
			t.Fatalf("Validate() error = %v, want missing llm.yaml rejection", err)
		}
	})

	t.Run("agent llm defaults resolve without explicit fields", func(t *testing.T) {
		c, err := llm.Load([]byte(testLLMYAML))
		if err != nil {
			t.Fatalf("load test llm config: %v", err)
		}
		def := base
		def.Nodes = map[string]workflow.NodeSpec{"coder": {Node: "coding-agent"}}
		if _, err := testValidator(t, WithLLMConfig(&c)).Validate(def); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("projects path check needs a file anchor", func(t *testing.T) {
		// 未注入 WithWorkflowFile 时跳过路径检查（内存形态无锚点），
		// 数量检查仍然生效。
		def := base
		def.Nodes = map[string]workflow.NodeSpec{"deploy": {Node: "cd"}}
		def.Projects = nil
		_, err := testValidator(t).Validate(def)
		if err == nil || !strings.Contains(err.Error(), "projects: must contain exactly 1 entry, got 0") {
			t.Fatalf("Validate() error = %v, want exactly-one rejection", err)
		}

		def.Projects = []workflow.ProjectSpec{{Name: "p", Repository: "./no/such/dir"}}
		if _, err := testValidator(t).Validate(def); err != nil {
			t.Fatalf("Validate() unexpected error without file anchor: %v", err)
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
		_, err := testValidator(t).Validate(def)
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
