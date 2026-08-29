// Package node 定义 Workflow 的基本执行单位。
//
// Node 只声明执行逻辑（Execute）；契约（Input/Output Contract）
// 唯一来源是 Node Definition YAML（设计文档 §6.9），
// ExecutorRegistry 按 (definition, version) 实例化，可被多个 Workflow 复用。
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

	Project     project.Context
	Store       artifact.Store
	Logger      *slog.Logger
	Run         RunContext
	Diagnostics *RunDiagnostics
}

// RunContext identifies the Local Data Root locations owned by one Node Run.
type RunContext struct {
	WorkflowRunID string
	NodeRunID     string
	LogsDir       string
	ToolOutputDir string
}

// HostDiagnostics records the operating system that launched an automation.
type HostDiagnostics struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

// ToolchainDiagnostics records the non-sensitive Go toolchain facts used by an automation.
type ToolchainDiagnostics struct {
	Tool            string `json:"tool,omitempty"`
	LauncherVersion string `json:"launcherVersion,omitempty"`
	FinalVersion    string `json:"finalVersion,omitempty"`
	GOROOT          string `json:"goroot,omitempty"`
	GOOS            string `json:"goos,omitempty"`
	GOARCH          string `json:"goarch,omitempty"`
	CGOEnabled      string `json:"cgoEnabled,omitempty"`
}

// RunDiagnostics records non-sensitive facts needed to explain host script execution.
type RunDiagnostics struct {
	BundleDigest  string                          `json:"bundleDigest,omitempty"`
	Host          *HostDiagnostics                `json:"host,omitempty"`
	CWD           string                          `json:"cwd,omitempty"`
	Arguments     []string                        `json:"arguments,omitempty"`
	Launcher      string                          `json:"launcher,omitempty"`
	Executables   map[string]string               `json:"executables,omitempty"`
	Toolchain     *ToolchainDiagnostics           `json:"toolchain,omitempty"`
	ResultAdapter string                          `json:"resultAdapter,omitempty"`
	Logs          map[string]artifact.ArtifactRef `json:"logs,omitempty"`
}

// Node 是整个 MVP 最核心的接口（设计计划 §30，设计文档 §6.9 瘦身版）。
// 对 Runtime 来说 Agent/Automation/Human 三类 Node 没有区别。
// 契约（inputs/outputs）不再由 Go 实现声明：唯一来源是
// Node Definition YAML，Engine/校验器从 definition Registry 读取。
type Node interface {
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

// ExecutorFactory 按 (Node Definition, version) 创建 Node 实例
// （设计文档 §6.9）：契约（inputs/outputs）唯一来源是 Node Definition
// YAML，Go 实现不再自带 Schema。同一定义多版本并存。
type ExecutorFactory interface {
	// Definition 返回所属 Node Definition 的 name（如 "coding-agent"）。
	Definition() string

	// Version 返回该执行器的版本（如 "v1"）。
	Version() string

	// Create 依据 config 创建 Node 实例，config 内容非法时返回错误。
	Create(config Config) (Node, error)
}

// ExecutorValidator lets immutable executors verify packaged assets during startup assembly.
type ExecutorValidator interface {
	ValidateExecutor() error
}

// HostRequirementValidator lets semantic validation diagnose unsupported hosts before Run.
type HostRequirementValidator interface {
	ValidateHostRequirements() error
}
