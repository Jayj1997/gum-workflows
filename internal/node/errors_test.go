package node

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorKindOf(t *testing.T) {
	cause := errors.New("model returned prose")
	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{name: "nil defaults structural", err: nil, want: ErrorKindStructural},
		{name: "plain defaults structural", err: cause, want: ErrorKindStructural},
		{name: "structural", err: Structural(cause), want: ErrorKindStructural},
		{name: "interaction", err: Interaction(cause), want: ErrorKindInteraction},
		{name: "classification survives wrapping", err: fmt.Errorf("execute agent: %w", Interaction(cause)), want: ErrorKindInteraction},
		{name: "classification survives joined errors", err: errors.Join(errors.New("context"), Interaction(cause)), want: ErrorKindInteraction},
		{name: "outer classification wins", err: Structural(Interaction(cause)), want: ErrorKindStructural},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorKindOf(tt.err); got != tt.want {
				t.Errorf("ErrorKindOf() = %q, want %q", got, tt.want)
			}
			if tt.err != nil && !errors.Is(tt.err, cause) {
				t.Errorf("wrapped error does not preserve cause: %v", tt.err)
			}
		})
	}
}

func TestErrorClassificationPreservesNil(t *testing.T) {
	if Structural(nil) != nil {
		t.Error("Structural(nil) must return nil")
	}
	if Interaction(nil) != nil {
		t.Error("Interaction(nil) must return nil")
	}
}
