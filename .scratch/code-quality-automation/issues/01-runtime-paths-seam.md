# 01: Runtime Paths seam

**What to build:** 在不改变当前用户行为的前提下，将数据库、Execution、Artifact、日志与临时产物的路径归属集中到一个可注入的 Runtime Paths seam，使后续 Local Data Root 切换不再依赖分散硬编码路径。

**Blocked by:** None (can start immediately)

**Status:** ready-for-human

- [x] `run`、`history`、Artifact Store 和 Execution 状态使用同一 Runtime Paths 解析结果。
- [x] 路径可在测试中注入，测试不依赖真实 HOME 或开发机目录。
- [x] 未显式注入时，当前平台核心行为与 CLI 结果保持不变。
- [x] `validate` 仍不解析或创建任何可写运行路径。
- [x] 现有完整测试、Race 与 Vet 保持通过。
