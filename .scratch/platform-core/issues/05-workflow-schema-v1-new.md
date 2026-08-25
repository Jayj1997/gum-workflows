# 05: workflow/v1 新 Schema 与临时示例迁移

**What to build:** workflow.yaml 的 Node Instance 新形态（设计文档 §3.6–§3.7）：`nodes.<id>.type` 改名 `node`，新增可选 `executor`/`llm`/`target_model`/`metadata`；`project`（单数）改 `projects`（列表，本票只做结构，恰一个的校验在 T06）；kind 值小写化。CUE schema、Go struct、loader 三者同步（DEVELOPMENT.md §5 规则）。旧 fullstack 示例退役（新契约下 requirement-analysis 有必填输入，旧示例必然失败且 human-input 源要到 T09 才存在--已与维护者确认此处置），替换为最小 human-free 临时示例（coding-agent 全 optional 输入当源 -> openapi-generator），CLI validate/run e2e 在临时示例上保持绿。

**Blocked by:** 03（新注册表；`node:` 字段引用 definition name）

**Status:** ready-for-agent

- [ ] NodeSpec：node（必填）/executor/llm/target_model/metadata（可选）+ 既有 inputs/dependsOn/config 语义不变
- [ ] `projects` 列表结构（name/repository，相对路径相对 workflow 文件）；`branch` 字段删除
- [ ] CUE 与 Go struct 同步；loader 严格模式继续拒绝未知字段；kind 小写
- [ ] 旧 examples/fullstack 退役（移入 git 历史；README 留一行说明待 T14 重写）
- [ ] 临时示例可 `workflow validate` 通过、`workflow run` 跑通（无 human 节点）
- [ ] 既有 e2e/测试改到临时示例后全绿
