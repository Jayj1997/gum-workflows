# 14: Product schema upgrade fixtures

**What to build:** 让安装了早期 Product Tracer 或首闭环版本的用户安全升级到完整 P11 schema。升级保留 Workflow、Draft、Revision、Provider/Model、Run 和 Artifact，并在失败时保持旧库可恢复。

**Blocked by:** 11: 删除 Model 后的悬空 UUID 诊断; 13: 重启后的 Run 与 Artifact 查询

**Status:** ready-for-agent

- [ ] 每个已发布 product schema version 都有代表性旧库 fixture 和顺序 upgrade 测试。
- [ ] 升级保留 Draft lock version、Revision hash、Model Slot UUID 和历史 Run Snapshot。
- [ ] 删除状态、显式 default 和 created-time fallback 在升级后保持确定行为。
- [ ] migration 重放幂等，不复制 Revision、Run、Node Run 或 Artifact 元数据。
- [ ] migration 失败不发布半迁移 schema 或半可见历史。
- [ ] workflow/v1 现有定义与 Run history 在 Product schema 升级后仍通过原有查询和测试。

