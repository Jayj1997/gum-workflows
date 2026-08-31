# 04: Node Catalog 与通用 Config Schema 表单

**What to build:** 让用户从 Node Catalog 添加 `human-chat` 和 `llm-chat` Node Instance，并由 Gum Config Schema 生成通用配置表单。该路径证明产品在创作 Workflow，而不是显示一个硬编码聊天页面。

**Blocked by:** 03: Draft autosave 与 lock-version CAS

**Status:** ready-for-agent

- [ ] Catalog 通过 Node Definition/Executor registry 展示首批两个 Node，而非在 UI 中硬编码合同。
- [ ] 用户可以添加、选择、重命名和移除 Node Instance，Node ID 与 Definition identity 分离。
- [ ] `llm-chat` 的 instructions、temperature 和 max output tokens 由通用 Config Schema 生成表单。
- [ ] Config Contract 支持首版字段类型、required/default、范围、枚举和 sensitive 标记。
- [ ] Presentation Hint 可以改变 label/help/editor，但不改变验证或运行语义。
- [ ] 非法 config 作为 Draft Diagnostic 返回，并定位到具体 Node 和字段。

