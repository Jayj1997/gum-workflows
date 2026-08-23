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
		{StatusReady, StatusSkipped, true},
		{StatusReady, StatusPending, false},
		{StatusRunning, StatusSucceeded, true},
		{StatusRunning, StatusFailed, true},
		{StatusRunning, StatusReady, false},
		{StatusSucceeded, StatusFailed, false},
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
	terminals := []Status{StatusSucceeded, StatusFailed, StatusSkipped}
	nonTerminals := []Status{StatusPending, StatusReady, StatusRunning}

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

func TestNodeExecutionTransitionTo(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Status: StatusPending}
		for _, next := range []Status{StatusReady, StatusRunning, StatusSucceeded} {
			if err := n.TransitionTo(next); err != nil {
				t.Fatalf("TransitionTo(%s) unexpected error: %v", next, err)
			}
		}
	})

	t.Run("illegal transition returns error", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Status: StatusPending}
		if err := n.TransitionTo(StatusRunning); err == nil {
			t.Fatal("TransitionTo(Running) from Pending = nil, want error")
		}
		if n.Status != StatusPending {
			t.Fatalf("illegal transition changed status to %s", n.Status)
		}
	})

	t.Run("terminal state is final", func(t *testing.T) {
		n := NodeExecution{Status: StatusFailed}
		for _, next := range []Status{StatusPending, StatusReady, StatusRunning, StatusSucceeded} {
			if err := n.TransitionTo(next); err == nil {
				t.Fatalf("TransitionTo(%s) from Failed = nil, want error", next)
			}
		}
	})

	t.Run("illegal transition error locates node", func(t *testing.T) {
		n := NodeExecution{NodeID: "backend", Status: StatusSucceeded}
		err := n.TransitionTo(StatusRunning)
		if err == nil || !strings.Contains(err.Error(), `"backend"`) {
			t.Fatalf("TransitionTo() error = %v, want node ID in message", err)
		}
	})
}
