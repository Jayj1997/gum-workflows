package execution

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// ---- 测试用可执行 Mock Node（仅测试内使用，内置 Mock Node 属后续里程碑）----

type mockNode struct {
	nodeType string
	schema   node.Schema
	fail     bool
	// skip 声明但不产出的输出名（模拟 Node 未产出某个声明过的产出）。
	skip map[string]bool
	// onRun 记录执行事件（含收到的输入），供顺序断言使用。
	onRun func(nodeType string, inputs map[string]artifact.ArtifactRef)
}

func (m mockNode) Type() string             { return m.nodeType }
func (m mockNode) InputSchema() node.Schema { return m.schema }
func (m mockNode) OutputSchema() node.Schema {
	return m.schema
}

func (m mockNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	if m.onRun != nil {
		m.onRun(m.nodeType, inputs)
	}
	if m.fail {
		return nil, fmt.Errorf("mock failure of %q", m.nodeType)
	}
	outputs := make(map[string]artifact.ArtifactRef, len(m.schema.Outputs))
	for name, kind := range m.schema.Outputs {
		if m.skip[name] {
			continue
		}
		ref, err := ctx.Store.Put(artifact.Artifact{
			ID:      name,
			Kind:    kind,
			Version: "1",
			Data:    map[string]any{"nodeType": m.nodeType, "inputs": len(inputs)},
		})
		if err != nil {
			return nil, fmt.Errorf("put output %q: %w", name, err)
		}
		outputs[name] = ref
	}
	return outputs, nil
}

type mockFactory struct {
	nodeType string
	schema   node.Schema
	fail     bool
	skip     map[string]bool
	onRun    func(nodeType string, inputs map[string]artifact.ArtifactRef)
}

func (f mockFactory) Type() string { return f.nodeType }

func (f mockFactory) Create(config node.Config) (node.Node, error) {
	return mockNode{
		nodeType: f.nodeType,
		schema:   f.schema,
		fail:     f.fail,
		skip:     f.skip,
		onRun:    f.onRun,
	}, nil
}

// fnFactory 用函数直接构造 Factory（测试辅助）。
type fnFactory struct {
	nodeType string
	create   func(config node.Config) (node.Node, error)
}

func (f fnFactory) Type() string { return f.nodeType }

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

// fullstackDef 是计划 §10 的 fullstack Workflow（纯数据依赖，无 dependsOn）。
func fullstackDef() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "fullstack-development"},
		Project:    workflow.ProjectSpec{Repository: "./examples/order-system", Branch: "main"},
		Nodes: map[string]workflow.NodeSpec{
			"requirement": {Type: "requirement-analysis"},
			"architecture": {
				Type:   "architecture-design",
				Inputs: map[string]workflow.InputBinding{"requirement": {From: "requirement.requirement"}},
			},
			"backend": {
				Type: "coding-agent",
				Inputs: map[string]workflow.InputBinding{
					"requirement":  {From: "requirement.requirement"},
					"architecture": {From: "architecture.architecture"},
				},
			},
			"openapi": {
				Type:   "openapi-generator",
				Inputs: map[string]workflow.InputBinding{"openapi": {From: "backend.openapi"}},
			},
			"frontend": {
				Type: "coding-agent",
				Inputs: map[string]workflow.InputBinding{
					"requirement":  {From: "requirement.requirement"},
					"openapi":      {From: "backend.openapi"},
					"frontend-sdk": {From: "openapi.frontend-sdk"},
				},
			},
		},
	}
}

// newFullstackEngine 构造带 fullstack Node Type 的引擎与执行记录器。
func newFullstackEngine(t *testing.T, mods ...func(nodeType string, f *mockFactory)) (*Engine, *recorder) {
	t.Helper()

	rec := new(recorder)
	reg := node.NewRegistry()
	for _, f := range []mockFactory{
		{
			nodeType: "requirement-analysis",
			schema:   node.Schema{Outputs: map[string]artifact.Kind{"requirement": artifact.KindRequirementSpec}},
		},
		{
			nodeType: "architecture-design",
			schema: node.Schema{
				Inputs:  map[string]artifact.Kind{"requirement": artifact.KindRequirementSpec},
				Outputs: map[string]artifact.Kind{"architecture": artifact.KindArchitectureSpec},
			},
		},
		{
			nodeType: "coding-agent",
			schema: node.Schema{
				OptionalInputs: map[string]artifact.Kind{
					"requirement":  artifact.KindRequirementSpec,
					"architecture": artifact.KindArchitectureSpec,
					"openapi":      artifact.KindOpenAPI,
					"frontend-sdk": artifact.KindFrontendSDK,
				},
				Outputs: map[string]artifact.Kind{
					"source-code": artifact.KindSourceCode,
					"openapi":     artifact.KindOpenAPI,
				},
			},
		},
		{
			nodeType: "openapi-generator",
			schema: node.Schema{
				Inputs:  map[string]artifact.Kind{"openapi": artifact.KindOpenAPI},
				Outputs: map[string]artifact.Kind{"frontend-sdk": artifact.KindFrontendSDK},
			},
		},
	} {
		for _, mod := range mods {
			mod(f.nodeType, &f)
		}
		f.onRun = rec.record
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %q: %v", f.Type(), err)
		}
	}
	return NewEngine(reg, artifact.NewMemStore(), nil), rec
}

