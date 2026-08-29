package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/project"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
)

// runCmd 执行 workflow run <file>（设计计划 §16：不接受业务参数）。
//
// 管线（计划 §25）：
//
//	Load YAML -> CUE Validate -> Parse -> Semantic Validate ->
//	Create Execution -> Resolve In-place Project Workspace -> Execute -> Persist
func runCmd(path string, paths runtimepath.Paths) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWorkflow(ctx, path, commandStdinIsInteractive(), newStdinHumanGateway(os.Stdin, os.Stdout), paths)
}

func runWorkflow(ctx context.Context, path string, interactive bool, gateway execution.HumanGateway, paths runtimepath.Paths) error {
	def, data, executors, defsRegistry, llmConfig, warnings, err := loadAndValidate(ctx, path)
	if err != nil {
		return err
	}
	printWarnings(warnings)
	if containsHumanNode(def, defsRegistry) && !interactive {
		return fmt.Errorf("workflow contains human nodes and requires an interactive terminal on stdin")
	}

	historyStore, err := history.Open(ctx, paths.Database())
	if err != nil {
		return fmt.Errorf("open history database: %w", err)
	}
	defer historyStore.Close()
	def, err = pinAndImportDefinitions(ctx, historyStore, def, executors, defsRegistry, llmConfig)
	if err != nil {
		return err
	}

	// ③ Project Runtime：解析 projects[0] 声明为原地 Workspace。
	// Run UUID 在任何运行文件写入前分配，数据库、Artifacts 与状态目录共用。
	// projects 的「恰好 1 个」校验属票 06；此处取第一个条目。
	projSpec := project.Spec{}
	if len(def.Projects) > 0 {
		projSpec = project.Spec{Name: def.Projects[0].Name, Repository: def.Projects[0].Repository}
	}
	projCtx, err := project.Resolve(path, projSpec)
	if err != nil {
		return fmt.Errorf("resolve project: %w", err)
	}

	runID := uuid.NewString()
	runDir := paths.RunDir(runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}

	// 定义快照：Run 目录包含本次输入的 workflow.yaml。
	if err := os.WriteFile(paths.WorkflowSnapshot(runID), data, 0o644); err != nil {
		return fmt.Errorf("snapshot workflow.yaml: %w", err)
	}

	// ④ 执行：FS Artifact Store + 状态持久化 + Workspace 注入。
	store, err := artifact.NewFilesystemStore(paths.ArtifactsDir(runID))
	if err != nil {
		return fmt.Errorf("create artifact store: %w", err)
	}

	engine := execution.NewEngine(
		executors,
		defsRegistry,
		store,
		nil,
		execution.WithStateDir(paths.RunsDir()),
		execution.WithProjectContext(projCtx),
		execution.WithRunID(runID),
		execution.WithWorkflowFile(path),
		execution.WithHumanGateway(gateway),
		execution.WithRunRecorder(historyStore),
	)
	exec, runErr := engine.Run(ctx, def)

	printExecutionSummary(exec)
	return runErr
}

func containsHumanNode(def workflow.Definition, defs *definition.Registry) bool {
	for _, spec := range def.Nodes {
		d, err := defs.Definition(spec.Node)
		if err == nil && d.Type == definition.TypeHuman {
			return true
		}
	}
	return false
}

// The standard library does not expose a portable terminal ioctl check;
// isatty covers Unix terminals and Windows Cygwin/MSYS terminals.
func stdinIsTerminal(file *os.File) bool {
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// printExecutionSummary 输出执行摘要（计划 §42 的验收形态）。
func printExecutionSummary(exec *execution.WorkflowExecution) {
	printExecutionSummaryTo(os.Stdout, exec)
}

func printExecutionSummaryTo(out io.Writer, exec *execution.WorkflowExecution) {
	if exec == nil {
		return
	}
	fmt.Fprintf(out, "\nWorkflow %s %s (%s)\n", exec.Workflow, exec.Status, exec.RunID)

	ids := make([]string, 0, len(exec.Nodes))
	for id := range exec.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Fprintln(out, "Nodes:")
	for _, id := range ids {
		ne := exec.Nodes[id]
		line := fmt.Sprintf("  %-13s %-9s %s", id, ne.Current.Status, ne.NodeDefinition)
		line += fmt.Sprintf("  rounds: %d", ne.Current.Round)
		if ne.Current.ErrorKind != "" {
			line += "  error_kind: " + string(ne.Current.ErrorKind)
		}
		if ne.Current.Error != "" {
			line += "  error: " + ne.Current.Error
		}
		fmt.Fprintln(out, line)
	}

	fmt.Fprintln(out, "Artifacts:")
	for _, id := range ids {
		ne := exec.Nodes[id]
		names := make([]string, 0, len(ne.Current.Outputs))
		for name := range ne.Current.Outputs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ref := ne.Current.Outputs[name]
			fmt.Fprintf(out, "  %-13s %-14s %s\n", id, name, ref.Kind)
		}
	}
}
