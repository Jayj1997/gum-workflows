// Package builtins 提供第一批 MVP Node（设计计划 §32）：
//
//	requirement-analysis  Mock，无输入，产出 RequirementSpec
//	architecture-design   Mock，RequirementSpec -> ArchitectureSpec
//	coding-agent          MockCodingAgent（§33：先跑通 Runtime 再接真实 Agent）
//	openapi-generator     Mock 版 Automation（真实生成器属后续里程碑接入 CLI）
//
// Node 通过 Registry 显式注册（禁止 init() 隐式注册，见 docs/DEVELOPMENT.md §3），
// 由 cmd/workflow 在启动时集中完成。
package builtins

import (
	"fmt"
	"os"

	"github.com/Jayj1997/gum-workflows/internal/agent"
	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// RegisterAll 把全部内置 Node 注册到 registry（幂等性由 Registry 保证：重复注册报错）。
func RegisterAll(registry *node.Registry) error {
	factories := []node.Factory{
		requirementFactory{},
		architectureFactory{},
		newCodingAgentFactory(agent.NewMockCodingAgent()),
		openapiGeneratorFactory{},
	}
	for _, f := range factories {
		if err := registry.Register(f); err != nil {
			return fmt.Errorf("register builtins: %w", err)
		}
	}
	return nil
}

// ---- requirement-analysis（§32.1）----

type requirementFactory struct{}

func (requirementFactory) Type() string { return "requirement-analysis" }

func (requirementFactory) Create(config node.Config) (node.Node, error) {
	return requirementNode{}, nil
}

type requirementNode struct{}

func (requirementNode) Type() string             { return "requirement-analysis" }
func (requirementNode) InputSchema() node.Schema { return node.Schema{} }

func (requirementNode) OutputSchema() node.Schema {
	return node.Schema{Outputs: map[string]artifact.Kind{"requirement": artifact.KindRequirementSpec}}
}

func (requirementNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return putSingle(ctx, "requirement", artifact.KindRequirementSpec, "用户需要一个全栈订单系统。")
}

// ---- architecture-design（§32.2）----

type architectureFactory struct{}

func (architectureFactory) Type() string { return "architecture-design" }

func (architectureFactory) Create(config node.Config) (node.Node, error) {
	return architectureNode{}, nil
}

type architectureNode struct{}

func (architectureNode) Type() string { return "architecture-design" }

func (architectureNode) InputSchema() node.Schema {
	return node.Schema{Inputs: map[string]artifact.Kind{"requirement": artifact.KindRequirementSpec}}
}

func (architectureNode) OutputSchema() node.Schema {
	return node.Schema{Outputs: map[string]artifact.Kind{"architecture": artifact.KindArchitectureSpec}}
}

func (architectureNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return putSingle(ctx, "architecture", artifact.KindArchitectureSpec, "三层架构：API + Service + Repository。")
}

// ---- coding-agent（§32.3，Mock 版）----

type codingAgentFactory struct {
	agent agent.CodingAgent
}

func newCodingAgentFactory(a agent.CodingAgent) codingAgentFactory {
	return codingAgentFactory{agent: a}
}

func (codingAgentFactory) Type() string { return "coding-agent" }

func (f codingAgentFactory) Create(config node.Config) (node.Node, error) {
	prompt := ""
	if p, ok := config["task"].(string); ok {
		prompt = p
	}
	return codingAgentNode{agent: f.agent, prompt: prompt}, nil
}

// codingAgentNode 持有 prompt；agent 实例在 Factory 创建时注入
// （Create 返回 Node 时绑定，保持 Node 接口无状态依赖）。
type codingAgentNode struct {
	agent  agent.CodingAgent
	prompt string
}

func (n codingAgentNode) Type() string { return "coding-agent" }

// InputSchema：任意定义好的 Task Artifact（§32.3），全部可选。
func (n codingAgentNode) InputSchema() node.Schema {
	return node.Schema{OptionalInputs: map[string]artifact.Kind{
		"requirement":  artifact.KindRequirementSpec,
		"architecture": artifact.KindArchitectureSpec,
		"openapi":      artifact.KindOpenAPI,
		"frontend-sdk": artifact.KindFrontendSDK,
	}}
}

// OutputSchema：SourceCode + OpenAPI（§32.3）。
// Mock 版产出策略：source-code 恒产出；openapi 仅在输入含
// ArchitectureSpec（后端语义）时由 Mock Agent 补产（见 mock.go）。
func (n codingAgentNode) OutputSchema() node.Schema {
	return node.Schema{Outputs: map[string]artifact.Kind{
		"source-code": artifact.KindSourceCode,
		"openapi":     artifact.KindOpenAPI,
	}}
}

func (n codingAgentNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	if n.agent == nil {
		return nil, fmt.Errorf("coding-agent: no agent configured")
	}

	var refs []artifact.ArtifactRef
	for _, name := range []string{"requirement", "architecture", "openapi", "frontend-sdk"} {
		if ref, ok := inputs[name]; ok {
			refs = append(refs, ref)
		}
	}

	// Agent 在 Project Workspace 内执行（§19）。
	agentCtx := ctx.Context
	produced, err := n.agent.Execute(agentCtx, agent.Task{Prompt: n.prompt}, ctx.Project, refs)
	if err != nil {
		return nil, fmt.Errorf("coding-agent: %w", err)
	}

	outputs := make(map[string]artifact.ArtifactRef)
	for _, ref := range produced {
		switch ref.Kind {
		case artifact.KindSourceCode:
			outputs["source-code"] = ref
		case artifact.KindOpenAPI:
			// Agent 返回的 URI 指向 Workspace 文件（外部引用）；
			// 重新写入 Store，使下游通过 Store 读取（Artifact 是唯一数据通道，§13）。
			content, err := os.ReadFile(ref.URI)
			if err != nil {
				return nil, fmt.Errorf("coding-agent: read openapi %q: %w", ref.URI, err)
			}
			stored, err := ctx.Store.Put(artifact.Artifact{
				ID: "openapi", Kind: artifact.KindOpenAPI, Version: "1", Data: string(content),
			})
			if err != nil {
				return nil, fmt.Errorf("coding-agent: put openapi: %w", err)
			}
			outputs["openapi"] = stored
		}
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("coding-agent: agent produced no recognizable artifacts")
	}
	return outputs, nil
}

// putSingle 向 Store 放入单个 Artifact 并返回输出映射。
func putSingle(ctx node.ExecutionContext, name string, kind artifact.Kind, data any) (map[string]artifact.ArtifactRef, error) {
	ref, err := ctx.Store.Put(artifact.Artifact{
		ID:      name,
		Kind:    kind,
		Version: "1",
		Data:    data,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: put output: %w", name, err)
	}
	return map[string]artifact.ArtifactRef{name: ref}, nil
}

// ---- openapi-generator（§32.4，Mock Automation）----

type openapiGeneratorFactory struct{}

func (openapiGeneratorFactory) Type() string { return "openapi-generator" }

func (openapiGeneratorFactory) Create(config node.Config) (node.Node, error) {
	return openapiGeneratorNode{}, nil
}

type openapiGeneratorNode struct{}

func (openapiGeneratorNode) Type() string { return "openapi-generator" }

func (openapiGeneratorNode) InputSchema() node.Schema {
	return node.Schema{Inputs: map[string]artifact.Kind{"openapi": artifact.KindOpenAPI}}
}

func (openapiGeneratorNode) OutputSchema() node.Schema {
	return node.Schema{Outputs: map[string]artifact.Kind{"frontend-sdk": artifact.KindFrontendSDK}}
}

// Execute 模拟生成 FrontendSDK：把 OpenAPI Artifact 数据作为 SDK 内容存入 Store。
func (openapiGeneratorNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	spec, err := ctx.Store.Get(inputs["openapi"])
	if err != nil {
		return nil, fmt.Errorf("openapi-generator: get openapi artifact: %w", err)
	}
	sdk := fmt.Sprintf("generated-sdk(%v)", spec.Data)
	ref, err := ctx.Store.Put(artifact.Artifact{
		ID:      "frontend-sdk",
		Kind:    artifact.KindFrontendSDK,
		Version: "1",
		Data:    sdk,
	})
	if err != nil {
		return nil, fmt.Errorf("openapi-generator: put frontend-sdk: %w", err)
	}
	return map[string]artifact.ArtifactRef{"frontend-sdk": ref}, nil
}
