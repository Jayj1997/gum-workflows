package execution

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// ---- 测试用可执行 Mock Node（仅测试内使用，内置 Mock Node 属后续里程碑）----

type mockNode struct {
	definition string
	outputs    map[string]definition.OutputPort
	fail       bool
	// skip 声明但不产出的输出名（模拟 Node 未产出某个声明过的产出）。
	skip map[string]bool
	// onRun 记录执行事件（含收到的输入），供顺序断言使用。
	onRun func(nodeType string, inputs map[string]artifact.ArtifactRef)
}

func (m mockNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	if m.onRun != nil {
		m.onRun(m.definition, inputs)
	}
	if m.fail {
		return nil, fmt.Errorf("mock failure of %q", m.definition)
	}
	outputs := make(map[string]artifact.ArtifactRef, len(m.outputs))
	for name, port := range m.outputs {
		if m.skip[name] {
			continue
		}
		ref, err := ctx.Store.Put(artifact.Artifact{
			ID:      name,
			Kind:    artifact.Kind(port.Type),
			Version: "1",
			Data:    map[string]any{"nodeType": m.definition, "inputs": len(inputs)},
		})
		if err != nil {
			return nil, fmt.Errorf("put output %q: %w", name, err)
		}
		outputs[name] = ref
	}
	return outputs, nil
}

type mockFactory struct {
	definition string
	inputs     map[string]definition.InputPort
	outputs    map[string]definition.OutputPort
	fail       bool
	skip       map[string]bool
	onRun      func(nodeType string, inputs map[string]artifact.ArtifactRef)
}

func (f mockFactory) Definition() string { return f.definition }
func (f mockFactory) Version() string    { return "v1" }

// Contract 声明端口契约（newTestRegistries 注册进内存 definition.Registry）。
func (f mockFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return f.inputs, f.outputs
}

func (f mockFactory) Create(config node.Config) (node.Node, error) {
	return mockNode{
		definition: f.definition,
		outputs:    f.outputs,
		fail:       f.fail,
		skip:       f.skip,
		onRun:      f.onRun,
	}, nil
}

// fnFactory 用函数直接构造 Factory（测试辅助）；
// inputs/outputs 直接携带端口契约（注册进内存 definition.Registry）。
type fnFactory struct {
	definition string
	inputs     map[string]definition.InputPort
	outputs    map[string]definition.OutputPort
	create     func(config node.Config) (node.Node, error)
}

func (f fnFactory) Definition() string { return f.definition }
func (f fnFactory) Version() string    { return "v1" }

// Contract 声明端口契约（newTestRegistries 注册进内存 definition.Registry）。
func (f fnFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return f.inputs, f.outputs
}

func (f fnFactory) Create(config node.Config) (node.Node, error) {
	return f.create(config)
}

// recorder 记录 Node 执行顺序与每次执行收到的输入数量。
type recorder struct {
	mu     sync.Mutex
	events []runEvent
}

type runEvent struct {
	nodeType string
	inputs   int
}

func (r *recorder) record(nodeType string, inputs map[string]artifact.ArtifactRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, runEvent{nodeType: nodeType, inputs: len(inputs)})
}

func (r *recorder) order() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	parts := make([]string, len(r.events))
	for i, e := range r.events {
		parts[i] = e.nodeType
	}
	return strings.Join(parts, ",")
}

// inputCount 返回第 n 次（0 起）执行的输入数量。
func (r *recorder) inputCount(nth int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events[nth].inputs
}

// chainDef 是最小 human-free 链（纯数据依赖，无 dependsOn）：
// coding-agent 全 optional 输入当源节点 -> openapi-generator。
func chainDef() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "fullstack-development"},
		Projects:   []workflow.ProjectSpec{{Name: "order-system", Repository: "./examples/order-system"}},
		Nodes: map[string]workflow.NodeSpec{
			"coder": {Node: "coding-agent"},
			"sdk": {
				Node:   "openapi-generator",
				Inputs: map[string]workflow.InputBinding{"openapi": {From: "coder.openapi"}},
			},
		},
	}
}

