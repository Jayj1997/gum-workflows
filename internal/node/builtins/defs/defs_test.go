package defs

import (
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
)

// TestSeedsNodeTypesExactlyThree 验收项：Node Type 种子恰 3 个，
// agent requires [llm]，automation/human 无 requires。
func TestSeedsNodeTypesExactlyThree(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	names := r.NodeTypeNames()
	want := []string{"agent", "automation", "human"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("NodeTypeNames() = %v, want %v", names, want)
	}

	agent, err := r.NodeType("agent")
	if err != nil {
		t.Fatalf("NodeType(agent) unexpected error: %v", err)
	}
	if len(agent.Requires) != 1 || agent.Requires[0] != definition.RequireLLM {
		t.Errorf("agent requires = %v, want [llm]", agent.Requires)
	}
	for _, name := range []string{"automation", "human"} {
		nt, err := r.NodeType(name)
		if err != nil {
			t.Fatalf("NodeType(%s) unexpected error: %v", name, err)
		}
		if len(nt.Requires) != 0 {
			t.Errorf("%s requires = %v, want empty", name, nt.Requires)
		}
	}
}

// TestSeedsDefinitions 验收项：已落地节点按设计文档 §12 定稿契约，
// 各有 v1 执行器，Latest 可查。
func TestSeedsDefinitions(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	want := []string{"architecture-design", "coding-agent", "human-approval", "human-input", "openapi-generator", "requirement-analysis"}
	if got := r.DefinitionNames(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("DefinitionNames() = %v, want %v", got, want)
	}

	for _, name := range want {
		if _, err := r.Latest(name); err != nil {
			t.Errorf("Latest(%q) error: %v", name, err)
		}
		if versions := r.ExecutorVersions(name); len(versions) != 1 || versions[0] != "v1" {
			t.Errorf("ExecutorVersions(%q) = %v, want [v1]", name, versions)
		}
	}
}

// TestSeedsContracts 验证 §12 契约定稿内容：端口名、类型与 optional。
func TestSeedsContracts(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	assertPort := func(t *testing.T, ports map[string]definition.InputPort, node, name, wantType string, wantOptional bool) {
		t.Helper()
		port, ok := ports[name]
		if !ok {
			t.Fatalf("node %q: input %q not declared", node, name)
		}
		if port.Type != wantType {
			t.Errorf("node %q input %q type = %q, want %q", node, name, port.Type, wantType)
		}
		if port.Optional != wantOptional {
			t.Errorf("node %q input %q optional = %v, want %v", node, name, port.Optional, wantOptional)
		}
	}

	ra, _ := r.Definition("requirement-analysis")
	// requirement-analysis 按 §12 定稿契约接收 requirement: markdown
	// （human-input 入口产出一轮需求文本；T09 落地入口节点）。
	assertPort(t, ra.Inputs, "requirement-analysis", "requirement", "markdown", false)
	if got := ra.Outputs["rationality"].Type; got != "int" {
		t.Errorf("requirement-analysis rationality type = %q, want int", got)
	}
	if got := ra.Outputs["analysis-output"].Type; got != "markdown" {
		t.Errorf("requirement-analysis analysis-output type = %q, want markdown", got)
	}

	hi, _ := r.Definition("human-input")
	if len(hi.Inputs) != 0 {
		t.Errorf("human-input inputs = %+v, want none", hi.Inputs)
	}
	if got := hi.Outputs["requirement"].Type; got != "markdown" {
		t.Errorf("human-input requirement type = %q, want markdown", got)
	}

	ha, _ := r.Definition("human-approval")
	if len(ha.Inputs) != 0 {
		t.Errorf("human-approval inputs = %+v, want none", ha.Inputs)
	}
	if got := ha.Outputs["approve"].Type; got != "bool" {
		t.Errorf("human-approval approve type = %q, want bool", got)
	}
	if got := ha.Outputs["advise"].Type; got != "markdown" {
		t.Errorf("human-approval advise type = %q, want markdown", got)
	}

	ad, _ := r.Definition("architecture-design")
	assertPort(t, ad.Inputs, "architecture-design", "analysis-output", "markdown", false)
	if got := ad.Outputs["architecture"].Type; got != "ArchitectureSpec" {
		t.Errorf("architecture-design architecture type = %q, want ArchitectureSpec", got)
	}

	ca, _ := r.Definition("coding-agent")
	// 全 optional 输入，advise 通过审批回环驱动重跑。
	assertPort(t, ca.Inputs, "coding-agent", "analysis-output", "markdown", true)
	assertPort(t, ca.Inputs, "coding-agent", "architecture", "ArchitectureSpec", true)
	assertPort(t, ca.Inputs, "coding-agent", "openapi", "OpenAPI", true)
	assertPort(t, ca.Inputs, "coding-agent", "frontend-sdk", "FrontendSDK", true)
	assertPort(t, ca.Inputs, "coding-agent", "advise", "markdown", true)
	if got := ca.Outputs["code"].Type; got != "SourceCode" {
		t.Errorf("coding-agent code type = %q, want SourceCode", got)
	}
	if got := ca.Outputs["openapi"].Type; got != "OpenAPI" {
		t.Errorf("coding-agent openapi type = %q, want OpenAPI", got)
	}

	og, _ := r.Definition("openapi-generator")
	assertPort(t, og.Inputs, "openapi-generator", "openapi", "OpenAPI", false)
	if got := og.Outputs["frontend-sdk"].Type; got != "FrontendSDK" {
		t.Errorf("openapi-generator frontend-sdk type = %q, want FrontendSDK", got)
	}
}

// TestSeedsTypeClassification 验证 §12 的 type 归类与 requires。
func TestSeedsTypeClassification(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	wantType := map[string]definition.NodeType{
		"human-input":          definition.TypeHuman,
		"human-approval":       definition.TypeHuman,
		"requirement-analysis": definition.TypeAgent,
		"architecture-design":  definition.TypeAgent,
		"coding-agent":         definition.TypeAgent,
		"openapi-generator":    definition.TypeAutomation,
	}
	for name, want := range wantType {
		d, err := r.Definition(name)
		if err != nil {
			t.Fatalf("Definition(%q) unexpected error: %v", name, err)
		}
		if d.Type != want {
			t.Errorf("node %q type = %q, want %q", name, d.Type, want)
		}
	}

	ca, _ := r.Definition("coding-agent")
	if len(ca.Requires) != 1 || ca.Requires[0] != definition.RequireProject {
		t.Errorf("coding-agent requires = %v, want [project]", ca.Requires)
	}
}

// TestSeedsKindsRegistered 验收项：契约中的语义 Kind 已注册
// （含 optional 端口）。经 ValidateKinds 全量校验，作为后续语义
// 校验接线前的守卫。
func TestSeedsKindsRegistered(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}

	kinds := artifact.NewRegistry()
	for _, name := range r.DefinitionNames() {
		d, err := r.Definition(name)
		if err != nil {
			t.Fatalf("Definition(%q) unexpected error: %v", name, err)
		}
		if err := d.ValidateKinds(kinds); err != nil {
			t.Errorf("node %q: ValidateKinds: %v", name, err)
		}
	}
}
