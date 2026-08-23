package node

import (
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

// fakeFactory / fakeNode 仅供 Registry 测试使用。
type fakeFactory struct {
	nodeType string
}

func (f fakeFactory) Type() string { return f.nodeType }

func (f fakeFactory) Create(config Config) (Node, error) {
	return fakeNode{nodeType: f.nodeType}, nil
}

type fakeNode struct {
	nodeType string
}

func (n fakeNode) Type() string         { return n.nodeType }
func (n fakeNode) InputSchema() Schema  { return Schema{} }
func (n fakeNode) OutputSchema() Schema { return Schema{} }
func (n fakeNode) Execute(ctx ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(fakeFactory{"coding-agent"}); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	if err := r.Register(fakeFactory{"openapi-generator"}); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	f, ok := r.Get("coding-agent")
	if !ok {
		t.Fatal("Get(coding-agent) not found")
	}
	if f.Type() != "coding-agent" {
		t.Fatalf("Get().Type() = %q", f.Type())
	}

	if _, ok := r.Get("unknown"); ok {
		t.Fatal("Get(unknown) found, want not found")
	}

	if got, want := strings.Join(r.Types(), ","), "coding-agent,openapi-generator"; got != want {
		t.Errorf("Types() = %q, want %q", got, want)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeFactory{"x"}); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	err := r.Register(fakeFactory{"x"})
	if err == nil {
		t.Fatal("duplicate Register() = nil error, want rejection")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error %q should mention already registered", err)
	}
}

func TestRegistryRejectsEmptyType(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeFactory{""}); err == nil {
		t.Fatal("Register(empty type) = nil error, want rejection")
	}
}