// chainContracts 是最小 human-free 链的端口契约（与种子
// Node Definition YAML 同构；此处内联声明使测试不依赖内置节点集）。
func chainContracts() map[string]mockFactory {
	return map[string]mockFactory{
		"coding-agent": {
			inputs: map[string]definition.InputPort{
				"analysis-output": {Type: "markdown", Optional: true},
				"architecture":    {Type: "ArchitectureSpec", Optional: true},
				"openapi":         {Type: "OpenAPI", Optional: true},
				"frontend-sdk":    {Type: "FrontendSDK", Optional: true},
			},
			outputs: map[string]definition.OutputPort{
				"source-code": {Type: "SourceCode"},
				"openapi":     {Type: "OpenAPI"},
			},
		},
		"openapi-generator": {
			inputs:  map[string]definition.InputPort{"openapi": {Type: "OpenAPI"}},
			outputs: map[string]definition.OutputPort{"frontend-sdk": {Type: "FrontendSDK"}},
		},
	}
}

// chainDefinitionNames 是链路引用的 Node Definition 顺序。
func chainDefinitionNames() []string {
	return []string{"coding-agent", "openapi-generator"}
}

// chainFactories 构造最小链场景的全部 ExecutorFactory
// （onRun 传入执行记录器；nil 表示不记录）。
func chainFactories(onRun func(nodeType string, inputs map[string]artifact.ArtifactRef)) []node.ExecutorFactory {
	contracts := chainContracts()
	var factories []node.ExecutorFactory
	for _, def := range []string{"requirement-analysis", "architecture-design", "coding-agent", "openapi-generator"} {
		f := contracts[def]
		f.definition = def
		f.onRun = onRun
		factories = append(factories, f)
	}
	return factories
}

// newChainEngine 构造带最小链 Node 契约的引擎与执行记录器。
func newChainEngine(t *testing.T, mods ...func(nodeType string, f *mockFactory)) (*Engine, *recorder) {
	t.Helper()

	rec := new(recorder)
	contracts := chainContracts()
	var factories []node.ExecutorFactory
	for _, def := range chainDefinitionNames() {
		f := contracts[def]
		f.definition = def
		for _, mod := range mods {
			mod(def, &f)
		}
		f.onRun = rec.record
		factories = append(factories, f)
	}
	dr, er := newTestRegistries(t, factories...)
	return NewEngine(er, dr, artifact.NewMemStore(), nil), rec
}

// ---- Engine 测试 ----

// TestRunCreatesIndependentExecutions 验证定义/运行区分：
// 同一个 Workflow 定义可以多次 Run，每次产生独立的 WorkflowExecution
// （#001、#002、#003...），运行对象互不影响、Artifact 各自独立。
func TestRunCreatesIndependentExecutions(t *testing.T) {
	e, _ := newChainEngine(t)
	def := chainDef()

	var execs []*WorkflowExecution
	for i := 0; i < 3; i++ {
		exec, err := e.Run(context.Background(), def)
		if err != nil {
			t.Fatalf("Run() #%d unexpected error: %v", i+1, err)
		}
		if exec.Status != StatusSucceeded {
			t.Fatalf("Run() #%d status = %s, want Succeeded", i+1, exec.Status)
		}
		execs = append(execs, exec)
	}

	// 三次运行的 ID 互不相同。
	if execs[0].ID == execs[1].ID || execs[1].ID == execs[2].ID {
		t.Fatalf("execution IDs not unique: %s %s %s", execs[0].ID, execs[1].ID, execs[2].ID)
	}

	// 每次 Run 都为自己的 WorkflowExecution 创建全部 NodeExecution，
	// 运行对象之间不共享（指针不同、Outputs 独立）。
	for i, exec := range execs {
		for id, ne := range exec.Nodes {
			if ne.NodeID != id {
				t.Errorf("run #%d node %q: NodeID = %q", i+1, id, ne.NodeID)
			}
			if ne.NodeType == "" {
				t.Errorf("run #%d node %q: NodeType empty", i+1, id)
			}
		}
		for j, other := range execs {
			if i == j {
				continue
			}
			for id := range def.Nodes {
				if exec.Nodes[id] == other.Nodes[id] {
					t.Fatalf("run #%d and #%d share NodeExecution pointer for %q", i+1, j+1, id)
				}
			}
		}
	}

	// 不同运行的 Artifact 引用不同（各自的 Store 写入），不串流。
	b1 := execs[0].Nodes["coder"].Outputs["openapi"]
	b2 := execs[1].Nodes["coder"].Outputs["openapi"]
	if b1.URI == b2.URI {
		t.Fatalf("coder.openapi artifact URI identical across runs: %q", b1.URI)
	}

	// 定义对象在三次 Run 之后保持不变（运行不回写定义）。
	if len(def.Nodes) != 2 || def.Nodes["coder"].Node != "coding-agent" {
		t.Fatalf("definition mutated by runs: %+v", def.Nodes["coder"])
	}
}

