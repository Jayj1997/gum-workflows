package workflow_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func TestDogfoodChecksRunConcurrentlyAndPersistIndependentResults(t *testing.T) {
	installDogfoodGo(t)
	workspace := t.TempDir()
	writeDogfoodFile(t, filepath.Join(workspace, "go.mod"), "module example.com/dogfood\n\ngo 1.25.0\n")
	writeDogfoodFile(t, filepath.Join(workspace, "app.go"), "package dogfood\n\nfunc Ready() bool { return true }\n")
	dataRoot := t.TempDir()
	runID := "11111111-1111-4111-8111-111111111111"
	paths, err := runtimepath.New(filepath.Join(dataRoot, "product.db"), filepath.Join(dataRoot, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	artifactStore, err := artifact.NewFilesystemStore(paths.ArtifactsDir(runID))
	if err != nil {
		t.Fatal(err)
	}
	historyStore, err := history.Open(context.Background(), paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer historyStore.Close()

	type checkCase struct {
		id         string
		definition string
		decode     func([]byte) error
	}
	checks := []checkCase{
		{id: "static-analysis", definition: "go-static-analysis", decode: func(data []byte) error { _, err := scriptnode.DecodeStaticResult(data); return err }},
		{id: "coverage-check", definition: "go-coverage-check", decode: func(data []byte) error { _, err := scriptnode.DecodeCoverageResult(data); return err }},
		{id: "race-check", definition: "go-race-check", decode: func(data []byte) error { _, err := scriptnode.DecodeRaceResult(data); return err }},
		{id: "complexity-check", definition: "go-complexity-check", decode: func(data []byte) error { _, err := scriptnode.DecodeComplexityResult(data); return err }},
	}
	actualExecutors := node.NewExecutorRegistry()
	if err := builtins.RegisterAll(actualExecutors); err != nil {
		t.Fatal(err)
	}
	started := make(chan dogfoodRunPaths, len(checks))
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseChecks := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseChecks()
	executors := node.NewExecutorRegistry()
	entry, err := actualExecutors.Get("human-input", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := executors.Register(entry); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks {
		actual, err := actualExecutors.Get(check.definition, "v1")
		if err != nil {
			t.Fatal(err)
		}
		if err := executors.Register(gatedFactory{ExecutorFactory: actual, started: started, release: release}); err != nil {
			t.Fatal(err)
		}
	}
	definitions, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definition, err := workflow.LoadFile(filepath.Join("..", "..", "examples", "dogfood", "workflow.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := &dogfoodRecorder{store: historyStore, cancel: cancel, nodes: len(definition.Nodes)}
	engine := execution.NewEngine(executors, definitions, artifactStore, nil,
		execution.WithParallelism(4),
		execution.WithRunID(runID),
		execution.WithStateDir(paths.RunsDir()),
		execution.WithProjectContext(project.Context{Workspace: workspace}),
		execution.WithHumanGateway(dogfoodGateway{}),
		execution.WithRunRecorder(recorder),
	)
	type runResult struct {
		exec *execution.WorkflowExecution
		err  error
	}
	done := make(chan runResult, 1)
	go func() {
		exec, runErr := engine.Run(ctx, definition)
		done <- runResult{exec: exec, err: runErr}
	}()

	runs := make([]dogfoodRunPaths, 0, len(checks))
	for len(runs) < len(checks) {
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
			assertWithin(t, dataRoot, path)
		}
		for _, name := range []string{"stdout.log", "stderr.log"} {
			if _, err := os.Stat(filepath.Join(run.logsDir, name)); err != nil {
				t.Errorf("%s log %s: %v", run.definition, name, err)
			}
		}
	}

	for _, check := range checks {
		run := result.exec.Node(check.id).Current
		if run.Status != execution.StatusSucceeded || run.Outputs["result"].Kind != artifact.KindQualityCheckResult {
			t.Fatalf("node %q = %+v, want succeeded QualityCheckResult", check.id, run)
		}
		detail, err := historyStore.GetNodeRun(context.Background(), runID, check.id)
		if err != nil || detail == nil || len(detail.Rounds) != 1 || detail.Rounds[0].Outputs["result"].Kind != artifact.KindQualityCheckResult {
			t.Fatalf("history node %q = %+v, %v", check.id, detail, err)
		}
		body, err := artifactStore.Get(run.Outputs["result"])
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(body.Data)
		if err != nil {
			t.Fatal(err)
		}
		if err := check.decode(encoded); err != nil {
			t.Errorf("node %q result does not satisfy its schema: %v", check.id, err)
		}
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 2 || entries[0].Name() != "app.go" || entries[1].Name() != "go.mod" {
		t.Fatalf("Project Workspace contains Gum output: %v, %v", entries, err)
	}
}

type dogfoodRunPaths struct {
	definition    string
	workspace     string
	logsDir       string
	toolOutputDir string
}

type gatedFactory struct {
	node.ExecutorFactory
	started chan<- dogfoodRunPaths
	release <-chan struct{}
}

func (f gatedFactory) Create(config node.Config) (node.Node, error) {
	actual, err := f.ExecutorFactory.Create(config)
	if err != nil {
		return nil, err
	}
	return gatedNode{definition: f.Definition(), actual: actual, started: f.started, release: f.release}, nil
}

type gatedNode struct {
	definition string
	actual     node.Node
	started    chan<- dogfoodRunPaths
	release    <-chan struct{}
}

func (n gatedNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	n.started <- dogfoodRunPaths{definition: n.definition, workspace: ctx.Project.Workspace, logsDir: ctx.Run.LogsDir, toolOutputDir: ctx.Run.ToolOutputDir}
	select {
	case <-n.release:
		return n.actual.Execute(ctx, inputs)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type dogfoodGateway struct{}

func (dogfoodGateway) RequestRound(context.Context, execution.RoundRequest) (execution.RoundResponse, error) {
	return execution.RoundResponse{Content: "run code quality checks", Finished: true}, nil
}

type dogfoodRecorder struct {
	store  *history.Store
	cancel context.CancelFunc
	nodes  int
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
	if succeeded == r.nodes {
		r.once.Do(r.cancel)
	}
	return nil
}

func installDogfoodGo(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  version)
    printf 'go version go1.25.0 darwin/arm64\n'
    ;;
  env)
    if [ "$2" = CC ]; then
      printf 'fake-cc\n'
	elif [ "$#" -eq 5 ]; then
	  printf 'darwin\narm64\n1\nfake-cc\n'
    else
      printf 'go1.25.0\n/go\ndarwin\narm64\n1\n'
    fi
    ;;
  list)
    printf 'example.com/dogfood\n'
    ;;
  vet)
    ;;
  test)
    case " $* " in
      *' -race '*)
        printf '%s\n' '{"Action":"pass","Package":"example.com/dogfood"}'
        ;;
      *)
        profile=
        for argument in "$@"; do
          case "$argument" in -coverprofile=*) profile=${argument#-coverprofile=} ;; esac
        done
        printf 'mode: atomic\napp.go:1.1,3.2 1 1\n' > "$profile"
        printf '%s\n' '{"Action":"pass","Package":"example.com/dogfood"}'
        ;;
    esac
    ;;
  run)
    for argument in "$@"; do destination=$argument; done
    printf '%s\n' '{"apiVersion":"goComplexityAnalyzer/v1","functions":[{"file":"app.go","line":3,"name":"Ready","complexity":1,"test":false,"generated":false,"vendor":false}],"syntaxErrors":[]}' > "$destination"
    ;;
  *)
    printf 'unexpected go command: %s\n' "$1" >&2
    exit 2
    ;;
esac
`
	path := filepath.Join(bin, "go")
	writeDogfoodFile(t, path, script)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	compiler := filepath.Join(bin, "fake-cc")
	writeDogfoodFile(t, compiler, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(compiler, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeDogfoodFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertWithin(t *testing.T, root, path string) {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatalf("path %q is outside Local Data Root %q", path, root)
	}
}
