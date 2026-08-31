# 13: 重启后的 Run 与 Artifact 查询

**What to build:** 让本地历史经得起应用重启。已完成 Run、Node Run、Conversation Artifact 和错误继续可查；进程退出时未结束的 Run 被标记为当前不可恢复的 Interrupted，而不是成功或自动恢复。

**Blocked by:** 10: 真实 OpenAI-compatible 单轮闭环

**Status:** ready-for-agent

- [ ] 已完成 Run 在关闭并重新打开应用后仍出现在 Workflow History。
- [ ] Node Run inputs/outputs、Resolved LLM Selection、错误和 Conversation Artifact 可完整查询。
- [ ] 启动时发现未结束 Run 会把其持久状态标记为 Interrupted。
- [ ] Interrupted Run 的已成功 Node Run 和 Artifact 保持不变。
- [ ] UI 明确显示当前版本不可 Resume，且不会自动重放任何 Node Run。
- [ ] 重启协调不依赖 append-only Run Event Log 或 replay。

