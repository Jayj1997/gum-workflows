package builtins

import (
	"context"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func TestEngineRunPublishesFailedComplexityResultAsSuccessfulNodeRun(t *testing.T) {
	workspace := t.TempDir()
	writeBuiltinFixture(t, workspace+"/go.mod", "module example.com/enginecomplexity\n\ngo 1.25.0\n")
	writeBuiltinFixture(t, workspace+"/app.go", "package app\nfunc Complex(a bool) { if a {} }\n")
	t.Setenv("GOCACHE", t.TempDir())
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOTOOLCHAIN", "local")

	executors := node.NewExecutorRegistry()
	if err := RegisterAll(executors); err != nil {
		t.Fatal(err)
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store := artifact.NewMemStore()
	engine := execution.NewEngine(executors, definitions, store, nil,
		execution.WithStateDir(t.TempDir()), execution.WithProjectContext(project.Context{Workspace: workspace}),
		execution.WithRunRecorder(cancelWhenNodeSucceeds{nodeID: "check", cancel: cancel}),
	)
	exec, err := engine.Run(ctx, workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow, Metadata: workflow.Metadata{Name: "complexity-engine"},
		Nodes: map[string]workflow.NodeSpec{"check": {Node: complexityDefinition, Config: map[string]any{"maximumCyclomaticComplexity": 1}, Inputs: map[string]workflow.InputBinding{"code": {From: "project.code"}}}},
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	run := exec.Node("check").Current
	if run.Status != execution.StatusSucceeded || run.Outputs["result"].Kind != artifact.KindQualityCheckResult {
		t.Fatalf("run = %+v", run)
	}
	body, err := store.Get(run.Outputs["result"])
	if err != nil {
		t.Fatal(err)
	}
	result := body.Data.(scriptnode.ComplexityResult)
	if result.Verdict != scriptnode.VerdictFailed || len(result.Findings) != 1 || result.EffectiveConfig.MaximumCyclomaticComplexity != 1 {
		t.Fatalf("result = %+v", result)
	}
}
