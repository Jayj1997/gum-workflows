# 03: Draft autosave 与 lock-version CAS

**What to build:** 让用户编辑 Workflow 的唯一可变 Draft，并获得可靠 autosave。内容不变时不写入；内容变化时更新同一 Draft；旧页面不能覆盖较新内容；非法中间态仍可保存并返回 Diagnostics。

**Blocked by:** 02: SQLite Workflow list/create

**Status:** ready-for-agent

- [ ] 每个 Workflow 同时只有一个可变 Draft。
- [ ] 规范化语义内容没有变化时 autosave 为 no-op，不更新时间或 lock version。
- [ ] 内容变化时 UpdateDraft 使用 expected lock version 更新同一 Draft 并递增 token。
- [ ] token 冲突不覆盖数据库内容，并向 UI 返回最新 Draft 与刷新提示。
- [ ] 非法 Draft 可以保存，同时返回完整 Preview/Diagnostics 结果形态。
- [ ] Autosave 不创建 Workflow Revision 或历史 Draft 副本。

