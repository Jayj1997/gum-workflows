package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
	"github.com/google/uuid"
)

const defaultConvergenceLimit = 10

var executionSeq atomic.Int64

// HumanGateway exposes human events to the engine. Tickets 09 and 10 replace
// the no-op implementation with input and approval gateways.
type HumanGateway interface {
	HumanEvents() <-chan struct{}
}

type noHumanGateway struct{}

func (noHumanGateway) HumanEvents() <-chan struct{} { return nil }

// Option configures optional engine behavior.
type Option func(*Engine)

// WithStateDir persists execution snapshots below dir.
func WithStateDir(dir string) Option { return func(e *Engine) { e.stateDir = dir } }

// WithProjectContext injects the project runtime context shared by all rounds.
func WithProjectContext(ctx project.Context) Option {
	return func(e *Engine) {
		copy := ctx
		e.projectCtx = &copy
	}
}

// WithExecutionID fixes the filesystem execution identifier.
func WithExecutionID(id string) Option { return func(e *Engine) { e.executionID = id } }

// WithParallelism sets the maximum number of distinct nodes that may run concurrently.
func WithParallelism(n int) Option {
	return func(e *Engine) {
		if n > 1 {
			e.parallelism = n
		}
	}
}

// WithConvergenceLimit sets how many consecutive machine-triggered rounds a node may start.
func WithConvergenceLimit(limit int) Option {
	return func(e *Engine) {
		if limit > 0 {
			e.convergenceLimit = limit
		}
	}
}

// WithHumanEvents injects notifications that reset all convergence counters.
// Human nodes replace this temporary ticket-08 hook in tickets 09 and 10.
func WithHumanEvents(events <-chan struct{}) Option {
	return func(e *Engine) { e.humanEvents = events }
}

// WithHumanGateway reserves the injected gateway used by later human-node tickets.
func WithHumanGateway(gateway HumanGateway) Option {
	return func(e *Engine) {
		if gateway == nil {
			gateway = noHumanGateway{}
		}
		e.humanGateway = gateway
		e.humanEvents = gateway.HumanEvents()
	}
}

// WithWorkflowFile records the source workflow path in state snapshots.
func WithWorkflowFile(path string) Option { return func(e *Engine) { e.workflowFile = path } }

// Engine schedules version-driven node rounds until failure or context cancellation.
type Engine struct {
	executors        *node.ExecutorRegistry
	defs             *definition.Registry
	store            artifact.Store
	logger           *slog.Logger
	stateDir         string
	parallelism      int
	convergenceLimit int
	projectCtx       *project.Context
	executionID      string
	workflowFile     string
	humanEvents      <-chan struct{}
	humanGateway     HumanGateway
}

