# 15: 日志脱敏与 Crash diagnostics

**What to build:** 让用户在 Provider、Node 或应用崩溃后获得足够诊断信息，同时确保 API Key、敏感 Header 和不应导出的 Artifact 内容不进入日志或 Crash bundle。

**Blocked by:** 12: Provider 与 Node 错误体验; 13: 重启后的 Run 与 Artifact 查询

**Status:** ready-for-agent

- [ ] Node Run 日志包含 Run/Node Run identity、阶段、latency、Provider request ID 和脱敏错误。
- [ ] Authorization、API Key、Secret 引用解析值和敏感 Header 在所有日志路径中被移除。
- [ ] Crash bundle 可以包含 schema version、应用版本、Run/Node Run 摘要和安全日志引用。
- [ ] Crash bundle 默认不包含 Prompt、Conversation body 或其他 Artifact 本体。
- [ ] 用户能在 UI 中看到 bundle 内容边界并显式生成诊断包。
- [ ] 自动测试使用已知 Secret canary 验证数据库、日志、错误、Artifact 和 bundle 均无泄漏。

