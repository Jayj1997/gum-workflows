// Package history 实现本地 SQLite 统一库（设计文档 §8）：
//
//   - Local Data Root/product.db，modernc.org/sqlite 驱动（纯 Go 无 CGO）；
//   - WAL / busy_timeout / foreign_keys，PRAGMA user_version 顺序迁移；
//   - run 启动时隐式导入内嵌种子与本次 workflow（定义侧五表）；
//   - validate 纯只读零副作用（不调用本包）。
//
// 本包不 import definition/workflow/llm：导入所需数据由消费方（cmd 层）
// 从已装好的 registry 与 workflow 定义收集后，以本包定义的 DTO 传入。
// 这样依赖方向只向下（§9：history 不被 execution import，被 cmd 消费）。
package history

// NodeTypeDefRow 是 node_type_definition 表的一行（设计文档 §8.3）。
// 全局恰三个种子（agent / automation / human），按 name 寻址。
type NodeTypeDefRow struct {
	ID          string // UUID
	Name        string // agent | automation | human
	Description string
	Requires    []string // 序列化为 requires_json
}

// NodeDefRow 是 node_definition 表的一行（设计文档 §8.3）。
// 契约（inputs/outputs）的唯一来源 = Node Definition YAML。
type NodeDefRow struct {
	ID          string // UUID
	Name        string
	Description string
	Type        string // 引用 node_type_definition.name
	Requires    []string
	Inputs      map[string]InputPort // 序列化为 inputs_json
	Outputs     map[string]OutputPort
}

// InputPort 对应定义侧 InputPort。
type InputPort struct {
	Type        string `json:"type"`
	Optional    bool   `json:"optional,omitempty"`
	Description string `json:"description,omitempty"`
}

// OutputPort 对应定义侧 OutputPort。
type OutputPort struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// NodeExecRow 是 node_executor 表的一行（设计文档 §8.3）。
// 按 (node_definition_id, version) 唯一。
// Node 是所属 Node Definition 的 name；ImportDefinitions 时在事务内
// 按 Node 查 node_definition.id 回填 NodeDefinitionID（UUID，FK）。
type NodeExecRow struct {
	ID               string // UUID
	Node             string // 所属 Node Definition 名（导入时按名解析 id）
	NodeDefinitionID string // 引用 node_definition.id（导入时回填）
	Version          string // v1、v2…
	Name             string
	Description      string
	Updates          string
}

// WorkflowRow 是 workflow 表的一行（设计文档 §8.3）。
// 按 (name, version) 覆盖式 upsert。
type WorkflowRow struct {
	ID          string // UUID
	Name        string
	Version     string
	Description string
	Projects    []ProjectRow // 序列化为 projects_json
}

// ProjectRow 是 projects_json 的一个条目（设计文档 §3.5）。
type ProjectRow struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
}

// NodeInstanceRow 是 node_instance 表的一行（设计文档 §8.3）。
// node_executor_id 与 llm_provider/llm_model 为 run 启动时解析后固定值。
type NodeInstanceRow struct {
	ID               string // UUID
	WorkflowID       string // 引用 workflow.id
	NodeID           string // Workflow 内 Node ID（map key）
	NodeDefinitionID string // 引用 node_definition.id
	NodeExecutorID   string // 引用 node_executor.id，解析后固定
	DisplayName      string
	Description      string
	LLMProvider      string // 解析后名字（llm.yaml 不落库）
	LLMModel         string
	Inputs           map[string]InputBinding // 序列化为 inputs_json
	DependsOn        []string                // 序列化为 depends_on_json
	Config           map[string]any          // 序列化为 config_json
}

// InputBinding 镜像 workflow.InputBinding 的数据形态。
type InputBinding struct {
	From string `json:"from"`
}
