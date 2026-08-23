package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

// runCmd 执行 workflow run <file>（设计计划 §16：不接受业务参数）。
//
// 管线（计划 §25）：
//
//	Load YAML -> CUE Validate -> Parse -> Semantic Validate ->
//	Create Execution -> Initialize Project/Workspace -> Execute -> Persist
func runCmd(path string) error {
	def, data, registry, err := loadAndValidate(path)
	if err != nil {
		return err
	}

	// ③ Project Runtime：解析 project 声明并创建本次 Execution 的 Workspace。
	// Execution ID 由 execution.NextExecutionID 分配（唯一来源），
	// workspace / artifacts / 状态目录与 Engine 使用同一 ID。
	runtime := project.NewRuntime(filepath.Join(".workflow", "executions"))
	projCtx, err := runtime.Resolve(path, project.Spec{
		Repository: def.Project.Repository,
		Branch:     def.Project.Branch,
	})
	if err != nil {
		return fmt.Errorf("resolve project: %w", err)
	}

	executionID, err := execution.NextExecutionID(runtime.BaseDir)
	if err != nil {
		return fmt.Errorf("allocate execution id: %w", err)
	}
	executionDir := filepath.Join(runtime.BaseDir, executionID)
	if err := os.MkdirAll(executionDir, 0o755); err != nil {
		return fmt.Errorf("create execution dir: %w", err)
	}

	// 定义快照（计划 §28：execution 目录含 workflow.yaml）。
	if err := os.WriteFile(filepath.Join(executionDir, "workflow.yaml"), data, 0o644); err != nil {
		return fmt.Errorf("snapshot workflow.yaml: %w", err)
	}

	wsCtx, err := runtime.CreateWorkspace(projCtx, executionID)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	// ④ 执行：FS Artifact Store + 状态持久化 + Workspace 注入。
	store, err := artifact.NewFilesystemStore(filepath.Join(executionDir, "artifacts"))
	if err != nil {
		return fmt.Errorf("create artifact store: %w", err)
	}

	engine := execution.NewEngine(
		registry,
		store,
		nil,
		execution.WithStateDir(runtime.BaseDir),
		execution.WithProjectContext(wsCtx),
		execution.WithExecutionID(executionID),
	)
	exec, runErr := engine.Run(context.Background(), def)

	printExecutionSummary(exec)
	return runErr
}

// printExecutionSummary 输出执行摘要（计划 §42 的验收形态）。
func printExecutionSummary(exec *execution.WorkflowExecution) {
	if exec == nil {
		return
	}
	fmt.Printf("\nWorkflow %s %s (%s)\n", exec.Workflow, exec.Status, exec.ID)

	ids := make([]string, 0, len(exec.Nodes))
	for id := range exec.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Println("Nodes:")
	for _, id := range ids {
		ne := exec.Nodes[id]
		line := fmt.Sprintf("  %-13s %-9s %s", id, ne.Status, ne.NodeType)
		if ne.Error != "" {
			line += "  error: " + ne.Error
		}
		fmt.Println(line)
	}

	fmt.Println("Artifacts:")
	for _, id := range ids {
		ne := exec.Nodes[id]
		names := make([]string, 0, len(ne.Outputs))
		for name := range ne.Outputs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ref := ne.Outputs[name]
			fmt.Printf("  %-13s %-14s %s\n", id, name, ref.Kind)
		}
	}
}
