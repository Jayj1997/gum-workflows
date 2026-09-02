# 12: Provider 与 Node 错误体验

**What to build:** 让用户从 UI 和 History 中准确理解真实模型调用为什么没有成功。Run 前定义/模型错误不创建 Run；Run 后 Provider 与底层错误是 Structural Error；只有成功响应违反 Node Contract 才是 Interaction Error。

**Blocked by:** 10: 真实 OpenAI-compatible 单轮闭环

**Status:** ready-for-agent

- [x] Run 前 Draft、端口、Model UUID、default 和 Secret 错误定位到具体 Node/字段，且不创建 Run。
- [x] 认证、网络、限流、服务不可用、协议损坏和 Provider 拒绝请求均记录为 Structural Error 并使 Run Failed。
- [ ] Provider 已成功返回但输出违反 Node Contract 时记录为 Interaction Error，Run 保持既有交互性语义。
- [x] 取消或进程中断导致结果不确定时显示 UnknownOutcome，不伪装成功。
- [x] UI 展示脱敏 Provider 错误、Node Run identity、时间和可采取的用户动作。
- [x] 首版不自动重试、切换 Provider 或 fallback Model。

## Comments

- 2026-09-02：开始实施时固定持久化边界：Draft/Preview、Model/default、Secret 解析与 authored source/input 校验都属 preflight，失败不创建 Run；preflight 通过后先原子物化 Draft/Revision/Run Snapshot 并创建 Running Run，再发起 Artifact/Provider 外部工作。执行后错误必须把同一 Run 终结为 Failed，保留已成功 Human Node Run/Conversation Artifact 并记录失败 Agent Node Run；不用“失败零写入”隐藏已发起的真实执行。
- 2026-09-02：Structural/UnknownOutcome 纵向路径已完成。Application 使用 BeginRun → progress → FinalizeRun 持久化生命周期；Provider 失败保留 Failed Run、成功 Human Node Run/source Artifact 与失败 Agent Node Run，并持久化 typed `ExecutionError`。进程内取消使用 `context.WithoutCancel` 完成本地记账，Run 转 Interrupted、Agent Node Run 转 UnknownOutcome。返回错误含已持久化 Run ID/脱敏分类/用户动作，UI 失败后自动刷新 History，历史详情展示 Node Run UUID、时间、错误与动作。剩余未完成项只有“Provider 成功但输出违反 Node Contract”的 Interaction Error 与同 Run Retry；当前 Product 尚无已批准 Retry 用例，本票保持 open，不把它错记为 Structural Failed 或自动新建 Run。
- 2026-09-02：补齐 terminal consistency：Provider 返回成功后若最终 Conversation Artifact 写入失败，Agent Node Run 以 runtime Structural Error 终结为 Failed，已完成 Human/source 进度保留，不允许父 Run 已 Failed 而子 Node Run 永久 Running。已知 Provider 结果后的 SQLite Finalize 使用有界、非取消 context；Artifact metadata 只允许同 ID 且全部字段相同的幂等重复写，其他唯一键/外键冲突必须显式失败。Browser Mock 在同一内存会话也先发布 Running/progress，再转 Failed 或由 workspace reopen 转 Interrupted。
