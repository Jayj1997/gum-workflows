package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func TestStaticAnalysisBundleRunsPOSIXContract(t *testing.T) {
	tests := []struct {
		name, mode string
		verdict    scriptnode.Verdict
		findings   int
	}{
		{name: "passed", mode: "passed", verdict: scriptnode.VerdictPassed},
		{name: "vet finding is a successful Node Run result", mode: "finding", verdict: scriptnode.VerdictFailed, findings: 1},
		{name: "no package", mode: "empty", verdict: scriptnode.VerdictNotApplicable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installFakeGo(t)
			t.Setenv("FAKE_GO_MODE", tt.mode)
			workspace := t.TempDir()
			nodeRunDir := t.TempDir()
			store := artifact.NewMemStore()
			diagnostics := &node.RunDiagnostics{}
			factory := staticAnalysisExecutor{}
			check, err := factory.Create(nil)
			if err != nil {
				t.Fatal(err)
			}
			outputs, err := check.Execute(node.ExecutionContext{
				Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: store,
				Run: node.RunContext{
					WorkflowRunID: "run", NodeRunID: "node-run",
					LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output"),
				},
				Diagnostics: diagnostics,
			}, map[string]artifact.ArtifactRef{
				"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace},
			})
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
			body, err := store.Get(outputs["result"])
			if err != nil {
				t.Fatal(err)
			}
			result, ok := body.Data.(scriptnode.StaticResult)
			if !ok || result.Verdict != tt.verdict || result.FindingsCount != tt.findings {
				t.Fatalf("result = %#v, want %s with %d findings", body.Data, tt.verdict, tt.findings)
			}
			if tt.mode == "finding" && (result.Findings[0].Analyzer != "printf" || result.Findings[0].Position != "app.go:9:2") {
				t.Errorf("structured vet finding = %+v", result.Findings[0])
			}
			stdout, _ := os.ReadFile(filepath.Join(nodeRunDir, "logs", "stdout.log"))
			if !strings.Contains(string(stdout), "running go static analysis") {
				t.Errorf("stdout log = %q", stdout)
			}
			if diagnostics.BundleDigest == "" || diagnostics.ResultAdapter != staticAnalysisAdapterID {
				t.Errorf("diagnostics = %+v", diagnostics)
			}
			entries, _ := os.ReadDir(workspace)
			if len(entries) != 0 {
				t.Errorf("workspace contains Gum output: %v", entries)
			}
		})
	}
}

func TestStaticAnalysisBundleParsesRealGoVetJSONStream(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("static analysis v1 supports Darwin and Linux")
	}
	t.Setenv("GOCACHE", t.TempDir())
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOTOOLCHAIN", "local")
	workspace := t.TempDir()
	writeBuiltinFixture(t, filepath.Join(workspace, "go.mod"), "module example.com/vetfixture\n\ngo 1.25.0\n")
	writeBuiltinFixture(t, filepath.Join(workspace, "main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Printf(\"%d\", \"wrong\") }\n")
	nodeRunDir := t.TempDir()
	store := artifact.NewMemStore()
	check, err := (staticAnalysisExecutor{}).Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: store,
		Run: node.RunContext{
			LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output"),
		},
	}, map[string]artifact.ArtifactRef{
		"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace},
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	body, err := store.Get(outputs["result"])
	if err != nil {
		t.Fatal(err)
	}
	result := body.Data.(scriptnode.StaticResult)
	if result.Verdict != scriptnode.VerdictFailed || result.FindingsCount != 1 || result.Findings[0].Analyzer != "printf" {
		t.Fatalf("real go vet result = %+v", result)
	}
}

func TestEngineRunPublishesStaticResultAndNodeRunDiagnostics(t *testing.T) {
	installFakeGo(t)
	t.Setenv("FAKE_GO_MODE", "finding")
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
		Metadata: workflow.Metadata{Name: "static-engine"},
		Nodes: map[string]workflow.NodeSpec{
			"check": {Node: staticAnalysisDefinition, Inputs: map[string]workflow.InputBinding{
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
	if run.Diagnostics.BundleDigest == "" || run.Diagnostics.ResultAdapter != staticAnalysisAdapterID || run.Diagnostics.Toolchain == nil || run.Diagnostics.Toolchain.FinalVersion != "go1.25.0" {
		t.Errorf("run diagnostics = %+v", run.Diagnostics)
	}
	body, err := store.Get(run.Outputs["result"])
	if err != nil {
		t.Fatal(err)
	}
	result := body.Data.(scriptnode.StaticResult)
	if result.Verdict != scriptnode.VerdictFailed {
		t.Errorf("result verdict = %s, want failed", result.Verdict)
	}
}

type cancelWhenNodeSucceeds struct {
	nodeID string
	cancel context.CancelFunc
}

func (r cancelWhenNodeSucceeds) Record(_ context.Context, exec *execution.WorkflowExecution) error {
	if n := exec.Node(r.nodeID); n != nil && n.Current.Status == execution.StatusSucceeded {
		r.cancel()
	}
	return nil
}

func TestStaticAnalysisExecutorRejectsNodeScriptOrToolOverrides(t *testing.T) {
	for _, config := range []node.Config{
		{"script": "echo bypass"},
		{"command": "go vet ./pkg/..."},
		{"packageScope": "./pkg/..."},
	} {
		if _, err := (staticAnalysisExecutor{}).Create(config); err == nil {
			t.Errorf("Create(%v) = nil error, want closed config rejection", config)
		}
	}
}

func installFakeGo(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  version)
    printf 'go version go1.25.0 darwin/arm64\n'
    ;;
  env)
    printf 'go1.25.0\n/go\ndarwin\narm64\n1\n'
    ;;
  list)
    if [ "$FAKE_GO_MODE" != empty ]; then printf 'example.com/app\n'; fi
    ;;
	  vet)
	    if [ "$FAKE_GO_MODE" = finding ]; then
	      printf '%s\n' '# example.com/app' '# [example.com/app]' >&2
	      printf '%s\n' '{"example.com/app":{"printf":[{"posn":"app.go:9:2","message":"wrong printf format"}]}}' >&2
	      exit 1
	    fi
	    printf '%s\n' '# example.com/app' '{}' >&2
    ;;
  *)
    printf 'unexpected fake go arguments: %s\n' "$*" >&2
    exit 2
    ;;
esac
`
	path := filepath.Join(bin, "go")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", bin, os.PathListSeparator, os.Getenv("PATH")))
}

func writeBuiltinFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
