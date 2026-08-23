// Package node 定义 Workflow 的基本执行单位。
//
// Node 声明能力（Input/Output Contract + Executor），
// 由 Registry 按 type 实例化，可被多个 Workflow 复用。
package node

import (
	"context"
	"log/slog"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

// ExecutionContext 是 Node 执行时的上下文（设计计划 §30）。
// 嵌入 context.Context 以获得取消与超时能力；
// 携带项目环境、Artifact 存取与日志。后续版本扩展字段不需要改动 Node 接口签名。
type ExecutionContext struct {
	context.Context

	Project project.Context
	Store   artifact.Store
	Logger  *slog.Logger
}

// Schema 是 Node 的 Input/Output Contract：
// 输入/输出名称到 Artifact Kind 的映射。
// 例如 coding-agent 的 OutputSchema 为 {source-code: SourceCode, openapi: OpenAPI}。
//
// Inputs 中的输入全部是 required（设计计划 §8：Node 可执行要求所有 required
// Input 已存在，且语义校验要求它们全部被绑定）；OptionalInputs 仅声明可以绑定的
// 可选输入（如 coding-agent 的「任意定义好的 Task Artifact」），可以不绑定。
type Schema struct {
	Inputs         map[string]artifact.Kind
	OptionalInputs map[string]artifact.Kind
	Outputs        map[string]artifact.Kind
}

// Node 是整个 MVP 最核心的接口（设计计划 §30，输出契约已修正，见下）。
// 对 Runtime 来说 Agent/Automation/Human 三类 Node 没有区别。
type Node interface {
	// Type 返回 Node Type 名称，与 Registry 注册名一致（如 "coding-agent"）。
	Type() string

	// InputSchema 声明全部 required Input 的名称与 Artifact Kind。
	InputSchema() Schema

	// OutputSchema 声明可能产出的 Output 名称与 Artifact Kind。
	OutputSchema() Schema

	// Execute 执行 Node 逻辑，返回「输出名 -> ArtifactRef」映射。
	//
	// 输出契约是对设计计划 §30 的修正：计划原定返回 []ArtifactRef，
	// 但 Runtime 解析 from: "<node-id>.<output-name>" 需要输出名到引用的
	// 映射，裸列表无法建立（见 docs/domain-model.md 的决策记录）。
	Execute(ctx ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error)
}

// Config 是 Node 的实例级配置（YAML 中 nodes.<id>.config 的原始形态），
// 由各 Node Type 自行解码，Runtime 不解释其内容。
type Config map[string]any

// Factory 按 Node Type 创建 Node 实例（设计计划 §12）。
// Runtime 加载 YAML 后通过 Registry 找到对应 Factory 完成实例化。
type Factory interface {
	// Type 返回工厂负责的 Node Type 名称。
	Type() string

	// Create 依据 config 创建 Node 实例，config 内容非法时返回错误。
	Create(config Config) (Node, error)
}
