package builtins

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/agent"
	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

type integrationFactory struct {
	definition string
	create     func() node.Node
}

func (f integrationFactory) Definition() string { return f.definition }
func (f integrationFactory) Version() string    { return "v1" }
func (f integrationFactory) Create(node.Config) (node.Node, error) {
	return f.create(), nil
}

type integrationNode func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error)

func (fn integrationNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return fn(ctx, inputs)
}

type integrationCodingAgent struct {
	fail bool
	runs atomic.Int32
}

func (a *integrationCodingAgent) Execute(context.Context, agent.Task, project.Context, []artifact.ArtifactRef) ([]artifact.ArtifactRef, error) {
	a.runs.Add(1)
	if a.fail {
		return nil, fmt.Errorf("agent failed")
	}
	return []artifact.ArtifactRef{{ID: "adapter-code", Kind: artifact.KindSourceCode, URI: "adapter://code"}}, nil
}

func TestEngineRunUsesCodingAgentCodeContract(t *testing.T) {
	t.Run("successful code versions retrigger backend binding", func(t *testing.T) {
		a := &integrationCodingAgent{}
		e, def := newCodingAgentIntegrationEngine(t, a)

		exec, err := e.Run(context.Background(), def)
		if err == nil || exec.Status != execution.StatusFailed {
			t.Fatalf("Run() = %s/%v, want convergence failure after repeated code versions", exec.Status, err)
		}
		check := exec.Node("check")
		runs := append(append([]execution.NodeRun{}, check.History...), check.Current)
		if len(runs) != 3 {
			t.Fatalf("check rounds = %d, want 3", len(runs))
		}
		for i, run := range runs {
			input := run.Inputs["code"]
			wantVersion := fmt.Sprint(i + 1)
			if input.From != "backend.code" || input.Ref.Kind != artifact.KindSourceCode || input.Ref.Version != wantVersion {
				t.Errorf("check round %d input = %+v, want backend.code SourceCode v%s", run.Round, input, wantVersion)
			}
		}
	})

	t.Run("failed agent publishes no code and does not trigger check", func(t *testing.T) {
		a := &integrationCodingAgent{fail: true}
		e, def := newCodingAgentIntegrationEngine(t, a)

		exec, err := e.Run(context.Background(), def)
		if err == nil {
			t.Fatal("Run() = nil error, want agent failure")
		}
		if outputs := exec.Node("backend").Current.Outputs; len(outputs) != 0 {
			t.Errorf("failed backend outputs = %+v, want none", outputs)
		}
		if status := exec.Node("check").Current.Status; status != execution.StatusPending {
			t.Errorf("check status = %s, want Pending", status)
		}
	})
}

func newCodingAgentIntegrationEngine(t *testing.T, codingAgent agent.CodingAgent) (*execution.Engine, workflow.Definition) {
	t.Helper()
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []definition.NodeDefinition{
		{
			APIVersion: definition.NodeDefinitionAPIVersionV1, Kind: definition.NodeDefinitionKind,
			Metadata: definition.Metadata{Name: "code-check", Description: "test"}, Type: definition.TypeAutomation,
			Inputs:  map[string]definition.InputPort{"code": {Type: "SourceCode"}},
			Outputs: map[string]definition.OutputPort{"result": {Type: "TestReport"}},
		},
		{
			APIVersion: definition.NodeDefinitionAPIVersionV1, Kind: definition.NodeDefinitionKind,
			Metadata: definition.Metadata{Name: "feedback", Description: "test"}, Type: definition.TypeAutomation,
			Inputs:  map[string]definition.InputPort{"result": {Type: "TestReport"}},
			Outputs: map[string]definition.OutputPort{"advise": {Type: "markdown"}},
		},
	} {
		if err := definitions.RegisterDefinition(d); err != nil {
			t.Fatal(err)
		}
	}

	executors := node.NewExecutorRegistry()
	for _, factory := range []node.ExecutorFactory{
		newCodingAgentExecutor(codingAgent),
		integrationFactory{definition: "code-check", create: func() node.Node {
			return integrationNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "result", Kind: artifact.KindTestReport})
				return map[string]artifact.ArtifactRef{"result": ref}, err
			})
		}},
		integrationFactory{definition: "feedback", create: func() node.Node {
			return integrationNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "advise", Kind: "markdown"})
				return map[string]artifact.ArtifactRef{"advise": ref}, err
			})
		}},
	} {
		if err := executors.Register(factory); err != nil {
			t.Fatal(err)
		}
	}

	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "coding-agent-code-contract"},
		Nodes: map[string]workflow.NodeSpec{
			"backend": {Node: "coding-agent", Inputs: map[string]workflow.InputBinding{
				"advise": {From: "feedback.advise"},
			}},
			"check": {Node: "code-check", Inputs: map[string]workflow.InputBinding{
				"code": {From: "backend.code"},
			}},
			"feedback": {Node: "feedback", Inputs: map[string]workflow.InputBinding{
				"result": {From: "check.result"},
			}},
		},
	}
	workspace := t.TempDir()
	engine := execution.NewEngine(executors, definitions, artifact.NewMemStore(), nil,
		execution.WithProjectContext(project.Context{
			Repository: project.Repository{Path: workspace}, Workspace: workspace,
		}),
		execution.WithConvergenceLimit(3),
	)
	return engine, def
}
