package execution

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

type fakeHumanGateway struct {
	mu        sync.Mutex
	responses []RoundResponse
	requests  []RoundRequest
}

func (g *fakeHumanGateway) RequestRound(ctx context.Context, req RoundRequest) (RoundResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, req)
	if len(g.responses) == 0 {
		return RoundResponse{}, fmt.Errorf("unexpected human round request")
	}
	response := g.responses[0]
	g.responses = g.responses[1:]
	return response, nil
}

func (g *fakeHumanGateway) Requests() []RoundRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]RoundRequest(nil), g.requests...)
}

type humanInputTestFactory struct{}

func (humanInputTestFactory) Definition() string            { return "human-input" }
func (humanInputTestFactory) Version() string               { return "v1" }
func (humanInputTestFactory) NodeType() definition.NodeType { return definition.TypeHuman }
func (humanInputTestFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return nil, map[string]definition.OutputPort{"requirement": {Type: "markdown"}}
}
func (humanInputTestFactory) Create(node.Config) (node.Node, error) {
	return humanInputTestNode{}, nil
}

type humanInputTestNode struct{}

func (humanInputTestNode) Execute(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, fmt.Errorf("human-input executor should be driven by HumanGateway")
}

func (humanInputTestNode) ExecuteHumanInput(ctx node.ExecutionContext, content string) (map[string]artifact.ArtifactRef, error) {
	ref, err := ctx.Store.Put(artifact.Artifact{ID: "requirement", Kind: "markdown", Data: content})
	return map[string]artifact.ArtifactRef{"requirement": ref}, err
}

func TestHumanInputRoundsCascadeAndFinishClosesEntry(t *testing.T) {
	store := artifact.NewMemStore()
	worker := fnFactory{
		definition: "worker",
		inputs:     map[string]definition.InputPort{"requirement": {Type: "markdown"}},
		outputs:    map[string]definition.OutputPort{"work": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				requirement, err := ctx.Store.Get(inputs["requirement"])
				if err != nil {
					return nil, err
				}
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "work", Kind: "markdown", Data: requirement.Data})
				return map[string]artifact.ArtifactRef{"work": ref}, err
			}), nil
		},
	}
	leaf := fnFactory{
		definition: "leaf",
		inputs:     map[string]definition.InputPort{"work": {Type: "markdown"}},
		outputs:    map[string]definition.OutputPort{"result": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				work, err := ctx.Store.Get(inputs["work"])
				if err != nil {
					return nil, err
				}
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "result", Kind: "markdown", Data: work.Data})
				return map[string]artifact.ArtifactRef{"result": ref}, err
			}), nil
		},
	}
	dr, er := newTestRegistries(t, humanInputTestFactory{}, worker, leaf)
	gateway := &fakeHumanGateway{responses: []RoundResponse{
		{Content: "first requirement", Finished: false},
		{Content: "second requirement", Finished: true},
	}}
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "human-rounds"},
		Nodes: map[string]workflow.NodeSpec{
			"input": {Node: "human-input"},
			"worker": {Node: "worker", Inputs: map[string]workflow.InputBinding{
				"requirement": {From: "input.requirement"},
			}},
			"leaf": {Node: "leaf", Inputs: map[string]workflow.InputBinding{
				"work": {From: "worker.work"},
			}},
		},
	}

	exec, err := runUntilStopped(t, NewEngine(er, dr, store, nil,
		WithHumanGateway(gateway), WithConvergenceLimit(1)), def)
	if err != nil || exec.Status != StatusStopped {
		t.Fatalf("Run() = %s/%v, want Stopped/nil", exec.Status, err)
	}
	requests := gateway.Requests()
	if len(requests) != 2 {
		t.Fatalf("gateway requests = %d, want 2", len(requests))
	}
	for _, req := range requests {
		if req.Kind != RoundRequestInput || req.NodeID != "input" || req.Definition != "human-input" {
			t.Errorf("gateway request = %+v", req)
		}
	}

	entry := exec.Node("input")
	if entry.Current.Round != 2 || len(entry.History) != 1 {
		t.Fatalf("input rounds = current %d/history %d, want 2/1", entry.Current.Round, len(entry.History))
	}
	for i, run := range []NodeRun{entry.History[0], entry.Current} {
		wantVersion := fmt.Sprint(i + 1)
		ref := run.Outputs["requirement"]
		if ref.Version != wantVersion {
			t.Errorf("input round %d version = %q, want %q", run.Round, ref.Version, wantVersion)
		}
	}
	for _, id := range []string{"worker", "leaf"} {
		ne := exec.Node(id)
		if ne.Current.Round != 2 || len(ne.History) != 1 {
			t.Errorf("%s rounds = current %d/history %d, want 2/1", id, ne.Current.Round, len(ne.History))
		}
	}
	if got := exec.Node("worker").Current.Inputs["requirement"].Ref.Version; got != "2" {
		t.Errorf("worker second round requirement version = %q, want 2", got)
	}
	result, err := store.Get(exec.Node("leaf").Current.Outputs["result"])
	if err != nil || result.Data != "second requirement" {
		t.Errorf("latest result = %+v/%v, want second requirement", result, err)
	}
}

type blockingHumanGateway struct {
	started chan struct{}
}

func (g blockingHumanGateway) RequestRound(ctx context.Context, _ RoundRequest) (RoundResponse, error) {
	close(g.started)
	<-ctx.Done()
	return RoundResponse{}, ctx.Err()
}

func TestCancellationWhileWaitingForHumanInputStopsRun(t *testing.T) {
	dr, er := newTestRegistries(t, humanInputTestFactory{})
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "cancel-human-input"},
		Nodes:      map[string]workflow.NodeSpec{"input": {Node: "human-input"}},
	}

	var exec *WorkflowExecution
	var runErr error
	done := make(chan struct{})
	go func() {
		exec, runErr = NewEngine(er, dr, artifact.NewMemStore(), nil,
			WithHumanGateway(blockingHumanGateway{started: started})).Run(ctx, def)
		close(done)
	}()
	<-started
	cancel()
	<-done

	if runErr != nil || exec.Status != StatusStopped {
		t.Fatalf("Run() = %s/%v, want Stopped/nil", exec.Status, runErr)
	}
	if input := exec.Node("input"); input.Current.Status != StatusFailed || input.Current.Error != context.Canceled.Error() {
		t.Errorf("input after cancellation = %+v, want Failed/context canceled", input.Current)
	}
}
