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

## Comments

- 2026-09-01：新增 `internal/chat` 作为 Canonical Conversation/ChatMessage/ContentPart/GenerateRequest 与 ProtocolAdapter seam；`OpenAIChatAdapter` 以注入 `*http.Client` 实现非流式 Chat Completions，URL 用 parser 拼接（尾斜杠、子路径、root 边界），instructions 按 Provider dialect 映射 developer/system，错误归类为 auth/rate-limit/provider/network/malformed-response 的 Structural Error。`internal/product/workflow` 的 Conversation/ChatMessage 改为 canonical 类型别名，领域层不再持有 Provider JSON 字段。
- Product Application 通过 `WithChatAdapter` 注入协议 Adapter（Desktop 默认 OpenAI Adapter），`StartRun` 的单轮执行把 Node config instructions 变为 canonical Instructions、按 Resolved LLM Selection（含 APIKeyRef，经 Secret Adapter 运行时解析，不落库）构造请求，成功后一次性追加 assistant 消息并写正式 Conversation Artifact；失败按 Structural Error 终止且零写入（无 Run/Revision/Artifact 残留），错误文本经脱敏。
- Node Run diagnostics（`product_workflow_node_run.diagnostics_json`，schema v9）持久化 Provider request ID、finish reason 与 usage，Run View 与 History 查询返回同一形态；Browser Mock startRun 改为通过共享 fixture chat Adapter 产生响应与 diagnostics，Key 只在 fixture 断言中出现，不进入任何 View。
- 覆盖测试：Adapter golden 请求体（message 顺序、instructions 映射、model/温度/max_tokens）、Base URL 边界、auth/限流/provider/malformed/cancel/网络错误与完整响应前不产出结果；Application 级 fixture server 单轮闭环（物化 UUID、diagnostics、真实 assistant 文本、重启后历史与数据库/Artifact 无明文 Key）、provider 失败零部分状态、auth 失败脱敏、Secret 解析失败先于任何写入；前端合同测试新增 Browser Mock 真实 fixture 调用与 diagnostics 断言。
