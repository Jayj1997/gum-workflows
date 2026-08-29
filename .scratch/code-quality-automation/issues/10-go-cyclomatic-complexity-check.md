# 10: Go Cyclomatic Complexity Check

**What to build:** 交付 `go-complexity-check`，使用固定 POSIX Script 运行内嵌 Go AST Analyzer，让用户获得单函数圈复杂度门禁与可定位 findings，而不需要另外安装 gocyclo。

**Blocked by:** 07: ScriptNode lifecycle hardening

**Status:** ready-for-human

- [x] Node Contract 为 `code: SourceCode` 到 `result: QualityCheckResult`。
- [x] Node Instance config 只包含单函数上限（默认 15）、是否包含测试（默认 false）和是否排除 generated files（默认 true）；vendor 始终排除。
- [x] 配置类型/范围错误在完整 Validation 中定位拒绝。
- [x] Script Bundle 内含仅使用 Go 标准库的 Analyzer 源码，通过用户 PATH 中的 Go 运行，不引入运行时下载。
- [x] Analyzer 产出固定结构化工具产物；Result Adapter 独立应用阈值和排除策略。
- [x] 所有函数低于/等于阈值为 passed，任一函数超过为 failed，无可分析函数为 not-applicable。
- [x] 项目源文件语法错误产生 failed finding；Analyzer 无法运行或产物损坏产生 Structural Error。
- [x] Result 包含 maxCyclomaticComplexity、functionsAnalyzed、functionsOverThreshold、超阈值函数位置和 effective config。
- [x] 表驱动测试覆盖阈值边界、多函数、排除规则、无函数、语法错误与产物损坏。

## Comments

- 2026-08-30：实现完成。新增 `go-complexity-check` v1 定义、不可变 POSIX Bundle、内嵌标准库 Go AST Analyzer、完整 config Validation、严格 Complexity Result/Adapter，以及真实 Bundle 与 Engine/Node Run 验收；等待人工验收。
- 2026-08-30：双轴 review 发现并修复原始文件树遍历与声明的 Go `./...` package scope 不一致、嵌套 vendor 风险，以及 package-level 匿名函数中的嵌套匿名函数漏计；Analyzer 改用用户 Go 的 `go list -e -json ./...` 确定源文件成员，复核无剩余发现。
