// Engine 是串行执行引擎（设计计划 §34 Milestone 6：单进程串行执行）。
//
// 契约：入参 def 是已通过两层 Validator 的 Workflow Definition，
// 且其引用的全部 Node Type 已在 Registry 注册（与 BuildGraph 同一契约）。
//
// 定义/运行区分：def 是定义（Workflow + Node）；Run 的返回值是运行对象
// （WorkflowExecution + NodeExecution），同一定义可多次 Run，互不影响。
package execution

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync/atomic"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

var executionSeq atomic.Int64

// Option 配置 Engine 的可选行为。
type Option func(*Engine)

// WithStateDir 让 Engine 在运行过程中将 WorkflowExecution 状态
// 持久化到 <dir>/<execution-id>/（计划 §28 布局），每个 Node 状态变化后刷新。
func WithStateDir(dir string) Option {
	return func(e *Engine) { e.stateDir = dir }
}

// WithProjectContext 注入 Project Runtime 解析好的运行环境（含 Workspace）。
// 未注入时 Engine 依据 YAML 的 project 段构造最小 Context（无 Workspace）。
func WithProjectContext(ctx project.Context) Option {
	return func(e *Engine) {
		c := ctx
		e.projectCtx = &c
	}
}

// WithExecutionID 注入本次运行的 Execution ID（应由 NextExecutionID 分配，
// 保证与磁盘目录一致）。未注入时使用进程内自增序号（适合单次进程内多次 Run
// 的库用法，如测试）。
func WithExecutionID(id string) Option {
	return func(e *Engine) { e.executionID = id }
}

// WithParallelism 设置最大并行 Node 数（M7 并行 DAG）。
// n <= 1 时保持串行（默认）。计划 §38：无依赖的 Node 同时执行。
func WithParallelism(n int) Option {
	return func(e *Engine) {
		if n > 1 {
			e.parallelism = n
		}
	}
}

// Engine 串行执行 Workflow：Ready 的 Node 依次执行，
// 完成后推进依赖计数并将新 Ready 的 Node 入队，直至完成或首个失败。
type Engine struct {
	executors   *node.ExecutorRegistry
	defs        *definition.Registry
	store       artifact.Store
	logger      *slog.Logger
	stateDir    string
	parallelism int
	projectCtx  *project.Context
	executionID string
}

