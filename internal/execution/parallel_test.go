package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// slowNodeFactory 产出可阻塞的 Node：block 释放前 Execute 不返回。
type slowNodeFactory struct {
	definition string
	inputs     map[string]definition.InputPort
	outputs    map[string]definition.OutputPort
	block      chan struct{}
	running    *atomic.Int32 // 当前正在执行的实例数
	maxSeen    *atomic.Int32 // 并发执行峰值
}

func (f *slowNodeFactory) Definition() string { return f.definition }
func (f *slowNodeFactory) Version() string    { return "v1" }

// Contract 声明端口契约（newTestRegistries 注册进内存 definition.Registry）。
func (f *slowNodeFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return f.inputs, f.outputs
}

func (f *slowNodeFactory) Create(config node.Config) (node.Node, error) {
	return slowNode{f: f}, nil
}

type slowNode struct {
	f *slowNodeFactory
}

func (n slowNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	cur := n.f.running.Add(1)
	for {
		old := n.f.maxSeen.Load()
		if cur <= old || n.f.maxSeen.CompareAndSwap(old, cur) {
			break
		}
	}
	defer n.f.running.Add(-1)

	select {
	case <-n.f.block:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	outputs := make(map[string]artifact.ArtifactRef, len(n.f.outputs))
	for name, port := range n.f.outputs {
		ref, err := ctx.Store.Put(artifact.Artifact{ID: name, Kind: artifact.Kind(port.Type)})
		if err != nil {
			return nil, err
		}
		outputs[name] = ref
	}
	return outputs, nil
}

// diamondDef 是计划 §38 / Case 2 的菱形 DAG：A -> B/C -> D。
func diamondDef() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "diamond"},
		Nodes: map[string]workflow.NodeSpec{
			"a": {Node: "x"},
			"b": {Node: "x", Inputs: map[string]workflow.InputBinding{"in": {From: "a.out"}}},
			"c": {Node: "x", Inputs: map[string]workflow.InputBinding{"in": {From: "a.out"}}},
			"d": {Node: "join",
				Inputs: map[string]workflow.InputBinding{
					"left":  {From: "b.out"},
					"right": {From: "c.out"},
				}},
		},
	}
}

// newDiamondEngine 构造菱形 DAG 引擎；所有同 Type Node 共享同一组闸门与计数器。
func newDiamondEngine(t *testing.T) (*Engine, *atomic.Int32, *atomic.Int32, chan struct{}) {
	t.Helper()

	running, maxSeen := &atomic.Int32{}, &atomic.Int32{}
	block := make(chan struct{})

	xFactory := &slowNodeFactory{
		definition: "x",
		inputs:     map[string]definition.InputPort{"in": {Type: "SourceCode"}},
		outputs:    map[string]definition.OutputPort{"out": {Type: "SourceCode"}},
		block:      block, running: running, maxSeen: maxSeen,
	}
	joinFactory := &slowNodeFactory{
		definition: "join",
		inputs: map[string]definition.InputPort{
			"left":  {Type: "SourceCode"},
			"right": {Type: "SourceCode"},
		},
		outputs: map[string]definition.OutputPort{"joined": {Type: "SourceCode"}},
		block:   block, running: running, maxSeen: maxSeen,
	}
	dr, er := newTestRegistries(t, xFactory, joinFactory)
	return NewEngine(er, dr, artifact.NewMemStore(), nil), running, maxSeen, block
}

