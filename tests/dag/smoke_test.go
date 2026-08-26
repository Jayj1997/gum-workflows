// tests/dag 冒烟测试：用测试内的模拟调度器干跑 DAG，验证设计计划的核心思路：
//
//	数据依赖（inputs.from）自动形成执行顺序，不需要 dependsOn。
//
// 模拟调度器采用设计计划 §26 的算法形态：Ready Queue + Dependency Counter。
// 保留独立模拟器（不迁移到真实 execution.Engine）：DAG 语义验收
// 应独立于 Runtime 实现，Engine 的行为由 internal/execution 覆盖。
package dag_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/validation"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// ---- 测试用 Executor 与契约（与种子 Node Definition 同构）----

// contractDeclarer 由携带端口契约的测试 factory 实现
// （newRegistries 注册进内存 definition.Registry）。
type contractDeclarer interface {
	Contract() (map[string]definition.InputPort, map[string]definition.OutputPort)
}

type fakeFactory struct {
	definition string
	inputs     map[string]definition.InputPort
	outputs    map[string]definition.OutputPort
}

func (f fakeFactory) Definition() string { return f.definition }
func (f fakeFactory) Version() string    { return "v1" }

// Contract 声明端口契约（newRegistries 注册进内存 definition.Registry）。
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

// fullstackFactories 是 fullstack 冒烟场景所需的 Node 定义与契约。
func fullstackFactories() []node.ExecutorFactory {
	return []node.ExecutorFactory{
		fakeFactory{
			definition: "requirement-analysis",
			outputs: map[string]definition.OutputPort{
				"rationality":     {Type: "int"},
				"analysis-output": {Type: "markdown"},
			},
		},
		fakeFactory{
			definition: "architecture-design",
			inputs:     map[string]definition.InputPort{"analysis-output": {Type: "markdown"}},
			outputs:    map[string]definition.OutputPort{"architecture": {Type: "ArchitectureSpec"}},
		},
		fakeFactory{
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
		fakeFactory{
			definition: "openapi-generator",
			inputs:     map[string]definition.InputPort{"openapi": {Type: "OpenAPI"}},
			outputs:    map[string]definition.OutputPort{"frontend-sdk": {Type: "FrontendSDK"}},
		},
		fakeFactory{
			definition: "human-approval",
			outputs:    map[string]definition.OutputPort{"approval": {Type: "ApprovalResult"}},
		},
		fakeFactory{definition: "cd"},
	}
}

// newRegistries 依据 factories 构造内存 definition.Registry 与
// ExecutorRegistry（冒烟测试不依赖内置节点集）。
func newRegistries(t *testing.T, factories []node.ExecutorFactory) (*definition.Registry, *node.ExecutorRegistry) {
	t.Helper()

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
			Metadata:   definition.Metadata{Name: f.Definition(), Description: "test"},
			Type:       definition.TypeAgent,
		}
		if c, ok := f.(contractDeclarer); ok {
			d.Inputs, d.Outputs = c.Contract()
		}
		if err := defs.RegisterDefinition(d); err != nil {
			t.Fatalf("register definition %q: %v", f.Definition(), err)
		}
		if err := executors.Register(f); err != nil {
			t.Fatalf("register executor %q: %v", f.Definition(), err)
		}
	}
	return defs, executors
}

// contractsFor 返回某 Node 定义的契约（冒烟调度器据此产出声明 Output）。
func contractsFor(t *testing.T, defs *definition.Registry, def workflow.Definition) map[string]definition.NodeDefinition {
	t.Helper()

	contracts := make(map[string]definition.NodeDefinition, len(def.Nodes))
	for id, spec := range def.Nodes {
		d, err := defs.Definition(spec.Type)
		if err != nil {
			t.Fatalf("node %q: unknown node definition %q", id, spec.Type)
		}
		contracts[id] = d
	}
	return contracts
}

// ---- 模拟调度器（设计计划 §26 的算法形态）----

// smokeResult 是一次干跑的结果。
type smokeResult struct {
	order      []string                        // 实际执行顺序（合法拓扑序）
	artifacts  map[string]artifact.ArtifactRef // 产出，key 为 "<node-id>.<output>"
	inputCount map[string]int                  // 每个 Node 执行时获得的输入数量
	maxReady   int                             // Ready Queue 峰值长度（并行就绪度）
}

