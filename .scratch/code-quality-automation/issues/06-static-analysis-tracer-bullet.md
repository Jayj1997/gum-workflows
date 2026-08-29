# 06: Static Analysis tracer bullet

**What to build:** 以 `go-static-analysis` 为第一个真实 tracer bullet，端到端打通 ScriptNode、Automation Script Manifest、不可变 POSIX Script Bundle、流式日志、内置 Result Adapter、Quality Check Result、Artifact 与 History。

**Blocked by:** 05: Workflow Context Binding and `code` contract

**Status:** ready-for-human

- [x] `go-static-analysis` 作为 automation Node Definition/Executor 注册，合同为 `code: SourceCode` 到 `result: QualityCheckResult`。
- [x] `automationScript/v1` Manifest 严格声明 Node/Executor、POSIX 入口、Darwin/Linux、Go Requirement、正式工具产物和 Result Adapter ID。
- [x] Script Bundle 内容随 Executor Version 不可变，摘要进入 Run/Node Run 诊断。
- [x] Shell 只使用用户 PATH/Go 配置运行 full-scope `go vet` JSON 诊断，不读取业务阈值，不接受 Node Instance 脚本覆盖。
- [x] Project Workspace 与 Node Run tool-output 目录通过固定位置参数传入，不注入 Gum 专用环境变量。
- [x] stdout/stderr 流式写入 Local Data Root 的 Node Run 日志；额外 print/echo 不会创建或修改 result。
- [x] `qualityCheckResult/v1` 严格校验 Static Result 的 verdict、code Ref、effective config、toolchain、findingsCount、findings、日志引用与时间。
- [x] 无诊断产生 passed；vet/package 诊断产生 failed；无 Go package 产生 not-applicable；工具/产物/Schema 故障产生 Structural Error。
- [x] Result 作为成功 Node Run Artifact 进入历史，可由下游消费或单独查看。
- [x] POSIX Script 合同测试、Result Adapter fixture、Validation、Engine 与 History 测试全部通过。

## Comments

- 2026-08-29：实现完成；真实 `go vet -json` stderr 行为已在双轴审查中复现并修复，structured finding、Bundle/host diagnostics 与 History evidence 均补齐。`go test ./...`、`go vet ./...`、`go test -race ./...` 通过，等待人工验收。进程组取消与日志磁盘上限按既定范围留在票 07。
