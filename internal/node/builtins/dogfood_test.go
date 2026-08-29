package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func TestDogfoodChecksRunConcurrentlyAndPersistIndependentResults(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/dogfood\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataRoot := t.TempDir()
	runID := "11111111-1111-4111-8111-111111111111"
	paths, err := runtimepath.New(filepath.Join(dataRoot, "product.db"), filepath.Join(dataRoot, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := artifact.NewFilesystemStore(paths.ArtifactsDir(runID))
	if err != nil {
		t.Fatal(err)
	}
	historyStore, err := history.Open(context.Background(), paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer historyStore.Close()

	started := make(chan dogfoodRunPaths, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseChecks := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseChecks()
	executors := node.NewExecutorRegistry()
	for _, definition := range []string{staticAnalysisDefinition, coverageDefinition, raceDefinition, complexityDefinition} {
		if err := executors.Register(dogfoodFactory{definition: definition, started: started, release: release}); err != nil {
			t.Fatal(err)
		}
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := &dogfoodRecorder{store: historyStore, cancel: cancel}
	engine := execution.NewEngine(executors, definitions, store, nil,
		execution.WithRunID(runID),
		execution.WithStateDir(paths.RunsDir()),
		execution.WithProjectContext(project.Context{Workspace: workspace}),
		execution.WithRunRecorder(recorder),
	)
	definition := dogfoodWorkflowDefinition()
	type runResult struct {
		exec *execution.WorkflowExecution
		err  error
	}
	done := make(chan runResult, 1)
	go func() {
		exec, runErr := engine.Run(ctx, definition)
		done <- runResult{exec: exec, err: runErr}
	}()

	runs := make([]dogfoodRunPaths, 0, 4)
	for len(runs) < 4 {
		select {
		case run := <-started:
			runs = append(runs, run)
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d quality checks entered Running; want all four without hidden serialization", len(runs))
		}
	}
	releaseChecks()

	result := <-done
	if result.err != nil || result.exec.Status != execution.StatusStopped {
		t.Fatalf("Engine.Run() = %s, %v; want Stopped without error", result.exec.Status, result.err)
	}
	seenLogs := map[string]bool{}
	seenToolOutput := map[string]bool{}
	for _, run := range runs {
		if run.workspace != workspace {
			t.Errorf("%s workspace = %q, want shared %q", run.definition, run.workspace, workspace)
		}
		if seenLogs[run.logsDir] || seenToolOutput[run.toolOutputDir] {
			t.Fatalf("Node Runs share logs/tool-output directories: %+v", runs)
		}
		seenLogs[run.logsDir] = true
		seenToolOutput[run.toolOutputDir] = true
		for _, path := range []string{run.logsDir, run.toolOutputDir} {
			if rel, err := filepath.Rel(dataRoot, path); err != nil || rel == ".." || filepath.IsAbs(rel) {
				t.Errorf("Node Run path %q is outside Local Data Root %q", path, dataRoot)
			}
		}
	}

	for id := range definition.Nodes {
		run := result.exec.Node(id).Current
		if run.Status != execution.StatusSucceeded || run.Outputs["result"].Kind != artifact.KindQualityCheckResult {
			t.Fatalf("node %q = %+v, want succeeded QualityCheckResult", id, run)
		}
		detail, err := historyStore.GetNodeRun(context.Background(), result.exec.RunID, id)
		if err != nil || detail == nil || len(detail.Rounds) != 1 || detail.Rounds[0].Outputs["result"].Kind != artifact.KindQualityCheckResult {
			t.Fatalf("history node %q = %+v, %v", id, detail, err)
		}
		body, err := store.Get(run.Outputs["result"])
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(body.Data)
		if err != nil {
			t.Fatal(err)
		}
		if err := decodeDogfoodResult(definition.Nodes[id].Node, encoded); err != nil {
			t.Errorf("node %q result does not satisfy its schema: %v", id, err)
		}
	}
	if entries, err := os.ReadDir(workspace); err != nil || len(entries) != 1 || entries[0].Name() != "go.mod" {
		t.Fatalf("Project Workspace contains Gum output: %v, %v", entries, err)
	}
}

type dogfoodRunPaths struct {
	definition    string
	workspace     string
	logsDir       string
	toolOutputDir string
}

type dogfoodFactory struct {
	definition string
	started    chan<- dogfoodRunPaths
	release    <-chan struct{}
}

func (f dogfoodFactory) Definition() string { return f.definition }
func (f dogfoodFactory) Version() string    { return "v1" }
func (f dogfoodFactory) Create(node.Config) (node.Node, error) {
	return dogfoodNode(f), nil
}

type dogfoodNode dogfoodFactory

func (n dogfoodNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	n.started <- dogfoodRunPaths{
		definition: n.definition, workspace: ctx.Project.Workspace,
		logsDir: ctx.Run.LogsDir, toolOutputDir: ctx.Run.ToolOutputDir,
	}
	select {
	case <-n.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := os.MkdirAll(ctx.Run.LogsDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(ctx.Run.ToolOutputDir, 0o755); err != nil {
		return nil, err
	}
	stdout := filepath.Join(ctx.Run.LogsDir, "stdout.log")
	stderr := filepath.Join(ctx.Run.LogsDir, "stderr.log")
	if err := os.WriteFile(stdout, []byte("dogfood check started\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(stderr, nil, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(ctx.Run.ToolOutputDir, "evidence.json"), []byte("{}\n"), 0o644); err != nil {
		return nil, err
	}
	code := inputs["code"]
	result, err := dogfoodResult(n.definition, code, stdout, stderr)
	if err != nil {
		return nil, err
	}
	ref, err := ctx.Store.Put(artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult, Version: "1", Data: result})
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"result": ref}, nil
}

type dogfoodRecorder struct {
	store  *history.Store
	cancel context.CancelFunc
	once   sync.Once
}

func (r *dogfoodRecorder) Record(ctx context.Context, exec *execution.WorkflowExecution) error {
	if err := r.store.Record(ctx, exec); err != nil {
		return err
	}
	succeeded := 0
	for _, n := range exec.Nodes {
		if n.Current.Status == execution.StatusSucceeded {
			succeeded++
		}
	}
	if succeeded == 4 {
		r.once.Do(r.cancel)
	}
	return nil
}

func dogfoodWorkflowDefinition() workflow.Definition {
	nodes := map[string]workflow.NodeSpec{}
	for id, definition := range map[string]string{
		"static-analysis":  staticAnalysisDefinition,
		"coverage-check":   coverageDefinition,
		"race-check":       raceDefinition,
		"complexity-check": complexityDefinition,
	} {
		nodes[id] = workflow.NodeSpec{Node: definition, Inputs: map[string]workflow.InputBinding{"code": {From: "project.code"}}}
	}
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "dogfood-acceptance"}, Nodes: nodes,
	}
}

func dogfoodResult(definition string, code artifact.ArtifactRef, stdout, stderr string) (any, error) {
	now := time.Now().UTC()
	logs := scriptnode.LogReferences{
		Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: stdout},
		Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: stderr},
	}
	toolchain := scriptnode.Toolchain{
		LauncherVersion: "go1.25.0", FinalVersion: "go1.25.0", GOROOT: "/go",
		GOOS: "darwin", GOARCH: "arm64", CGOEnabled: "1",
	}
	percent, zero, one := 100.0, 0, 1
	var result any
	switch definition {
	case staticAnalysisDefinition:
		toolchain.Tool = "go vet"
		result = scriptnode.StaticResult{
			APIVersion: "qualityCheckResult/v1", Check: scriptnode.StaticAnalysisCheck, Verdict: scriptnode.VerdictPassed,
			Code: code, EffectiveConfig: scriptnode.StaticEffectiveConfig{PackageScope: "./..."}, Toolchain: toolchain,
			Logs: logs, StartedAt: now, FinishedAt: now,
		}
	case coverageDefinition:
		toolchain.Tool = "go test"
		result = scriptnode.CoverageResult{
			APIVersion: "qualityCheckResult/v1", Check: scriptnode.CoverageCheck, Verdict: scriptnode.VerdictPassed,
			Code: code, EffectiveConfig: scriptnode.CoverageEffectiveConfig{PackageScope: "./...", MinimumStatementCoverage: 80}, Toolchain: toolchain,
			Metrics: scriptnode.CoverageMetrics{StatementCoverage: scriptnode.CoverageMetric{Available: true, Value: &percent, Unit: "percent"}},
			Logs:    logs, StartedAt: now, FinishedAt: now,
		}
	case raceDefinition:
		toolchain.Tool, toolchain.CCompiler = "go test -race", "clang"
		result = scriptnode.RaceResult{
			APIVersion: "qualityCheckResult/v1", Check: scriptnode.RaceCheck, Verdict: scriptnode.VerdictPassed,
			Code: code, EffectiveConfig: scriptnode.RaceEffectiveConfig{PackageScope: "./..."}, Toolchain: toolchain,
			Metrics: scriptnode.RaceMetrics{RacesDetected: scriptnode.RaceMetric{Available: true, Value: &zero, Unit: "count"}},
			Logs:    logs, StartedAt: now, FinishedAt: now,
		}
	case complexityDefinition:
		toolchain.Tool = "go ast"
		result = scriptnode.ComplexityResult{
			APIVersion: "qualityCheckResult/v1", Check: scriptnode.ComplexityCheck, Verdict: scriptnode.VerdictPassed,
			Code: code, EffectiveConfig: scriptnode.ComplexityEffectiveConfig{PackageScope: "./...", MaximumCyclomaticComplexity: 15, ExcludeGeneratedFiles: true, ExcludeVendor: true}, Toolchain: toolchain,
			Metrics: scriptnode.ComplexityMetrics{
				MaxCyclomaticComplexity: scriptnode.ComplexityMetric{Available: true, Value: &one, Unit: "count"},
				FunctionsAnalyzed:       scriptnode.ComplexityMetric{Available: true, Value: &one, Unit: "count"},
				FunctionsOverThreshold:  scriptnode.ComplexityMetric{Available: true, Value: &zero, Unit: "count"},
			}, Logs: logs, StartedAt: now, FinishedAt: now,
		}
	default:
		return nil, fmt.Errorf("unknown dogfood definition %q", definition)
	}
	return result, nil
}

func decodeDogfoodResult(definition string, data []byte) error {
	switch definition {
	case staticAnalysisDefinition:
		_, err := scriptnode.DecodeStaticResult(data)
		return err
	case coverageDefinition:
		_, err := scriptnode.DecodeCoverageResult(data)
		return err
	case raceDefinition:
		_, err := scriptnode.DecodeRaceResult(data)
		return err
	case complexityDefinition:
		_, err := scriptnode.DecodeComplexityResult(data)
		return err
	default:
		return fmt.Errorf("unknown dogfood definition %q", definition)
	}
}
