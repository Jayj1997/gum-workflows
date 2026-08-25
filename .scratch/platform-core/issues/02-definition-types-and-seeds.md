# 02: 定义层类型与内嵌种子

**What to build:** 定义侧三类组件的 YAML 声明形态与内存注册表（设计文档 §3.1–§3.3、§3.8）：Node Type Definition（agent/automation/human 种子）、Node Definition（契约以 YAML 为唯一来源，含 requires/inputs/outputs）、Node Executor Definition（node + version + updates）。种子数据（3 个 node type、4 个既有节点按设计文档 §12 定稿契约、各 v1 执行器）以 go:embed 内嵌。交付后：种子可枚举、(definition, version) 唯一、`Latest(definition)` 可查、契约中 Kind 已注册且 TypeExpr 语法合法。

内置契约定稿（设计文档 §12）：requirement-analysis 输入 `requirement: markdown`、输出 `rationality: int` + `analysis-output: markdown`；architecture-design 输入 `analysis-output: markdown`；coding-agent 全 optional 输入（analysis-output/architecture/openapi/frontend-sdk，本票先不加 advise，T10 随审批循环加）；openapi-generator 输入 `openapi: OpenAPI`。

**Blocked by:** 01（TypeExpr 解析器）

**Status:** ready-for-agent

- [ ] 三类声明各带信封 apiVersion/kind，loader 严格模式（KnownFields）拒绝未知字段
- [ ] Node Type 种子恰 3 个：agent（requires [llm]）、automation、human
- [ ] Node Definition 校验：type ∈ 三值、requires 合法值、TypeExpr 语法合法、Kind 已注册（含 optional 端口）
- [ ] Executor 声明：node 按 name 引用、version 同定义内唯一
- [ ] 内存 definition registry：按 name / (definition, version) / Latest 查询
- [ ] metadata.name/description 必填规则按设计文档 §3
