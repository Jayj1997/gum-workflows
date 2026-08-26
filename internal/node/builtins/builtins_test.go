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

	for _, def := range []string{"requirement-analysis", "architecture-design", "coding-agent", "openapi-generator"} {
		if _, err := reg.Get(def, "v1"); err != nil {
			t.Errorf("executor (%s, v1) not registered: %v", def, err)
		}
	}
	// 重复注册报错。
	if err := RegisterAll(reg); err == nil {
		t.Error("duplicate RegisterAll() = nil error, want rejection")
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
	if ref := outputs["source-code"]; ref.Kind != artifact.KindSourceCode {
		t.Errorf("source-code kind = %q", ref.Kind)
	}
	// Mock Agent 的产出 URI 指向 Workspace 内文件。
	if !strings.HasPrefix(outputs["source-code"].URI, ws) {
		t.Errorf("source-code URI = %q, want under workspace %q", outputs["source-code"].URI, ws)
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
// 节点无法运行）。Mock Agent 在源节点场景（无输入）即产出 source-code + openapi。
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
	if _, ok := outputs["source-code"]; !ok {
		t.Errorf("outputs missing source-code: %+v", outputs)
	}
}
