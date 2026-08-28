package execution

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

type recordingRunRecorder struct {
	snapshots []*WorkflowExecution
	err       error
}

func (r *recordingRunRecorder) Record(exec *WorkflowExecution) error {
	data, err := json.Marshal(exec)
	if err != nil {
		return err
	}
	var snapshot WorkflowExecution
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	r.snapshots = append(r.snapshots, &snapshot)
	return r.err
}

func TestRunRecorderObservesInitialRoundAndTerminalSnapshots(t *testing.T) {
	recorder := &recordingRunRecorder{err: errors.New("database unavailable")}
	engine, _ := newChainEngine(t)
	engine.store = artifact.NewMemStore()
	WithRunRecorder(recorder)(engine)

	exec, err := runUntilStopped(t, engine, workflowWithOnlyCoder())
	if err != nil {
		t.Fatalf("Run() unexpected error when recorder fails: %v", err)
	}
	if exec.Status != StatusStopped {
		t.Fatalf("execution status = %s, want Stopped", exec.Status)
	}

	want := []struct {
		workflow Status
		node     Status
		round    int
	}{
		{StatusRunning, StatusPending, 0},
		{StatusRunning, StatusReady, 1},
		{StatusRunning, StatusRunning, 1},
		{StatusRunning, StatusSucceeded, 1},
		{StatusStopped, StatusSucceeded, 1},
	}
	if len(recorder.snapshots) != len(want) {
		t.Fatalf("Record() snapshots = %d, want %d", len(recorder.snapshots), len(want))
	}
	for i, expected := range want {
		got := recorder.snapshots[i]
		if got.Status != expected.workflow || got.Node("coder").Current.Status != expected.node || got.Node("coder").Current.Round != expected.round {
			t.Errorf("snapshot %d = workflow %s, node %s round %d; want %s/%s round %d",
				i, got.Status, got.Node("coder").Current.Status, got.Node("coder").Current.Round,
				expected.workflow, expected.node, expected.round)
		}
	}
}

func workflowWithOnlyCoder() workflow.Definition {
	def := chainDef()
	delete(def.Nodes, "sdk")
	return def
}
