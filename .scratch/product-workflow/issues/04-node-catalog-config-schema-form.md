# 04: Node Catalog 与通用 Config Schema 表单

**What to build:** 让用户从 Node Catalog 添加 `human-chat` 和 `llm-chat` Node Instance，并由 Gum Config Schema 生成通用配置表单。该路径证明产品在创作 Workflow，而不是显示一个硬编码聊天页面。

**Blocked by:** 03: Draft autosave 与 lock-version CAS

**Status:** complete

- [x] Catalog 通过 Node Definition/Executor registry 展示首批两个 Node，而非在 UI 中硬编码合同。
- [x] 用户可以添加、选择、重命名和移除 Node Instance，Node ID 与 Definition identity 分离。
- [x] `llm-chat` 的 instructions、temperature 和 max output tokens 由通用 Config Schema 生成表单。
- [x] Config Contract 支持首版字段类型、required/default、范围、枚举和 sensitive 标记。
- [x] Presentation Hint 可以改变 label/help/editor，但不改变验证或运行语义。
- [x] 非法 config 作为 Draft Diagnostic 返回，并定位到具体 Node 和字段。

## Comments

- 2026-08-31：实现与文档同步完成。验证通过 `go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...` 与前端 WorkflowClient 合同测试。
- Standards/Spec 双轴审查完成：Spec 无剩余发现；Standards 无硬性违规，保留 Go/Desktop 与独立 Browser Mock 跨语言 fixture、双静态 HTML 入口两项非阻断 duplication 判断，避免为本票引入生成或模板构建系统。
