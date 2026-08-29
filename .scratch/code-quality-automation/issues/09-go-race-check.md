# 09: Go Race Check

**What to build:** 交付 `go-race-check`，让用户在当前项目环境中运行 full-scope Go Race Detector，获得“本次是否观察到 race”的诚实结构化结果。

**Blocked by:** 07: ScriptNode lifecycle hardening

**Status:** ready-for-human

- [x] Node Contract 为 `code: SourceCode` 到 `result: QualityCheckResult`，不公开业务 config。
- [x] Run 前诊断 GOOS/GOARCH、CGO 与 C 编译器等 Race Requirement，不满足时返回 Structural Error。
- [x] POSIX Script 使用 full scope、禁用 Go test cache，并产生 Go JSON/Race 诊断日志。
- [x] 未观察到 race 且测试成功时产生 passed，报告不声称项目“无数据竞争”。
- [x] 观察到 race、普通测试失败或项目编译失败产生 failed Result，并保留可诊断 finding/日志。
- [x] 无 Go package 产生 not-applicable；工具无法启动、协议/日志无法解析产生 Structural Error。
- [x] racesDetected metric、code Ref、toolchain、findings、日志引用和时间满足 Result Schema。
- [x] 用 fixture 覆盖未观察/观察 race、测试/编译失败、Requirement 失败与非法产物。

## Comments

- 2026-08-30：实现完成。新增 `go-race-check` v1 定义、不可变 POSIX Bundle、运行前 Race Requirement 诊断、严格 Race Result/Adapter 与 Engine/Node Run 验收；Result 只报告本次执行观察到的 race，不作全局无竞态承诺。
