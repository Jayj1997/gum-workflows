package execution

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

type humanApprovalTestFactory struct{}

func (humanApprovalTestFactory) Definition() string            { return "human-approval" }
func (humanApprovalTestFactory) Version() string               { return "v1" }
func (humanApprovalTestFactory) NodeType() definition.NodeType { return definition.TypeHuman }
func (humanApprovalTestFactory) Contract() (map[string]definition.InputPort, map[string]definition.OutputPort) {
	return nil, map[string]definition.OutputPort{
		"approve": {Type: "bool"},
		"advise":  {Type: "markdown"},
	}
}

func TestApprovalDecisionFeedsFreshDataAndGatesControl(t *testing.T) {
	for _, tt := range []struct {
		name       string
		approved   bool
		deployRuns int
	}{
		{name: "reject", approved: false, deployRuns: 0},
		{name: "approve", approved: true, deployRuns: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := artifact.NewMemStore()
			factory := func(name string, inputs map[string]definition.InputPort, outputs map[string]definition.OutputPort) fnFactory {
				return fnFactory{definition: name, inputs: inputs, outputs: outputs, create: func(node.Config) (node.Node, error) {
					return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
						result := make(map[string]artifact.ArtifactRef, len(outputs))
						for output, port := range outputs {
							ref, err := ctx.Store.Put(artifact.Artifact{ID: output, Kind: artifact.Kind(port.Type), Data: output})
							if err != nil {
								return nil, err
							}
							result[output] = ref
						}
						return result, nil
					}), nil
				}}
			}
			source := factory("source", nil, map[string]definition.OutputPort{"work": {Type: "markdown"}})
			consumer := factory("consumer", map[string]definition.InputPort{"advise": {Type: "markdown"}}, map[string]definition.OutputPort{"result": {Type: "markdown"}})
			deploy := factory("deploy", nil, map[string]definition.OutputPort{"deployed": {Type: "markdown"}})
			dr, er := newTestRegistries(t, source, consumer, humanApprovalTestFactory{}, deploy)
			def := workflow.Definition{
				APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
				Metadata: workflow.Metadata{Name: "approval-quadrants"},
				Nodes: map[string]workflow.NodeSpec{
					"source":   {Node: "source"},
					"review":   {Node: "human-approval", DependsOn: []string{"source"}},
					"consumer": {Node: "consumer", Inputs: map[string]workflow.InputBinding{"advise": {From: "review.advise"}}},
					"deploy":   {Node: "deploy", DependsOn: []string{"review"}},
				},
			}
			gateway := &fakeHumanGateway{responses: []RoundResponse{{Approved: tt.approved, Advise: "reviewed"}}}

			exec, err := runUntilStopped(t, NewEngine(er, dr, store, nil, WithHumanGateway(gateway)), def)
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if got := exec.Node("consumer").Current.Round; got != 1 {
				t.Errorf("fresh data consumer rounds = %d, want 1", got)
			}
			if got := exec.Node("deploy").Current.Round; got != tt.deployRuns {
				t.Errorf("control consumer rounds = %d, want %d", got, tt.deployRuns)
			}
		})
	}
}

func TestApprovalWaitPersistsWaitingHumanAndCancellationStopsRun(t *testing.T) {
	store := artifact.NewMemStore()
	source := fnFactory{
		definition: "source", outputs: map[string]definition.OutputPort{"work": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "work", Kind: "markdown", Data: "work"})
				return map[string]artifact.ArtifactRef{"work": ref}, err
			}), nil
		},
	}
	dr, er := newTestRegistries(t, source, humanApprovalTestFactory{})
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "waiting-approval"},
		Nodes: map[string]workflow.NodeSpec{
			"source": {Node: "source"},
			"review": {Node: "human-approval", DependsOn: []string{"source"}},
		},
	}
	stateRoot := t.TempDir()
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var exec *WorkflowExecution
	var runErr error
	go func() {
		exec, runErr = NewEngine(er, dr, store, nil,
			WithHumanGateway(blockingHumanGateway{started: started}),
			WithStateDir(stateRoot), WithExecutionID("execution-approval-wait")).Run(ctx, def)
		close(done)
	}()
	<-started
	loaded, err := LoadNodeState(filepath.Join(stateRoot, "execution-approval-wait"), "review")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Current.Status != StatusWaitingHuman {
		t.Fatalf("persisted review status = %s, want WaitingHuman", loaded.Current.Status)
	}
	cancel()
	<-done
	if runErr != nil || exec.Status != StatusStopped {
		t.Fatalf("Run() = %s/%v, want Stopped/nil", exec.Status, runErr)
	}
}
func (humanApprovalTestFactory) Create(node.Config) (node.Node, error) {
	return humanApprovalTestNode{}, nil
}