// TestNodeExecutionCarriesDefinitionIdentity 验证 NodeExecution 快照了
// 定义侧身份：NodeID 来自 Workflow 组合（"backend 这个节点"），
// NodeType 是本次运行实际实例化的类型。
func TestNodeExecutionCarriesDefinitionIdentity(t *testing.T) {
	e, _ := newChainEngine(t)
	exec, err := e.Run(context.Background(), chainDef())
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// coder 与 sdk 是两个不同 Node Definition 的 Node Instance；
	// 各自的 NodeExecution 记录各自的 NodeID 与引用的定义。
	coder := exec.Node("coder")
	sdk := exec.Node("sdk")
	if coder.NodeID != "coder" || sdk.NodeID != "sdk" {
		t.Fatalf("NodeIDs = %q/%q", coder.NodeID, sdk.NodeID)
	}
	if coder.NodeType != "coding-agent" || sdk.NodeType != "openapi-generator" {
		t.Fatalf("NodeTypes = %q/%q", coder.NodeType, sdk.NodeType)
	}
	if coder == sdk {
		t.Fatal("coder and sdk share the same NodeExecution")
	}
}

func TestRunMinimalChain(t *testing.T) {
	e, rec := newChainEngine(t)
	exec, err := e.Run(context.Background(), chainDef())
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if exec.Status != StatusSucceeded {
		t.Fatalf("exec.Status = %s, want Succeeded", exec.Status)
	}
	for id, ns := range exec.Nodes {
		if ns.Status != StatusSucceeded {
			t.Errorf("node %q status = %s, want Succeeded", id, ns.Status)
		}
	}

	// 执行顺序（串行拓扑序）与输入数量：
	// 0:coder(0) 1:sdk(1)
	wantOrder := "coding-agent,openapi-generator"
	if got := rec.order(); got != wantOrder {
		t.Errorf("execution order = %q, want %q", got, wantOrder)
	}
	wantInputs := []int{0, 1}
	for i, want := range wantInputs {
		if got := rec.inputCount(i); got != want {
			t.Errorf("run #%d inputs = %d, want %d", i, got, want)
		}
	}

	// 输出数量：coder 2（source-code + openapi）、sdk 1（frontend-sdk），共 3。
	total := 0
	for _, ns := range exec.Nodes {
		total += len(ns.Outputs)
	}
	if total != 3 {
		t.Errorf("total outputs = %d, want 3", total)
	}

	// 全部输出引用都能从 Store 取回且 Kind 一致。
	for id, ns := range exec.Nodes {
		for name, ref := range ns.Outputs {
			a, err := e.store.Get(ref)
			if err != nil {
				t.Fatalf("Get(%s.%s): %v", id, name, err)
			}
			if a.Kind != ref.Kind {
				t.Errorf("artifact %s.%s kind = %q, want %q", id, name, a.Kind, ref.Kind)
			}
		}
	}
}

