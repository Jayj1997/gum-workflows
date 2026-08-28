// Package execution defines the runtime state of one workflow run.
package execution

import (
	"fmt"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// Status is shared by workflow executions and node runs.
type Status string

const (
	StatusPending Status = "Pending"
	StatusReady   Status = "Ready"
	// StatusWaitingHuman marks a human node blocked on terminal or UI input.
	StatusWaitingHuman Status = "WaitingHuman"
	StatusRunning      Status = "Running"
	StatusSucceeded    Status = "Succeeded"
	StatusFailed       Status = "Failed"
	StatusSkipped      Status = "Skipped"
	StatusStopped      Status = "Stopped"
)

var transitions = map[Status][]Status{
	StatusPending:      {StatusReady, StatusSkipped},
	StatusReady:        {StatusRunning, StatusWaitingHuman, StatusSkipped},
	StatusWaitingHuman: {StatusRunning},
	StatusRunning:      {StatusSucceeded, StatusFailed},
	StatusSucceeded:    {StatusReady},
	StatusFailed:       {StatusReady},
	StatusSkipped:      {},
	StatusStopped:      {},
}

// CanTransitionTo reports whether from may transition to next.
func CanTransitionTo(from, next Status) bool {
	for _, candidate := range transitions[from] {
		if candidate == next {
			return true
		}
	}
	return false
}

// Terminal reports whether a status can never transition again.
func Terminal(status Status) bool { return len(transitions[status]) == 0 }

// InputSnapshot records both an input binding and the artifact version used by a node run.
type InputSnapshot struct {
	From string               `json:"from"`
	Ref  artifact.ArtifactRef `json:"ref"`
}

// NodeRun is one execution round of a node instance.
type NodeRun struct {
	RunID      string                          `json:"run_id"`
	Round      int                             `json:"round"`
	Status     Status                          `json:"status"`
	Inputs     map[string]InputSnapshot        `json:"inputs,omitempty"`
	Outputs    map[string]artifact.ArtifactRef `json:"outputs,omitempty"`
	Error      string                          `json:"error,omitempty"`
	ErrorKind  node.ErrorKind                  `json:"error_kind,omitempty"`
	StartedAt  time.Time                       `json:"started_at,omitempty"`
	FinishedAt time.Time                       `json:"finished_at,omitempty"`
}

// NodeExecution holds the current node run and all completed earlier rounds.
type NodeExecution struct {
	NodeID         string    `json:"node_id"`
	NodeDefinition string    `json:"node_definition"`
	Current        NodeRun   `json:"current"`
	History        []NodeRun `json:"history,omitempty"`

	dirty           bool
	machineRuns     int
	consumedControl map[string]int
	consumedInputs  map[string]artifact.ArtifactRef
	outputVersions  map[string]int
	humanClosed     bool
	approvalRounds  map[int]approvalDecision
	adviseRetry     *InputSnapshot
}

// TransitionTo moves the current round to next when the state machine permits it.
func (n *NodeExecution) TransitionTo(next Status) error {
	if !CanTransitionTo(n.Current.Status, next) {
		return fmt.Errorf("node execution %q: illegal transition %s -> %s", n.NodeID, n.Current.Status, next)
	}
	if n.Current.Status == StatusFailed && next == StatusReady && n.Current.ErrorKind != node.ErrorKindInteraction {
		return fmt.Errorf("node execution %q: only interaction failures may transition Failed -> Ready", n.NodeID)
	}
	n.Current.Status = next
	return nil
}

// StartRun archives a prior completed round and starts the next running round.
func (n *NodeExecution) StartRun(runID string, inputs map[string]InputSnapshot) error {
	return n.startRun(runID, inputs, StatusRunning)
}

// StartWaitingRun archives a prior completed round and starts a human round waiting for a response.
func (n *NodeExecution) StartWaitingRun(runID string, inputs map[string]InputSnapshot) error {
	return n.startRun(runID, inputs, StatusWaitingHuman)
}

func (n *NodeExecution) startRun(runID string, inputs map[string]InputSnapshot, next Status) error {
	previousRound := n.Current.Round
	if previousRound == 0 {
		if n.Current.Status != StatusReady {
			return fmt.Errorf("node execution %q: start run from %s", n.NodeID, n.Current.Status)
		}
	} else {
		previous := n.Current
		if err := n.TransitionTo(StatusReady); err != nil {
			return err
		}
		n.History = append(n.History, previous)
	}

	n.Current = NodeRun{
		RunID:     runID,
		Round:     previousRound + 1,
		Status:    StatusReady,
		Inputs:    inputs,
		StartedAt: time.Now().UTC(),
	}
	return n.TransitionTo(next)
}

// WorkflowExecution is one independent run of a workflow definition.
type WorkflowExecution struct {
	ID            string                    `json:"id"`
	RunID         string                    `json:"run_id"`
	Workflow      string                    `json:"workflow"`
	WorkflowFile  string                    `json:"workflow_file,omitempty"`
	Status        Status                    `json:"status"`
	StoppedReason string                    `json:"stopped_reason,omitempty"`
	Error         string                    `json:"error,omitempty"`
	StartedAt     time.Time                 `json:"started_at"`
	FinishedAt    time.Time                 `json:"finished_at,omitempty"`
	Nodes         map[string]*NodeExecution `json:"nodes"`
}

// Node returns a node execution by workflow node ID.
func (w *WorkflowExecution) Node(nodeID string) *NodeExecution { return w.Nodes[nodeID] }