type humanApprovalTestNode struct{}

func (humanApprovalTestNode) Execute(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, fmt.Errorf("human-approval executor should be driven by HumanGateway")
}

func (humanApprovalTestNode) ExecuteHumanApproval(ctx node.ExecutionContext, approved bool, advise string) (map[string]artifact.ArtifactRef, error) {
	approve, err := ctx.Store.Put(artifact.Artifact{ID: "approve", Kind: "bool", Data: approved})
	if err != nil {
		return nil, err
	}
	advice, err := ctx.Store.Put(artifact.Artifact{ID: "advise", Kind: "markdown", Data: advise})
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"approve": approve, "advise": advice}, nil
}

func TestApprovalRejectsDriveReworkAndApproveSettlesWithoutExtraRounds(t *testing.T) {
	store := artifact.NewMemStore()
	put := func(ctx node.ExecutionContext, id string) (map[string]artifact.ArtifactRef, error) {
		ref, err := ctx.Store.Put(artifact.Artifact{ID: id, Kind: "markdown", Data: id})
		return map[string]artifact.ArtifactRef{id: ref}, err
	}
	source := fnFactory{
		definition: "source", outputs: map[string]definition.OutputPort{"work": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "work")
			}), nil
		},
	}
	backend := fnFactory{
		definition: "backend",
		inputs: map[string]definition.InputPort{
			"work": {Type: "markdown"}, "advise": {Type: "markdown", Optional: true},
		},
		outputs: map[string]definition.OutputPort{"backend": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "backend")
			}), nil
		},
	}
	frontend := fnFactory{
		definition: "frontend",
		inputs: map[string]definition.InputPort{
			"backend": {Type: "markdown"}, "advise": {Type: "markdown", Optional: true},
		},
		outputs: map[string]definition.OutputPort{"frontend": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "frontend")
			}), nil
		},
	}
	deploy := fnFactory{
		definition: "deploy", outputs: map[string]definition.OutputPort{"deployed": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "deployed")
			}), nil
		},
	}
	dr, er := newTestRegistries(t, source, backend, frontend, humanApprovalTestFactory{}, deploy)
	gateway := &fakeHumanGateway{responses: []RoundResponse{
		{Approved: false, Advise: "add tests"},
		{Approved: false, Advise: "fix layout"},
		{Approved: true},
	}}
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "approval-loop"},
		Nodes: map[string]workflow.NodeSpec{
			"source": {Node: "source"},
			"backend": {Node: "backend", Inputs: map[string]workflow.InputBinding{
				"work": {From: "source.work"}, "advise": {From: "review.advise"},
			}},
			"frontend": {Node: "frontend", Inputs: map[string]workflow.InputBinding{
				"backend": {From: "backend.backend"}, "advise": {From: "review.advise"},
			}},
			"review": {Node: "human-approval", DependsOn: []string{"backend", "frontend"}},
			"deploy": {Node: "deploy", DependsOn: []string{"review"}},
		},
	}

	exec, err := runUntilStopped(t, NewEngine(er, dr, store, nil,
		WithHumanGateway(gateway), WithConvergenceLimit(1)), def)
	if err != nil || exec.Status != StatusStopped {
		t.Fatalf("Run() = %s/%v, want Stopped/nil", exec.Status, err)
	}
	for id, wantRounds := range map[string]int{"source": 1, "backend": 3, "frontend": 3, "review": 3, "deploy": 1} {
		if got := exec.Node(id).Current.Round; got != wantRounds {
			t.Errorf("%s rounds = %d, want %d", id, got, wantRounds)
		}
	}
	backendRuns := append(append([]NodeRun{}, exec.Node("backend").History...), exec.Node("backend").Current)
	for i, wantVersion := range []string{"1", "2"} {
		if got := backendRuns[i+1].Inputs["advise"].Ref.Version; got != wantVersion {
			t.Errorf("backend round %d advise version = %q, want %q", i+2, got, wantVersion)
		}
	}
	requests := gateway.Requests()
	if len(requests) != 3 {
		t.Fatalf("approval requests = %d, want 3", len(requests))
	}
	for _, req := range requests {
		if req.Kind != RoundRequestApproval || req.NodeID != "review" {
			t.Errorf("request = %+v", req)
		}
	}
	if got := requests[1].AdviseHistory; len(got) != 1 || got[0] != "add tests" {
		t.Errorf("second request advise history = %v, want [add tests]", got)
	}
	if len(requests[0].Artifacts) == 0 {
		t.Error("first approval request has no artifact summaries")
	}
}
