package definition

import (
	"sort"
	"strings"
)

// 三类定义侧声明的信封固定值（设计文档 §2 组件总览表）。
const (
	NodeTypeAPIVersionV1       = "nodeTypeDefinition/v1"
	NodeTypeDefinitionKind     = "nodeTypeDefinition"
	NodeDefinitionAPIVersionV1 = "nodeDefinition/v1"
	NodeDefinitionKind         = "nodeDefinition"
	NodeExecutorAPIVersionV1   = "nodeExecutor/v1"
	NodeExecutorKind           = "nodeExecutor"
)

// Node Type 名（设计文档 §3.1：全局恰三个，种子数据，不可由
// Workflow 作者增删）。
type NodeType string

// 三个 Node Type 名。
const (
	TypeAgent      NodeType = "agent"
	TypeAutomation NodeType = "automation"
	TypeHuman      NodeType = "human"
)

// Requirement 是 requires 列表条目的类型化枚举（v1 合法值，设计文档 §3.1）。
type Requirement string

// requires 的 v1 合法值。
const (
	RequireLLM     Requirement = "llm"
	RequireProject Requirement = "project"
)

// isNodeTypeName 报告 name 是否为三个 Node Type 名之一。
func isNodeTypeName(name string) bool {
	switch NodeType(name) {
	case TypeAgent, TypeAutomation, TypeHuman:
		return true
	}
	return false
}

// nodeTypeNames 返回三个 Node Type 名（有序），供错误信息列出可选项。
func nodeTypeNames() []string {
	return []string{string(TypeAgent), string(TypeAutomation), string(TypeHuman)}
}

// Metadata 是三类定义共用的元信息。name/description 是否必填
// 因组件而异（§3.1/§3.2 必填，§3.3 展示用选填）。
type Metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// NodeTypeDefinition 是节点按执行主体划分的类别声明（设计文档 §3.1），
// 全局恰好三个，以种子数据内嵌分发。
type NodeTypeDefinition struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   Metadata      `yaml:"metadata"`
	Requires   []Requirement `yaml:"requires"`
}

// InputPort 是 Node Definition 的一个输入端口声明。
// Type 是 TypeExpr 文本（§4）；语法与 Kind 注册由 Validate 检查。
// optional: true 仅 inputs 合法（§3.2）。
type InputPort struct {
	Type        string `yaml:"type"`
	Optional    bool   `yaml:"optional"`
	Description string `yaml:"description"`
}

// OutputPort 是 Node Definition 的一个输出端口声明（无 optional，§3.2）。
type OutputPort struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// NodeDefinition 是平台认识的一个节点本体：inputs/outputs 契约的
// 唯一声明处（设计文档 §3.2）。契约自此以 YAML 声明，不再来自 Go 代码。
type NodeDefinition struct {
	APIVersion string                `yaml:"apiVersion"`
	Kind       string                `yaml:"kind"`
	Metadata   Metadata              `yaml:"metadata"`
	Type       NodeType              `yaml:"type"`
	Requires   []Requirement         `yaml:"requires"`
	Inputs     map[string]InputPort  `yaml:"inputs"`
	Outputs    map[string]OutputPort `yaml:"outputs"`
}

// NodeExecutorDefinition 是某 Node Definition 的一个可执行版本
// （设计文档 §3.3）：一条数据 = 一份 YAML 声明 + 一个编译进二进制的
// Go 实现，按 (node, version) 寻址；同一定义多版本并存。
type NodeExecutorDefinition struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Node       string   `yaml:"node"`
	Version    string   `yaml:"version"`
	Updates    string   `yaml:"updates"`
}

// sortedKeys 返回 map key 的有序列表（保证校验错误顺序稳定）。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// joinQuote 返回 ["a", "b"] 形态，供错误信息列出合法值集合。
func joinQuote(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = `"` + v + `"`
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
