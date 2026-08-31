# 10: 真实 OpenAI-compatible 单轮闭环

**What to build:** 完成 P10 首个真实产品闭环：用户在 macOS UI 中运行通用 `human-chat(source) -> llm-chat` Workflow，通过非流式 OpenAI-compatible Chat Completions 获得并持久化正式 Conversation Artifact。

**Blocked by:** 08: Revision reuse 与 Run history UI; 09: macOS Keychain Secret Adapter

**Status:** ready-for-agent

- [ ] 用户手工配置 Provider/Model 后可从同一通用 Workflow UI 发起真实协议请求。
- [ ] Canonical Conversation、ChatMessage、text ContentPart 和 GenerateRequest 不泄漏 Provider JSON 字段。
- [ ] OpenAI-compatible 请求正确映射 instructions、user message、Model ID 和生成参数。
- [ ] 完整成功响应后追加恰好一个 assistant text message，并产生正式 Conversation Artifact。
- [ ] usage、finish reason 和 Provider request ID 进入 Node Run diagnostics/history。
- [ ] API Key 和敏感 Header 不进入数据库、Artifact、日志、错误或 golden 输出。
- [ ] 协议测试使用本地 fixture server，覆盖 Base URL、认证、限流、malformed response 和取消，不访问真实网络。
- [ ] 用户可从 History UI 查看真实 Run、Node Run 和 Conversation Artifact。
