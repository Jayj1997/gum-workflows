# 07: ScriptNode lifecycle hardening

**What to build:** 将已打通的 ScriptNode 执行链加固为可取消、有磁盘保护、可诊断且不会在失败时发布伪完整 Result 的通用内置 automation 执行能力。

**Blocked by:** 06: Static Analysis tracer bullet

**Status:** ready-for-human

- [x] Run 前诊断当前平台、POSIX Shell、required executable 和重要工具能力；不支持的 Windows 环境不尝试运行。
- [x] Runtime 在执行前与执行后验证 Manifest/Registry/Bundle 摘要与工具产物路径约束。
- [x] Context 取消终止 Shell 及其子进程组，Workflow 停止后不留下后台进程。
- [x] stdout/stderr 使用固定磁盘保护上限；超限立即终止执行并返回 Structural Error。
- [x] 日志超限、取消、I/O 失败、Manifest 不匹配或 Adapter 失败都不发布 Quality Check Result。
- [x] Node Run 只保存脚本/摘要、cwd/位置参数、Go 路径/版本、GOROOT、GOOS/GOARCH/CGO 与日志等非敏感诊断，不持久化完整环境。
- [x] Node Run 结束后删除非持久 tool-output，已引用日志/Result 仍可查询。
- [x] 本票不引入自动 timeout，只验收 Context 取消与日志资源保护。
- [x] 进程组、日志超限、敏感环境不落盘和残留产物清理测试通过。

## Comments

- 2026-08-29：实现完成。ScriptNode 增加运行前主机/Go 能力探测、执行前后内存与磁盘 Bundle 校验、严格产物路径检查、POSIX 进程组取消、stdout/stderr 合计 32 MiB 固定上限与临时 tool-output 清理；失败路径不调用 Result Adapter 或发布 Result。focused、全量、vet 与 race 验证结果见实现提交，等待人工验收。
