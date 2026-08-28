package execution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

type callbackNode func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error)

func (fn callbackNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return fn(ctx, inputs)
}

type versionedFactory struct {
	fnFactory
	version string
}

func (f versionedFactory) Version() string { return f.version }

func iterativeDefinition() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "iterative"},
		Nodes: map[string]workflow.NodeSpec{
			"source": {Node: "source"},
			"worker": {
				Node: "worker",
				Inputs: map[string]workflow.InputBinding{
					"seed":     {From: "source.seed"},
					"feedback": {From: "feedback.feedback"},
				},
			},
			"feedback": {
				Node:   "feedback",
				Inputs: map[string]workflow.InputBinding{"work": {From: "worker.work"}},
			},
			"leaf": {
				Node:   "leaf",
				Inputs: map[string]workflow.InputBinding{"work": {From: "worker.work"}},
			},
		},
	}
}

func newIterativeEngine(t *testing.T, store artifact.Store, opts ...Option) *Engine {
	t.Helper()
	put := func(ctx node.ExecutionContext, name string) (map[string]artifact.ArtifactRef, error) {
		ref, err := ctx.Store.Put(artifact.Artifact{ID: name, Kind: "KindA"})
		return map[string]artifact.ArtifactRef{name: ref}, err
	}
	factories := []node.ExecutorFactory{
		fnFactory{definition: "source", outputs: map[string]definition.OutputPort{"seed": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "seed")
			}), nil
		}},
		fnFactory{definition: "worker", inputs: map[string]definition.InputPort{
			"seed": {Type: "KindA"}, "feedback": {Type: "KindA", Optional: true},
		}, outputs: map[string]definition.OutputPort{"work": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			var reusable artifact.ArtifactRef
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				if reusable.URI == "" {
					var err error
					reusable, err = ctx.Store.Put(artifact.Artifact{ID: "work", Kind: "KindA", Version: "1"})
					if err != nil {
						return nil, err
					}
				}
				return map[string]artifact.ArtifactRef{"work": reusable}, nil
			}), nil
		}},
		fnFactory{definition: "feedback", inputs: map[string]definition.InputPort{"work": {Type: "KindA"}}, outputs: map[string]definition.OutputPort{"feedback": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "feedback")
			}), nil
		}},
		fnFactory{definition: "leaf", inputs: map[string]definition.InputPort{"work": {Type: "KindA"}}, outputs: map[string]definition.OutputPort{"result": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "result")
			}), nil
		}},
	}
	dr, er := newTestRegistries(t, factories...)
	return NewEngine(er, dr, store, nil, opts...)
}

func TestRunIteratesWithLatestVersionsUntilConvergenceGuard(t *testing.T) {
	store := artifact.NewMemStore()
	e := newIterativeEngine(t, store, WithConvergenceLimit(3))

	exec, err := e.Run(context.Background(), iterativeDefinition())
	if err == nil || exec.Status != StatusFailed {
		t.Fatalf("Run() = status %s, error %v; want convergence failure", exec.Status, err)
	}
	worker := exec.Node("worker")
	if worker.Current.Round != 4 || worker.Current.Status != StatusFailed || worker.Current.Error != "convergence-guard" {
		t.Fatalf("worker current = %+v", worker.Current)
	}
	if len(worker.History) != 3 {
		t.Fatalf("worker history rounds = %d, want 3", len(worker.History))
	}
	outputURIs := map[string]bool{}
	for i, run := range worker.History {
		wantVersion := fmt.Sprint(i + 1)
		ref := run.Outputs["work"]
		if ref.Version != wantVersion || !store.Exists(ref) {
			t.Errorf("round %d output = %+v, want existing version %s", run.Round, ref, wantVersion)
		}
		if outputURIs[ref.URI] {
			t.Errorf("round %d reused prior output URI %q", run.Round, ref.URI)
		}
		outputURIs[ref.URI] = true
		stored, getErr := store.Get(ref)
		if getErr != nil || stored.Version != wantVersion {
			t.Errorf("round %d stored artifact = %+v, error %v", run.Round, stored, getErr)
		}
		if i > 0 && run.Inputs["feedback"].Ref.Version != fmt.Sprint(i) {
			t.Errorf("round %d feedback version = %q, want %d", run.Round, run.Inputs["feedback"].Ref.Version, i)
		}
	}
	leaf := exec.Node("leaf")
	if leaf.Current.Round != 3 {
		t.Fatalf("leaf rounds = %d, want cascade through all three worker versions", leaf.Current.Round)
	}
	for i, run := range append(append([]NodeRun{}, leaf.History...), leaf.Current) {
		if run.Inputs["work"].Ref.Version != fmt.Sprint(i+1) {
			t.Errorf("leaf round %d consumed work version %q", run.Round, run.Inputs["work"].Ref.Version)
		}
	}
}

