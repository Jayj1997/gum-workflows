package node_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// stubExecutorFactory 是测试用 Factory：按 (definition, version) 寻址。
type stubExecutorFactory struct {
	definition string
	version    string
}

func (f stubExecutorFactory) Definition() string { return f.definition }
func (f stubExecutorFactory) Version() string    { return f.version }
func (f stubExecutorFactory) Create(config node.Config) (node.Node, error) {
	return stubNode{}, nil
}

type stubNode struct{}

func (stubNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, nil
}

func TestExecutorRegistryRegisterAndGet(t *testing.T) {
	reg := node.NewExecutorRegistry()
	if err := reg.Register(stubExecutorFactory{definition: "coding-agent", version: "v1"}); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	f, err := reg.Get("coding-agent", "v1")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if f.Definition() != "coding-agent" || f.Version() != "v1" {
		t.Fatalf("Get() = (%q, %q), want (coding-agent, v1)", f.Definition(), f.Version())
	}
}

func TestExecutorRegistryDuplicateRegistration(t *testing.T) {
	reg := node.NewExecutorRegistry()
	if err := reg.Register(stubExecutorFactory{definition: "coding-agent", version: "v1"}); err != nil {
		t.Fatalf("Register() #1 unexpected error: %v", err)
	}
	err := reg.Register(stubExecutorFactory{definition: "coding-agent", version: "v1"})
	if err == nil {
		t.Fatal("duplicate Register() = nil error, want rejection")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error %q should mention already registered", err)
	}
	// 同 definition 不同 version 合法（多版本并存）。
	if err := reg.Register(stubExecutorFactory{definition: "coding-agent", version: "v2"}); err != nil {
		t.Fatalf("Register() v2 unexpected error: %v", err)
	}
}

func TestExecutorRegistryLatest(t *testing.T) {
	reg := node.NewExecutorRegistry()
	for _, v := range []string{"v2", "v9", "v10"} {
		if err := reg.Register(stubExecutorFactory{definition: "coding-agent", version: v}); err != nil {
			t.Fatalf("Register(%s) unexpected error: %v", v, err)
		}
	}
	f, err := reg.Latest("coding-agent")
	if err != nil {
		t.Fatalf("Latest() unexpected error: %v", err)
	}
	if f.Version() != "v10" {
		t.Fatalf("Latest() version = %q, want v10 (numeric compare, not lexicographic)", f.Version())
	}
}

func TestExecutorRegistryLatestNotFound(t *testing.T) {
	reg := node.NewExecutorRegistry()
	_, err := reg.Latest("nonexistent")
	if !errors.Is(err, node.ErrExecutorNotFound) {
		t.Fatalf("Latest(unknown) error = %v, want ErrExecutorNotFound", err)
	}
}

func TestExecutorRegistryGetNotFound(t *testing.T) {
	reg := node.NewExecutorRegistry()
	if err := reg.Register(stubExecutorFactory{definition: "coding-agent", version: "v1"}); err != nil {
		t.Fatal(err)
	}
	_, err := reg.Get("coding-agent", "v2")
	if !errors.Is(err, node.ErrExecutorNotFound) {
		t.Fatalf("Get(unknown version) error = %v, want ErrExecutorNotFound", err)
	}
	_, err = reg.Get("nonexistent", "v1")
	if !errors.Is(err, node.ErrExecutorNotFound) {
		t.Fatalf("Get(unknown definition) error = %v, want ErrExecutorNotFound", err)
	}
}
