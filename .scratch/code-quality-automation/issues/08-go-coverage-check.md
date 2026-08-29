# 08: Go Coverage Check

**What to build:** 交付 `go-coverage-check`，让用户在同一 In-place Project Workspace 上运行 full-scope Go 测试、获得可校验的 statement coverage 和阈值 verdict，且 Gum 产物全部位于 Local Data Root。

**Blocked by:** 07: ScriptNode lifecycle hardening

**Status:** ready-for-human

- [x] Node Contract 为 `code: SourceCode` 到 `result: QualityCheckResult`，Node Instance 只允许配置 statement coverage 最低阈值。
- [x] 默认阈值为 80，非数值或越界配置在完整 Validation 中定位拒绝。
- [x] POSIX Script 禁用 Go test cache，使用 full scope 和 Go JSON 事件，将 coverprofile 写入当前 Node Run 独立 tool-output。
- [x] Result Adapter 验证 profile 存在、完整且可解析，计算 statementCoverage 并应用有效阈值。
- [x] 阈值下为 failed，等于/高于为 passed；无可插桩 statement 为 not-applicable。
- [x] 项目测试/编译失败产生 failed Result，statementCoverage metric 为 unavailable 并记录 reason，不伪造 0%。
- [x] 工具声称成功但 profile 缺失/损坏时返回 Structural Error；单独 toolchain 诊断不得在有效 profile 存在时被简化为“只看 exit code”。
- [x] Result 包含 effective threshold、code Ref、toolchain、metric、findings/原因、日志引用与时间。
- [x] 项目目录不出现 coverprofile 或 Gum 报告文件。

## Comments

- 2026-08-30：实现完成。新增 `go-coverage-check` v1 定义、不可变 POSIX Bundle、完整阈值 Validation、严格 Coverage Result/Adapter、真实 Go profile 与 Engine/Node Run 验收。focused、全量、build、vet 与 race 验证通过，等待人工验收。
- 2026-08-30：双轴 review 的 Spec 侧无发现；Standards 侧发现实现状态文档未同步，已补齐 `AGENTS.md`、`CLAUDE.md`、开发规范目录说明与 domain model。重复 Bundle/Result plumbing 仅记为判断性 smell；当前两个 Adapter 的业务合同不同，本票不为后续 Race/Complexity 提前引入通用抽象。
