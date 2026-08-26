package definition

import (
	"errors"
	"strings"
	"testing"
)

// newTestRegistry 建一个含三个 node type 与一个 definition 的注册表，
// 供 executor 相关测试复用。
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	for _, nt := range []NodeTypeDefinition{
		{APIVersion: NodeTypeAPIVersionV1, Kind: NodeTypeDefinitionKind,
			Metadata: Metadata{Name: string(TypeAgent), Description: "agent"}},
		{APIVersion: NodeTypeAPIVersionV1, Kind: NodeTypeDefinitionKind,
			Metadata: Metadata{Name: string(TypeAutomation), Description: "automation"}},
		{APIVersion: NodeTypeAPIVersionV1, Kind: NodeTypeDefinitionKind,
			Metadata: Metadata{Name: string(TypeHuman), Description: "human"}},
	} {
		if err := r.RegisterNodeType(nt); err != nil {
			t.Fatalf("register node type: %v", err)
		}
	}
	if err := r.RegisterDefinition(NodeDefinition{
		APIVersion: NodeDefinitionAPIVersionV1, Kind: NodeDefinitionKind,
		Metadata: Metadata{Name: "coding-agent", Description: "d"},
		Type:     TypeAgent,
	}); err != nil {
		t.Fatalf("register definition: %v", err)
	}
	return r
}

func TestRegistryNodeTypes(t *testing.T) {
	r := newTestRegistry(t)

	got, err := r.NodeType(string(TypeAgent))
	if err != nil {
		t.Fatalf("NodeType(agent) unexpected error: %v", err)
	}
	if got.Metadata.Name != string(TypeAgent) {
		t.Errorf("NodeType(agent) = %q, want agent", got.Metadata.Name)
	}
	if names := r.NodeTypeNames(); strings.Join(names, ",") != "agent,automation,human" {
		t.Errorf("NodeTypeNames() = %v, want [agent automation human]", names)
	}

	_, err = r.NodeType("robot")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("NodeType(robot) error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "robot") {
		t.Errorf("error %q should name the missing type", err)
	}
}

func TestRegistryDefinitionRequiresNodeType(t *testing.T) {
	r := NewRegistry() // 未注册任何 node type。

	err := r.RegisterDefinition(NodeDefinition{
		Metadata: Metadata{Name: "x", Description: "d"},
		Type:     TypeAgent,
	})
	if err == nil {
		t.Fatal("RegisterDefinition() = nil error, want unknown-node-type rejection")
	}
	if !strings.Contains(err.Error(), string(TypeAgent)) {
		t.Errorf("error %q should name the unknown type", err)
	}
}

func TestRegistryDefinitionDuplicate(t *testing.T) {
	r := newTestRegistry(t)
	d := NodeDefinition{Metadata: Metadata{Name: "coding-agent", Description: "d"}, Type: TypeAgent}
	if err := r.RegisterDefinition(d); err == nil {
		t.Fatal("RegisterDefinition() duplicate = nil error, want rejection")
	}
}

func TestRegistryExecutor(t *testing.T) {
	r := newTestRegistry(t)
	e1 := NodeExecutorDefinition{Metadata: Metadata{Name: "coding-agent-v1"},
		Node: "coding-agent", Version: "v1"}
	if err := r.RegisterExecutor(e1); err != nil {
		t.Fatalf("RegisterExecutor(v1) unexpected error: %v", err)
	}

	got, err := r.Executor("coding-agent", "v1")
	if err != nil {
		t.Fatalf("Executor(coding-agent, v1) unexpected error: %v", err)
	}
	if got.Version != "v1" {
		t.Errorf("Executor() version = %q, want v1", got.Version)
	}

	// 同定义第二版本并存；旧版本仍可寻址。
	e2 := NodeExecutorDefinition{Metadata: Metadata{Name: "coding-agent-v2"},
		Node: "coding-agent", Version: "v2"}
	if err := r.RegisterExecutor(e2); err != nil {
		t.Fatalf("RegisterExecutor(v2) unexpected error: %v", err)
	}
	if _, err := r.Executor("coding-agent", "v1"); err != nil {
		t.Fatalf("Executor(coding-agent, v1) after v2 = %v, want still addressable", err)
	}

	latest, err := r.Latest("coding-agent")
	if err != nil {
		t.Fatalf("Latest(coding-agent) unexpected error: %v", err)
	}
	if latest.Version != "v2" {
		t.Errorf("Latest() = %q, want v2", latest.Version)
	}

	// 版本按数字大小取最新：v10 > v9，不受字典序影响。
	e10 := NodeExecutorDefinition{Metadata: Metadata{Name: "coding-agent-v10"},
		Node: "coding-agent", Version: "v10"}
	if err := r.RegisterExecutor(e10); err != nil {
		t.Fatalf("RegisterExecutor(v10) unexpected error: %v", err)
	}
	latest, err = r.Latest("coding-agent")
	if err != nil {
		t.Fatalf("Latest(coding-agent) unexpected error: %v", err)
	}
	if latest.Version != "v10" {
		t.Errorf("Latest() = %q, want v10 (numeric compare)", latest.Version)
	}

	// (definition, version) 重复注册报错。
	if err := r.RegisterExecutor(e1); err == nil {
		t.Fatal("RegisterExecutor(duplicate) = nil error, want rejection")
	}
}

func TestRegistryExecutorUnknownDefinition(t *testing.T) {
	r := newTestRegistry(t)
	err := r.RegisterExecutor(NodeExecutorDefinition{
		Node: "no-such-definition", Version: "v1",
	})
	if err == nil {
		t.Fatal("RegisterExecutor() = nil error, want unknown-definition rejection")
	}
	if !strings.Contains(err.Error(), "no-such-definition") {
		t.Errorf("error %q should name the unknown definition", err)
	}
}

func TestRegistryExecutorNotFound(t *testing.T) {
	r := newTestRegistry(t)
	if err := r.RegisterExecutor(NodeExecutorDefinition{
		Node: "coding-agent", Version: "v1",
	}); err != nil {
		t.Fatalf("RegisterExecutor() unexpected error: %v", err)
	}

	_, err := r.Executor("coding-agent", "v9")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Executor(coding-agent, v9) error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "v9") {
		t.Errorf("error %q should name the missing version", err)
	}

	_, err = r.Latest("unknown-definition")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Latest(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestRegistryLatestNoExecutors(t *testing.T) {
	r := newTestRegistry(t)
	// definition 存在但没有任何 executor。
	_, err := r.Latest("coding-agent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Latest(no executors) error = %v, want ErrNotFound", err)
	}
}
