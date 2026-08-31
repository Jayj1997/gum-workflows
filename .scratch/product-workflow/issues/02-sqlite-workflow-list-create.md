# 02: SQLite Workflow list/create

**What to build:** 让用户通过 Desktop UI 创建并查看 SQLite Product Workflow。新产品定义与现有 workflow/v1、CLI history 数据在同一个本地产品数据库中安全共存，但不会隐式导入 YAML 或复用 YAML Workflow identity。

**Blocked by:** 01: macOS Desktop / WorkflowClient tracer

**Status:** complete

- [x] 用户可以在 UI 中创建具有稳定 UUID 和显示名称的 Product Workflow。
- [x] Workflow 列表在应用重启后保持一致，并按稳定规则展示。
- [x] Product Workflow 只来自 SQLite，不读取、导入或修改 YAML Workflow。
- [x] 新 schema migration 可幂等执行，并保留现有定义与 Run history。
- [x] Browser Mock 与 Desktop Adapter 通过同一 Application 用例完成创建和列表查询。

**2026-08-31（agent 实施记录）：** 新增独立 `product_workflow` schema 与按 `(created_at ASC, id ASC)` 的稳定列表顺序，避免复用 workflow/v1 的 `workflow` 表或身份。真实 macOS Desktop 通过 Local Data Root 的 `product.db` 组装 Product Application；Desktop Adapter 与 Browser Mock 的 WorkflowClient 均使用 `createWorkflow` / `listWorkflows`，产品壳可提交显示名称并渲染 UUID 列表。迁移测试覆盖 v4 数据库升级、旧定义和 Run history 保留、重复 Open 幂等，以及 YAML workflow/v1 导入不进入 Product Workflow 列表。