// simulate 用 Ready Queue + Dependency Counter 干跑 DAG：
// 依次执行 Ready 的 Node，产出其全部声明 Output（Mock 行为，设计计划 §33），
// 完成后递减后继的计数器并将新 Ready 的 Node 入队。
//
// 契约来自 contracts（Node Definition 内存形态，同构于种子 YAML）。
// 每一步都在验证设计假设：
//   - Node 执行时其全部前驱（Data + Control）已完成（计划 §8 Ready 条件）
//   - 每个输入绑定都能解析到已存在的 Artifact，且类型匹配
//   - 全部 Node 都被执行（无 stall、无环）
func simulate(t *testing.T, def workflow.Definition, contracts map[string]definition.NodeDefinition) smokeResult {
	t.Helper()

	g, err := workflow.BuildGraph(def)
	if err != nil {
		t.Fatalf("BuildGraph() unexpected error: %v", err)
	}

	// Dependency Counter：未完成前驱数量。
	remaining := make(map[string]int, len(g.NodeIDs))
	for _, id := range g.NodeIDs {
		remaining[id] = len(g.Predecessors(id))
	}

	completed := make(map[string]bool, len(g.NodeIDs))
	res := smokeResult{
		artifacts:  make(map[string]artifact.ArtifactRef),
		inputCount: make(map[string]int, len(g.NodeIDs)),
		maxReady:   0,
	}
	pos := make(map[string]int, len(g.NodeIDs))

	// Ready Queue：初始为全部源节点。
	var queue []string
	for _, id := range g.NodeIDs {
		if remaining[id] == 0 {
			queue = append(queue, id)
		}
	}

	for len(queue) > 0 {
		res.maxReady = max(res.maxReady, len(queue))

		id := queue[0]
		queue = queue[1:]
		pos[id] = len(res.order)
		res.order = append(res.order, id)

		// 计划 §8：Ready(Node) = 前驱（Data + Control）全部完成。
		for _, p := range g.Predecessors(id) {
			if !completed[p] {
				t.Fatalf("node %q started before predecessor %q completed", id, p)
			}
		}

		// 数据依赖解析：每个绑定必须已能取到 Artifact，且类型匹配。
		spec := def.Nodes[id]
		contract := contracts[id]
		var gotInputs []string
		for name, binding := range spec.Inputs {
			ref, ok := res.artifacts[binding.From]
			if !ok {
				t.Fatalf("node %q input %q: artifact %q not available at execution time", id, name, binding.From)
			}
			if port, declared := contract.Inputs[name]; declared {
				want, err := definition.ParseTypeExpr(port.Type)
				if err != nil {
					t.Fatalf("node %q input %q: parse contract type %q: %v", id, name, port.Type, err)
				}
				if !definition.MatchesKind(want, string(ref.Kind)) {
					t.Fatalf("node %q input %q: artifact %q has kind %q, want %q", id, name, binding.From, ref.Kind, want.String())
				}
			}
			gotInputs = append(gotInputs, name)
		}
		sort.Strings(gotInputs)
		res.inputCount[id] = len(gotInputs)

		// Mock 行为：产出全部声明 Output（计划 §33）。
		var produced []string
		for _, outName := range sortedPortKeys(contract.Outputs) {
			key := id + "." + outName
			res.artifacts[key] = artifact.ArtifactRef{
				ID:      key,
				Kind:    artifact.Kind(contract.Outputs[outName].Type),
				Version: "1",
				URI:     "mem://" + key,
			}
			produced = append(produced, outName)
		}

		completed[id] = true
		t.Logf("[run] %-13s inputs=%v outputs=%v", id, gotInputs, produced)

		// 依赖计数推进：后继 Ready 则入队。
		for _, s := range g.Successors(id) {
			remaining[s]--
			if remaining[s] == 0 {
				queue = append(queue, s)
			}
		}
	}

	// 全部 Node 必须被执行（无 stall；环已在 Validator 拦截，此处兜底）。
	if len(res.order) != len(def.Nodes) {
		t.Fatalf("executed %d of %d nodes", len(res.order), len(def.Nodes))
	}

	// 拓扑完整性：每个 Node 必须出现在其全部前驱之后。
	for _, id := range g.NodeIDs {
		for _, p := range g.Predecessors(id) {
			if pos[id] < pos[p] {
				t.Fatalf("topological order violated: %q runs before its predecessor %q", id, p)
			}
		}
	}
	return res
}

func sortedPortKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedNodeIDs(def workflow.Definition) []string {
	keys := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	return keys
}

// ---- 冒烟场景 ----

