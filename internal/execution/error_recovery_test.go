package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func TestInteractionFailureRetriesWithImmediateAdvise(t *testing.T) {
	store := artifact.NewMemStore()
	producer := fnFactory{
		definition: "advice-source",
		outputs:    map[string]definition.OutputPort{"advise": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "advice", Kind: "markdown", Data: "stale advice"})
				return map[string]artifact.ArtifactRef{"advise": ref}, err
			}), nil
		},
	}
	var runs atomic.Int32
	var retriedWith string
	agent := fnFactory{
		definition: "agent",
		inputs:     map[string]definition.InputPort{"advise": {Type: "markdown", Optional: true}},
		outputs:    map[string]definition.OutputPort{"result": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				if runs.Add(1) == 1 {
					return nil, node.Interaction(errors.New("expected JSON but received prose"))
				}
				advice, err := ctx.Store.Get(inputs["advise"])
				if err != nil {
					return nil, err
				}
				retriedWith, _ = advice.Data.(string)
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "result", Kind: "markdown", Data: "fixed"})
				return map[string]artifact.ArtifactRef{"result": ref}, err
			}), nil
		},
	}
	dr, er := newTestRegistries(t, producer, agent)
	gateway := &fakeHumanGateway{responses: []RoundResponse{{Advise: "return valid JSON"}}}
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "advise-retry"},
		Nodes: map[string]workflow.NodeSpec{
			"source": {Node: "advice-source"},
			"agent": {
				Node:      "agent",
				DependsOn: []string{"source"},
				Inputs: map[string]workflow.InputBinding{
					"advise": {From: "source.advise"},
				},
			},
		},
	}

	exec, err := runUntilStopped(t, NewEngine(er, dr, store, nil,
		WithHumanGateway(gateway), WithConvergenceLimit(1)), def)
	if err != nil || exec.Status != StatusStopped {
		t.Fatalf("Run() = %s/%v, want Stopped/nil", exec.Status, err)
	}
	if retriedWith != "return valid JSON" {
		t.Errorf("retry advise = %q, want immediate advise", retriedWith)
	}
	got := exec.Node("agent")
	if len(got.History) != 1 || got.History[0].Status != StatusFailed || got.History[0].ErrorKind != "interaction" {
		t.Fatalf("failed round = %+v, want interaction failure", got.History)
	}
	if got.Current.Round != 2 || got.Current.Status != StatusSucceeded {
		t.Fatalf("retry round = %+v, want round 2 Succeeded", got.Current)
	}
	if snapshot := got.Current.Inputs["advise"]; snapshot.From != "#advise-retry" {
		t.Errorf("retry advise snapshot = %+v, want #advise-retry", snapshot)
	}
	requests := gateway.Requests()
	if len(requests) != 1 || requests[0].Kind != RoundRequestAdviseRetry || requests[0].NodeID != "agent" {
		t.Fatalf("gateway requests = %+v, want one advise retry for agent", requests)
	}
}

func TestInteractionFailureWithoutAdvisePortIsStructural(t *testing.T) {
	agent := fnFactory{
		definition: "agent",
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return nil, node.Interaction(errors.New("invalid response"))
			}), nil
		},
	}
	dr, er := newTestRegistries(t, agent)
	exec, err := NewEngine(er, dr, artifact.NewMemStore(), nil).Run(context.Background(), workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "no-recovery-port"},
		Nodes:      map[string]workflow.NodeSpec{"agent": {Node: "agent"}},
	})
	if err == nil || exec.Status != StatusFailed {
		t.Fatalf("Run() = %s/%v, want Failed/error", exec.Status, err)
	}
	if got := exec.Node("agent").Current.ErrorKind; got != "structural" {
		t.Errorf("error kind = %q, want structural", got)
	}
}

