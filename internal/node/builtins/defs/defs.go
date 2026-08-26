// Package defs 内嵌定义侧种子数据（设计文档 §3.8）：
//
//	nodetypes/     agent / automation / human（恰三个）
//	definitions/   内置 Node Definition（§12 定稿契约）
//	executors/     每 definition 每版本一份（内置先各 v1）
//
// 经 go:embed 随二进制分发；Load 解析为内存 definition.Registry，
// 供语义校验与 run 启动导入复用。
package defs

import (
	"embed"
	"fmt"

	"github.com/Jayj1997/gum-workflows/internal/definition"
)

//go:embed nodetypes/agent.yaml
//go:embed nodetypes/automation.yaml
//go:embed nodetypes/human.yaml
//go:embed definitions/*.yaml
//go:embed executors/*.yaml
var fs embed.FS

// nodeTypeFiles 是 Node Type 种子的确定顺序（agent / automation / human）。
var nodeTypeFiles = []string{
	"nodetypes/agent.yaml",
	"nodetypes/automation.yaml",
	"nodetypes/human.yaml",
}

// Load 把全部种子解析并注册进 registry。
//
// 注册顺序即依赖顺序：node types -> definitions -> executors
// （executor 按 node 名引用 definition，definition 按 type 引用 node type）。
// 任何一份种子非法即整体失败（种子随二进制分发，失败属程序缺陷）。
func Load(registry *definition.Registry) error {
	type seedStep struct {
		files []string // 有序种子文件清单
		load  func(*definition.Registry, []byte) error
	}
	definitions, err := sortedFiles("definitions")
	if err != nil {
		return fmt.Errorf("read seed dir definitions: %w", err)
	}
	executors, err := sortedFiles("executors")
	if err != nil {
		return fmt.Errorf("read seed dir executors: %w", err)
	}
	steps := []seedStep{
		{files: nodeTypeFiles, load: func(r *definition.Registry, data []byte) error {
			t, err := definition.LoadNodeType(data)
			if err != nil {
				return err
			}
			return r.RegisterNodeType(t)
		}},
		{files: definitions, load: func(r *definition.Registry, data []byte) error {
			d, err := definition.LoadNodeDefinition(data)
			if err != nil {
				return err
			}
			return r.RegisterDefinition(d)
		}},
		{files: executors, load: func(r *definition.Registry, data []byte) error {
			e, err := definition.LoadNodeExecutor(data)
			if err != nil {
				return err
			}
			return r.RegisterExecutor(e)
		}},
	}

	for _, step := range steps {
		for _, file := range step.files {
			data, err := fs.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read seed %s: %w", file, err)
			}
			if err := step.load(registry, data); err != nil {
				return fmt.Errorf("load seed %s: %w", file, err)
			}
		}
	}
	return nil
}

// NewRegistry 返回已装载全部种子的内存 definition.Registry。
func NewRegistry() (*definition.Registry, error) {
	r := definition.NewRegistry()
	if err := Load(r); err != nil {
		return nil, err
	}
	return r, nil
}

// sortedFiles 返回 dir 下全部文件（embed.FS 的 ReadDir 按文件名有序，
// 保证注册顺序稳定、错误可复现）；目录不可读即报错，
// 避免坏 embed 以空注册表静默「成功」。
func sortedFiles(dir string) ([]string, error) {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, dir+"/"+e.Name())
		}
	}
	return files, nil
}
