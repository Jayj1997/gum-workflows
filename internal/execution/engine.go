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

// WithHumanGateway injects the blocking human interaction adapter.
func WithHumanGateway(gateway HumanGateway) Option {
	return func(e *Engine) {
		if gateway == nil {
			gateway = noHumanGateway{}
		}
		e.humanGateway = gateway
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
	humanGateway     HumanGateway
	onIdle           func()
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
	id               string
	outputs          map[string]artifact.ArtifactRef
	err              error
	humanEvent       bool
	closeHumanInput  bool
	approvalDecision *RoundResponse
	adviseRetry      *RoundResponse
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
			NodeID: id, NodeDefinition: def.Nodes[id].Node,
			Current:         NodeRun{Status: StatusPending},
			consumedControl: map[string]int{}, consumedInputs: map[string]artifact.ArtifactRef{},
			outputVersions: map[string]int{}, approvalRounds: map[int]approvalDecision{},
		}
	}
	e.persist(exec)

	execCtx := node.ExecutionContext{
		Context: ctx, Project: e.projectContext(def), Store: e.store, Logger: e.logger,
	}
	adviseCtx, cancelAdvise := context.WithCancel(ctx)
	defer cancelAdvise()
	results := make(chan nodeResult, max(1, e.parallelism))
	ready := make([]string, 0, len(g.NodeIDs))
	queued := map[string]bool{}
	inflight := 0
	pendingAdvise := 0
	stopping := false
	var runErr error

	for {
		if ctx.Err() != nil {
			stopping = true
		}
		if stopping {
			cancelAdvise()
		}
		if !stopping {
			for _, id := range g.NodeIDs {
				if !queued[id] && e.ready(def, exec, nodes[id], id) {
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
				dataInputs := e.resolveDataInputs(def, exec, id)
				ne := exec.Nodes[id]
				humanNode, humanInput := asHumanInputNode(nodes[id])
				_, humanApproval := asHumanApprovalNode(nodes[id])
				if !humanInput && !humanApproval {
					ne.machineRuns++
				}
				var startErr error
				if humanApproval {
					startErr = ne.StartWaitingRun(uuid.NewString(), inputs)
				} else {
					startErr = ne.StartRun(uuid.NewString(), inputs)
				}
				if startErr != nil {
					return e.fail(exec, fmt.Errorf("node %q: %w", id, startErr))
				}
				ne.adviseRetry = nil
				for _, dep := range def.Nodes[id].DependsOn {
					ne.consumedControl[dep] = e.controlCompletedRound(def, exec, dep)
				}
				for name, snapshot := range dataInputs {
					ne.consumedInputs[name] = snapshot.Ref
				}
				ne.dirty = false

				if !humanInput && !humanApproval && ne.machineRuns > e.convergenceLimit {
					ne.Current.Error = "convergence-guard"
					ne.Current.ErrorKind = node.ErrorKindStructural
					ne.Current.FinishedAt = time.Now().UTC()
					_ = ne.TransitionTo(StatusFailed)
					runErr = fmt.Errorf("node %q failed: convergence-guard", id)
					stopping = true
					exec.Status = StatusFailed
					exec.Error = runErr.Error()
					e.persist(exec)
					break
				}

				e.persist(exec)
				inflight++
				if humanInput {
					go e.executeHumanInput(execCtx, def.Nodes[id].Node, humanNode, id, results)
				} else if humanApproval {
					request := RoundRequest{
						NodeID: id, Definition: def.Nodes[id].Node, Kind: RoundRequestApproval,
						Artifacts: e.artifactSummaries(exec), AdviseHistory: e.adviseHistory(exec.Nodes[id]),
					}
					go e.requestHumanApproval(execCtx, request, id, results)
				} else {
					go e.executeNode(execCtx, def, nodes[id], id, inputs, results)
				}
			}
		}

		if inflight == 0 && stopping {
			break
		}
		if inflight == 0 && pendingAdvise == 0 {
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
			if id := e.nextHumanInput(def, exec, nodes); id != "" {
				ready = append(ready, id)
				queued[id] = true
				continue
			}
			if e.onIdle != nil {
				e.onIdle()
			}
			select {
			case <-ctx.Done():
				stopping = true
			}
			continue
		}

		select {
		case result := <-results:
			if result.adviseRetry != nil {
				pendingAdvise--
				if result.err != nil {
					if ctx.Err() == nil || (!errors.Is(result.err, context.Canceled) && !errors.Is(result.err, context.DeadlineExceeded)) {
						ne := exec.Nodes[result.id]
						ne.Current.Error = result.err.Error()
						ne.Current.ErrorKind = node.ErrorKindStructural
						runErr = fmt.Errorf("node %q advise retry failed: %w", result.id, result.err)
						stopping = true
						exec.Status = StatusFailed
						exec.Error = runErr.Error()
					}
					e.persist(exec)
					continue
				}
				if result.adviseRetry.Skip || result.adviseRetry.Advise == "" {
					e.persist(exec)
					continue
				}
				if err := e.injectAdviseRetry(exec, result.id, result.adviseRetry.Advise); err != nil {
					runErr = err
					stopping = true
					exec.Status = StatusFailed
					exec.Error = err.Error()
				} else {
					e.resetConvergence(exec)
				}
				e.persist(exec)
				continue
			}
			if result.approvalDecision != nil {
				ne := exec.Nodes[result.id]
				if err := ne.TransitionTo(StatusRunning); err != nil {
					return e.fail(exec, fmt.Errorf("node %q: %w", result.id, err))
				}
				e.resetConvergence(exec)
				ne.approvalRounds[ne.Current.Round] = approvalDecision{
					approved: result.approvalDecision.Approved,
					advise:   result.approvalDecision.Advise,
				}
				e.persist(exec)
				approvalNode, _ := asHumanApprovalNode(nodes[result.id])
				go e.executeHumanApproval(execCtx, def.Nodes[result.id].Node, approvalNode, result.id, *result.approvalDecision, results)
				continue
			}
			inflight--
			if result.humanEvent {
				e.resetConvergence(exec)
				exec.Nodes[result.id].humanClosed = result.closeHumanInput
			}
			if result.err != nil && ctx.Err() != nil && (errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded)) {
				e.finishCanceledNode(exec, result)
			} else {
				if exec.Nodes[result.id].Current.Status == StatusWaitingHuman {
					_ = exec.Nodes[result.id].TransitionTo(StatusRunning)
				}
				recoverable, finishErr := e.finishNode(def, exec, result)
				if finishErr != nil && runErr == nil {
					runErr = finishErr
					stopping = true
					exec.Status = StatusFailed
					exec.Error = finishErr.Error()
				} else if recoverable {
					pendingAdvise++
					go e.requestAdviseRetry(adviseCtx, def.Nodes[result.id].Node, result.id, result.err, results)
				}
			}
			e.persist(exec)
		case <-ctx.Done():
			stopping = true
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

func (e *Engine) requestAdviseRetry(ctx context.Context, definitionName, id string, cause error, results chan<- nodeResult) {
	response, err := e.humanGateway.RequestRound(ctx, RoundRequest{
		NodeID: id, Definition: definitionName, Kind: RoundRequestAdviseRetry, Error: cause.Error(),
	})
	select {
	case results <- nodeResult{id: id, adviseRetry: &response, err: err}:
	case <-ctx.Done():
	}
}

func (e *Engine) requestHumanApproval(execCtx node.ExecutionContext, request RoundRequest, id string, results chan<- nodeResult) {
	response, err := e.humanGateway.RequestRound(execCtx.Context, request)
	if err != nil {
		results <- nodeResult{id: id, err: err}
		return
	}
	results <- nodeResult{id: id, approvalDecision: &response}
}

func (e *Engine) executeHumanApproval(execCtx node.ExecutionContext, definitionName string, n humanApprovalNode, id string, response RoundResponse, results chan<- nodeResult) {
	outputs, err := n.ExecuteHumanApproval(execCtx, response.Approved, response.Advise)
	if err == nil {
		err = e.validateOutputs(definitionName, id, outputs)
	}
	results <- nodeResult{id: id, outputs: outputs, err: err}
}

func (e *Engine) executeHumanInput(execCtx node.ExecutionContext, definitionName string, n humanInputNode, id string, results chan<- nodeResult) {
	response, err := e.humanGateway.RequestRound(execCtx.Context, RoundRequest{
		NodeID: id, Definition: definitionName, Kind: RoundRequestInput,
	})
	if err != nil {
		results <- nodeResult{id: id, err: err}
		return
	}
	outputs, err := n.ExecuteHumanInput(execCtx, response.Content)
	if err != nil {
		results <- nodeResult{id: id, err: err}
		return
	}
	if err := e.validateOutputs(definitionName, id, outputs); err != nil {
		results <- nodeResult{id: id, err: err}
		return
	}
	results <- nodeResult{
		id: id, outputs: outputs,
		humanEvent: true, closeHumanInput: response.Finished,
	}
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

func (e *Engine) finishNode(def workflow.Definition, exec *WorkflowExecution, result nodeResult) (bool, error) {
	ne := exec.Nodes[result.id]
	ne.Current.FinishedAt = time.Now().UTC()
	if result.err != nil {
		ne.Current.Error = result.err.Error()
		kind := node.ErrorKindOf(result.err)
		if kind == node.ErrorKindInteraction && !e.canAdviseRetry(def, result.id, result.err) {
			kind = node.ErrorKindStructural
		}
		ne.Current.ErrorKind = kind
		_ = ne.TransitionTo(StatusFailed)
		if kind == node.ErrorKindInteraction {
			return true, nil
		}
		return false, fmt.Errorf("node %q failed: %w", result.id, result.err)
	}

	versioned := make(map[string]artifact.ArtifactRef, len(result.outputs))
	for name, ref := range result.outputs {
		ne.outputVersions[name]++
		version := strconv.Itoa(ne.outputVersions[name])
		updated := ref
		if e.store.Exists(ref) {
			body, err := e.store.Get(ref)
			if err != nil {
				ne.Current.Error = err.Error()
				ne.Current.ErrorKind = node.ErrorKindStructural
				_ = ne.TransitionTo(StatusFailed)
				return false, fmt.Errorf("node %q output %q: load artifact for versioning: %w", result.id, name, err)
			}
			if body.Version == version {
				updated.Version = version
			} else {
				body.Version = version
				updated, err = e.store.Put(body)
				if err != nil {
					ne.Current.Error = err.Error()
					ne.Current.ErrorKind = node.ErrorKindStructural
					_ = ne.TransitionTo(StatusFailed)
					return false, fmt.Errorf("node %q output %q: store versioned artifact: %w", result.id, name, err)
				}
			}
		} else {
			updated.Version = version
		}
		versioned[name] = updated
	}
	ne.Current.Outputs = versioned
	if err := ne.TransitionTo(StatusSucceeded); err != nil {
		return false, fmt.Errorf("node %q: %w", result.id, err)
	}
	return false, nil
}

func (e *Engine) canAdviseRetry(def workflow.Definition, id string, err error) bool {
	if node.ErrorKindOf(err) != node.ErrorKindInteraction {
		return false
	}
	d, lookupErr := e.defs.Definition(def.Nodes[id].Node)
	if lookupErr != nil || d.Type != definition.TypeAgent {
		return false
	}
	_, declared := d.Inputs["advise"]
	return declared
}

func (e *Engine) injectAdviseRetry(exec *WorkflowExecution, id, advice string) error {
	ne := exec.Nodes[id]
	ref, err := e.store.Put(artifact.Artifact{
		ID:      fmt.Sprintf("%s-advise-retry-%d", id, ne.Current.Round+1),
		Kind:    artifact.Kind("markdown"),
		Version: strconv.Itoa(ne.Current.Round + 1),
		Data:    advice,
	})
	if err != nil {
		ne.Current.Error = err.Error()
		ne.Current.ErrorKind = node.ErrorKindStructural
		return fmt.Errorf("node %q advise retry: store advise: %w", id, err)
	}
	ne.adviseRetry = &InputSnapshot{From: "#advise-retry", Ref: ref}
	return nil
}

func (e *Engine) finishCanceledNode(exec *WorkflowExecution, result nodeResult) {
	ne := exec.Nodes[result.id]
	ne.Current.FinishedAt = time.Now().UTC()
	ne.Current.Error = result.err.Error()
	ne.Current.ErrorKind = node.ErrorKindStructural
	_ = ne.TransitionTo(StatusFailed)
}

func (e *Engine) ready(def workflow.Definition, exec *WorkflowExecution, n node.Node, id string) bool {
	ne := exec.Nodes[id]
	status := ne.Current.Status
	if status == StatusReady {
		return true
	}
	if status != StatusPending && status != StatusSucceeded && !(status == StatusFailed && ne.Current.ErrorKind == node.ErrorKindInteraction) {
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
		if e.controlCompletedRound(def, exec, dep) == 0 {
			return false
		}
	}
	if status == StatusPending {
		return true
	}
	if _, ok := asHumanInputNode(n); ok {
		return false // Further rounds are requested only after the downstream graph settles.
	}

	for name, snapshot := range snapshots {
		previous, ok := ne.consumedInputs[name]
		if !ok || previous.URI != snapshot.Ref.URI || previous.Version != snapshot.Ref.Version {
			fromNode, _, _ := workflow.ParseRef(def.Nodes[id].Inputs[name].From)
			if def.Nodes[fromNode].Node == humanApprovalDefinition && e.approvalPassed(exec.Nodes[fromNode]) {
				continue
			}
			ne.dirty = true
			return true
		}
	}
	for _, dep := range def.Nodes[id].DependsOn {
		if e.controlCompletedRound(def, exec, dep) > ne.consumedControl[dep] {
			return true
		}
	}
	return ne.dirty
}

func (e *Engine) nextHumanInput(def workflow.Definition, exec *WorkflowExecution, nodes map[string]node.Node) string {
	for _, id := range sortedNodeIDs(def.Nodes) {
		ne := exec.Nodes[id]
		_, humanInput := asHumanInputNode(nodes[id])
		if humanInput && ne.Current.Status == StatusSucceeded && !ne.humanClosed {
			return id
		}
	}
	return ""
}

func asHumanInputNode(n node.Node) (humanInputNode, bool) {
	human, ok := n.(humanInputNode)
	return human, ok
}

func asHumanApprovalNode(n node.Node) (humanApprovalNode, bool) {
	human, ok := n.(humanApprovalNode)
	return human, ok
}

func (e *Engine) controlCompletedRound(def workflow.Definition, exec *WorkflowExecution, id string) int {
	if def.Nodes[id].Node != humanApprovalDefinition {
		return latestCompletedRound(exec.Nodes[id])
	}
	ne := exec.Nodes[id]
	runs := append(append([]NodeRun{}, ne.History...), ne.Current)
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Status == StatusSucceeded && runApproved(ne, runs[i]) {
			return runs[i].Round
		}
	}
	return 0
}

func (e *Engine) approvalPassed(ne *NodeExecution) bool {
	run, ok := latestCompleted(ne)
	return ok && runApproved(ne, run)
}

func runApproved(ne *NodeExecution, run NodeRun) bool {
	decision, ok := ne.approvalRounds[run.Round]
	return ok && decision.approved
}

func (e *Engine) artifactSummaries(exec *WorkflowExecution) []ArtifactSummary {
	var summaries []ArtifactSummary
	for _, id := range sortedNodeExecutionIDs(exec.Nodes) {
		run, ok := latestCompleted(exec.Nodes[id])
		if !ok {
			continue
		}
		for _, name := range sortedArtifactNames(run.Outputs) {
			ref := run.Outputs[name]
			summaries = append(summaries, ArtifactSummary{Name: name, Kind: string(ref.Kind), Version: ref.Version, URI: ref.URI})
		}
	}
	return summaries
}

func (e *Engine) adviseHistory(ne *NodeExecution) []string {
	var history []string
	for _, run := range ne.History {
		if decision, ok := ne.approvalRounds[run.Round]; ok && decision.advise != "" {
			history = append(history, decision.advise)
		}
	}
	return history
}

func sortedNodeExecutionIDs(nodes map[string]*NodeExecution) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedArtifactNames(outputs map[string]artifact.ArtifactRef) []string {
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedNodeIDs(nodes map[string]workflow.NodeSpec) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (e *Engine) resolveInputs(def workflow.Definition, exec *WorkflowExecution, id string) map[string]InputSnapshot {
	inputs := e.resolveDataInputs(def, exec, id)
	if retry := exec.Nodes[id].adviseRetry; retry != nil {
		inputs["advise"] = *retry
	}
	return inputs
}

func (e *Engine) resolveDataInputs(def workflow.Definition, exec *WorkflowExecution, id string) map[string]InputSnapshot {
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
