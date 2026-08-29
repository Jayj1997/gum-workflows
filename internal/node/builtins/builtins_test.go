package builtins

import (
	"context"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

// executorsWithBuiltins 返回已注册全部内置 Executor 的注册表。
func executorsWithBuiltins(t *testing.T) *node.ExecutorRegistry {
	t.Helper()

	reg := node.NewExecutorRegistry()
	if err := RegisterAll(reg); err != nil {
		t.Fatalf("RegisterAll() unexpected error: %v", err)
	}
	return reg
}

// newExecCtx 构造带 Workspace 的 ExecutionContext（Mock Agent 需要）。
func newExecCtx(t *testing.T, ws string) node.ExecutionContext {
	t.Helper()
	return node.ExecutionContext{
		Context: context.Background(),
		Project: project.Context{
			Repository: project.Repository{Path: "/repo"},
			Workspace:  ws,
		},
		Store:  artifact.NewMemStore(),
		Logger: nil,
	}
}

func TestRegisterAllRegistersMVPNodeTypes(t *testing.T) {
	reg := executorsWithBuiltins(t)

	for _, def := range []string{"human-input", "human-approval", "requirement-analysis", "architecture-design", "coding-agent", "openapi-generator", "go-static-analysis", "go-coverage-check", "go-race-check", "go-complexity-check"} {
		if _, err := reg.Get(def, "v1"); err != nil {
			t.Errorf("executor (%s, v1) not registered: %v", def, err)
		}
	}
	// 重复注册报错。
	if err := RegisterAll(reg); err == nil {
		t.Error("duplicate RegisterAll() = nil error, want rejection")
	}
}

func TestHumanApprovalExecutorProducesDecisionArtifacts(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	n, err := humanApprovalExecutor{}.Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	human, ok := n.(interface {
		ExecuteHumanApproval(node.ExecutionContext, bool, string) (map[string]artifact.ArtifactRef, error)
	})
	if !ok {
		t.Fatal("human-approval v1 does not implement the human approval execution seam")
	}
	outputs, err := human.ExecuteHumanApproval(ctx, false, "add tests")
	if err != nil {
		t.Fatalf("ExecuteHumanApproval() unexpected error: %v", err)
	}
	approve, err := ctx.Store.Get(outputs["approve"])
	if err != nil || approve.Kind != "bool" || approve.Data != false {
		t.Errorf("approve = %+v/%v", approve, err)
	}
	advise, err := ctx.Store.Get(outputs["advise"])
	if err != nil || advise.Kind != "markdown" || advise.Data != "add tests" {
		t.Errorf("advise = %+v/%v", advise, err)
	}
}

func TestRequirementNode(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	n, _ := requirementExecutor{}.Create(nil)

	outputs, err := n.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if ref, ok := outputs["rationality"]; !ok {
		t.Fatalf("outputs = %+v, want rationality", outputs)
	} else if _, err := ctx.Store.Get(ref); err != nil {
		t.Errorf("Get(rationality): %v", err)
	}
	if ref, ok := outputs["analysis-output"]; !ok {
		t.Fatalf("outputs = %+v, want analysis-output", outputs)
	} else if _, err := ctx.Store.Get(ref); err != nil {
		t.Errorf("Get(analysis-output): %v", err)
	}
}

func TestHumanInputExecutorProducesRequirement(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	n, err := humanInputExecutor{}.Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	human, ok := n.(interface {
		ExecuteHumanInput(node.ExecutionContext, string) (map[string]artifact.ArtifactRef, error)
	})
	if !ok {
		t.Fatal("human-input v1 does not implement the human input execution seam")
	}
	outputs, err := human.ExecuteHumanInput(ctx, "build an API")
	if err != nil {
		t.Fatalf("ExecuteHumanInput() unexpected error: %v", err)
	}
	requirement, err := ctx.Store.Get(outputs["requirement"])
	if err != nil || requirement.Kind != "markdown" || requirement.Data != "build an API" {
		t.Errorf("requirement = %+v/%v", requirement, err)
	}
}

func TestArchitectureNode(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	req, err := ctx.Store.Put(artifact.Artifact{ID: "analysis-output", Kind: artifact.KindArchitectureSpec})
	if err != nil {
		t.Fatal(err)
	}

	n, _ := architectureExecutor{}.Create(nil)
	outputs, err := n.Execute(ctx, map[string]artifact.ArtifactRef{"analysis-output": req})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if ref := outputs["architecture"]; ref.Kind != artifact.KindArchitectureSpec {
		t.Errorf("architecture kind = %q", ref.Kind)
	}
}

func TestCodingAgentNodeWritesTaskInWorkspace(t *testing.T) {
	ws := t.TempDir()
	ctx := newExecCtx(t, ws)

	f := newCodingAgentExecutor(newMockAgent())
	n, err := f.Create(node.Config{"task": "实现订单系统后端"})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := ctx.Store.Put(artifact.Artifact{ID: "requirement", Kind: artifact.KindRequirementSpec})
	inputs := map[string]artifact.ArtifactRef{"requirement": req}

	outputs, err := n.Execute(ctx, inputs)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if ref := outputs["code"]; ref.Kind != artifact.KindSourceCode {
		t.Errorf("code kind = %q", ref.Kind)
	}
	// Code Artifact 指向共享的 In-place Project Workspace，不是某个文件或源码副本。
	if outputs["code"].URI != ws {
		t.Errorf("code URI = %q, want workspace %q", outputs["code"].URI, ws)
	}
}

func TestCodingAgentPassesApprovalAdviseToAgent(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	capture := &capturingAgent{}
	n, err := newCodingAgentExecutor(capture).Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	advise, err := ctx.Store.Put(artifact.Artifact{ID: "advise", Kind: "markdown", Data: "add tests"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.Execute(ctx, map[string]artifact.ArtifactRef{"advise": advise}); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if len(capture.inputs) != 1 || capture.inputs[0].URI != advise.URI {
		t.Fatalf("agent inputs = %+v, want advise ref %+v", capture.inputs, advise)
	}
}

func TestOpenAPIGeneratorNode(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	spec, err := ctx.Store.Put(artifact.Artifact{
		ID: "openapi", Kind: artifact.KindOpenAPI, Data: "paths: /orders",
	})
	if err != nil {
		t.Fatal(err)
	}

	n, _ := openapiGeneratorExecutor{}.Create(nil)
	outputs, err := n.Execute(ctx, map[string]artifact.ArtifactRef{"openapi": spec})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if ref := outputs["frontend-sdk"]; ref.Kind != artifact.KindFrontendSDK {
		t.Errorf("frontend-sdk kind = %q", ref.Kind)
	}
	// SDK 内容派生自 OpenAPI 输入。
	sdk, err := ctx.Store.Get(outputs["frontend-sdk"])
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := sdk.Data.(string); !ok || !strings.Contains(s, "/orders") {
		t.Errorf("sdk data = %v, want derived from openapi", sdk.Data)
	}
}

// 最小链路的 coding-agent 输出契约：必须产出 openapi（否则 openapi-generator
// 节点无法运行）。Mock Agent 在源节点场景（无输入）即产出 code + openapi。
func TestCodingAgentProducesOpenAPIAsSource(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())

	f := newCodingAgentExecutor(newMockAgent())
	n, _ := f.Create(node.Config{"task": "实现后端"})

	outputs, err := n.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if _, ok := outputs["openapi"]; !ok {
		t.Errorf("outputs missing openapi: %+v", outputs)
	}
	if _, ok := outputs["code"]; !ok {
		t.Errorf("outputs missing code: %+v", outputs)
	}
}

func TestCodingAgentPublishesCodeAfterSuccessfulAdapterCall(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	n, err := newCodingAgentExecutor(openAPIOnlyAgent{}).Create(nil)
	if err != nil {
		t.Fatal(err)
	}

	outputs, err := n.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if got := outputs["code"]; got.Kind != artifact.KindSourceCode || got.URI != ctx.Project.Workspace {
		t.Errorf("code output = %+v, want SourceCode ref to %q", got, ctx.Project.Workspace)
	}
}