// ---- Engine 测试 ----

// TestRunCreatesIndependentExecutions 验证定义/运行区分：
// 同一个 Workflow 定义可以多次 Run，每次产生独立的 WorkflowExecution
// （#001、#002、#003...），运行对象互不影响、Artifact 各自独立。
func TestRunCreatesIndependentExecutions(t *testing.T) {
	e, _ := newFullstackEngine(t)
	def := fullstackDef()

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
	b1 := execs[0].Nodes["backend"].Outputs["openapi"]
	b2 := execs[1].Nodes["backend"].Outputs["openapi"]
	if b1.URI == b2.URI {
		t.Fatalf("backend.openapi artifact URI identical across runs: %q", b1.URI)
	}

	// 定义对象在三次 Run 之后保持不变（运行不回写定义）。
	if len(def.Nodes) != 5 || def.Nodes["backend"].Type != "coding-agent" {
		t.Fatalf("definition mutated by runs: %+v", def.Nodes["backend"])
	}
}

// TestNodeExecutionCarriesDefinitionIdentity 验证 NodeExecution 快照了
// 定义侧身份：NodeID 来自 Workflow 组合（"backend 这个节点"），
// NodeType 是本次运行实际实例化的类型。
func TestNodeExecutionCarriesDefinitionIdentity(t *testing.T) {
	e, _ := newFullstackEngine(t)
	exec, err := e.Run(context.Background(), fullstackDef())
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// backend 与 frontend 是同一 Node Type 的两个不同 Node 定义；
	// 各自的 NodeExecution 记录各自的 NodeID 与同一 NodeType。
	backend := exec.Node("backend")
	frontend := exec.Node("frontend")
	if backend.NodeID != "backend" || frontend.NodeID != "frontend" {
		t.Fatalf("NodeIDs = %q/%q", backend.NodeID, frontend.NodeID)
	}
	if backend.NodeType != "coding-agent" || frontend.NodeType != "coding-agent" {
		t.Fatalf("NodeTypes = %q/%q", backend.NodeType, frontend.NodeType)
	}
	if backend == frontend {
		t.Fatal("backend and frontend share the same NodeExecution")
	}
}

