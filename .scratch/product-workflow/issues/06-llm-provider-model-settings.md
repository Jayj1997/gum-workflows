# 06: LLM Provider / Model 设置

**What to build:** 让用户在 UI 中创建和编辑 `Provider -> Models` 设置，包括稳定 Gum UUID、可编辑 Provider Model ID、Secret 引用和双层 default。首版不依赖 `/models`、Capability、排序或 enable/disable。

**Blocked by:** 02: SQLite Workflow list/create

**Status:** ready-for-agent

- [ ] 用户可以创建多个 Provider，并在每个 Provider 下手工创建多个 Model Slot。
- [ ] Provider 和 Model 使用稳定 Gum UUID；编辑名称、Base URL 或 Provider Model ID 不改变 UUID。
- [ ] 每层最多一个显式 default；没有显式 default 时从未删除项按 created time、UUID 升序选择第一个。
- [ ] 删除显式 default 后，同一规则产生新的有效 default。
- [ ] 没有可用 Provider 或 Model 时，resolver 返回用户可理解的设置 Diagnostic。
- [ ] SQLite 只保存 API Key 引用，不保存明文 Secret。
- [ ] 当前实现没有 `/models`、Capability 目录、position 或 enable/disable。