// TestParallelDiamondRunsConcurrently（计划 Case 2 / §38）：
// B 与 C 无依赖关系，parallelism=2 时必须同时在跑（并发峰值 >= 2），
// D 等待两者完成。
//
// 门控策略：A 与 B/C 用同一闸门会因 close 的广播语义一次性放行全部节点，
// 无法区分「A 在跑」与「B/C 同时在跑」。因此 A 的 Execute 只等 gateA，
// B/C/D 只等 gateBC：先放 A，观察到 B、C 进入（running==2）即证明并发派发。
func TestParallelDiamondRunsConcurrently(t *testing.T) {
	running, maxSeen := &atomic.Int32{}, &atomic.Int32{}
	gateA, gateBC := make(chan struct{}), make(chan struct{})

	// 通过 config 无从区分节点位置，这里用输入数量区分：
	// A 无输入（源），B/C 各 1 输入，D 2 输入。
	stageFactory := &stagedNodeFactory{
		definition: "x",
		gates: func(inputs int) <-chan struct{} {
			if inputs == 0 {
				return gateA
			}
			return gateBC
		},
		running: running, maxSeen: maxSeen,
		inputs:  map[string]definition.InputPort{"in": {Type: "SourceCode"}},
		outputs: map[string]definition.OutputPort{"out": {Type: "SourceCode"}},
	}
	joinFactory := &stagedNodeFactory{
		definition: "join",
		gates:      func(inputs int) <-chan struct{} { return gateBC },
		running:    running, maxSeen: maxSeen,
		inputs:  map[string]definition.InputPort{"left": {Type: "SourceCode"}, "right": {Type: "SourceCode"}},
		outputs: map[string]definition.OutputPort{"joined": {Type: "SourceCode"}},
	}
	dr, er := newTestRegistries(t, stageFactory, joinFactory)

	e := NewEngine(er, dr, artifact.NewMemStore(), nil, WithParallelism(2))

	done := make(chan struct{})
	go func() {
		exec, err := e.Run(context.Background(), diamondDef())
		if err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
		if exec.Status != StatusSucceeded {
			t.Errorf("status = %s", exec.Status)
		}
		close(done)
	}()

	// 第一步：等到 A 在跑（running==1），放行 A。
	waitFor(t, running, 1, 5*time.Second, "node A to start")
	close(gateA)

	// 第二步：B 与 C 必须被并发派发（running==2）。
	waitFor(t, running, 2, 5*time.Second, "b and c to run concurrently")
	if got := maxSeen.Load(); got < 2 {
		t.Fatalf("max concurrent nodes = %d, want >= 2", got)
	}

	// 第三步：放行 B/C，D 随后执行并完成。
	close(gateBC)
	<-done
}

