// Package builtins 提供第一批 MVP Node 的 Executor 实现（设计文档 §3.3、§12）：
//
//	human-input          由 execution.HumanGateway 驱动的内置入口
//	human-approval       由 execution.HumanGateway 驱动的内置审批
//
//	requirement-analysis  Mock，无输入，产出 rationality + analysis-output
//	architecture-design   Mock，analysis-output -> ArchitectureSpec
//	coding-agent          MockCodingAgent（§33：先跑通 Runtime 再接真实 Agent）
//	openapi-generator     Mock 版 Automation（真实生成器属后续里程碑接入 CLI）
//
// 契约唯一来源是种子 Node Definition YAML（internal/node/builtins/defs），
// Go 实现不再自带 Schema；RegisterAll 后必须经一致性检查
// （Go 注册集与 YAML 声明集双向 diff）方可投入使用。
package builtins

import (
	"fmt"
	"os"

	"github.com/Jayj1997/gum-workflows/internal/agent"
	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// RegisterAll 把全部内置 Executor 注册到 registry（幂等性由
// ExecutorRegistry 保证：重复注册报错）。
func RegisterAll(registry *node.ExecutorRegistry) error {
	factories := []node.ExecutorFactory{
		humanInputExecutor{},
		humanApprovalExecutor{},
		requirementExecutor{},
		architectureExecutor{},
		newCodingAgentExecutor(agent.NewMockCodingAgent()),
		openapiGeneratorExecutor{},
		staticAnalysisExecutor{},
		coverageExecutor{},
		raceExecutor{},
	}
	for _, f := range factories {
		if err := registry.Register(f); err != nil {
			return fmt.Errorf("register builtins: %w", err)
		}
	}
	return nil
}

// ---- human-approval（§7.2、§12）----

type humanApprovalExecutor struct{}

func (humanApprovalExecutor) Definition() string { return "human-approval" }
func (humanApprovalExecutor) Version() string    { return "v1" }

func (humanApprovalExecutor) Create(node.Config) (node.Node, error) {
	return humanApprovalNode{}, nil
}

type humanApprovalNode struct{}

func (humanApprovalNode) Execute(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, fmt.Errorf("human-approval must be executed through the execution human gateway")
}

func (humanApprovalNode) ExecuteHumanApproval(ctx node.ExecutionContext, approved bool, advise string) (map[string]artifact.ArtifactRef, error) {
	approveRef, err := putArtifact(ctx, "approve", artifact.Kind("bool"), approved)
	if err != nil {
		return nil, err
	}
	adviseRef, err := putArtifact(ctx, "advise", artifact.Kind("markdown"), advise)
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"approve": approveRef, "advise": adviseRef}, nil
}

// ---- human-input（§7.1、§12）----

type humanInputExecutor struct{}

func (humanInputExecutor) Definition() string { return "human-input" }
func (humanInputExecutor) Version() string    { return "v1" }

func (humanInputExecutor) Create(node.Config) (node.Node, error) {
	return humanInputNode{}, nil
}

type humanInputNode struct{}

func (humanInputNode) Execute(node.ExecutionContext, map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, fmt.Errorf("human-input must be executed through the execution human gateway")
}

func (humanInputNode) ExecuteHumanInput(ctx node.ExecutionContext, content string) (map[string]artifact.ArtifactRef, error) {
	ref, err := putArtifact(ctx, "requirement", artifact.Kind("markdown"), content)
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"requirement": ref}, nil
}

// ---- requirement-analysis（§12）----

type requirementExecutor struct{}

func (requirementExecutor) Definition() string { return "requirement-analysis" }
func (requirementExecutor) Version() string    { return "v1" }

func (requirementExecutor) Create(config node.Config) (node.Node, error) {
	return requirementNode{}, nil
}

type requirementNode struct{}

func (requirementNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	rationality, err := putArtifact(ctx, "rationality", artifact.Kind("int"), 7)
	if err != nil {
		return nil, err
	}
	analysis, err := putArtifact(ctx, "analysis-output", artifact.Kind("markdown"), "用户需要一个全栈订单系统。")
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{
		"rationality":     rationality,
		"analysis-output": analysis,
	}, nil
}

// ---- architecture-design（§12）----

type architectureExecutor struct{}

func (architectureExecutor) Definition() string { return "architecture-design" }
func (architectureExecutor) Version() string    { return "v1" }

func (architectureExecutor) Create(config node.Config) (node.Node, error) {
	return architectureNode{}, nil
}

type architectureNode struct{}

func (architectureNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	ref, err := putArtifact(ctx, "architecture", artifact.KindArchitectureSpec, "三层架构：API + Service + Repository。")
	if err != nil {
		return nil, err
	}
	return map[string]artifact.ArtifactRef{"architecture": ref}, nil
}

// ---- coding-agent（§12，Mock 版）----

type codingAgentExecutor struct {
	agent agent.CodingAgent
}

func newCodingAgentExecutor(a agent.CodingAgent) codingAgentExecutor {
	return codingAgentExecutor{agent: a}
}

func (codingAgentExecutor) Definition() string { return "coding-agent" }
func (codingAgentExecutor) Version() string    { return "v1" }

func (f codingAgentExecutor) Create(config node.Config) (node.Node, error) {
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

func (n codingAgentNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	if n.agent == nil {
		return nil, fmt.Errorf("coding-agent: no agent configured")
	}

	var refs []artifact.ArtifactRef
	for _, name := range []string{"analysis-output", "architecture", "openapi", "frontend-sdk", "advise"} {
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

	// Coding Agent 成功轮总会发布共享 Workspace 的 code 事件；版本由 Engine 分配。
	outputs := map[string]artifact.ArtifactRef{
		"code": {ID: "code", Kind: artifact.KindSourceCode, URI: ctx.Project.Workspace},
	}
	for _, ref := range produced {
		if ref.Kind != artifact.KindOpenAPI {
			continue
		}
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
	return outputs, nil
}

// putArtifact 向 Store 放入单个 Artifact 并返回其引用。
func putArtifact(ctx node.ExecutionContext, name string, kind artifact.Kind, data any) (artifact.ArtifactRef, error) {
	ref, err := ctx.Store.Put(artifact.Artifact{
		ID:      name,
		Kind:    kind,
		Version: "1",
		Data:    data,
	})
	if err != nil {
		return artifact.ArtifactRef{}, fmt.Errorf("%s: put output: %w", name, err)
	}
	return ref, nil
}

// ---- openapi-generator（§12，Mock Automation）----

type openapiGeneratorExecutor struct{}

func (openapiGeneratorExecutor) Definition() string { return "openapi-generator" }
func (openapiGeneratorExecutor) Version() string    { return "v1" }

func (openapiGeneratorExecutor) Create(config node.Config) (node.Node, error) {
	return openapiGeneratorNode{}, nil
}

type openapiGeneratorNode struct{}

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
