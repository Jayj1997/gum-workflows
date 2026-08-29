// Package project 提供 Project Runtime：把 Workflow YAML 的 projects 声明
// 解析为原地运行的 ProjectContext。
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Runtime 解析 Project 声明为原地 Workspace。
type Runtime struct{}

// NewRuntime 创建 Project Runtime。
func NewRuntime() *Runtime {
	return &Runtime{}
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
	info, err := os.Stat(absRepo)
	if err != nil {
		return Context{}, fmt.Errorf("resolve project: stat repository: %w", err)
	}
	if !info.IsDir() {
		return Context{}, fmt.Errorf("resolve project: repository %q is not a directory", absRepo)
	}

	return Context{
		Repository: Repository{Path: absRepo},
		Workspace:  absRepo,
	}, nil
}

// Spec 是 Workflow YAML projects 条目的解析输入（与 workflow.ProjectSpec
// 解耦，避免基础包 project 反向依赖 workflow 包）。
// 本期无 branch（设计文档 §3.5）。
type Spec struct {
	Name       string
	Repository string
}