func TestMultipleNewInputVersionsMergeIntoOneRound(t *testing.T) {
	put := func(ctx node.ExecutionContext, names ...string) (map[string]artifact.ArtifactRef, error) {
		outputs := make(map[string]artifact.ArtifactRef, len(names))
		for _, name := range names {
			ref, err := ctx.Store.Put(artifact.Artifact{ID: name, Kind: "KindA"})
			if err != nil {
				return nil, err
			}
			outputs[name] = ref
		}
		return outputs, nil
	}
	factories := []node.ExecutorFactory{
		fnFactory{definition: "source", outputs: map[string]definition.OutputPort{"seed": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "seed")
			}), nil
		}},
		fnFactory{definition: "worker", inputs: map[string]definition.InputPort{
			"seed": {Type: "KindA"}, "left": {Type: "KindA", Optional: true}, "right": {Type: "KindA", Optional: true},
		}, outputs: map[string]definition.OutputPort{"work": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "work")
			}), nil
		}},
		fnFactory{definition: "feedback", inputs: map[string]definition.InputPort{"work": {Type: "KindA"}}, outputs: map[string]definition.OutputPort{
			"left": {Type: "KindA"}, "right": {Type: "KindA"},
		}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "left", "right")
			}), nil
		}},
	}
	dr, er := newTestRegistries(t, factories...)
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "merge-rounds"},
		Nodes: map[string]workflow.NodeSpec{
			"source": {Node: "source"},
			"worker": {Node: "worker", Inputs: map[string]workflow.InputBinding{
				"seed": {From: "source.seed"}, "left": {From: "feedback.left"}, "right": {From: "feedback.right"},
			}},
			"feedback": {Node: "feedback", Inputs: map[string]workflow.InputBinding{"work": {From: "worker.work"}}},
		},
	}
	e := NewEngine(er, dr, artifact.NewMemStore(), nil, WithConvergenceLimit(3))

	exec, err := e.Run(context.Background(), def)
	if err == nil {
		t.Fatal("Run() = nil error, want convergence failure")
	}
	worker := exec.Node("worker")
	if worker.Current.Round != 4 || len(worker.History) != 3 {
		t.Fatalf("worker rounds = current %d/history %d, want 4/3", worker.Current.Round, len(worker.History))
	}
	for _, run := range worker.History[1:] {
		if run.Inputs["left"].Ref.Version != run.Inputs["right"].Ref.Version {
			t.Errorf("round %d did not merge matching input versions: %+v", run.Round, run.Inputs)
		}
	}
}

func TestNewVersionOnSameURITriggersDownstream(t *testing.T) {
	factories := []node.ExecutorFactory{
		fnFactory{definition: "source", outputs: map[string]definition.OutputPort{"seed": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "seed", Kind: "KindA"})
				return map[string]artifact.ArtifactRef{"seed": ref}, err
			}), nil
		}},
		fnFactory{definition: "worker", inputs: map[string]definition.InputPort{
			"seed": {Type: "KindA"}, "feedback": {Type: "KindA", Optional: true},
		}, outputs: map[string]definition.OutputPort{"work": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return map[string]artifact.ArtifactRef{"work": {ID: "work", Kind: "KindA", URI: "workspace://work"}}, nil
			}), nil
		}},
		fnFactory{definition: "feedback", inputs: map[string]definition.InputPort{"work": {Type: "KindA"}}, outputs: map[string]definition.OutputPort{"feedback": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "feedback", Kind: "KindA"})
				return map[string]artifact.ArtifactRef{"feedback": ref}, err
			}), nil
		}},
	}
	dr, er := newTestRegistries(t, factories...)
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "same-uri"},
		Nodes: map[string]workflow.NodeSpec{
			"source": {Node: "source"},
			"worker": {Node: "worker", Inputs: map[string]workflow.InputBinding{
				"seed": {From: "source.seed"}, "feedback": {From: "feedback.feedback"},
			}},
			"feedback": {Node: "feedback", Inputs: map[string]workflow.InputBinding{"work": {From: "worker.work"}}},
		},
	}
	exec, err := NewEngine(er, dr, artifact.NewMemStore(), nil, WithConvergenceLimit(3)).Run(context.Background(), def)
	if err == nil || exec.Status != StatusFailed {
		t.Fatalf("Run() = %s/%v, want convergence failure from same-URI versions", exec.Status, err)
	}
	if exec.Node("feedback").Current.Round != 3 {
		t.Fatalf("feedback rounds = %d, want 3", exec.Node("feedback").Current.Round)
	}
}