func TestRunControlDependency(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(nodeType string, inputs map[string]artifact.ArtifactRef) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, nodeType)
	}

	dr, er := newTestRegistries(t,
		mockFactory{
			definition: "approval",
			outputs:    map[string]definition.OutputPort{"approval": {Type: "ApprovalResult"}},
			onRun:      record,
		},
		mockFactory{definition: "cd", onRun: record},
	)

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "deploy"},
		Nodes: map[string]workflow.NodeSpec{
			"approval": {Node: "approval"},
			"deploy":   {Node: "cd", DependsOn: []string{"approval"}},
		},
	}

	e := NewEngine(er, dr, artifact.NewMemStore(), nil)
	exec, err := e.Run(context.Background(), def)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if got, want := strings.Join(order, ","), "approval,cd"; got != want {
		t.Fatalf("execution order = %q, want %q (control edge without data)", got, want)
	}
	// Control Edge 不携带数据：deploy 不产出也不消费。
	if len(exec.Nodes["deploy"].Outputs) != 0 {
		t.Errorf("deploy outputs = %d, want 0", len(exec.Nodes["deploy"].Outputs))
	}
}

func TestRunNodeFailure(t *testing.T) {
	e, _ := newChainEngine(t, func(nodeType string, f *mockFactory) {
		if nodeType == "coding-agent" {
			f.fail = true
		}
	})

	exec, err := e.Run(context.Background(), chainDef())
	if err == nil {
		t.Fatal("Run() = nil error, want failure")
	}
	if !strings.Contains(err.Error(), `node "coder" failed`) {
		t.Errorf("error %q should mention the failed node", err)
	}
	if exec.Status != StatusFailed {
		t.Errorf("exec.Status = %s, want Failed", exec.Status)
	}

	// 失败 Node 记录 Error；下游保持 Pending（最小 Runtime 不做 Skipped 传播）。
	if ns := exec.Nodes["coder"]; ns.Status != StatusFailed || ns.Error == "" {
		t.Errorf("coder state = %+v, want Failed with Error", ns)
	}
	if ns := exec.Nodes["sdk"]; ns.Status != StatusPending {
		t.Errorf("node %q status = %s, want Pending", "sdk", ns.Status)
	}
}

func TestRunUnknownNodeDefinition(t *testing.T) {
	// 空定义注册表：workflow 引用的定义不存在。
	dr, er := newTestRegistries(t)
	e := NewEngine(er, dr, artifact.NewMemStore(), nil)

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "x"},
		Nodes:      map[string]workflow.NodeSpec{"a": {Node: "nonexistent"}},
	}
	_, err := e.Run(context.Background(), def)
	if err == nil {
		t.Fatal("Run() = nil error, want unknown node definition")
	}
	if !strings.Contains(err.Error(), "unknown node definition") {
		t.Errorf("error %q should mention unknown node definition", err)
	}
}

func TestRunMissingOutput(t *testing.T) {
	// 生产者声明了 out 但跳过产出，消费者绑定 out -> 执行期报错。
	dr, er := newTestRegistries(t,
		mockFactory{
			definition: "producer",
			outputs:    map[string]definition.OutputPort{"out": {Type: "KindA"}},
			skip:       map[string]bool{"out": true},
		},
		mockFactory{
			definition: "consumer",
			inputs:     map[string]definition.InputPort{"in": {Type: "KindA"}},
		},
	)

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "missing-output"},
		Nodes: map[string]workflow.NodeSpec{
			"p": {Node: "producer"},
			"c": {Node: "consumer", Inputs: map[string]workflow.InputBinding{"in": {From: "p.out"}}},
		},
	}

	e := NewEngine(er, dr, artifact.NewMemStore(), nil)
	exec, err := e.Run(context.Background(), def)
	if err == nil {
		t.Fatal("Run() = nil error, want missing-output failure")
	}
	if !strings.Contains(err.Error(), `did not produce output "out"`) {
		t.Errorf("error %q should mention missing output", err)
	}
	if exec.Status != StatusFailed {
		t.Errorf("exec.Status = %s, want Failed", exec.Status)
	}
}

