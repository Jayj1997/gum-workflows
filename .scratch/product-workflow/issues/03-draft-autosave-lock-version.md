# 03: Draft autosave 与 lock-version CAS

**What to build:** 让用户编辑 Workflow 的唯一可变 Draft，并获得可靠 autosave。内容不变时不写入；内容变化时更新同一 Draft；旧页面不能覆盖较新内容；非法中间态仍可保存并返回 Diagnostics。

**Blocked by:** 02: SQLite Workflow list/create

**Status:** complete

- [x] 每个 Workflow 同时只有一个可变 Draft。
- [x] 规范化语义内容没有变化时 autosave 为 no-op，不更新时间或 lock version。
- [x] 内容变化时 UpdateDraft 使用 expected lock version 更新同一 Draft 并递增 token。
- [x] token 冲突不覆盖数据库内容，并向 UI 返回最新 Draft 与刷新提示。
- [x] 非法 Draft 可以保存，同时返回完整 Preview/Diagnostics 结果形态。
- [x] Autosave 不创建 Workflow Revision 或历史 Draft 副本。

## Comments

**2026-08-31（agent 实施记录）：** 新增每个 Product Workflow 唯一的 `product_workflow_draft` 行，新建 Workflow 与初始 Draft 在同一事务写入，v5 数据库升级会为既有 Product Workflow 回填 Draft。Application / Repository 按规范化 JSON 比较语义内容；no-op 保留 `updated_at` 与 token，变化更新同一行并递增 `lock_version`，旧 token 返回数据库最新 Draft、Preview/Diagnostics 与刷新提示而不覆盖。Desktop 与 Browser Mock 共享 `getDraft` / `updateDraft` WorkflowClient，当前通用 JSON 编辑器以 250ms 去抖并串行 autosave；非法语义 Draft 可持久化并聚合基础 schema/nodes Diagnostics。Node、端口与 config 的具体 Preview/Diagnostics 继续由 04–05 实现；本票没有 Revision 表或 Draft 历史副本。

验证：`go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...` 与前端 WorkflowClient/DOM 合同测试全绿。真实 Browser Mock 交互验证覆盖 Workflow 创建、Draft 选择、非法 `{}` autosave 和聚合 Diagnostics；code review 后新增连续编辑串行化、较新 token 传递、旧响应不覆盖新文本以及真实 250ms debounce 回归。
