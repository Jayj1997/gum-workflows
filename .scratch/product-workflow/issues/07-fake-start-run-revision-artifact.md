# 07: Fake StartRun、Revision 与 Conversation Artifact

**What to build:** 完成 P9 Product Tracer：用户从真实 UI 点击 Run，Application 校验用户看到的 Draft、物化默认 Model UUID、创建或复用 immutable Revision，并用 fake executor 产生 Run、Node Run 和 Conversation Artifact。

**Blocked by:** 05: Input binding 与只读 Workflow Preview; 06: LLM Provider / Model 设置

**Status:** ready-for-agent

- [ ] UI 点击 Run 前 flush 已变化 autosave，并传递 expected Draft lock version。
- [ ] lock version 冲突时不物化 UUID、不创建 Revision、不创建 Run。
- [ ] 空 Model preference 按双层 default 解析并写回 Draft；无默认候选时零写入失败。
- [ ] StartRun 基于物化后的 Draft 创建或复用 immutable Revision，并创建独立 Run。
- [ ] fake `human-chat(source) -> llm-chat` 产生可在 UI 查看的一次 Conversation Artifact。
- [ ] Draft、Revision、Run、Node Run 和 Artifact 任一步失败都不留下用户可见半状态。
- [ ] 真正的 Workflow Run 启动后不再回写 Draft 或 Revision。