// TestSmokeFullstackDataDriven（计划 Case 1/3 + §10）：
// 无任何 dependsOn 的 fullstack Workflow 仅靠数据依赖即可完整执行，
// 每个 Node 执行时其输入 Artifact 全部就绪、Kind 匹配。
func TestSmokeFullstackDataDriven(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "validation", "testdata", "valid", "fullstack.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := validation.ValidateSchema(path, data); err != nil {
		t.Fatalf("ValidateSchema() unexpected error: %v", err)
	}
	def, err := workflow.Load(data)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	defs, executors := newRegistries(t, fullstackFactories())
	v := validation.NewSemanticValidator(executors, defs, artifact.NewRegistry())
	if err := v.Validate(def); err != nil {
		t.Fatalf("semantic validation unexpected error:\n%v", err)
	}
	res := simulate(t, def, contractsFor(t, defs, def))

	wantOrder := []string{"requirement", "architecture", "backend", "openapi", "frontend"}
	if !reflect.DeepEqual(res.order, wantOrder) {
		t.Fatalf("execution order = %v, want %v", res.order, wantOrder)
	}

	// 每个声明 Output 都产出了 Artifact（Mock 行为）：
	// requirement 2 个、architecture/openapi 各 1 个，backend/frontend 各 2 个，共 8 个。
	if len(res.artifacts) != 8 {
		t.Fatalf("produced %d artifacts, want 8", len(res.artifacts))
	}

	// 输入数量：backend 消费 2 个 Artifact，frontend 消费 3 个。
	if res.inputCount["backend"] != 2 {
		t.Errorf("backend inputs = %d, want 2", res.inputCount["backend"])
	}
	if res.inputCount["frontend"] != 3 {
		t.Errorf("frontend inputs = %d, want 3", res.inputCount["frontend"])
	}

	// 串行链路：任一时刻 Ready Queue 至多 1 个 Node。
	if res.maxReady != 1 {
		t.Errorf("maxReady = %d, want 1 (fullstack is a serial chain)", res.maxReady)
	}
}

// TestSmokeControlDependency（计划 Case 4 / §6）：
// approval 与 deploy 之间没有数据传递，仅靠 dependsOn 保证 deploy 等 approval 完成。
func TestSmokeControlDependency(t *testing.T) {
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "deploy"},
		Nodes: map[string]workflow.NodeSpec{
			"approval": {Type: "human-approval"},
			"deploy":   {Type: "cd", DependsOn: []string{"approval"}},
		},
	}

	defs, executors := newRegistries(t, fullstackFactories())
	if err := validation.NewSemanticValidator(executors, defs, artifact.NewRegistry()).Validate(def); err != nil {
		t.Fatalf("semantic validation unexpected error:\n%v", err)
	}
	res := simulate(t, def, contractsFor(t, defs, def))

	pos := map[string]int{}
	for i, id := range res.order {
		pos[id] = i
	}
	if pos["deploy"] < pos["approval"] {
		t.Fatalf("deploy ran before approval: order = %v", res.order)
	}
	// Control Edge 不携带数据：deploy 的输入数为 0。
	if res.inputCount["deploy"] != 0 {
		t.Errorf("deploy inputs = %d, want 0", res.inputCount["deploy"])
	}
	t.Logf("order = %v", res.order)
}

// TestSmokeParallelReadiness（计划 Case 2，为 M7 并行执行提供前提验证）：
// 菱形 DAG（A -> B/C -> D）中 B 与 C 在 A 完成后同时 Ready，D 等待两者。
func TestSmokeParallelReadiness(t *testing.T) {
	factories := append(fullstackFactories(), fakeFactory{
		definition: "code-reviewer",
		inputs: map[string]definition.InputPort{
			"left":  {Type: "SourceCode"},
			"right": {Type: "SourceCode"},
		},
		outputs: map[string]definition.OutputPort{"report": {Type: "TestReport"}},
	})

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "diamond"},
		Nodes: map[string]workflow.NodeSpec{
			"a": {Type: "requirement-analysis"},
			"b": {
				Type:   "coding-agent",
				Inputs: map[string]workflow.InputBinding{"analysis-output": {From: "a.analysis-output"}},
			},
			"c": {
				Type:   "coding-agent",
				Inputs: map[string]workflow.InputBinding{"analysis-output": {From: "a.analysis-output"}},
			},
			"d": {
				Type: "code-reviewer",
				Inputs: map[string]workflow.InputBinding{
					"left":  {From: "b.source-code"},
					"right": {From: "c.source-code"},
				},
			},
		},
	}

	defs, executors := newRegistries(t, factories)
	if err := validation.NewSemanticValidator(executors, defs, artifact.NewRegistry()).Validate(def); err != nil {
		t.Fatalf("semantic validation unexpected error:\n%v", err)
	}
	res := simulate(t, def, contractsFor(t, defs, def))

	// A 完成后 B 与 C 同时 Ready：Ready Queue 峰值为 2。
	if res.maxReady != 2 {
		t.Errorf("maxReady = %d, want 2 (b and c ready in parallel after a)", res.maxReady)
	}
	// D 汇聚两个分支的输入。
	if res.inputCount["d"] != 2 {
		t.Errorf("d inputs = %d, want 2 (d waits for both b and c)", res.inputCount["d"])
	}

	// A 必须最先执行，D 必须最后执行。
	if res.order[0] != "a" {
		t.Errorf("first executed = %q, want %q", res.order[0], "a")
	}
	if res.order[len(res.order)-1] != "d" {
		t.Errorf("last executed = %q, want %q", res.order[len(res.order)-1], "d")
	}
	t.Logf("order = %v, maxReady = %d", res.order, res.maxReady)
}