// NewEngine creates an iterative execution engine.
func NewEngine(executors *node.ExecutorRegistry, defs *definition.Registry, store artifact.Store, logger *slog.Logger, opts ...Option) *Engine {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Engine{
		executors: executors, defs: defs, store: store, logger: logger,
		parallelism: 1, convergenceLimit: defaultConvergenceLimit,
		humanGateway: noHumanGateway{},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

type nodeResult struct {
	id      string
	outputs map[string]artifact.ArtifactRef
	err     error
}

// Run keeps a settled workflow Running at an idle wait point. Context cancellation
// stops the run cleanly; structural node errors and convergence protection fail it.
func (e *Engine) Run(ctx context.Context, def workflow.Definition) (*WorkflowExecution, error) {
	g, err := workflow.BuildGraph(def)
	if err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}
	nodes, err := e.instantiate(def)
	if err != nil {
		return nil, err
	}

	execID := e.executionID
	if execID == "" {
		execID = fmt.Sprintf("execution-%06d", executionSeq.Add(1))
	}
	exec := &WorkflowExecution{
		ID: execID, RunID: uuid.NewString(), Workflow: def.Metadata.Name,
		WorkflowFile: e.workflowFile, Status: StatusRunning, StartedAt: time.Now().UTC(),
		Nodes: make(map[string]*NodeExecution, len(g.NodeIDs)),
	}
	for _, id := range g.NodeIDs {
		exec.Nodes[id] = &NodeExecution{
			NodeID: id, NodeType: def.Nodes[id].Node,
			Current:         NodeRun{Status: StatusPending},
			consumedControl: map[string]int{}, outputVersions: map[string]int{},
		}
	}
	e.persist(exec)

	execCtx := node.ExecutionContext{
		Context: ctx, Project: e.projectContext(def), Store: e.store, Logger: e.logger,
	}
	results := make(chan nodeResult, max(1, e.parallelism))
	ready := make([]string, 0, len(g.NodeIDs))
	queued := map[string]bool{}
	inflight := 0
	stopping := false
	var runErr error

	for {
		if ctx.Err() != nil {
			stopping = true
		}
		e.consumeHumanEvents(exec)
		if !stopping {
			for _, id := range g.NodeIDs {
				if !queued[id] && e.ready(def, exec, id) {
					if exec.Nodes[id].Current.Round == 0 {
						if err := exec.Nodes[id].TransitionTo(StatusReady); err != nil {
							return e.fail(exec, err)
						}
					}
					ready = append(ready, id)
					queued[id] = true
				}
			}
			sort.Strings(ready)

			for inflight < e.parallelism && len(ready) > 0 {
				id := ready[0]
				ready = ready[1:]
				queued[id] = false
				inputs := e.resolveInputs(def, exec, id)
				ne := exec.Nodes[id]
				ne.machineRuns++
				if err := ne.StartRun(uuid.NewString(), inputs); err != nil {
					return e.fail(exec, fmt.Errorf("node %q: %w", id, err))
				}
				for _, dep := range def.Nodes[id].DependsOn {
					ne.consumedControl[dep] = latestCompletedRound(exec.Nodes[dep])
				}
				ne.dirty = false

				if ne.machineRuns > e.convergenceLimit {
					ne.Current.Error = "convergence-guard"
					ne.Current.ErrorKind = "structural"
					ne.Current.FinishedAt = time.Now().UTC()
					_ = ne.TransitionTo(StatusFailed)
					runErr = fmt.Errorf("node %q failed: convergence-guard", id)
					stopping = true
					exec.Status = StatusFailed
					exec.Error = runErr.Error()
					exec.StoppedReason = "convergence-guard"
					e.persist(exec)
					break
				}

				e.persist(exec)
				inflight++
				go e.executeNode(execCtx, def, nodes[id], id, inputs, results)
			}
		}

		if inflight == 0 {
			if stopping {
				break
			}
			if len(ready) > 0 {
				continue
			}
			if err := e.blockedInputError(def, exec); err != nil {
				runErr = err
				stopping = true
				exec.Status = StatusFailed
				exec.Error = err.Error()
				continue
			}
			select {
			case <-ctx.Done():
				stopping = true
			case _, ok := <-e.humanEvents:
				if ok {
					e.resetConvergence(exec)
				} else {
					e.humanEvents = nil
				}
			}
			continue
		}

		select {
		case result := <-results:
			inflight--
			if result.err != nil && ctx.Err() != nil && (errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded)) {
				e.finishCanceledNode(exec, result)
			} else if err := e.finishNode(exec, result); err != nil && runErr == nil {
				runErr = err
				stopping = true
				exec.Status = StatusFailed
				exec.Error = err.Error()
			}
			e.persist(exec)
		case <-ctx.Done():
			stopping = true
		case _, ok := <-e.humanEvents:
			if ok {
				e.resetConvergence(exec)
			} else {
				e.humanEvents = nil
			}
		}
	}

	exec.FinishedAt = time.Now().UTC()
	if runErr != nil {
		exec.Status = StatusFailed
		e.persist(exec)
		return exec, runErr
	}
	exec.Status = StatusStopped
	exec.StoppedReason = "user_interrupt"
	e.persist(exec)
	return exec, nil
}

func (e *Engine) blockedInputError(def workflow.Definition, exec *WorkflowExecution) error {
	for id, spec := range def.Nodes {
		if exec.Nodes[id].Current.Status != StatusPending {
			continue
		}
		d, err := e.defs.Definition(spec.Node)
		if err != nil {
			continue
		}
		for name, binding := range spec.Inputs {
			if d.Inputs[name].Optional {
				continue
			}
			fromNode, fromOutput, err := workflow.ParseRef(binding.From)
			if err != nil {
				continue
			}
			completed, ok := latestCompleted(exec.Nodes[fromNode])
			if !ok {
				continue
			}
			if _, ok := completed.Outputs[fromOutput]; !ok {
				return fmt.Errorf("node %q input %q: node %q did not produce output %q", id, name, fromNode, fromOutput)
			}
		}
	}
	return nil
}

