package history

import (
	"context"
	"fmt"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

type engineTestFactory struct {
	definition string
	create     func() node.Node
}

func (f engineTestFactory) Definition() string { return f.definition }
func (f engineTestFactory) Version() string    { return "v1" }
func (f engineTestFactory) Create(node.Config) (node.Node, error) {
	return f.create(), nil
}

type engineTestNode func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error)

func (n engineTestNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return n(ctx, inputs)
}

type engineTestApprovalNode struct{}

func (engineTestApprovalNode) Execute(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, fmt.Errorf("approval must be driven by HumanGateway")
}

func (engineTestApprovalNode) ExecuteHumanApproval(ctx node.ExecutionContext, approved bool, advise string) (map[string]artifact.ArtifactRef, error) {
	approveRef, err := ctx.Store.Put(artifact.Artifact{ID: "approve", Kind: "bool", Data: approved})
	if err != nil {
		return nil, err
	}
	adviseRef, err := ctx.Store.Put(artifact.Artifact{ID: "advise", Kind: "markdown", Data: advise})
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"approve": approveRef, "advise": adviseRef}, nil
}

type engineTestGateway struct {
	responses []execution.RoundResponse
}

func (g *engineTestGateway) RequestRound(context.Context, execution.RoundRequest) (execution.RoundResponse, error) {
	if len(g.responses) == 0 {
		return execution.RoundResponse{}, fmt.Errorf("no approval response")
	}
	response := g.responses[0]
	g.responses = g.responses[1:]
	return response, nil
}

type cancelingRecorder struct {
	store  *Store
	cancel context.CancelFunc
}

func (r cancelingRecorder) Record(ctx context.Context, exec *execution.WorkflowExecution) error {
	if err := r.store.Record(ctx, exec); err != nil {
		return err
	}
	if review := exec.Node("review"); review != nil && review.Current.Round == 2 && review.Current.Status == execution.StatusSucceeded {
		r.cancel()
	}
	return nil
}

func TestEngineApprovalRoundsAreRecordedOneRowPerRound(t *testing.T) {
	store, _ := openTest(t)
	definitions := definition.NewRegistry()
	for _, nodeType := range []definition.NodeType{definition.TypeAgent, definition.TypeHuman} {
		if err := definitions.RegisterNodeType(definition.NodeTypeDefinition{
			APIVersion: definition.NodeTypeAPIVersionV1, Kind: definition.NodeTypeDefinitionKind,
			Metadata: definition.Metadata{Name: string(nodeType)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, def := range []definition.NodeDefinition{
		{
			APIVersion: definition.NodeDefinitionAPIVersionV1, Kind: definition.NodeDefinitionKind,
			Metadata: definition.Metadata{Name: "worker"}, Type: definition.TypeAgent,
			Inputs:  map[string]definition.InputPort{"advise": {Type: "markdown", Optional: true}},
			Outputs: map[string]definition.OutputPort{"result": {Type: "markdown"}},
		},
		{
			APIVersion: definition.NodeDefinitionAPIVersionV1, Kind: definition.NodeDefinitionKind,
			Metadata: definition.Metadata{Name: "human-approval"}, Type: definition.TypeHuman,
			Outputs: map[string]definition.OutputPort{"approve": {Type: "bool"}, "advise": {Type: "markdown"}},
		},
	} {
		if err := definitions.RegisterDefinition(def); err != nil {
			t.Fatal(err)
		}
	}

	executors := node.NewExecutorRegistry()
	worker := engineTestFactory{definition: "worker", create: func() node.Node {
		return engineTestNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
			ref, err := ctx.Store.Put(artifact.Artifact{ID: "result", Kind: "markdown", Data: "done"})
			return map[string]artifact.ArtifactRef{"result": ref}, err
		})
	}}
	review := engineTestFactory{definition: "human-approval", create: func() node.Node { return engineTestApprovalNode{} }}
	for _, factory := range []node.ExecutorFactory{worker, review} {
		if err := executors.Register(factory); err != nil {
			t.Fatal(err)
		}
	}

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "recorded-approval-loop"},
		Nodes: map[string]workflow.NodeSpec{
			"worker": {Node: "worker", Inputs: map[string]workflow.InputBinding{"advise": {From: "review.advise"}}},
			"review": {Node: "human-approval", DependsOn: []string{"worker"}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	gateway := &engineTestGateway{responses: []execution.RoundResponse{
		{Approved: false, Advise: "retry"}, {Approved: true},
	}}
	engine := execution.NewEngine(executors, definitions, artifact.NewMemStore(), nil,
		execution.WithHumanGateway(gateway), execution.WithRunRecorder(cancelingRecorder{store: store, cancel: cancel}))
	exec, err := engine.Run(ctx, def)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if exec.Status != execution.StatusStopped {
		t.Fatalf("execution status = %s, want Stopped", exec.Status)
	}
	var recordedStatus string
	if err := store.db.QueryRow(`SELECT status FROM workflow_run_history WHERE id = ?`, exec.RunID).Scan(&recordedStatus); err != nil {
		t.Fatal(err)
	}
	if recordedStatus != "Stopped" {
		t.Errorf("recorded workflow status = %q, want Stopped after cancellation", recordedStatus)
	}

	rows, err := store.db.Query(`SELECT id, round FROM workflow_node_run_history WHERE run_id = ? AND node_id = 'review' ORDER BY round`, exec.RunID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	var rounds []int
	for rows.Next() {
		var id string
		var round int
		if err := rows.Scan(&id, &round); err != nil {
			t.Fatal(err)
		}
		ids, rounds = append(ids, id), append(rounds, round)
	}
	if len(rounds) != 2 || rounds[0] != 1 || rounds[1] != 2 {
		t.Fatalf("review rounds = %v, want [1 2]", rounds)
	}
	allRuns := append(append([]execution.NodeRun{}, exec.Node("review").History...), exec.Node("review").Current)
	if ids[0] != allRuns[0].RunID || ids[1] != allRuns[1].RunID {
		t.Errorf("database node-run ids = %v, execution ids = [%s %s]", ids, allRuns[0].RunID, allRuns[1].RunID)
	}
}