func TestDirtyInputQueuesWithoutConcurrentNodeRuns(t *testing.T) {
	var active, maxActive atomic.Int32
	var slowRuns, consumerRuns atomic.Int32
	releaseSlow := make(chan struct{})
	consumerStarted := make(chan struct{})
	releaseConsumer := make(chan struct{})
	put := func(ctx node.ExecutionContext, name string) (map[string]artifact.ArtifactRef, error) {
		ref, err := ctx.Store.Put(artifact.Artifact{ID: name, Kind: "KindA"})
		return map[string]artifact.ArtifactRef{name: ref}, err
	}
	producer := func(name string, gateSecond bool) fnFactory {
		return fnFactory{definition: name, inputs: map[string]definition.InputPort{
			"seed": {Type: "KindA"}, "feedback": {Type: "KindA", Optional: true},
		}, outputs: map[string]definition.OutputPort{"out": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				if gateSecond && slowRuns.Add(1) == 2 {
					<-releaseSlow
				}
				return put(ctx, "out")
			}), nil
		}}
	}
	factories := []node.ExecutorFactory{
		fnFactory{definition: "source", outputs: map[string]definition.OutputPort{"seed": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return put(ctx, "seed")
			}), nil
		}},
		producer("fast", false),
		producer("slow", true),
		fnFactory{definition: "consumer", inputs: map[string]definition.InputPort{
			"fast": {Type: "KindA"}, "slow": {Type: "KindA"},
		}, outputs: map[string]definition.OutputPort{"feedback": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				current := active.Add(1)
				for {
					seen := maxActive.Load()
					if current <= seen || maxActive.CompareAndSwap(seen, current) {
						break
					}
				}
				defer active.Add(-1)
				if consumerRuns.Add(1) == 2 {
					close(consumerStarted)
					<-releaseConsumer
				}
				return put(ctx, "feedback")
			}), nil
		}},
	}
	dr, er := newTestRegistries(t, factories...)
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "queued-dirty"},
		Nodes: map[string]workflow.NodeSpec{
			"source": {Node: "source"},
			"fast": {Node: "fast", Inputs: map[string]workflow.InputBinding{
				"seed": {From: "source.seed"}, "feedback": {From: "consumer.feedback"},
			}},
			"slow": {Node: "slow", Inputs: map[string]workflow.InputBinding{
				"seed": {From: "source.seed"}, "feedback": {From: "consumer.feedback"},
			}},
			"consumer": {Node: "consumer", Inputs: map[string]workflow.InputBinding{
				"fast": {From: "fast.out"}, "slow": {From: "slow.out"},
			}},
		},
	}
	e := NewEngine(er, dr, artifact.NewMemStore(), nil, WithParallelism(4), WithConvergenceLimit(3))

	var exec *WorkflowExecution
	var err error
	done := make(chan struct{})
	go func() {
		exec, err = e.Run(context.Background(), def)
		close(done)
	}()
	<-consumerStarted
	close(releaseSlow)
	close(releaseConsumer)
	<-done
	if err == nil || exec.Status != StatusFailed {
		t.Fatalf("Run() = %s/%v, want convergence failure", exec.Status, err)
	}
	if maxActive.Load() != 1 {
		t.Fatalf("consumer max concurrent runs = %d, want 1", maxActive.Load())
	}
	consumer := exec.Node("consumer")
	if consumer.Current.Round < 3 {
		t.Fatalf("consumer rounds = %d, want queued rerun", consumer.Current.Round)
	}
}

