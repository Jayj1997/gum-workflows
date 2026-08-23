package builtins

import (
	"context"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// registryWithBuiltins 返回已注册全部内置 Node 的 Registry，
// 并走一遍语义校验（内置 Node 的 Schema 必须自洽）。
func registryWithBuiltins(t *testing.T) *node.Registry {
	t.Helper()

	reg := node.NewRegistry()
	if err := RegisterAll(reg); err != nil {
		t.Fatalf("RegisterAll() unexpected error: %v", err)
	}
	return reg
}

// fullstackDef 与设计计划 §10/§42 的 Workflow 一致（零 dependsOn）。
func fullstackDef() workflow.Definition {
	return workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "fullstack-development"},
		Project:    workflow.ProjectSpec{Repository: "./project"},
		Nodes: map[string]workflow.NodeSpec{
			"requirement": {Type: "requirement-analysis"},
			"architecture": {
				Type:   "architecture-design",
				Inputs: map[string]workflow.InputBinding{"requirement": {From: "requirement.requirement"}},
			},
			"backend": {
				Type: "coding-agent",
				Config: map[string]any{
					"task": "实现后端",
				},
				Inputs: map[string]workflow.InputBinding{
					"requirement":  {From: "requirement.requirement"},
					"architecture": {From: "architecture.architecture"},
				},
			},
			"openapi": {
				Type:   "openapi-generator",
				Inputs: map[string]workflow.InputBinding{"openapi": {From: "backend.openapi"}},
			},
			"frontend": {
				Type:   "coding-agent",
				Config: map[string]any{"task": "实现前端"},
				Inputs: map[string]workflow.InputBinding{
					"requirement":  {From: "requirement.requirement"},
					"openapi":      {From: "backend.openapi"},
					"frontend-sdk": {From: "openapi.frontend-sdk"},
				},
			},
		},
	}
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
	reg := registryWithBuiltins(t)

	for _, typ := range []string{"requirement-analysis", "architecture-design", "coding-agent", "openapi-generator"} {
		if _, ok := reg.Get(typ); !ok {
			t.Errorf("type %q not registered", typ)
		}
	}
	// 重复注册报错。
	if err := RegisterAll(reg); err == nil {
		t.Error("duplicate RegisterAll() = nil error, want rejection")
	}
}

func TestRequirementNode(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	n, _ := requirementFactory{}.Create(nil)

	outputs, err := n.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if ref, ok := outputs["requirement"]; !ok || ref.Kind != artifact.KindRequirementSpec {
		t.Fatalf("outputs = %+v, want requirement/RequirementSpec", outputs)
	}
	if _, err := ctx.Store.Get(outputs["requirement"]); err != nil {
		t.Errorf("Get(): %v", err)
	}
}

func TestArchitectureNode(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())
	req, err := ctx.Store.Put(artifact.Artifact{ID: "requirement", Kind: artifact.KindRequirementSpec})
	if err != nil {
		t.Fatal(err)
	}

	n, _ := architectureFactory{}.Create(nil)
	outputs, err := n.Execute(ctx, map[string]artifact.ArtifactRef{"requirement": req})
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

	f := newCodingAgentFactory(newMockAgent())
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

	n, _ := openapiGeneratorFactory{}.Create(nil)
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

// fullstack 的 coding-agent 输出契约：backend 必须产出 openapi（否则 openapi
// 节点无法运行）。Mock Agent 只产 source-code，因此 Mock 版补产 openapi。
// 这里的策略：coding-agent 收到含 architecture 的输入时产出 openapi。
func TestCodingAgentProducesOpenAPIForBackend(t *testing.T) {
	ctx := newExecCtx(t, t.TempDir())

	f := newCodingAgentFactory(newMockAgent())
	n, _ := f.Create(node.Config{"task": "实现后端"})

	req, _ := ctx.Store.Put(artifact.Artifact{ID: "requirement", Kind: artifact.KindRequirementSpec})
	arch, _ := ctx.Store.Put(artifact.Artifact{ID: "architecture", Kind: artifact.KindArchitectureSpec})

	outputs, err := n.Execute(ctx, map[string]artifact.ArtifactRef{
		"requirement":  req,
		"architecture": arch,
	})
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