// waitFor 轮询等待计数达到 want。
func waitFor(t *testing.T, c *atomic.Int32, want int32, timeout time.Duration, what string) {
	t.Helper()
	deadline := time.After(timeout)
	for c.Load() < want {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s (count=%d, want>=%d)", what, c.Load(), want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// stagedNodeFactory 产出按「本次 Execute 收到的输入数量」选择闸门的 Node。
// definition 由构造方给定（同一工厂逻辑可注册成不同定义）。
type stagedNodeFactory struct {
	definition string
	inputs     map[string]definition.InputPort
	outputs    map[string]definition.OutputPort
	gates      func(inputs int) <-chan struct{}
	running    *atomic.Int32
	maxSeen    *atomic.Int32
}

func (f *stagedNodeFactory) Definition() string { return f.definition }
func (f *stagedNodeFactory) Version() string    { return "v1" }

// Contract 声明端口契约（newTestRegistries 注册进内存 definition.Registry）。
func (f *stagedNodeFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return f.inputs, f.outputs
}

func (f *stagedNodeFactory) Create(config node.Config) (node.Node, error) {
	return stagedNode{f: f}, nil
}

type stagedNode struct{ f *stagedNodeFactory }

func (n stagedNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	cur := n.f.running.Add(1)
	for {
		old := n.f.maxSeen.Load()
		if cur <= old || n.f.maxSeen.CompareAndSwap(old, cur) {
			break
		}
	}
	defer n.f.running.Add(-1)

	select {
	case <-n.f.gates(len(inputs)):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 输出名与声明一致：join 输出 joined，其余输出 out。
	outName := "out"
	if _, ok := n.f.outputs["joined"]; ok {
		outName = "joined"
	}
	ref, err := ctx.Store.Put(artifact.Artifact{ID: outName, Kind: artifact.KindSourceCode})
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{outName: ref}, nil
}

// TestParallelSerialFallback：parallelism<=1 时保持串行（并发峰值恒为 1）。
func TestParallelSerialFallback(t *testing.T) {
	running, maxSeen := &atomic.Int32{}, &atomic.Int32{}
	gateA, gateBC := make(chan struct{}), make(chan struct{})

	// 三节点扇出（A -> B, A -> C），全部同一 Type；A 等待 gateA，
	// B/C 等待 gateBC。串行模式下 A 阻塞期间 B/C 绝不能进入。
	dr, er := newTestRegistries(t, &stagedNodeFactory{
		definition: "staged",
		gates: func(inputs int) <-chan struct{} {
			if inputs == 0 {
				return gateA
			}
			return gateBC
		},
		running: running, maxSeen: maxSeen,
		inputs:  map[string]definition.InputPort{"in": {Type: "SourceCode"}},
		outputs: map[string]definition.OutputPort{"out": {Type: "SourceCode"}},
	})

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "fanout-serial"},
		Nodes: map[string]workflow.NodeSpec{
			"a": {Node: "staged"},
			"b": {Node: "staged", Inputs: map[string]workflow.InputBinding{"in": {From: "a.out"}}},
			"c": {Node: "staged", Inputs: map[string]workflow.InputBinding{"in": {From: "a.out"}}},
		},
	}

	e := NewEngine(er, dr, artifact.NewMemStore(), nil) // 默认串行
	done := make(chan struct{})
	go func() {
		exec, err := e.Run(context.Background(), def)
		if err != nil {
			t.Errorf("Run() unexpected error: %v", err)
		}
		if exec.Status != StatusSucceeded {
			t.Errorf("status = %s", exec.Status)
		}
		close(done)
	}()

	waitFor(t, running, 1, 5*time.Second, "node A to start")
	time.Sleep(50 * time.Millisecond) // 串行下 B/C 不可能在 A 阻塞时进入。
	if got := running.Load(); got != 1 {
		t.Fatalf("running = %d during A, want 1 (serial)", got)
	}
	close(gateA)
	close(gateBC) // 放行剩余节点。
	<-done

	if got := maxSeen.Load(); got != 1 {
		t.Fatalf("max concurrent nodes = %d, want 1 (serial)", got)
	}
}

// TestParallelFullstackSucceeds：并行模式跑完整 fullstack（链路本身无并行度，
// 验证并行路径不破坏依赖正确性）。
func TestParallelFullstackSucceeds(t *testing.T) {
	dr, er := newTestRegistries(t, fullstackFactories(nil)...)
	e := NewEngine(er, dr, artifact.NewMemStore(), nil, WithParallelism(4))

	exec, err := e.Run(context.Background(), fullstackDef())
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if exec.Status != StatusSucceeded {
		t.Fatalf("status = %s, want Succeeded", exec.Status)
	}
	for id, ne := range exec.Nodes {
		if ne.Status != StatusSucceeded {
			t.Errorf("node %q status = %s", id, ne.Status)
		}
	}
}

// TestParallelFailureStopsDispatch：A 失败后，B/C 不得被执行（失败即停派发）。
func TestParallelFailureStopsDispatch(t *testing.T) {
	// 记录哪些下游 Node 真正执行过。
	var mu sync.Mutex
	ran := map[string]bool{}

	dr, er := newTestRegistries(t,
		fnFactory{
			definition: "fail",
			outputs:    map[string]definition.OutputPort{"out": {Type: "SourceCode"}},
			create: func(config node.Config) (node.Node, error) {
				return failingNode{}, nil
			},
		},
		fnFactory{
			definition: "ok",
			outputs:    map[string]definition.OutputPort{"out": {Type: "SourceCode"}},
			create: func(config node.Config) (node.Node, error) {
				return recordNode{ran: ran, mu: &mu, kind: artifact.KindSourceCode}, nil
			},
		},
	)

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "fail-fast"},
		Nodes: map[string]workflow.NodeSpec{
			"a": {Node: "fail"},
			"b": {Node: "ok", Inputs: map[string]workflow.InputBinding{"in": {From: "a.out"}}},
			"c": {Node: "ok", Inputs: map[string]workflow.InputBinding{"in": {From: "a.out"}}},
		},
	}

	e := NewEngine(er, dr, artifact.NewMemStore(), nil, WithParallelism(4))
	exec, err := e.Run(context.Background(), def)
	if err == nil {
		t.Fatal("Run() = nil error, want failure")
	}
	if !strings.Contains(err.Error(), `"a"`) {
		t.Errorf("error %q should mention node a", err)
	}
	if exec.Status != StatusFailed {
		t.Errorf("status = %s, want Failed", exec.Status)
	}
	if exec.Nodes["a"].Status != StatusFailed {
		t.Errorf("a status = %s", exec.Nodes["a"].Status)
	}
	// 下游不被派发。
	mu.Lock()
	defer mu.Unlock()
	if len(ran) != 0 {
		t.Errorf("downstream nodes executed after failure: %v", ran)
	}
}

// recordNode 记录自己被执行过。
type recordNode struct {
	ran  map[string]bool
	mu   *sync.Mutex
	kind artifact.Kind
}

func (n recordNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	n.mu.Lock()
	n.ran["ok-node"] = true
	n.mu.Unlock()
	ref, err := ctx.Store.Put(artifact.Artifact{ID: "out", Kind: n.kind})
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"out": ref}, nil
}

// failingNode 立即失败。
type failingNode struct{}

func (failingNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, errors.New("planned failure")
}