func (e *Engine) executeNode(execCtx node.ExecutionContext, def workflow.Definition, n node.Node, id string, snapshots map[string]InputSnapshot, results chan<- nodeResult) {
	inputs := make(map[string]artifact.ArtifactRef, len(snapshots))
	for name, snapshot := range snapshots {
		inputs[name] = snapshot.Ref
	}
	outputs, err := n.Execute(execCtx, inputs)
	if err == nil {
		err = e.validateOutputs(def.Nodes[id].Node, id, outputs)
	}
	results <- nodeResult{id: id, outputs: outputs, err: err}
}

func (e *Engine) finishNode(exec *WorkflowExecution, result nodeResult) error {
	ne := exec.Nodes[result.id]
	ne.Current.FinishedAt = time.Now().UTC()
	if result.err != nil {
		ne.Current.Error = result.err.Error()
		ne.Current.ErrorKind = "structural"
		_ = ne.TransitionTo(StatusFailed)
		return fmt.Errorf("node %q failed: %w", result.id, result.err)
	}

	versioned := make(map[string]artifact.ArtifactRef, len(result.outputs))
	for name, ref := range result.outputs {
		ne.outputVersions[name]++
		version := strconv.Itoa(ne.outputVersions[name])
		updated := ref
		if e.store.Exists(ref) {
			updater, ok := e.store.(interface {
				UpdateVersion(artifact.ArtifactRef, string) (artifact.ArtifactRef, error)
			})
			if !ok {
				updated.Version = version
				versioned[name] = updated
				continue
			}
			var err error
			updated, err = updater.UpdateVersion(ref, version)
			if err != nil {
				ne.Current.Error = err.Error()
				ne.Current.ErrorKind = "structural"
				_ = ne.TransitionTo(StatusFailed)
				return fmt.Errorf("node %q output %q: assign version: %w", result.id, name, err)
			}
		} else {
			updated.Version = version
		}
		versioned[name] = updated
	}
	ne.Current.Outputs = versioned
	if err := ne.TransitionTo(StatusSucceeded); err != nil {
		return fmt.Errorf("node %q: %w", result.id, err)
	}
	return nil
}

func (e *Engine) finishCanceledNode(exec *WorkflowExecution, result nodeResult) {
	ne := exec.Nodes[result.id]
	ne.Current.FinishedAt = time.Now().UTC()
	ne.Current.Error = result.err.Error()
	ne.Current.ErrorKind = "structural"
	_ = ne.TransitionTo(StatusFailed)
}

func (e *Engine) ready(def workflow.Definition, exec *WorkflowExecution, id string) bool {
	ne := exec.Nodes[id]
	status := ne.Current.Status
	if status == StatusReady {
		return true
	}
	if status != StatusPending && status != StatusSucceeded {
		return false
	}

	snapshots := e.resolveInputs(def, exec, id)
	d, err := e.defs.Definition(def.Nodes[id].Node)
	if err != nil {
		return false
	}
	for name := range def.Nodes[id].Inputs {
		if _, ok := snapshots[name]; !ok && !d.Inputs[name].Optional {
			return false
		}
	}
	for _, dep := range def.Nodes[id].DependsOn {
		if latestCompletedRound(exec.Nodes[dep]) == 0 {
			return false
		}
	}
	if status == StatusPending {
		return true
	}

	for name, snapshot := range snapshots {
		previous, ok := ne.Current.Inputs[name]
		if !ok || previous.Ref.URI != snapshot.Ref.URI {
			ne.dirty = true
			return true
		}
	}
	for _, dep := range def.Nodes[id].DependsOn {
		if latestCompletedRound(exec.Nodes[dep]) > ne.consumedControl[dep] {
			return true
		}
	}
	return ne.dirty
}

func (e *Engine) resolveInputs(def workflow.Definition, exec *WorkflowExecution, id string) map[string]InputSnapshot {
	inputs := make(map[string]InputSnapshot, len(def.Nodes[id].Inputs))
	for name, binding := range def.Nodes[id].Inputs {
		fromNode, fromOutput, err := workflow.ParseRef(binding.From)
		if err != nil {
			continue
		}
		completed, ok := latestCompleted(exec.Nodes[fromNode])
		if !ok {
			continue
		}
		ref, ok := completed.Outputs[fromOutput]
		if ok {
			inputs[name] = InputSnapshot{From: binding.From, Ref: ref}
		}
	}
	return inputs
}

