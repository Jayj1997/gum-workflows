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
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/validation"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// ---- 测试用 Node（Schema 与设计计划 §32 第一批 MVP Node 一致）----

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

func (n fakeNode) Type() string             { return "" }
func (n fakeNode) InputSchema() node.Schema { return n.schema }
func (n fakeNode) OutputSchema() node.Schema {
	return n.schema
}

func (n fakeNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, nil
}

// fullstackFactories 是 fullstack 冒烟场景所需的 Node Type
// （Schema 与设计计划 §32 一致）。
func fullstackFactories() []node.Factory {
	return []node.Factory{
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
}

// newRegistry 依据 factories 构造 Node Registry。
func newRegistry(t *testing.T, factories ...node.Factory) *node.Registry {
	t.Helper()

	reg := node.NewRegistry()
	for _, f := range factories {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %q: %v", f.Type(), err)
		}
	}
	return reg
}

// validateAndInstantiate 走语义校验并实例化全部 Node。
func validateAndInstantiate(t *testing.T, def workflow.Definition, factories []node.Factory) map[string]node.Node {
	t.Helper()

	reg := newRegistry(t, factories...)
	v := validation.NewSemanticValidator(reg, artifact.NewRegistry())
	if err := v.Validate(def); err != nil {
		t.Fatalf("semantic validation unexpected error:\n%v", err)
	}

	instances := make(map[string]node.Node, len(def.Nodes))
	for _, id := range sortedNodeIDs(def) {
		spec := def.Nodes[id]
		f, ok := reg.Get(spec.Type)
		if !ok {
			t.Fatalf("node %q: unknown node type %q", id, spec.Type)
		}
		n, err := f.Create(node.Config(spec.Config))
		if err != nil {
			t.Fatalf("node %q: create: %v", id, err)
		}
		instances[id] = n
	}
	return instances
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
// 每一步都在验证设计假设：
//   - Node 执行时其全部前驱（Data + Control）已完成（计划 §8 Ready 条件）
//   - 每个输入绑定都能解析到已存在的 Artifact，且 Kind 匹配
//   - 全部 Node 都被执行（无 stall、无环）
func simulate(t *testing.T, def workflow.Definition, instances map[string]node.Node) smokeResult {
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

		// 数据依赖解析：每个绑定必须已能取到 Artifact，且 Kind 匹配。
		spec := def.Nodes[id]
		var gotInputs []string
		for name, binding := range spec.Inputs {
			ref, ok := res.artifacts[binding.From]
			if !ok {
				t.Fatalf("node %q input %q: artifact %q not available at execution time", id, name, binding.From)
			}
			if want := lookupInputKind(instances[id], name); want != "" && ref.Kind != want {
				t.Fatalf("node %q input %q: artifact %q has kind %q, want %q", id, name, binding.From, ref.Kind, want)
			}
			gotInputs = append(gotInputs, name)
		}
		sort.Strings(gotInputs)
		res.inputCount[id] = len(gotInputs)

		// Mock 行为：产出全部声明 Output（计划 §33）。
		outputs := instances[id].OutputSchema().Outputs
		var produced []string
		for _, outName := range sortedKindKeys(outputs) {
			key := id + "." + outName
			res.artifacts[key] = artifact.ArtifactRef{
				ID:      key,
				Kind:    outputs[outName],
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

func lookupInputKind(n node.Node, name string) artifact.Kind {
	s := n.InputSchema()
	if k, ok := s.Inputs[name]; ok {
		return k
	}
	return s.OptionalInputs[name]
}

func sortedKindKeys(m map[string]artifact.Kind) []string {
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

	instances := validateAndInstantiate(t, def, fullstackFactories())
	res := simulate(t, def, instances)

	wantOrder := []string{"requirement", "architecture", "backend", "openapi", "frontend"}
	if !reflect.DeepEqual(res.order, wantOrder) {
		t.Fatalf("execution order = %v, want %v", res.order, wantOrder)
	}

	// 每个声明 Output 都产出了 Artifact（Mock 行为）：
	// requirement/architecture/openapi 各 1 个，backend/frontend 各 2 个，共 7 个。
	if len(res.artifacts) != 7 {
		t.Fatalf("produced %d artifacts, want 7", len(res.artifacts))
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

	instances := validateAndInstantiate(t, def, fullstackFactories())
	res := simulate(t, def, instances)

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
		nodeType: "code-reviewer",
		inputs: map[string]artifact.Kind{
			"left":  artifact.KindSourceCode,
			"right": artifact.KindSourceCode,
		},
		outputs: map[string]artifact.Kind{"report": artifact.KindTestReport},
	})

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "diamond"},
		Nodes: map[string]workflow.NodeSpec{
			"a": {Type: "requirement-analysis"},
			"b": {
				Type:   "coding-agent",
				Inputs: map[string]workflow.InputBinding{"requirement": {From: "a.requirement"}},
			},
			"c": {
				Type:   "coding-agent",
				Inputs: map[string]workflow.InputBinding{"requirement": {From: "a.requirement"}},
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

	instances := validateAndInstantiate(t, def, factories)
	res := simulate(t, def, instances)

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
