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

func TestEngineRunPublishesRaceFailureResultAndNodeRunDiagnostics(t *testing.T) {
	installFakeRaceToolchain(t)
	t.Setenv("FAKE_GO_MODE", "race")
	executors := node.NewExecutorRegistry()
	if err := RegisterAll(executors); err != nil {
		t.Fatal(err)
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	recorder := cancelWhenNodeSucceeds{nodeID: "check", cancel: cancel}
	workspace := t.TempDir()
	store := artifact.NewMemStore()
	engine := execution.NewEngine(executors, definitions, store, nil,
		execution.WithStateDir(t.TempDir()),
		execution.WithProjectContext(project.Context{Workspace: workspace}),
		execution.WithRunRecorder(recorder),
	)
	exec, err := engine.Run(ctx, workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "race-engine"},
		Nodes: map[string]workflow.NodeSpec{
			"check": {Node: raceDefinition, Inputs: map[string]workflow.InputBinding{
				"code": {From: "project.code"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	run := exec.Node("check").Current
	if run.Status != execution.StatusSucceeded || run.Outputs["result"].Kind != artifact.KindQualityCheckResult {
		t.Fatalf("check run = %+v", run)
	}
	if run.Diagnostics.BundleDigest != raceBundleDigest || run.Diagnostics.ResultAdapter != raceAdapterID || run.Diagnostics.Toolchain == nil || run.Diagnostics.Toolchain.Tool != "go test -race" || run.Diagnostics.Toolchain.CCompiler != "fake-cc" {
		t.Errorf("run diagnostics = %+v", run.Diagnostics)
	}
	body, err := store.Get(run.Outputs["result"])
	if err != nil {
		t.Fatal(err)
	}
	result := body.Data.(scriptnode.RaceResult)
	if result.Verdict != scriptnode.VerdictFailed || result.Metrics.RacesDetected.Value == nil || *result.Metrics.RacesDetected.Value != 1 {
		t.Errorf("result = %+v, want one observed race as business failure", result)
	}
}
