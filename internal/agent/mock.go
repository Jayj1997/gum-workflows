package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

// MockCodingAgent 不启动真实 Agent（设计计划 §33：先把 Runtime 跑通）。
// 它在 Workspace 中写入一个 task 说明文件，模拟「Agent 修改代码」，
// 并返回 SourceCode Artifact（只引用 Workspace 路径，不携带源码本体，§14）。
type MockCodingAgent struct{}

// NewMockCodingAgent 创建 Mock Agent。
func NewMockCodingAgent() *MockCodingAgent { return &MockCodingAgent{} }

// Execute 在 Workspace 写入 .mock-agent/<task>.md 并产出 SourceCode 引用；
// 若输入包含 ArchitectureSpec（后端语义），补产 OpenAPI 引用，
// 使 fullstack 的 backend 节点满足 openapi-generator 的输入契约（计划 §10）。
func (m *MockCodingAgent) Execute(
	ctx context.Context,
	task Task,
	proj project.Context,
	inputs []artifact.ArtifactRef,
) ([]artifact.ArtifactRef, error) {
	if proj.Workspace == "" {
		return nil, fmt.Errorf("mock agent: project context has no workspace")
	}

	outDir := filepath.Join(proj.Workspace, ".mock-agent")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("mock agent: create %s: %w", outDir, err)
	}

	content := fmt.Sprintf("# mock coding agent\n\ntask: %s\n\ninputs:\n", task.Prompt)
	for _, in := range inputs {
		content += fmt.Sprintf("- %s (%s)\n", in.ID, in.Kind)
	}
	taskFile := filepath.Join(outDir, "task.md")
	if err := os.WriteFile(taskFile, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("mock agent: write task file: %w", err)
	}

	refs := []artifact.ArtifactRef{
		{
			ID:      "source-code",
			Kind:    artifact.KindSourceCode,
			Version: "1",
			URI:     taskFile,
		},
	}
	for _, in := range inputs {
		if in.Kind == artifact.KindArchitectureSpec {
			openapiFile := filepath.Join(outDir, "openapi.yaml")
			spec := "openapi: 3.1.0\npaths:\n  /orders:\n    get: {}\n"
			if err := os.WriteFile(openapiFile, []byte(spec), 0o644); err != nil {
				return nil, fmt.Errorf("mock agent: write openapi: %w", err)
			}
			refs = append(refs, artifact.ArtifactRef{
				ID:      "openapi",
				Kind:    artifact.KindOpenAPI,
				Version: "1",
				URI:     openapiFile,
			})
			break
		}
	}
	return refs, nil
}
