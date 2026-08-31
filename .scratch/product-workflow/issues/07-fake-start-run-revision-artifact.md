# 07: Fake StartRun、Revision 与 Conversation Artifact

**What to build:** 完成 P9 Product Tracer：用户从真实 UI 点击 Run，Application 校验用户看到的 Draft、物化默认 Model UUID、创建或复用 immutable Revision，并用 fake executor 产生 Run、Node Run 和 Conversation Artifact。

**Blocked by:** 05: Input binding 与只读 Workflow Preview; 06: LLM Provider / Model 设置

**Status:** done

- [x] UI 点击 Run 前 flush 已变化 autosave，并传递 expected Draft lock version。
- [x] lock version 冲突时不物化 UUID、不创建 Revision、不创建 Run。
- [x] 空 Model preference 按双层 default 解析并写回 Draft；无默认候选时零写入失败。
- [x] StartRun 基于物化后的 Draft 创建或复用 immutable Revision，并创建独立 Run。
- [x] fake `human-chat(source) -> llm-chat` 产生可在 UI 查看的一次 Conversation Artifact。
- [x] Draft、Revision、Run、Node Run 和 Artifact 任一步失败都不留下用户可见半状态。
- [x] 真正的 Workflow Run 启动后不再回写 Draft 或 Revision。

## Comments

- 2026-09-01：共享 WorkflowClient 新增 `startRun`；Desktop 与 Browser Mock 的 Run 动作都会先 flush autosave，再用最新 expected lock version 调 Product Application。真实 Desktop 使用 Local Data Root 的 SQLite 与 Artifact 目录，UI 展示 fake Run、两次成功 Node Run 和 Conversation 消息。
- StartRun preflight 在 SQLite 原子写链中校验 token、物化默认 Gum Model UUID、创建或复用 semantic-hash Revision、固定不含 Secret 的 Resolved LLM Selection Snapshot，并创建独立 Run。重复运行复用 Revision，但生成新的 Run 与 Artifact 目录。
- 聚焦测试覆盖旧 token、缺失 default、Artifact 写入失败、SQLite 链末端失败回滚和相同 Revision 重用；P9 仍是 deterministic fake executor，不包含真实 LLM、人工输入、运行历史分层 UI、Interrupted 或 Resume。
- 完整验证通过 `go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...`、27 项前端合同测试、`git diff --check` 与 Wails `build -clean`。真实 macOS `.app` smoke 从 UI 创建 Provider/Model 和两节点 Draft，点击 Run 后可见 Model UUID 物化、两次成功 Node Run 与 user/assistant Conversation。
- Standards/Spec 双轴审查及复核完成：文档/依赖图/导出注释、Run Snapshot 完整性、authored Data Edge 执行和 Browser/Desktop Revision 规范化差异均已修复；保留跨语言 Browser Mock 模拟重复与部分字符串领域值两项非阻断判断。