func TestRunFullstack(t *testing.T) {
	e, rec := newFullstackEngine(t)
	exec, err := e.Run(context.Background(), fullstackDef())
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
	// 0:requirement(0) 1:architecture(1) 2:backend(2) 3:openapi(1) 4:frontend(3)
	wantOrder := "requirement-analysis,architecture-design,coding-agent,openapi-generator,coding-agent"
	if got := rec.order(); got != wantOrder {
		t.Errorf("execution order = %q, want %q", got, wantOrder)
	}
	wantInputs := []int{0, 1, 2, 1, 3}
	for i, want := range wantInputs {
		if got := rec.inputCount(i); got != want {
			t.Errorf("run #%d inputs = %d, want %d", i, got, want)
		}
	}

	// 输出数量：requirement/architecture/openapi 各 1，backend/frontend 各 2，共 7。
	total := 0
	for _, ns := range exec.Nodes {
		total += len(ns.Outputs)
	}
	if total != 7 {
		t.Errorf("total outputs = %d, want 7", total)
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

	reg := node.NewRegistry()
	for _, f := range []mockFactory{
		{nodeType: "approval", schema: node.Schema{Outputs: map[string]artifact.Kind{"approval": artifact.KindApprovalResult}}, onRun: record},
		{nodeType: "cd", onRun: record},
	} {
		if err := reg.Register(f); err != nil {
			t.Fatal(err)
		}
	}

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "deploy"},
		Nodes: map[string]workflow.NodeSpec{
			"approval": {Type: "approval"},
			"deploy":   {Type: "cd", DependsOn: []string{"approval"}},
		},
	}

	e := NewEngine(reg, artifact.NewMemStore(), nil)
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
	e, _ := newFullstackEngine(t, func(nodeType string, f *mockFactory) {
		if nodeType == "architecture-design" {
			f.fail = true
		}
	})

	exec, err := e.Run(context.Background(), fullstackDef())
	if err == nil {
		t.Fatal("Run() = nil error, want failure")
	}
	if !strings.Contains(err.Error(), `node "architecture" failed`) {
		t.Errorf("error %q should mention the failed node", err)
	}
	if exec.Status != StatusFailed {
		t.Errorf("exec.Status = %s, want Failed", exec.Status)
	}

	// 失败 Node 记录 Error；下游保持 Pending（最小 Runtime 不做 Skipped 传播）。
	if ns := exec.Nodes["architecture"]; ns.Status != StatusFailed || ns.Error == "" {
		t.Errorf("architecture state = %+v, want Failed with Error", ns)
	}
	for _, id := range []string{"backend", "openapi", "frontend"} {
		if ns := exec.Nodes[id]; ns.Status != StatusPending {
			t.Errorf("node %q status = %s, want Pending", id, ns.Status)
		}
	}
	// 失败前已完成的 Node 不受影响。
	if ns := exec.Nodes["requirement"]; ns.Status != StatusSucceeded {
		t.Errorf("requirement status = %s, want Succeeded", ns.Status)
	}
}

func TestRunUnknownNodeType(t *testing.T) {
	e := NewEngine(node.NewRegistry(), artifact.NewMemStore(), nil)

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "x"},
		Nodes:      map[string]workflow.NodeSpec{"a": {Type: "nonexistent"}},
	}
	_, err := e.Run(context.Background(), def)
	if err == nil {
		t.Fatal("Run() = nil error, want unknown node type")
	}
	if !strings.Contains(err.Error(), "unknown node type") {
		t.Errorf("error %q should mention unknown node type", err)
	}
}

func TestRunMissingOutput(t *testing.T) {
	// 生产者声明了 out 但跳过产出，消费者绑定 out -> 执行期报错。
	reg := node.NewRegistry()
	for _, f := range []mockFactory{
		{nodeType: "producer", schema: node.Schema{Outputs: map[string]artifact.Kind{"out": "KindA"}}, skip: map[string]bool{"out": true}},
		{nodeType: "consumer", schema: node.Schema{Inputs: map[string]artifact.Kind{"in": "KindA"}}},
	} {
		if err := reg.Register(f); err != nil {
			t.Fatal(err)
		}
	}

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "missing-output"},
		Nodes: map[string]workflow.NodeSpec{
			"p": {Type: "producer"},
			"c": {Type: "consumer", Inputs: map[string]workflow.InputBinding{"in": {From: "p.out"}}},
		},
	}

	e := NewEngine(reg, artifact.NewMemStore(), nil)
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

// rogueNode 返回 OutputSchema 未声明的输出，验证引擎的输出契约检查。
type rogueNode struct{}

func (rogueNode) Type() string             { return "rogue" }
func (rogueNode) InputSchema() node.Schema { return node.Schema{} }
func (rogueNode) OutputSchema() node.Schema {
	return node.Schema{Outputs: map[string]artifact.Kind{"declared": "KindA"}}
}

func (rogueNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	ref, err := ctx.Store.Put(artifact.Artifact{ID: "surprise", Kind: "KindA"})
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"surprise": ref}, nil
}

func TestRunUndeclaredOutput(t *testing.T) {
	reg := node.NewRegistry()
	if err := reg.Register(fnFactory{
		nodeType: "rogue",
		create:   func(config node.Config) (node.Node, error) { return rogueNode{}, nil },
	}); err != nil {
		t.Fatal(err)
	}

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "rogue"},
		Nodes:      map[string]workflow.NodeSpec{"a": {Type: "rogue"}},
	}

	e := NewEngine(reg, artifact.NewMemStore(), nil)
	_, err := e.Run(context.Background(), def)
	if err == nil {
		t.Fatal("Run() = nil error, want undeclared-output rejection")
	}
	if !strings.Contains(err.Error(), "undeclared output") {
		t.Errorf("error %q should mention undeclared output", err)
	}
}

func TestRunContextCanceled(t *testing.T) {
	e, _ := newFullstackEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec, err := e.Run(ctx, fullstackDef())
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
