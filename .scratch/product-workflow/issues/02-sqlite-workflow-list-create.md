# 02: SQLite Workflow list/create

**What to build:** 让用户通过 Desktop UI 创建并查看 SQLite Product Workflow。新产品定义与现有 workflow/v1、CLI history 数据在同一个本地产品数据库中安全共存，但不会隐式导入 YAML 或复用 YAML Workflow identity。

**Blocked by:** 01: macOS Desktop / WorkflowClient tracer

**Status:** ready-for-agent

- [ ] 用户可以在 UI 中创建具有稳定 UUID 和显示名称的 Product Workflow。
- [ ] Workflow 列表在应用重启后保持一致，并按稳定规则展示。
- [ ] Product Workflow 只来自 SQLite，不读取、导入或修改 YAML Workflow。
- [ ] 新 schema migration 可幂等执行，并保留现有定义与 Run history。
- [ ] Browser Mock 与 Desktop Adapter 通过同一 Application 用例完成创建和列表查询。