func TestHumanEventResetsConvergenceProtection(t *testing.T) {
	events := make(chan struct{}, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var workerRuns atomic.Int32

	store := artifact.NewMemStore()
	e := newIterativeEngine(t, store, WithConvergenceLimit(2), WithHumanEvents(events))
	original, err := e.executors.Get("worker", "v1")
	if err != nil {
		t.Fatal(err)
	}
	_ = original

	// Replace the worker implementation before Run; its human-event hook resets the
	// guard after each completed rerun and cancels after five observable rounds.
	dr := e.defs
	er := node.NewExecutorRegistry()
	for _, name := range []string{"source", "feedback", "leaf"} {
		factory, getErr := e.executors.Get(name, "v1")
		if getErr != nil {
			t.Fatal(getErr)
		}
		if registerErr := er.Register(factory); registerErr != nil {
			t.Fatal(registerErr)
		}
	}
	if registerErr := er.Register(fnFactory{definition: "worker", inputs: map[string]definition.InputPort{
		"seed": {Type: "KindA"}, "feedback": {Type: "KindA", Optional: true},
	}, outputs: map[string]definition.OutputPort{"work": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
		return callbackNode(func(execCtx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
			round := workerRuns.Add(1)
			if round > 1 {
				events <- struct{}{}
			}
			ref, putErr := execCtx.Store.Put(artifact.Artifact{ID: "work", Kind: "KindA"})
			if round == 5 {
				cancel()
			}
			return map[string]artifact.ArtifactRef{"work": ref}, putErr
		}), nil
	}}); registerErr != nil {
		t.Fatal(registerErr)
	}
	e = NewEngine(er, dr, store, nil, WithConvergenceLimit(2), WithHumanEvents(events))

	exec, runErr := e.Run(ctx, iterativeDefinition())
	if runErr != nil {
		t.Fatalf("Run() unexpected error: %v", runErr)
	}
	if exec.Status != StatusStopped || workerRuns.Load() != 5 {
		t.Fatalf("status/runs = %s/%d, want Stopped/5", exec.Status, workerRuns.Load())
	}
}

func TestExecutorVersionIsFixedBeforeIterativeRoundsStart(t *testing.T) {
	store := artifact.NewMemStore()
	base := newIterativeEngine(t, store)
	er := node.NewExecutorRegistry()
	for _, name := range []string{"source", "feedback", "leaf"} {
		factory, err := base.executors.Get(name, "v1")
		if err != nil {
			t.Fatal(err)
		}
		if err := er.Register(factory); err != nil {
			t.Fatal(err)
		}
	}

	var v1Runs, v2Creates atomic.Int32
	v2 := versionedFactory{fnFactory: fnFactory{definition: "worker", inputs: map[string]definition.InputPort{
		"seed": {Type: "KindA"}, "feedback": {Type: "KindA", Optional: true},
	}, outputs: map[string]definition.OutputPort{"work": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
		v2Creates.Add(1)
		return nil, fmt.Errorf("v2 must not be instantiated during an active run")
	}}, version: "v2"}
	v1 := fnFactory{definition: "worker", inputs: map[string]definition.InputPort{
		"seed": {Type: "KindA"}, "feedback": {Type: "KindA", Optional: true},
	}, outputs: map[string]definition.OutputPort{"work": {Type: "KindA"}}, create: func(node.Config) (node.Node, error) {
		return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
			if v1Runs.Add(1) == 1 {
				if err := er.Register(v2); err != nil {
					return nil, err
				}
			}
			ref, err := ctx.Store.Put(artifact.Artifact{ID: "work", Kind: "KindA"})
			return map[string]artifact.ArtifactRef{"work": ref}, err
		}), nil
	}}
	if err := er.Register(v1); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(er, base.defs, store, nil, WithConvergenceLimit(2))

	exec, err := e.Run(context.Background(), iterativeDefinition())
	if err == nil || exec.Status != StatusFailed {
		t.Fatalf("Run() = %s/%v, want convergence failure", exec.Status, err)
	}
	if v1Runs.Load() != 2 || v2Creates.Load() != 0 {
		t.Fatalf("v1 runs/v2 creates = %d/%d, want 2/0", v1Runs.Load(), v2Creates.Load())
	}
}

func TestCancellationWhileNodeRunsStopsWorkflow(t *testing.T) {
	started := make(chan struct{})
	dr, er := newTestRegistries(t, fnFactory{
		definition: "blocking",
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}), nil
		},
	})
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1, Kind: workflow.KindWorkflow,
		Metadata: workflow.Metadata{Name: "cancel-running"},
		Nodes:    map[string]workflow.NodeSpec{"blocking": {Node: "blocking"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	e := NewEngine(er, dr, artifact.NewMemStore(), nil)
	var exec *WorkflowExecution
	var runErr error
	go func() {
		exec, runErr = e.Run(ctx, def)
		close(done)
	}()
	<-started
	cancel()
	<-done
	if runErr != nil || exec.Status != StatusStopped {
		t.Fatalf("Run() = %s/%v, want clean stop", exec.Status, runErr)
	}
}

func TestIterativeStatePersistsCurrentHistoryAndRoundDetails(t *testing.T) {
	root := t.TempDir()
	e := newIterativeEngine(t, artifact.NewMemStore(), WithConvergenceLimit(2), WithStateDir(root), WithExecutionID("execution-000008"))

	exec, err := e.Run(context.Background(), iterativeDefinition())
	if err == nil {
		t.Fatal("Run() = nil error, want convergence failure")
	}
	nodeDir := filepath.Join(root, exec.ID, "nodes", "worker")
	for _, relative := range []string{"state.json", "runs/1.json", "runs/2.json", "runs/3.json"} {
		if _, statErr := os.Stat(filepath.Join(nodeDir, relative)); statErr != nil {
			t.Errorf("missing %s: %v", relative, statErr)
		}
	}
	loaded, loadErr := LoadNodeState(filepath.Join(root, exec.ID), "worker")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if loaded.Current.Round != 3 || len(loaded.History) != 2 {
		t.Fatalf("loaded state = current %d, history %d", loaded.Current.Round, len(loaded.History))
	}
	if loaded.History[0].Inputs != nil || loaded.History[0].Outputs != nil {
		t.Fatalf("state history contains round details: %+v", loaded.History[0])
	}
}
