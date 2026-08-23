// Package workflow 定义 workflow/v1 的静态模型。
//
// 本包只描述 Workflow 的声明（有哪些 Node、如何配置、如何连接数据），
// 不感知执行；语义校验在 validation 包（依赖 Node Registry）。
package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// workflow/v1 的 apiVersion 与 kind 固定值。
const (
	APIVersionV1 = "workflow/v1"
	KindWorkflow = "Workflow"
)

// Metadata 是 Workflow 的元信息。
type Metadata struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// ProjectSpec 对应 YAML 的 project 段，声明 Workflow 操作的项目。
type ProjectSpec struct {
	Repository string `yaml:"repository"`
	Branch     string `yaml:"branch"`
}

// InputBinding 是一条数据连接：From 格式为 "<node-id>.<output-name>"。
// Runtime 依据它自动建立 Data Edge，不需要显式 dependsOn。
type InputBinding struct {
	From string `yaml:"from"`
}

// NodeSpec 是 Workflow 中一个 Node 实例的声明。
// Node ID（map key）与 Node Type 分离，同一 Type 可实例化多次。
// DependsOn 是可选的 Control Edge 声明：仅表达执行顺序约束，
// 不携带数据；没有输入也没有 dependsOn 的 Node 是合法的源节点。
type NodeSpec struct {
	Type      string                  `yaml:"type"`
	Inputs    map[string]InputBinding `yaml:"inputs"`
	DependsOn []string                `yaml:"dependsOn"`
	Config    map[string]any          `yaml:"config"`
}

// Definition 是计划中的 Workflow：Node 的组合声明（YAML 静态形态）。
type Definition struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   Metadata            `yaml:"metadata"`
	Project    ProjectSpec         `yaml:"project"`
	Nodes      map[string]NodeSpec `yaml:"nodes"`
}

// Validate 做结构层检查（不依赖 Registry，语义校验属于 validation 包）：
// 必填字段非空、Node ID 约束、From 格式、dependsOn 的局部一致性。
// 错误信息必须定位到具体 Node 与字段（设计计划 M3 验收要求）。
func (d Definition) Validate() error {
	if d.APIVersion == "" {
		return fmt.Errorf("apiVersion: must not be empty")
	}
	if d.Kind == "" {
		return fmt.Errorf("kind: must not be empty")
	}
	if d.Metadata.Name == "" {
		return fmt.Errorf("metadata.name: must not be empty")
	}
	if len(d.Nodes) == 0 {
		return fmt.Errorf("nodes: must contain at least one node")
	}

	// 遍历顺序稳定，保证同样的定义产生同样的错误顺序。
	ids := make([]string, 0, len(d.Nodes))
	for id := range d.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("nodes: node ID must not be empty")
		}
		// Node ID 不允许包含 "."，否则与 "<node-id>.<output>" 引用格式产生歧义。
		if strings.Contains(id, ".") {
			return fmt.Errorf("node %q: ID must not contain %q", id, ".")
		}

		spec := d.Nodes[id]
		if spec.Type == "" {
			return fmt.Errorf("node %q: type must not be empty", id)
		}
		for name, binding := range spec.Inputs {
			if binding.From == "" {
				return fmt.Errorf("node %q input %q: from must not be empty", id, name)
			}
			if _, _, err := ParseRef(binding.From); err != nil {
				return fmt.Errorf("node %q input %q: %w", id, name, err)
			}
		}
		if err := validateDependsOn(id, spec.DependsOn); err != nil {
			return err
		}
	}
	return nil
}

// validateDependsOn 检查 dependsOn 的局部一致性（可选字段，仅在存在时检查）：
// 条目非空、不引用自身、不重复。引用的 Node 是否存在属于语义校验。
func validateDependsOn(nodeID string, dependsOn []string) error {
	seen := make(map[string]bool, len(dependsOn))
	for _, dep := range dependsOn {
		if dep == "" {
			return fmt.Errorf("node %q: dependsOn entries must not be empty", nodeID)
		}
		if dep == nodeID {
			return fmt.Errorf("node %q: dependsOn must not include itself", nodeID)
		}
		if seen[dep] {
			return fmt.Errorf("node %q: duplicate dependsOn entry %q", nodeID, dep)
		}
		seen[dep] = true
	}
	return nil
}