func TestSkippedAdviseKeepsInteractionFailureWhileOtherBranchCompletes(t *testing.T) {
	failing := fnFactory{
		definition: "failing-agent",
		inputs:     map[string]definition.InputPort{"advise": {Type: "markdown", Optional: true}},
		outputs:    map[string]definition.OutputPort{"result": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				return nil, node.Interaction(errors.New("invalid response"))
			}), nil
		},
	}
	var downstreamRuns atomic.Int32
	downstream := fnFactory{
		definition: "downstream",
		inputs:     map[string]definition.InputPort{"result": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				downstreamRuns.Add(1)
				return nil, nil
			}), nil
		},
	}
	sibling := fnFactory{
		definition: "sibling",
		outputs:    map[string]definition.OutputPort{"result": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "result", Kind: "markdown", Data: "done"})
				return map[string]artifact.ArtifactRef{"result": ref}, err
			}), nil
		},
	}
	dr, er := newTestRegistries(t, failing, sibling, downstream)
	gateway := &fakeHumanGateway{responses: []RoundResponse{{Skip: true}}}
	exec, err := runUntilStopped(t, NewEngine(er, dr, artifact.NewMemStore(), nil,
		WithHumanGateway(gateway), WithParallelism(2)), workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "independent-branches"},
		Nodes: map[string]workflow.NodeSpec{
			"failed":  {Node: "failing-agent"},
			"sibling": {Node: "sibling"},
			"downstream": {Node: "downstream", Inputs: map[string]workflow.InputBinding{
				"result": {From: "failed.result"},
			}},
		},
	})
	if err != nil || exec.Status != StatusStopped {
		t.Fatalf("Run() = %s/%v, want Stopped/nil", exec.Status, err)
	}
	if got := exec.Node("failed").Current; got.Status != StatusFailed || got.ErrorKind != "interaction" {
		t.Errorf("failed branch = %+v, want interaction Failed", got)
	}
	if got := exec.Node("sibling").Current.Status; got != StatusSucceeded {
		t.Errorf("sibling status = %s, want Succeeded", got)
	}
	if got := exec.Node("downstream").Current.Status; got != StatusPending {
		t.Errorf("downstream status = %s, want Pending", got)
	}
	if got := downstreamRuns.Load(); got != 0 {
		t.Errorf("downstream runs = %d, want 0", got)
	}
}

func TestStructuralFailureWaitsForInflightRound(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	failing := fnFactory{
		definition: "a-failing",
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				<-started
				return nil, errors.New("network unavailable")
			}), nil
		},
	}
	slow := fnFactory{
		definition: "b-inflight",
		outputs:    map[string]definition.OutputPort{"result": {Type: "markdown"}},
		create: func(node.Config) (node.Node, error) {
			return callbackNode(func(ctx node.ExecutionContext, _ map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
				close(started)
				<-release
				ref, err := ctx.Store.Put(artifact.Artifact{ID: "result", Kind: "markdown", Data: "done"})
				return map[string]artifact.ArtifactRef{"result": ref}, err
			}), nil
		},
	}
	dr, er := newTestRegistries(t, failing, slow)
	done := make(chan struct{})
	var exec *WorkflowExecution
	var runErr error
	go func() {
		exec, runErr = NewEngine(er, dr, artifact.NewMemStore(), nil, WithParallelism(2)).Run(context.Background(), workflow.Definition{
			APIVersion: workflow.APIVersionV1,
			Kind:       workflow.KindWorkflow,
			Metadata:   workflow.Metadata{Name: "fail-fast"},
			Nodes: map[string]workflow.NodeSpec{
				"failure":  {Node: "a-failing"},
				"inflight": {Node: "b-inflight"},
			},
		})
		close(done)
	}()
	<-started
	close(release)
	<-done

	if runErr == nil || exec.Status != StatusFailed {
		t.Fatalf("Run() = %s/%v, want Failed/error", exec.Status, runErr)
	}
	if got := exec.Node("inflight").Current.Status; got != StatusSucceeded {
		t.Errorf("inflight status = %s, want Succeeded", got)
	}
}
