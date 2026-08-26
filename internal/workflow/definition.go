// Package workflow 定义 workflow/v1 的静态模型。
//
// 本包只描述 Workflow 的声明（有哪些 Node Instance、如何配置、如何连接数据），
// 不感知执行；语义校验在 validation 包（依赖 definition Registry）。
package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// workflow/v1 的 apiVersion 与 kind 固定值。
// kind 值小写化（设计文档 §3.7：与各定义侧信封一致，小写鉴别）。
const (
	APIVersionV1 = "workflow/v1"
	KindWorkflow = "workflow"
)

// Metadata 是 Workflow 的元信息。
// description 为纯展示字段（设计文档 §3.7）。
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

// ProjectSpec 是 projects 列表的一个条目（设计文档 §3.5）。
// repository 支持相对（相对 workflow 文件所在目录）与绝对路径。
// 本期无 branch（本地项目无意义）；列表结构就位，
// 「恰好 1 个」的校验属票 06。
type ProjectSpec struct {
	Name       string `yaml:"name"`
	Repository string `yaml:"repository"`
}

// InstanceMetadata 是 Node Instance 的展示性元信息（设计文档 §3.6）。
type InstanceMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// InputBinding 是一条数据连接：From 格式为 "<node-id>.<output-name>"。
// Runtime 依据它自动建立 Data Edge，不需要显式 dependsOn。
type InputBinding struct {
	From string `yaml:"from"`
}

// NodeSpec 是 Workflow 中一个 Node Instance 的声明（设计文档 §3.6）。
// Node ID（map key）在 Workflow 内唯一，引用寻址用（不含 "."）；
// Node 按 Node Definition 的 name 引用，同一定义可实例化多次。
//
// Executor/LLM/TargetModel/Metadata 均可选：
//
//	executor     缺省 = run 启动时发现的最新版本，解析后固定；
//	llm          仅 agent 类节点合法（校验属票 06）；
//	target_model 仅 agent 类节点合法（校验属票 06）；
//	metadata     纯展示。
//
// DependsOn 是可选的 Control Edge 声明：仅表达执行顺序约束，
// 不携带数据；没有输入也没有 dependsOn 的 Node 是合法的源节点。
type NodeSpec struct {
	Node        string                  `yaml:"node"`
	Executor    string                  `yaml:"executor"`
	LLM         string                  `yaml:"llm"`
	TargetModel string                  `yaml:"target_model"`
	Metadata    InstanceMetadata        `yaml:"metadata"`
	Inputs      map[string]InputBinding `yaml:"inputs"`
	DependsOn   []string                `yaml:"dependsOn"`
	Config      map[string]any          `yaml:"config"`
}

// Definition 是计划中的 Workflow：Node Instance 的组合声明
// （YAML 静态形态，设计文档 §3.7）。
type Definition struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   Metadata            `yaml:"metadata"`
	Projects   []ProjectSpec       `yaml:"projects"`
	Nodes      map[string]NodeSpec `yaml:"nodes"`
}

// Validate 做结构层检查（不依赖 Registry，语义校验属于 validation 包）：
// 必填字段非空、Node ID 约束、From 格式、dependsOn 的局部一致性。
// projects 的数量与路径检查属票 06。
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

	for i, p := range d.Projects {
		if p.Name == "" {
			return fmt.Errorf("projects[%d]: name must not be empty", i)
		}
		if strings.TrimSpace(p.Repository) == "" {
			return fmt.Errorf("projects[%d] %q: repository must not be empty", i, p.Name)
		}
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
		if spec.Node == "" {
			return fmt.Errorf("node %q: node must not be empty", id)
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
