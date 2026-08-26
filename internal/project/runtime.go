// Package project 提供 Project Runtime：把 Workflow YAML 的 projects 声明
// 解析为运行环境（ProjectContext + Workspace）。
//
// 计划 §15/§17：Workflow -> Project Resolver -> ProjectContext；
// Execution -> Workspace -> Project。每个 Execution 拥有独立 Workspace，
// Coding Agent 在 Workspace 内执行（§19）。
package project

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Runtime 解析 Project 声明并创建 Workspace。
type Runtime struct {
	// BaseDir 是 executions 根目录（默认 .workflow/executions），
	// 各 Execution 的 Workspace 创建在其下。
	BaseDir string
}

// NewRuntime 创建 Project Runtime。baseDir 为空时使用 ".workflow/executions"。
func NewRuntime(baseDir string) *Runtime {
	if baseDir == "" {
		baseDir = filepath.Join(".workflow", "executions")
	}
	return &Runtime{BaseDir: baseDir}
}

// Resolve 解析 Workflow 的 projects[0] 声明（计划 §15，设计文档 §3.5）：
// repository 相对于 workflowFile 所在目录解析为绝对路径。
func (r *Runtime) Resolve(workflowFile string, spec Spec) (Context, error) {
	if strings.TrimSpace(spec.Repository) == "" {
		return Context{}, fmt.Errorf("resolve project: repository must not be empty")
	}

	repoPath := spec.Repository
	if !filepath.IsAbs(repoPath) && workflowFile != "" {
		// 相对路径相对于 workflow 文件所在目录（而非进程 CWD）。
		absWorkflow, err := filepath.Abs(workflowFile)
		if err != nil {
			return Context{}, fmt.Errorf("resolve project: %w", err)
		}
		repoPath = filepath.Join(filepath.Dir(absWorkflow), spec.Repository)
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return Context{}, fmt.Errorf("resolve project: %w", err)
	}

	return Context{Repository: Repository{Path: absRepo}}, nil
}

// CreateWorkspace 为指定 Execution 创建 Workspace 并把项目复制进去
// （计划 §17：.workflow/executions/<id>/workspace/project）。
// 返回填好 Workspace 路径的 ProjectContext。
func (r *Runtime) CreateWorkspace(ctx Context, executionID string) (Context, error) {
	if ctx.Repository.Path == "" {
		return Context{}, fmt.Errorf("create workspace: project context has no repository")
	}

	info, err := os.Stat(ctx.Repository.Path)
	if err != nil {
		return Context{}, fmt.Errorf("create workspace: stat repository: %w", err)
	}
	if !info.IsDir() {
		return Context{}, fmt.Errorf("create workspace: repository %q is not a directory", ctx.Repository.Path)
	}

	wsProject := filepath.Join(r.BaseDir, executionID, "workspace", "project")
	if err := copyDir(ctx.Repository.Path, wsProject); err != nil {
		return Context{}, fmt.Errorf("create workspace: %w", err)
	}

	ctx.Workspace = wsProject
	return ctx, nil
}

// copyDir 递归复制目录（跳过隐藏目录如 .git/.workflow，保持项目文件干净）。
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			return nil
		}

		// 跳过版本库与运行时目录。
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".workflow") {
			return fs.SkipDir
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, in)
		return err
	})
}

// Spec 是 Workflow YAML projects 条目的解析输入（与 workflow.ProjectSpec
// 解耦，避免基础包 project 反向依赖 workflow 包）。
// 本期无 branch（设计文档 §3.5）。
type Spec struct {
	Name       string
	Repository string
}
