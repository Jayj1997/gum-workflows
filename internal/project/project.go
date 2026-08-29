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

// Context 是一次 Execution 的项目运行环境。
// Workflow YAML 的 projects 列表（name/repository）经 Resolver 解析后得到本结构；
// Workspace 是 Repository 的规范化绝对路径，Agent 与 Automation 在原地共享使用。
type Context struct {
	Repository Repository
	Workspace  string
}