// NewEngine 创建引擎。契约（inputs/outputs）从 defs（Node Definition
// YAML）读取；Go 实现经 executors 实例化。logger 为 nil 时丢弃日志。
func NewEngine(executors *node.ExecutorRegistry, defs *definition.Registry, store artifact.Store, logger *slog.Logger, opts ...Option) *Engine {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Engine{executors: executors, defs: defs, store: store, logger: logger}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run 执行 def 直到完成或首个 Node 失败，返回本次运行的 WorkflowExecution。
//
// 调度模型：parallelism=1（默认）严格串行；>1 时用 worker 池并发消费
// Ready Queue（计划 §38）。无论并行与否，状态迁移与依赖计数推进都只在
// 调度 goroutine 内发生，worker 只执行 Node 本身（Execute）。
//
// 失败语义（最小 Runtime）：首个失败的 NodeExecution 记录 Error 并置 Failed，
// WorkflowExecution 置 Failed 后等待在途 Node 结束再返回（不强行中断），
// 未执行的 NodeExecution 保持 Pending（不做 Skipped 传播与重试，属后续版本）。
func (e *Engine) Run(ctx context.Context, def workflow.Definition) (*WorkflowExecution, error) {
	g, err := workflow.BuildGraph(def)
	if err != nil {
		return nil, fmt.Errorf("build dag: %w", err)
	}

	// 依据 Registry 实例化全部 Node 定义（设计计划 §12）。
	nodes, err := e.instantiate(def)
	if err != nil {
		return nil, err
	}

	// 为本次运行创建运行时对象：每个 Node 定义对应一个 NodeExecution。
	// ID 优先取注入值（与磁盘目录一致），否则进程内自增。
	execID := e.executionID
	if execID == "" {
		execID = fmt.Sprintf("execution-%06d", executionSeq.Add(1))
	}
	exec := &WorkflowExecution{
		ID:       execID,
		Workflow: def.Metadata.Name,
		Status:   StatusRunning,
		Nodes:    make(map[string]*NodeExecution, len(g.NodeIDs)),
	}
	for _, id := range g.NodeIDs {
		exec.Nodes[id] = &NodeExecution{
			NodeID:   id,
			NodeType: def.Nodes[id].Type,
			Status:   StatusPending,
		}
	}

	sched := newScheduler(g)

	execCtx := node.ExecutionContext{
		Context: ctx,
		Project: e.projectContext(def),
		Store:   e.store,
		Logger:  e.logger,
	}

	// 启动前检查取消：保证取消发生在任何状态迁移之前（全部 NodeExecution 保持 Pending）。
	if err := ctx.Err(); err != nil {
		exec.Status = StatusFailed
		e.persist(exec)
		return exec, fmt.Errorf("execution %s canceled: %w", exec.ID, err)
	}

	// 源节点（无入边）立即 Ready：Ready(Node) = 前驱全部完成，源节点恒成立（计划 §8）。
	for _, id := range g.Roots() {
		if err := markReady(exec, id); err != nil {
			exec.Status = StatusFailed
			return exec, err
		}
	}

	if e.parallelism > 1 {
		return e.runParallel(ctx, exec, execCtx, def, nodes, sched)
	}

	for !sched.done() {
		if err := ctx.Err(); err != nil {
			exec.Status = StatusFailed
			return exec, fmt.Errorf("execution %s canceled: %w", exec.ID, err)
		}

		id := sched.next()

		if err := e.runNodeExecution(exec, execCtx, def, nodes, id); err != nil {
			exec.Status = StatusFailed
			e.persist(exec)
			return exec, err
		}
		e.persist(exec)

		// 依赖计数推进：新 Ready 的 Node 入队并迁移状态。
		for _, next := range sched.complete(id) {
			if err := markReady(exec, next); err != nil {
				exec.Status = StatusFailed
				e.persist(exec)
				return exec, err
			}
		}
	}

	exec.Status = StatusSucceeded
	e.logger.Info("workflow succeeded", "execution", exec.ID, "workflow", exec.Workflow)
	e.persist(exec)
	return exec, nil
}

// runParallel 用 worker 池并发执行（M7，计划 §38：无依赖的 Node 同时执行）。
// 调度决策（状态迁移、依赖计数、Ready Queue、失败即停）全部在主循环；
// worker goroutine 只执行 runNodeExecution（含状态写入，但每个 NodeExecution
// 只被自己的 worker 触碰，且结果串行回主循环消费，无竞态）。
func (e *Engine) runParallel(
	ctx context.Context,
	exec *WorkflowExecution,
	execCtx node.ExecutionContext,
	def workflow.Definition,
	nodes map[string]node.Node,
	sched *scheduler,
) (*WorkflowExecution, error) {
	type result struct {
		id  string
		err error
	}

	results := make(chan result)
	inflight := 0
	var runErr error

	launch := func(id string) {
		inflight++
		go func() {
			results <- result{id: id, err: e.runNodeExecution(exec, execCtx, def, nodes, id)}
		}()
	}

	for {
		// 失败即停或取消：不再派发新任务，等待在途 Node 结束（不强杀）。
		stopping := runErr != nil || ctx.Err() != nil

		if !stopping {
			// 按可用 worker 容量派发 Ready Node。
			// fan-out 场景：A 在跑时 B/C 尚未入队（等 A 的结果回主循环后
			// complete 才会把它们入队），inflight>0 保证循环继续等结果而非提前退出。
			for inflight < e.parallelism && !sched.done() {
				launch(sched.next())
			}
		}

		if inflight == 0 {
			// 无在途任务且不再派发：全部 Node 已完成（或失败后 drain 完毕）。
			break
		}

		r := <-results
		inflight--
		if r.err != nil {
			if runErr == nil {
				runErr = r.err
				exec.Status = StatusFailed
			}
		} else {
			for _, next := range sched.complete(r.id) {
				if err := markReady(exec, next); err != nil {
					runErr = err
					exec.Status = StatusFailed
				}
			}
		}
		e.persist(exec)
	}

	if runErr != nil {
		return exec, runErr
	}
	if err := ctx.Err(); err != nil {
		exec.Status = StatusFailed
		return exec, fmt.Errorf("execution %s canceled: %w", exec.ID, err)
	}

	exec.Status = StatusSucceeded
	e.logger.Info("workflow succeeded", "execution", exec.ID, "workflow", exec.Workflow, "parallelism", e.parallelism)
	e.persist(exec)
	return exec, nil
}

// runNodeExecution 执行单个 Node 实例：迁移到 Running、解析输入、
// 调用 Node Execute、记录输出。输入引用来自生产者 NodeExecution 的 Outputs。
func (e *Engine) runNodeExecution(
	exec *WorkflowExecution,
	execCtx node.ExecutionContext,
	def workflow.Definition,
	nodes map[string]node.Node,
	id string,
) error {
	ne := exec.Nodes[id]
	if err := ne.TransitionTo(StatusRunning); err != nil {
		return fmt.Errorf("node %q: %w", id, err)
	}
	e.logger.Info("node running", "execution", exec.ID, "node", id)

	// 解析输入：每个绑定的 <node-id>.<output> 必须已由生产者 NodeExecution 产出。
	inputs := make(map[string]artifact.ArtifactRef, len(def.Nodes[id].Inputs))
	for name, binding := range def.Nodes[id].Inputs {
		fromNode, fromOutput, err := workflow.ParseRef(binding.From)
		if err != nil {
			return fmt.Errorf("node %q input %q: %w", id, name, err)
		}
		ref, ok := exec.Nodes[fromNode].Outputs[fromOutput]
		if !ok {
			return fmt.Errorf("node %q input %q: node %q did not produce output %q",
				id, name, fromNode, fromOutput)
		}
		inputs[name] = ref
	}

	outputs, err := nodes[id].Execute(execCtx, inputs)
	if err != nil {
		ne.Error = err.Error()
		if terr := ne.TransitionTo(StatusFailed); terr != nil {
			return fmt.Errorf("node %q: %w; also: %w", id, terr, err)
		}
		return fmt.Errorf("node %q failed: %w", id, err)
	}

	// 输出契约检查（按 YAML 契约执行，设计文档 §6.9）：
	// 返回的输出名必须已声明，且产出 Kind 落在声明类型的取值集合内。
	declared, err := e.declaredOutputs(def.Nodes[id].Type)
	if err != nil {
		return fmt.Errorf("node %q: %w", id, err)
	}
	for name, ref := range outputs {
		want, ok := declared[name]
		if !ok {
			return fmt.Errorf("node %q returned undeclared output %q", id, name)
		}
		if !definition.MatchesKind(want, string(ref.Kind)) {
			return fmt.Errorf("node %q output %q has kind %q, want %q", id, name, ref.Kind, want.String())
		}
	}

	ne.Outputs = outputs
	if terr := ne.TransitionTo(StatusSucceeded); terr != nil {
		return fmt.Errorf("node %q: %w", id, terr)
	}

	e.logger.Info("node succeeded", "execution", exec.ID, "node", id, "outputs", len(outputs))
	return nil
}

// declaredOutputs 解析 Node Definition 的输出契约：输出名 -> 已解析的
// TypeExpr。契约语法非法在定义层校验报出；此处解析失败即运行期错误。
func (e *Engine) declaredOutputs(definitionName string) (map[string]definition.TypeExpr, error) {
	d, err := e.defs.Definition(definitionName)
	if err != nil {
		return nil, err
	}
	declared := make(map[string]definition.TypeExpr, len(d.Outputs))
	for name, port := range d.Outputs {
		t, err := definition.ParseTypeExpr(port.Type)
		if err != nil {
			return nil, fmt.Errorf("output %q: %w", name, err)
		}
		declared[name] = t
	}
	return declared, nil
}

// projectContext 返回本次运行的 ProjectContext：优先使用注入的
// Project Runtime 产物（含 Workspace），否则依据 YAML 声明构造最小 Context。
func (e *Engine) projectContext(def workflow.Definition) project.Context {
	if e.projectCtx != nil {
		return *e.projectCtx
	}
	return project.Context{
		Repository: project.Repository{Path: def.Project.Repository},
		Branch:     def.Project.Branch,
	}
}

// instantiate 依据 ExecutorRegistry 实例化全部 Node 定义：
// 缺省解析取 Latest(definition)（显式 executor 版本字段属票 05 的
// Schema 变更，届时接入 Get(definition, version)）。
func (e *Engine) instantiate(def workflow.Definition) (map[string]node.Node, error) {
	ids := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	nodes := make(map[string]node.Node, len(def.Nodes))
	for _, id := range ids {
		spec := def.Nodes[id]
		if _, err := e.defs.Definition(spec.Type); err != nil {
			return nil, fmt.Errorf("node %q: unknown node definition %q (registered: %v)",
				id, spec.Type, e.defs.DefinitionNames())
		}
		f, err := e.executors.Latest(spec.Type)
		if err != nil {
			return nil, fmt.Errorf("node %q: %w", id, err)
		}
		n, err := f.Create(node.Config(spec.Config))
		if err != nil {
			return nil, fmt.Errorf("node %q: create %q: %w", id, spec.Type, err)
		}
		nodes[id] = n
	}
	return nodes, nil
}

func markReady(exec *WorkflowExecution, id string) error {
	if err := exec.Nodes[id].TransitionTo(StatusReady); err != nil {
		return fmt.Errorf("node %q: mark ready: %w", id, err)
	}
	return nil
}

// persist 在配置了 stateDir 时写入状态快照；写入失败记日志不中断执行
// （状态持久化是可观测性设施，不应使运行本身失败）。
func (e *Engine) persist(exec *WorkflowExecution) {
	if e.stateDir == "" {
		return
	}
	dir := filepath.Join(e.stateDir, exec.ID)
	if err := PersistState(dir, exec); err != nil {
		e.logger.Error("persist state failed", "execution", exec.ID, "error", err)
	}
}
