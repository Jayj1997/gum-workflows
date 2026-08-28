package execution

import (
	"strings"
	"testing"
)

func TestCanTransitionTo(t *testing.T) {
	tests := []struct {
		from, to Status
		want     bool
	}{
		{StatusPending, StatusReady, true},
		{StatusPending, StatusSkipped, true},
		{StatusPending, StatusRunning, false},
		{StatusPending, StatusSucceeded, false},
		{StatusReady, StatusRunning, true},
		{StatusReady, StatusWaitingHuman, true},
		{StatusReady, StatusSkipped, true},
		{StatusReady, StatusPending, false},
		{StatusRunning, StatusSucceeded, true},
		{StatusRunning, StatusFailed, true},
		{StatusRunning, StatusReady, false},
		{StatusWaitingHuman, StatusRunning, true},
		{StatusWaitingHuman, StatusSucceeded, false},
		{StatusSucceeded, StatusReady, true},
		{StatusSucceeded, StatusFailed, false},
		{StatusFailed, StatusReady, true},
		{StatusFailed, StatusRunning, false},
		{StatusSkipped, StatusReady, false},
		// 未定义状态
		{Status(""), StatusPending, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if got := CanTransitionTo(tt.from, tt.to); got != tt.want {
				t.Fatalf("CanTransitionTo(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTerminal(t *testing.T) {
	terminals := []Status{StatusSkipped, StatusStopped}
	nonTerminals := []Status{StatusPending, StatusReady, StatusWaitingHuman, StatusRunning, StatusSucceeded, StatusFailed}

	for _, s := range terminals {
		if !Terminal(s) {
			t.Errorf("Terminal(%s) = false, want true", s)
		}
	}
	for _, s := range nonTerminals {
		if Terminal(s) {
			t.Errorf("Terminal(%s) = true, want false", s)
		}
	}
}

func TestNodeExecutionStartsANewRunWithoutLosingHistory(t *testing.T) {
	n := NodeExecution{
		NodeID:  "backend",
		Current: NodeRun{RunID: "run-1", Round: 1, Status: StatusSucceeded},
	}

	if err := n.StartRun("run-2", map[string]InputSnapshot{}); err != nil {
		t.Fatalf("StartRun() unexpected error: %v", err)
	}
	if n.Current.RunID != "run-2" || n.Current.Round != 2 || n.Current.Status != StatusRunning {
		t.Fatalf("current = %+v", n.Current)
	}
	if len(n.History) != 1 || n.History[0].RunID != "run-1" || n.History[0].Status != StatusSucceeded {
		t.Fatalf("history = %+v", n.History)
	}
}

func TestNodeExecutionRetainsRecorderAllocatedIDForFirstRound(t *testing.T) {
	n := NodeExecution{
		NodeID:  "backend",
		Current: NodeRun{RunID: "allocated-while-pending", Status: StatusReady},
	}
	if err := n.StartRun("engine-generated", nil); err != nil {
		t.Fatalf("StartRun() unexpected error: %v", err)
	}
	if n.Current.RunID != "allocated-while-pending" || n.Current.Round != 1 {
		t.Fatalf("current identity = %q round %d, want allocated-while-pending round 1", n.Current.RunID, n.Current.Round)
	}
}

func TestNodeExecutionCannotStartNewRunAfterStructuralFailure(t *testing.T) {
	n := NodeExecution{
		NodeID:  "backend",
		Current: NodeRun{RunID: "run-1", Round: 1, Status: StatusFailed, ErrorKind: "structural"},
	}

	if err := n.StartRun("run-2", nil); err == nil {
		t.Fatal("StartRun() after structural failure = nil, want error")
	}
	if n.Current.RunID != "run-1" || len(n.History) != 0 {
		t.Fatalf("rejected StartRun() changed execution: %+v", n)
	}
}

func TestNodeExecutionTransitionTo(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Current: NodeRun{Status: StatusPending}}
		for _, next := range []Status{StatusReady, StatusRunning, StatusSucceeded} {
			if err := n.TransitionTo(next); err != nil {
				t.Fatalf("TransitionTo(%s) unexpected error: %v", next, err)
			}
		}
	})

	t.Run("human approval path", func(t *testing.T) {
		n := NodeExecution{NodeID: "review", Current: NodeRun{Status: StatusReady}}
		for _, next := range []Status{StatusWaitingHuman, StatusRunning, StatusSucceeded} {
			if err := n.TransitionTo(next); err != nil {
				t.Fatalf("TransitionTo(%s) unexpected error: %v", next, err)
			}
		}
	})

	t.Run("illegal transition returns error", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Current: NodeRun{Status: StatusPending}}
		if err := n.TransitionTo(StatusRunning); err == nil {
			t.Fatal("TransitionTo(Running) from Pending = nil, want error")
		}
		if n.Current.Status != StatusPending {
			t.Fatalf("illegal transition changed status to %s", n.Current.Status)
		}
	})

	t.Run("failed interaction may be revived", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Current: NodeRun{Status: StatusFailed, ErrorKind: "interaction"}}
		if err := n.TransitionTo(StatusReady); err != nil {
			t.Fatalf("TransitionTo(Ready) unexpected error: %v", err)
		}
	})

	t.Run("failed structural error may not be revived", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Current: NodeRun{Status: StatusFailed, ErrorKind: "structural"}}
		if err := n.TransitionTo(StatusReady); err == nil {
			t.Fatal("TransitionTo(Ready) from structural failure = nil, want error")
		}
	})

	t.Run("failed state rejects transitions other than ready", func(t *testing.T) {
		n := NodeExecution{Current: NodeRun{Status: StatusFailed}}
		for _, next := range []Status{StatusPending, StatusRunning, StatusSucceeded} {
			if err := n.TransitionTo(next); err == nil {
				t.Fatalf("TransitionTo(%s) from Failed = nil, want error", next)
			}
		}
	})

	t.Run("illegal transition error locates node", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Current: NodeRun{Status: StatusSucceeded}}
		err := n.TransitionTo(StatusRunning)
		if err == nil || !strings.Contains(err.Error(), `"backend"`) {
			t.Fatalf("TransitionTo() error = %v, want node ID in message", err)
		}
	})
}
