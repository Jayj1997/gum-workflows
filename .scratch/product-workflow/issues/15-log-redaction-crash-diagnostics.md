# 15: 日志脱敏与 Crash diagnostics

**What to build:** 让用户在 Provider、Node 或应用崩溃后获得足够诊断信息，同时确保 API Key、敏感 Header 和不应导出的 Artifact 内容不进入日志或 Crash bundle。

**Blocked by:** 12: Provider 与 Node 错误体验; 13: 重启后的 Run 与 Artifact 查询

**Status:** complete

- [x] Node Run 日志包含 Run/Node Run identity、阶段、latency、Provider request ID 和脱敏错误。
- [x] Authorization、API Key、Secret 引用解析值和敏感 Header 在所有日志路径中被移除。
- [x] Crash bundle 可以包含 schema version、应用版本、Run/Node Run 摘要和安全日志引用。
- [x] Crash bundle 默认不包含 Prompt、Conversation body 或其他 Artifact 本体。
- [x] 用户能在 UI 中看到 bundle 内容边界并显式生成诊断包。
- [x] 自动测试使用已知 Secret canary 验证数据库、日志、错误、Artifact 和 bundle 均无泄漏。

## Comments

- 2026-09-03：实施完成。新增 `internal/redaction` 共享脱敏 seam（注册的 Secret canary 精确子串替换 + Authorization/Proxy-Authorization/Cookie/Set-Cookie/X-API-Key Header 结构化移除，长前缀优先），Product Application 在 StartRun 解析 Provider API Key 时注册该值；`internal/product` 为每个 Run 打开 `runs/<run-id>/logs/run.log`（JSON 行、schema `productRunLog/v1`），Node Run start/finish 行携带 run/node-run identity、phase、latencyMs、providerRequestId 与脱敏错误，Run 级事件记录 preflight-failed/run-succeeded/run-failed/run-interrupted。日志打开失败降级为丢弃 handler，不阻塞 Run——SQLite 历史仍是持久事实。
- 2026-09-03：Crash bundle 由 `GenerateDiagnosticsBundle(runID)` 显式生成于 `runs/<run-id>/diagnostics/`：`manifest.json`（bundle schema `productDiagnosticsBundle/v1`、app version、product schema version、Run/Node Run 摘要含 identity/phase/latency/typed error/ArtifactRef 文本、includes/excludes 边界声明）+ 脱敏 run.log 副本（防御性二次脱敏）。Artifact 仅以 `Kind:ID:Version` 引用文本出现，Prompt/Conversation body 与 Secret 明文永不进入 bundle。UI（Desktop Adapter、WorkflowClient、Browser Mock、DOM View）暴露 generateDiagnosticsBundle，历史面板显示内容边界与显式生成按钮及产物路径。
- 2026-09-03：验收测试：已知 canary（Provider 错误体回显 secret 的最坏情形）扫描 SQLite、run 目录（日志+Artifact）、GetRunHistory 视图与 bundle 全部无泄漏；run log 契约断言 identity/phase/latency/requestID；bundle 边界测试断言 Prompt 标记文本与 Conversation body 不出现；未知 Run 报错。appVersion 暂为 `0.0.0-dev`，真实构建版本号属 issue 16。