// rogueNode 返回契约未声明的输出，验证引擎按 YAML 契约的输出检查。
type rogueNode struct{}

func (rogueNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	ref, err := ctx.Store.Put(artifact.Artifact{ID: "surprise", Kind: "KindA"})
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"surprise": ref}, nil
}

// rogueFactory 的契约只声明 declared 输出，实现却产出 surprise。
type rogueFactory struct{}

func (rogueFactory) Definition() string { return "rogue" }
func (rogueFactory) Version() string    { return "v1" }
func (rogueFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return nil, map[string]definition.OutputPort{"declared": {Type: "KindA"}}
}
func (rogueFactory) Create(config node.Config) (node.Node, error) {
	return rogueNode{}, nil
}

func TestRunUndeclaredOutput(t *testing.T) {
	dr, er := newTestRegistries(t, rogueFactory{})

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "rogue"},
		Nodes:      map[string]workflow.NodeSpec{"a": {Node: "rogue"}},
	}

	e := NewEngine(er, dr, artifact.NewMemStore(), nil)
	_, err := e.Run(context.Background(), def)
	if err == nil {
		t.Fatal("Run() = nil error, want undeclared-output rejection")
	}
	if !strings.Contains(err.Error(), "undeclared output") {
		t.Errorf("error %q should mention undeclared output", err)
	}
}

func TestRunContextCanceled(t *testing.T) {
	e, _ := newChainEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec, err := e.Run(ctx, chainDef())
	if err == nil {
		t.Fatal("Run() = nil error, want cancellation")
	}
	if exec == nil {
		t.Fatal("Run() exec = nil, want execution with Pending nodes")
	}
	for id, ns := range exec.Nodes {
		if ns.Status != StatusPending {
			t.Errorf("node %q status = %s, want Pending (canceled before start)", id, ns.Status)
		}
	}
}

// ---- MemStore 测试 ----

func TestMemStoreRoundtrip(t *testing.T) {
	s := artifact.NewMemStore()

	a := artifact.Artifact{ID: "openapi", Kind: artifact.KindOpenAPI, Version: "1", Data: "spec"}
	ref, err := s.Put(a)
	if err != nil {
		t.Fatalf("Put() unexpected error: %v", err)
	}
	if !s.Exists(ref) {
		t.Error("Exists(ref) = false after Put")
	}

	got, err := s.Get(ref)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.ID != a.ID || got.Kind != a.Kind || got.Data != a.Data {
		t.Fatalf("Get() = %+v, want %+v", got, a)
	}
}

func TestMemStoreSameIDDistinctRefs(t *testing.T) {
	s := artifact.NewMemStore()

	r1, err := s.Put(artifact.Artifact{ID: "source-code", Kind: artifact.KindSourceCode})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.Put(artifact.Artifact{ID: "source-code", Kind: artifact.KindSourceCode})
	if err != nil {
		t.Fatal(err)
	}
	if r1.URI == r2.URI {
		t.Fatalf("two Puts of same ID produced same URI %q", r1.URI)
	}
	if _, err := s.Get(r1); err != nil {
		t.Errorf("Get(r1): %v", err)
	}
	if _, err := s.Get(r2); err != nil {
		t.Errorf("Get(r2): %v", err)
	}
}

func TestMemStoreGetUnknown(t *testing.T) {
	s := artifact.NewMemStore()
	ref := artifact.ArtifactRef{ID: "x", Kind: artifact.KindOpenAPI, URI: "mem://999"}
	if s.Exists(ref) {
		t.Error("Exists(unknown) = true, want false")
	}
	if _, err := s.Get(ref); err == nil {
		t.Error("Get(unknown) = nil error, want not-found")
	}
}

func TestMemStorePutValidates(t *testing.T) {
	s := artifact.NewMemStore()
	if _, err := s.Put(artifact.Artifact{}); err == nil {
		t.Error("Put(empty) = nil error, want validation failure")
	}
}
