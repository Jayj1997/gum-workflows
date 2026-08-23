// Package project 定义 Workflow Execution 的运行上下文。
//
// Project 是基础值类型包：不依赖其他 internal 包，
// 供 node / execution 等上层包直接引用。
package project

// Repository 表示 Workflow 操作的项目仓库。
// Path 为本地路径或远端地址，解析逻辑由后续的 Project Resolver 决定。
type Repository struct {
	Path string
}

// Context 是计划中的 ProjectContext：一次 Execution 的项目运行环境。
// Workflow YAML 的 project 段（repository/branch）经 Resolver 解析后得到本结构。
type Context struct {
	Repository Repository
	Branch     string
	Workspace  string
}
