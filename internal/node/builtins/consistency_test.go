package builtins

import (
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
)

// newBuiltins 装配内置 Executor 与种子定义，并完成启动一致性检查
// （CLI 每次启动走的同一管线）。
func newBuiltins(t *testing.T) (*node.ExecutorRegistry, *definition.Registry) {
	t.Helper()

	executors := node.NewExecutorRegistry()
	if err := RegisterAll(executors); err != nil {
		t.Fatalf("RegisterAll() unexpected error: %v", err)
	}
	dr, err := defs.NewRegistry()
	if err != nil {
		t.Fatalf("defs.NewRegistry() unexpected error: %v", err)
	}
	if err := CheckConsistency(executors, dr); err != nil {
		t.Fatalf("CheckConsistency() unexpected error: %v", err)
	}
	return executors, dr
}

// TestRegisterAllRegistersExecutors 验证内置 Executor 按
// (definition, v1) 注册且 Latest 可解析。
func TestRegisterAllRegistersExecutors(t *testing.T) {
	executors, _ := newBuiltins(t)

	for _, def := range []string{"human-input", "human-approval", "requirement-analysis", "architecture-design", "coding-agent", "openapi-generator"} {
		f, err := executors.Get(def, "v1")
		if err != nil {
			t.Fatalf("Get(%s, v1) unexpected error: %v", def, err)
		}
		if f.Definition() != def || f.Version() != "v1" {
			t.Errorf("Get(%s, v1) = (%q, %q)", def, f.Definition(), f.Version())
		}
		if _, err := executors.Latest(def); err != nil {
			t.Errorf("Latest(%s) error: %v", def, err)
		}
	}
	// 重复注册报错。
	if err := RegisterAll(executors); err == nil {
		t.Error("duplicate RegisterAll() = nil error, want rejection")
	}
}

// TestConsistencyDetectsMissingYAML：Go 注册了种子未声明的 executor 时报错。
func TestConsistencyDetectsMissingYAML(t *testing.T) {
	orphan := node.NewExecutorRegistry()
	if err := orphan.Register(orphanFactory{}); err != nil {
		t.Fatal(err)
	}
	dr, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = CheckConsistency(orphan, dr)
	if err == nil {
		t.Fatal("CheckConsistency(orphan) = nil error, want missing-YAML rejection")
	}
	if !strings.Contains(err.Error(), "without YAML declaration") || !strings.Contains(err.Error(), "(ghost-node, v1)") {
		t.Errorf("error %q should name the missing declaration", err)
	}
}

// TestConsistencyDetectsMissingGo：种子声明了没有 Go 实现的 executor 时报错。
func TestConsistencyDetectsMissingGo(t *testing.T) {
	partial := node.NewExecutorRegistry()
	if err := partial.Register(stubFactory{definition: "requirement-analysis", version: "v1"}); err != nil {
		t.Fatal(err)
	}
	dr, err := defs.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = CheckConsistency(partial, dr)
	if err == nil {
		t.Fatal("CheckConsistency(partial) = nil error, want missing-Go rejection")
	}
	if !strings.Contains(err.Error(), "without Go executor") || !strings.Contains(err.Error(), "(architecture-design, v1)") {
		t.Errorf("error %q should list the missing Go executors", err)
	}
}

// TestConsistencyExposesYAMLContract：一致性检查通过时种子契约可查。
func TestConsistencyExposesYAMLContract(t *testing.T) {
	_, dr := newBuiltins(t)

	d, err := dr.Definition("coding-agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Outputs["code"]; !ok {
		t.Errorf("coding-agent outputs missing code: %+v", d.Outputs)
	}
	if d.Inputs["architecture"].Type != "ArchitectureSpec" {
		t.Errorf("coding-agent architecture input type = %q", d.Inputs["architecture"].Type)
	}
	if d.Inputs["analysis-output"].Type != "markdown" {
		t.Errorf("coding-agent analysis-output input type = %q", d.Inputs["analysis-output"].Type)
	}
}
