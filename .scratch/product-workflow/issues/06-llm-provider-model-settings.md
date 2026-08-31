# 06: LLM Provider / Model 设置

**What to build:** 让用户在 UI 中创建和编辑 `Provider -> Models` 设置，包括稳定 Gum UUID、可编辑 Provider Model ID、Secret 引用和双层 default。首版不依赖 `/models`、Capability、排序或 enable/disable。

**Blocked by:** 02: SQLite Workflow list/create

**Status:** complete

- [x] 用户可以创建多个 Provider，并在每个 Provider 下手工创建多个 Model Slot。
- [x] Provider 和 Model 使用稳定 Gum UUID；编辑名称、Base URL 或 Provider Model ID 不改变 UUID。
- [x] 每层最多一个显式 default；没有显式 default 时从未删除项按 created time、UUID 升序选择第一个。
- [x] 删除显式 default 后，同一规则产生新的有效 default。
- [x] 没有可用 Provider 或 Model 时，resolver 返回用户可理解的设置 Diagnostic。
- [x] SQLite 只保存 API Key 引用，不保存明文 Secret。
- [x] 当前实现没有 `/models`、Capability 目录、position 或 enable/disable。

## Comments

- 2026-09-01：SQLite Provider/Model Slot、双层 default resolver、Product Application 用例、共享 WorkflowClient、Browser Mock 与 Desktop 通用设置 UI 已完成。Provider/Model 与生成默认值编辑保持 Gum UUID，删除后 resolver 只考虑未删除项；API Key 只接受 Secret reference URI。
- 完整验证通过 `go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...`、24 项前端 WorkflowClient/DOM 合同测试与 `git diff --check`。
- Standards/Spec 双轴审查完成并复核关闭全部硬性发现：Model Slot 补齐生成默认值；SQLite 与 Browser Mock mutation 均返回真实 effective default；Browser Mock 对相同 created time 使用 UUID tie-break；错误消息符合仓库规范。保留 WorkflowClient 双 Adapter 显式映射与 Provider/Model 分离事务 SQL 两项非阻断 duplication 判断。
