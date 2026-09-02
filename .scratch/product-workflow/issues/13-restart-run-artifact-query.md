# 13: 重启后的 Run 与 Artifact 查询

**What to build:** 让本地历史经得起应用重启。已完成 Run、Node Run、Conversation Artifact 和错误继续可查；进程退出时未结束的 Run 被标记为当前不可恢复的 Interrupted，而不是成功或自动恢复。

**Blocked by:** 10: 真实 OpenAI-compatible 单轮闭环

**Status:** complete

- [x] 已完成 Run 在关闭并重新打开应用后仍出现在 Workflow History。
- [x] Node Run inputs/outputs、Resolved LLM Selection、错误和 Conversation Artifact 可完整查询。
- [x] 启动时发现未结束 Run 会把其持久状态标记为 Interrupted。
- [x] Interrupted Run 的已成功 Node Run 和 Artifact 保持不变。
- [x] UI 明确显示当前版本不可 Resume，且不会自动重放任何 Node Run。
- [x] 重启协调不依赖 append-only Run Event Log 或 replay。

## Comments

- 2026-09-02：与 Ticket 12 共用同一生命周期边界：应用首次 OpenWorkspace 时原子把数据库中遗留的 Running Run 标记为 Interrupted，不重放任何 Node Run；已成功 Node Run/Artifact 只读保留。Interrupted 在当前版本是可查询、不可 Resume 的非成功状态。
- 2026-09-02：实施完成。Product Application 每个进程首次 OpenWorkspace 调用 Recovery Repository，在同一 SQLite 事务把遗留 Running Run 标记 Interrupted、in-flight Agent Node Run 标记 UnknownOutcome，写入安全错误与当前版本不可 Resume 的用户动作。BeginRun 后会在 Provider 调用前持久化成功 Human Node Run、source Artifact 和 running Agent Node Run，因此重启协调只改状态而不删除已成功进度。旧 Provider 结果即使随后返回，Finalize 的 `WHERE status = 'running'` CAS 也不允许它覆盖 Interrupted。历史 View 返回 Node Run inputs/outputs、Resolved LLM Selection、typed error/diagnostics、时间与 Conversation Artifact；全程无 Event Log、replay 或自动重放。
- 2026-09-02：恢复协调的当前部署边界是 macOS 单进程、单窗口拥有一个 Local Data Root。并发进程打开同一数据库尚无 owner lease，因此不受支持；如果未来要支持多窗口/多进程，必须先设计 Run owner/lease，不能让新 Application 把仍由另一进程执行的 Run 误判为遗留状态。Browser Mock 只在同一内存会话复现状态转换，不承诺刷新后的磁盘恢复。
