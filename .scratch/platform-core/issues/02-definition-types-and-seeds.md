# 02: 定义层类型与内嵌种子

**What to build:** 定义侧三类组件的 YAML 声明形态与内存注册表（设计文档 §3.1–§3.3、§3.8）：Node Type Definition（agent/automation/human 种子）、Node Definition（契约以 YAML 为唯一来源，含 requires/inputs/outputs）、Node Executor Definition（node + version + updates）。种子数据（3 个 node type、4 个既有节点按设计文档 §12 定稿契约、各 v1 执行器）以 go:embed 内嵌。交付后：种子可枚举、(definition, version) 唯一、`Latest(definition)` 可查、契约中 Kind 已注册且 TypeExpr 语法合法。

内置契约定稿（设计文档 §12）：requirement-analysis 输入 `requirement: markdown`、输出 `rationality: int` + `analysis-output: markdown`；architecture-design 输入 `analysis-output: markdown`；coding-agent 全 optional 输入（analysis-output/architecture/openapi/frontend-sdk，本票先不加 advise，T10 随审批循环加）；openapi-generator 输入 `openapi: OpenAPI`。

**Blocked by:** 01（TypeExpr 解析器）

**Status:** done

- [x] 三类声明各带信封 apiVersion/kind，loader 严格模式（KnownFields）拒绝未知字段
- [x] Node Type 种子恰 3 个：agent（requires [llm]）、automation、human
- [x] Node Definition 校验：type ∈ 三值、requires 合法值、TypeExpr 语法合法、Kind 已注册（含 optional 端口）
- [x] Executor 声明：node 按 name 引用、version 同定义内唯一
- [x] 内存 definition registry：按 name / (definition, version) / Latest 查询
- [x] metadata.name/description 必填规则按设计文档 §3

## Comments

**2026-08-26（agent 实施记录）**：已交付于 commit `87f33c9`（main）。`internal/definition` 新增三类声明（信封常量、类型化 NodeType/Requirement 枚举）、严格模式 loader（单文档、未知字段拒绝）、聚合校验（信封/必填/type 三值/requires 合法值/TypeExpr 语法逐端口定位）、`Registry`（name / (definition, version) / `Latest` 查询，Latest 按数字比较 v10 > v9）、`ValidateKinds`（Kind 注册检查，含 optional 端口）。种子内嵌于 `internal/node/builtins/defs/`（设计文档 §3.8 布局：3 nodetypes + 4 definitions 按 §12 定稿 + 4 executors 各 v1），依赖序加载。

与票面字样的口径决定：
1. 种子包放在 `internal/node/builtins/defs`（§3.8/§9 规定其归属 node 树），`internal/definition` 保持纯类型层不依赖 embed。
2. Kind 注册检查以 `ValidateKinds` 方法提供（需要 artifact.Registry 入参，保持解析/注册校验分离）；接线到主校验流程在 06（语义校验扩展）完成。
3. Latest 版本比较按 v 后数字大小实现（票面未细化；字典序会把 v10 排在 v2 前是实 bug，已修）。
