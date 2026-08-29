# 10: Go Cyclomatic Complexity Check

**What to build:** 交付 `go-complexity-check`，使用固定 POSIX Script 运行内嵌 Go AST Analyzer，让用户获得单函数圈复杂度门禁与可定位 findings，而不需要另外安装 gocyclo。

**Blocked by:** 07: ScriptNode lifecycle hardening

**Status:** ready-for-agent

- [ ] Node Contract 为 `code: SourceCode` 到 `result: QualityCheckResult`。
- [ ] Node Instance config 只包含单函数上限（默认 15）、是否包含测试（默认 false）和是否排除 generated files（默认 true）；vendor 始终排除。
- [ ] 配置类型/范围错误在完整 Validation 中定位拒绝。
- [ ] Script Bundle 内含仅使用 Go 标准库的 Analyzer 源码，通过用户 PATH 中的 Go 运行，不引入运行时下载。
- [ ] Analyzer 产出固定结构化工具产物；Result Adapter 独立应用阈值和排除策略。
- [ ] 所有函数低于/等于阈值为 passed，任一函数超过为 failed，无可分析函数为 not-applicable。
- [ ] 项目源文件语法错误产生 failed finding；Analyzer 无法运行或产物损坏产生 Structural Error。
- [ ] Result 包含 maxCyclomaticComplexity、functionsAnalyzed、functionsOverThreshold、超阈值函数位置和 effective config。
- [ ] 表驱动测试覆盖阈值边界、多函数、排除规则、无函数、语法错误与产物损坏。

