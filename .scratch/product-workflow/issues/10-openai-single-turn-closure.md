# 10: 真实 OpenAI-compatible 单轮闭环

**What to build:** 完成 P10 首个真实产品闭环：用户在 macOS UI 中运行通用 `human-chat(source) -> llm-chat` Workflow，通过非流式 OpenAI-compatible Chat Completions 获得并持久化正式 Conversation Artifact。

**Blocked by:** 08: Revision reuse 与 Run history UI; 09: macOS Keychain Secret Adapter

**Status:** complete

- [x] 用户手工配置 Provider/Model 后可从同一通用 Workflow UI 发起真实协议请求。
- [x] Canonical Conversation、ChatMessage、text ContentPart 和 GenerateRequest 不泄漏 Provider JSON 字段。
- [x] OpenAI-compatible 请求正确映射 instructions、user message、Model ID 和生成参数。
- [x] 完整成功响应后追加恰好一个 assistant text message，并产生正式 Conversation Artifact。
- [x] usage、finish reason 和 Provider request ID 进入 Node Run diagnostics/history。
- [x] API Key 和敏感 Header 不进入数据库、Artifact、日志、错误或 golden 输出。
- [x] 协议测试使用本地 fixture server，覆盖 Base URL、认证、限流、malformed response 和取消，不访问真实网络。
- [x] 用户可从 History UI 查看真实 Run、Node Run 和 Conversation Artifact。
- [x] Provider dialect 由 SQLite 设置固定进 Run Snapshot，并在真实 Application seam 分别验证 developer/system 映射。
- [x] 用户从通用 Workflow UI 为 authored `human-chat(source)` 提交本轮 text，StartRun 不再使用硬编码消息。
- [x] 当前 Run 和 History UI 可查看 usage、finish reason 和 Provider request ID。
- [x] 任意格式的 API Key canary 即使被 Provider 错误回显，也不进入返回错误、数据库、Artifact 或 View。

## Comments

- 2026-09-02：复查 `71257f0..e66f4c5` 后重新打开本票。已确认原实现只在 Adapter 单测直接设置 system dialect，产品 Provider 始终使用 developer；StartRun 仍使用固定 user text；成功 diagnostics 虽已持久化，DOM 未渲染；Secret 防御只覆盖 `sk-*` 形状。这四项都属于已批准 P10 spec 的未完成闭环，按公共 Application/WorkflowClient/Protocol Adapter seam 以 TDD 补齐。
- 2026-09-02：补缺实现完成。schema v10 为 Provider 增加 developer/system dialect，旧 Provider 默认 developer，选择在 StartRun 固定进 Run Snapshot 并通过每次 `chat.Connection` 传入 Adapter，不使用全局可变设置。通用 DOM 根据 `llm-chat.inputs.conversation` 的真实 Data Edge 找到 authored Human Source，把本轮用户原文（只 trim 判空，不改写内容）经 WorkflowClient/Application 送入 Conversation Artifact。Node Run diagnostics 改为保持原 JSON tags 的 typed View，并在当前 Run/History DOM 渲染。OpenAI Adapter 在持有本次实际 API Key 的协议边界精确脱敏 Provider message，未知 Adapter 错误保持完整 `%w` 链且由 Adapter 合同禁止携带凭据。
- 验证：31 项前端合同测试、`go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...`与 `git diff --check` 通过。首次完整 Go 测试中 SQLite 并发 Open 和 fullstack PTY 各有一次时序失败，两者分别重跑通过，随后整套 `go test ./...` 再次通过。Standards/Spec 双轴审查发现的 Browser dialect 负例、事后脱敏错误链、authored source 选择、Browser fixture dialect 和用户文本 trim 问题均已修复并补测试。
- 2026-09-02：本票早期注释中的“Provider 失败零部分状态”只代表原 P10 实现，已由 issue 12/13 批准并落地的生命周期取代：preflight 失败仍不创建 Run；一旦创建 Running Run 并开始真实执行，Structural Error 必须保留 Failed Run、已完成进度与失败 Node Run，重启遗留调用标记 Interrupted/UnknownOutcome。

- 2026-09-01：新增 `internal/chat` 作为 Canonical Conversation/ChatMessage/ContentPart/GenerateRequest 与 ProtocolAdapter seam；`OpenAIChatAdapter` 以注入 `*http.Client` 实现非流式 Chat Completions，URL 用 parser 拼接（尾斜杠、子路径、root 边界），instructions 按 Provider dialect 映射 developer/system，错误归类为 auth/rate-limit/provider/network/malformed-response 的 Structural Error。`internal/product/workflow` 的 Conversation/ChatMessage 改为 canonical 类型别名，领域层不再持有 Provider JSON 字段。
- Product Application 通过 `WithChatAdapter` 注入协议 Adapter（Desktop 默认 OpenAI Adapter），`StartRun` 的单轮执行把 Node config instructions 变为 canonical Instructions、按 Resolved LLM Selection（含 APIKeyRef，经 Secret Adapter 运行时解析，不落库）构造请求，成功后一次性追加 assistant 消息并写正式 Conversation Artifact；失败按 Structural Error 终止且零写入（无 Run/Revision/Artifact 残留），错误文本经脱敏。
- Node Run diagnostics（`product_workflow_node_run.diagnostics_json`，schema v9）持久化 Provider request ID、finish reason 与 usage，Run View 与 History 查询返回同一形态；Browser Mock startRun 改为通过共享 fixture chat Adapter 产生响应与 diagnostics，Key 只在 fixture 断言中出现，不进入任何 View。
- 覆盖测试：Adapter golden 请求体（message 顺序、instructions 映射、model/温度/max_tokens）、Base URL 边界、auth/限流/provider/malformed/cancel/网络错误与完整响应前不产出结果；Application 级 fixture server 单轮闭环（物化 UUID、diagnostics、真实 assistant 文本、重启后历史与数据库/Artifact 无明文 Key）、provider 失败零部分状态、auth 失败脱敏、Secret 解析失败先于任何写入；前端合同测试新增 Browser Mock 真实 fixture 调用与 diagnostics 断言。
