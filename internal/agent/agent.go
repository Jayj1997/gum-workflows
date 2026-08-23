// Package agent 定义 Coding Agent 的适配层（设计计划 §20）。
//
// Runtime 不直接绑定某个 Coding Agent：Workflow 只提供
// Project + Workspace + Task + Input Artifacts（§18），
// Agent 自行进入 Project Workspace 并发现 .agents/skills/、.claude/skills/
// 等项目约定。Workflow 与 Skill 完全解耦。
package agent

import (
	"context"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

// Task 是交给 Coding Agent 的任务描述（来自 Node config）。
type Task struct {
	// Prompt 是任务的自然语言描述。
	Prompt string
}

// CodingAgent 是 Agent 适配器接口（设计计划 §20）：
// 在 ProjectContext 指定的 Workspace 中执行任务，产出 Artifact 引用。
// 第一阶段实现 MockCodingAgent，真实 Agent Adapter 是第二步（§33 策略）。
type CodingAgent interface {
	Execute(
		ctx context.Context,
		task Task,
		project project.Context,
		inputs []artifact.ArtifactRef,
	) ([]artifact.ArtifactRef, error)
}