func latestCompleted(ne *NodeExecution) (NodeRun, bool) {
	if ne.Current.Status == StatusSucceeded {
		return ne.Current, true
	}
	for i := len(ne.History) - 1; i >= 0; i-- {
		if ne.History[i].Status == StatusSucceeded {
			return ne.History[i], true
		}
	}
	return NodeRun{}, false
}

func latestCompletedRound(ne *NodeExecution) int {
	run, ok := latestCompleted(ne)
	if !ok {
		return 0
	}
	return run.Round
}

func (e *Engine) resetConvergence(exec *WorkflowExecution) {
	for _, ne := range exec.Nodes {
		ne.machineRuns = 0
	}
}

func (e *Engine) consumeHumanEvents(exec *WorkflowExecution) {
	for e.humanEvents != nil {
		select {
		case _, ok := <-e.humanEvents:
			if !ok {
				e.humanEvents = nil
				return
			}
			e.resetConvergence(exec)
		default:
			return
		}
	}
}

func (e *Engine) validateOutputs(definitionName, _ string, outputs map[string]artifact.ArtifactRef) error {
	declared, err := e.declaredOutputs(definitionName)
	if err != nil {
		return err
	}
	for name, ref := range outputs {
		want, ok := declared[name]
		if !ok {
			return fmt.Errorf("returned undeclared output %q", name)
		}
		if !definition.MatchesKind(want, string(ref.Kind)) {
			return fmt.Errorf("output %q has kind %q, want %q", name, ref.Kind, want.String())
		}
	}
	return nil
}

func (e *Engine) declaredOutputs(definitionName string) (map[string]definition.TypeExpr, error) {
	d, err := e.defs.Definition(definitionName)
	if err != nil {
		return nil, err
	}
	declared := make(map[string]definition.TypeExpr, len(d.Outputs))
	for name, port := range d.Outputs {
		typeExpr, err := definition.ParseTypeExpr(port.Type)
		if err != nil {
			return nil, fmt.Errorf("output %q: %w", name, err)
		}
		declared[name] = typeExpr
	}
	return declared, nil
}

func (e *Engine) instantiate(def workflow.Definition) (map[string]node.Node, error) {
	ids := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	nodes := make(map[string]node.Node, len(ids))
	for _, id := range ids {
		spec := def.Nodes[id]
		if _, err := e.defs.Definition(spec.Node); err != nil {
			return nil, fmt.Errorf("node %q: unknown node definition %q (registered: %v)", id, spec.Node, e.defs.DefinitionNames())
		}
		var factory node.ExecutorFactory
		var err error
		if spec.Executor == "" {
			factory, err = e.executors.Latest(spec.Node)
		} else {
			factory, err = e.executors.Get(spec.Node, spec.Executor)
		}
		if err != nil {
			return nil, fmt.Errorf("node %q: %w", id, err)
		}
		nodes[id], err = factory.Create(node.Config(spec.Config))
		if err != nil {
			return nil, fmt.Errorf("node %q: create %q: %w", id, spec.Node, err)
		}
	}
	return nodes, nil
}

func (e *Engine) projectContext(def workflow.Definition) project.Context {
	if e.projectCtx != nil {
		return *e.projectCtx
	}
	ctx := project.Context{}
	if len(def.Projects) > 0 {
		ctx.Repository = project.Repository{Path: def.Projects[0].Repository}
	}
	return ctx
}

func (e *Engine) fail(exec *WorkflowExecution, err error) (*WorkflowExecution, error) {
	exec.Status = StatusFailed
	exec.Error = err.Error()
	exec.FinishedAt = time.Now().UTC()
	e.persist(exec)
	return exec, err
}

func (e *Engine) persist(exec *WorkflowExecution) {
	if e.stateDir == "" {
		return
	}
	if err := PersistState(filepath.Join(e.stateDir, exec.ID), exec); err != nil && !errors.Is(err, context.Canceled) {
		e.logger.Error("persist state failed", "execution", exec.ID, "error", err)
	}
}
