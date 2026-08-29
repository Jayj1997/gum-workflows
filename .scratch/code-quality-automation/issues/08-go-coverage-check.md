# 08: Go Coverage Check

**What to build:** 交付 `go-coverage-check`，让用户在同一 In-place Project Workspace 上运行 full-scope Go 测试、获得可校验的 statement coverage 和阈值 verdict，且 Gum 产物全部位于 Local Data Root。

**Blocked by:** 07: ScriptNode lifecycle hardening

**Status:** ready-for-agent

- [ ] Node Contract 为 `code: SourceCode` 到 `result: QualityCheckResult`，Node Instance 只允许配置 statement coverage 最低阈值。
- [ ] 默认阈值为 80，非数值或越界配置在完整 Validation 中定位拒绝。
- [ ] POSIX Script 禁用 Go test cache，使用 full scope 和 Go JSON 事件，将 coverprofile 写入当前 Node Run 独立 tool-output。
- [ ] Result Adapter 验证 profile 存在、完整且可解析，计算 statementCoverage 并应用有效阈值。
- [ ] 阈值下为 failed，等于/高于为 passed；无可插桩 statement 为 not-applicable。
- [ ] 项目测试/编译失败产生 failed Result，statementCoverage metric 为 unavailable 并记录 reason，不伪造 0%。
- [ ] 工具声称成功但 profile 缺失/损坏时返回 Structural Error；单独 toolchain 诊断不得在有效 profile 存在时被简化为“只看 exit code”。
- [ ] Result 包含 effective threshold、code Ref、toolchain、metric、findings/原因、日志引用与时间。
- [ ] 项目目录不出现 coverprofile 或 Gum 报告文件。

