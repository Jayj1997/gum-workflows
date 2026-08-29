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

func TestEngineRunPublishesCoverageResultAndNodeRunDiagnostics(t *testing.T) {
	installFakeCoverageGo(t)
	t.Setenv("FAKE_GO_MODE", "passed")
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
		Metadata: workflow.Metadata{Name: "coverage-engine"},
		Nodes: map[string]workflow.NodeSpec{
			"check": {Node: coverageDefinition, Inputs: map[string]workflow.InputBinding{
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
	if run.Diagnostics.BundleDigest != coverageBundleDigest || run.Diagnostics.ResultAdapter != coverageAdapterID || run.Diagnostics.Toolchain == nil || run.Diagnostics.Toolchain.Tool != "go test" {
		t.Errorf("run diagnostics = %+v", run.Diagnostics)
	}
	body, err := store.Get(run.Outputs["result"])
	if err != nil {
		t.Fatal(err)
	}
	result := body.Data.(scriptnode.CoverageResult)
	if result.Verdict != scriptnode.VerdictFailed || result.EffectiveConfig.MinimumStatementCoverage != 80 {
		t.Errorf("result = %+v, want business failure at default threshold 80", result)
	}
}
