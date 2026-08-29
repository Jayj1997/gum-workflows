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

// mockAgent 是测试内 Agent：与 agent.MockCodingAgent 行为一致，
// 产出 code 与 openapi（满足最小链路的输出契约）。
type mockAgent struct{}

func newMockAgent() *mockAgent { return &mockAgent{} }

type capturingAgent struct {
	inputs []artifact.ArtifactRef
}

type openAPIOnlyAgent struct{}

func (openAPIOnlyAgent) Execute(_ context.Context, _ agent.Task, proj project.Context, _ []artifact.ArtifactRef) ([]artifact.ArtifactRef, error) {
	path := filepath.Join(proj.Workspace, "openapi-only.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		return nil, err
	}
	return []artifact.ArtifactRef{{ID: "openapi", Kind: artifact.KindOpenAPI, URI: path}}, nil
}

func (a *capturingAgent) Execute(_ context.Context, _ agent.Task, proj project.Context, inputs []artifact.ArtifactRef) ([]artifact.ArtifactRef, error) {
	a.inputs = append([]artifact.ArtifactRef(nil), inputs...)
	taskFile := filepath.Join(proj.Workspace, "captured-task.md")
	if err := os.WriteFile(taskFile, []byte("task"), 0o644); err != nil {
		return nil, err
	}
	return []artifact.ArtifactRef{{ID: "code", Kind: artifact.KindSourceCode, URI: taskFile}}, nil
}

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

	openapiFile := filepath.Join(outDir, "openapi.yaml")
	if err := os.WriteFile(openapiFile, []byte("openapi: 3.1.0\n"), 0o644); err != nil {
		return nil, err
	}
	return []artifact.ArtifactRef{
		{ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: taskFile},
		{ID: "openapi", Kind: artifact.KindOpenAPI, Version: "1", URI: openapiFile},
	}, nil
}
