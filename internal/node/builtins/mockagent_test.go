package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jayj1997/gum-workflows/internal/agent"
	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

// mockAgent 是测试内 Agent：与 agent.MockCodingAgent 行为类似，
// 但依据输入推断产出（收到 architecture -> 额外产出 openapi），
// 以满足 fullstack 中 backend 节点的输出契约。
type mockAgent struct{}

func newMockAgent() *mockAgent { return &mockAgent{} }

func (m *mockAgent) Execute(
	ctx context.Context,
	task agent.Task,
	proj project.Context,
	inputs []artifact.ArtifactRef,
) ([]artifact.ArtifactRef, error) {
	if proj.Workspace == "" {
		return nil, fmt.Errorf("mock agent: no workspace")
	}

	outDir := filepath.Join(proj.Workspace, ".mock-agent")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	taskFile := filepath.Join(outDir, "task.md")
	if err := os.WriteFile(taskFile, []byte("task"), 0o644); err != nil {
		return nil, err
	}

	out := []artifact.ArtifactRef{
		{ID: "source-code", Kind: artifact.KindSourceCode, Version: "1", URI: taskFile},
	}
	for _, in := range inputs {
		if in.Kind == artifact.KindArchitectureSpec {
			openapiFile := filepath.Join(outDir, "openapi.yaml")
			if err := os.WriteFile(openapiFile, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
				return nil, err
			}
			out = append(out, artifact.ArtifactRef{
				ID: "openapi", Kind: artifact.KindOpenAPI, Version: "1", URI: openapiFile,
			})
			break
		}
	}
	return out, nil
}
