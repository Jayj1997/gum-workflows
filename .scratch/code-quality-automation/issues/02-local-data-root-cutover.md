# 02: Local Data Root cutover

**What to build:** 将 Gum 的产品数据与运行产物切换到用户级 Local Data Root，使新 Run 不再往用户项目中写入 `.workflow`，并让全局历史仍可通过既有查询行为访问。

**Blocked by:** 01: Runtime Paths seam

**Status:** ready-for-human

- [x] Local Data Root 按已确认的优先级解析，且不成为 Workflow 业务字段或 CLI 业务 flag。
- [x] 全局产品库、Run、Node Run、Artifact、日志与工具产物按稳定 ID 组织，不依赖可变 Project/Workflow 名称。
- [x] 新 `run` 在用户项目内不创建、迁移或更新 `.workflow`。
- [x] `history` 从 Local Data Root 查询列表、Run 详情和 Node Run 详情，保持既有 ID/前缀语义。
- [x] `validate` 仍保持零写入和零迁移副作用。
- [x] 新位置与旧项目内位置不双写。
- [x] CLI adapter 端到端测试在临时 Local Data Root 验证上述行为。
