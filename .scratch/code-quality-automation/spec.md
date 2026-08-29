# Spec: Code Quality Check Automation

Status: ready-for-agent

## Problem Statement

用户的代码工作流通常已经包含“开发代码、运行若干检查、阅读结果、继续开发”这类重复的人工中间处理。Gum-Workflows 目前只有 Mock automation，无法让用户以可组合 Node 快速引入专业代码质量检查，也无法用同一能力验证 Gum-Workflows 自身。

现有平台核心会将 Project 复制到项目内 `.workflow` 目录，并将数据库、Workspace、Artifact 和日志分散在用户项目中。这与真实的本地开发流程不一致：Agent 对代码的修改应当在用户项目中实时生效，检查也应当直接运行于同一份工作状态；只有 Gum 自身的数据库、日志、工具产物和结果需要移出用户项目。

首批能力需要优先支持 Go，并在不引入任意 Shell 命令、用户自定义脚本、每 Node 工作区副本、内部代码 Revision 或自动代码恢复的前提下，保持 Node Definition 的固定 Input/Output Contract、Executor Version 的不可变行为和可追溯的 Node Run 结果。

## Solution

产品将预制四个独立的 Go Code Quality Check Node：并发与竞态检查、静态分析、测试覆盖率和圈复杂度。它们全部是 automation Node，消费 `code: SourceCode`，在用户的 In-place Project Workspace 上按正常 Workflow Graph 调度执行，并统一产出 `result: QualityCheckResult`。

每个内置 Node Executor Version 固定一份不可变 Automation Script Bundle。Bundle 只包含一份 Darwin/Linux 共用的 POSIX Shell 脚本和必要的内嵌辅助源码。Shell 只负责调用用户 PATH 中的固定工具并生成正式工具产物；stdout/stderr 始终是日志。Gum 内置 Result Adapter 独立解析退出码、日志和工具产物，生成通过版本化 Schema 校验的 Quality Check Result。

Gum 的产品数据统一进入用户级 Local Data Root。Project Definition 指向的用户项目目录直接成为 In-place Project Workspace：Agent 修改实时生效，多个质量节点在同一目录上按图正常调度，每个 Node Run 只拥有独立的日志和 tool-output 目录。Gum 不复制用户项目、不保存源码快照、不创建内部代码 Revision，也不承担代码恢复。

## User Stories

1. As a Go 项目开发者, I want 从 Node Catalog 选择预制质量检查, so that 我不需要自己设计脚本协议和报告解析。
2. As a Workflow 作者, I want Race、Static Analysis、Coverage 和 Complexity 是四个独立 Node Definition, so that 我可以自由选择、连接或省略任意检查。
3. As a Workflow 作者, I want 通过选择 Go 专用 Node 明确语言, so that 我不需要在每个 Node Instance 重复填写 language。
4. As a 代码开发 Workflow 作者, I want 将 backend.code 绑定到质量节点的 code input, so that 每次成功开发都触发新一轮检查。
5. As a 只想检查当前项目的用户, I want 通过 project.code 获得 Project Workspace, so that 我不需要伪开发 Node 或 Snapshot Node。
6. As a Coding Agent Node 开发者, I want 成功 Node Run 发布新版本 code Artifact, so that 新一轮代码完成成为明确验证触发点。
7. As a Workflow 作者, I want 端口名为 code, so that 用户不会将 source-code 误解为修改前代码。
8. As a 本地开发者, I want Agent 直接在我的项目目录修改代码, so that 修改实时可见且可由我的 Git/项目工具管理。
9. As a 本地开发者, I want 四个检查直接在同一 Project Workspace 运行, so that 它们使用我实际的依赖与工作状态。
10. As a 大型项目用户, I want Gum 不为检查复制项目或 node_modules, so that 自动化不会迅速耗尽磁盘。
11. As a Workflow 作者, I want 检查按正常 Graph 并发调度, so that Gum 不添加隐藏串行。
12. As a Workflow 作者, I want 自己决定哪些 Node 消费结果, so that Runtime 不擅自规划后续流程。
13. As a 只需查看报告的用户, I want 无下游消费者的 Result 仍进入历史, so that 我可在 UI 阅读结果。
14. As a 下游 Node 开发者, I want 每个检查只发布 result: QualityCheckResult, so that 我只理解一份不会内部矛盾的输出。
15. As a 下游 Node 开发者, I want Result 具有版本化 Schema, so that 我能可靠读取 verdict、metrics、findings 和 code 引用。
16. As a 下游 Node 开发者, I want 固定指标名, so that 未来条件功能可进行类型化字段引用。
17. As a UI 用户, I want 查看阈值、工具链、指标、finding 和日志, so that 我能理解结果。
18. As a 调试者, I want stdout/stderr 与业务输出分离, so that Shell 的 print/echo 不会伪造 result。
19. As a 调试者, I want stdout/stderr 流式保存, so that 大量输出不会占满内存或丢失。
20. As a Workflow 作者, I want 检查发现代码问题时得到 verdict=failed, so that 业务失败不被当作 Executor 崩溃。
21. As a Workflow 作者, I want 检查无法完成时得到 Structural Error, so that 不完整结果不会伪装成正常 Result。
22. As a Workflow 作者, I want 无可适用对象时得到 not-applicable, so that 其不与失败混淆。
23. As a Go 开发者, I want Race Result 只声称本次执行是否观察到 race, so that 报告不过度承诺。
24. As a Go 开发者, I want Static Result 明确标注 go vet, so that 启发式诊断不被夸大。
25. As a Go 开发者, I want Coverage 报告 statement coverage, so that 指标与 Go 实际语义一致。
26. As a Go 开发者, I want Coverage 默认 80% 且可覆盖, so that 节点开箱即用又能匹配项目标准。
27. As a Go 开发者, I want Complexity 默认单函数上限 15 且可覆盖, so that 复杂函数及时暴露。
28. As a Go 开发者, I want Complexity 默认排除测试、generated files 和 vendor, so that 门禁聚焦普通源码。
29. As a Node Executor 发布者, I want Executor Version 固定不可变 Script Bundle, so that Run 和旧 Revision 不发生行为漂移。
30. As a Node Executor 发布者, I want Bundle 摘要进入 Run Snapshot, so that 历史能证明当时脚本。
31. As a Workflow 作者, I want Node Instance 不能替换脚本或工具调用, so that Executor Version 行为稳定。
32. As a Workflow 作者, I want 只配置公开的 Coverage/Complexity 阈值, so that config 不变成脚本语言。
33. As a Darwin/Linux 用户, I want 内置 automation 共用一份 POSIX check.sh, so that 脚本行为保持简单。
34. As a Windows 用户, I want 不支持的平台在运行前明确诊断, so that Gum 不碰运气启动脚本。
35. As a Host Execution Environment 用户, I want 脚本继承我的 PATH、Go config、cache、GOTOOLCHAIN 和网络策略, so that Workflow 与本地环境一致。
36. As a Host Execution Environment 用户, I want Gum 不注入隐藏环境变量, so that 我能理解和手工调试执行方式。
37. As a 安全意识用户, I want Gum 不持久化完整用户环境, so that 密钥不进入历史。
38. As a 调试者, I want 记录脚本摘要、cwd、位置参数和 Go/平台信息, so that 我能使用同一环境复现检查。
39. As a 用户, I want Gum 不自行安装检查工具, so that 工具链不在背景不可预期地变化。
40. As a 用户, I want Gum 继承我已选择的 Go 自动 toolchain/module 行为, so that 我的配置不被改写。
41. As a 用户, I want Gum 数据库、Artifact、日志和 tool-output 位于 Local Data Root, so that 项目不出现 Gum 产物。
42. As a 用户, I want Local Data Root 使用稳定 ID, so that Project/Workflow 重命名不破坏历史定位。
43. As a 用户, I want 旧 `.workflow` 数据经显式一次性迁移, so that 产品不长期双写。
44. As a 历史查询者, I want 保留结果、日志、配置和 code ArtifactRef, so that 我能审计当时发生了什么。
45. As a 代码所有者, I want 代码版本和恢复由我现有工具负责, so that Gum 不另发明代码版本模型。
46. As a 运行者, I want Context 取消时终止脚本及子进程, so that Workflow 停止后不留下后台检查。
47. As a 运行者, I want 日志超限时终止检查并返回 Structural Error, so that 无界输出不填满磁盘或生成伪完整 Result。
48. As a gum-workflows 维护者, I want 用四个内置 Node 检查项目自身, so that 新能力持续 dogfood。

## Implementation Decisions

- 本 spec 属于 14 后产品设计，不重写平台核心 01–14 历史范围；实施通过新设计或显式 Schema 升级落地。
- 设计遵循“现实工作流优先”：Runtime 连接用户已有环节、传递 Artifact、调度和留存结果；不增加未要求的副本、Snapshot/Revision、隐藏串行、自动回滚或恢复层。
- 14 后代码工作流采用 In-place Project Workspace。Project Definition 指向的用户项目目录直接作为 Agent/Automation 工作目录，Agent 修改实时生效。
- Gum 数据库、Artifact、Node Run 日志、tool-output 和 Quality Check Result 写入 Local Data Root。内置检查不在项目内写 Gum 报告/Coverage profile；用户测试自身的文件副作用不额外拦截或回滚。
- Local Data Root 配置优先级为测试注入、专用环境覆盖、产品设置、操作系统默认应用数据目录。它不是 Workflow YAML 字段或 CLI 业务 flag。
- Local Data Root 使用稳定 ID 组织全局产品库、Run、Node Run、Artifact、日志和工具产物，不将可变 Project/Workflow 名称编码进事实路径。
- 当前项目内 `.workflow` 不与 Local Data Root 双写；保留旧历史时使用显式一次性迁移。
- 首批 Node Definition 为 `go-race-check`、`go-static-analysis`、`go-coverage-check` 和 `go-complexity-check`；选择定义即选择 Go。
- 四个 Node 均为 automation，声明 Project Requirement，使用 `code: SourceCode` input 与 `result: QualityCheckResult` output。
- Code Artifact 指向 In-place Project Workspace，不保存源码本体。成功开发 Node Run 发布新 code Version 作为验证触发；失败轮不发布。历史保留 ArtifactRef 但不承诺重建当时代码。
- Workflow Context Binding 是前置能力。有开发 Node 时 code 绑定 backend.code；直接检查项目时绑定 project.code。Context Binding 传递类型化 ArtifactRef，不是字符串模板、OS 环境变量或数据内联。
- 四个检查按 Data/Control Graph 正常调度，不添加 Workspace Lease 或隐藏串行。它们可在同一项目目录并发执行，每个 Node Run 使用独立日志/tool-output 目录。
- 本 spec 不引入 Quality Gate 或条件调度。Result 是普通 Artifact：有下游绑定就由下游消费，无绑定就只进入历史。
- Quality Check Result 使用 `qualityCheckResult/v1` Schema，是唯一业务输出。Envelope 包含 check discriminator、verdict、输入 code ArtifactRef、effective config、工具链、metrics、findings、日志 ArtifactRef 和时间。
- 每种 check 的 config/metrics 字段集固。Metric 以 available 加 value/unit，或 unavailable 加 reason 表达；不伪造零值。
- 固定指标为 Coverage `statementCoverage`；Complexity `maxCyclomaticComplexity`、`functionsAnalyzed`、`functionsOverThreshold`；Race `racesDetected`；Static `findingsCount`。
- verdict 只有 passed、failed 和 not-applicable。项目测试/编译/语法失败、race/vet finding 和阈值不达标是 failed；无 Go package/函数/可插桩 statement 是 not-applicable。
- 工具无法启动、平台/Requirement 不满足、Bundle 摘要不匹配、Context 取消、I/O 失败、Adapter/Schema 错误或成功工具缺少正式产物是 Structural Error。
- Coverage config 只公开 statement coverage 最低阈值，默认 80。Complexity config 只公开单函数上限（默认 15）、包含测试（默认 false）和排除 generated files（默认 true）；vendor 始终排除。Race/Static 无业务 config。
- Node Instance 不允许覆盖脚本、入口、工具调用、工作目录、package scope 或 Shell 参数。首批固定 full scope。
- Script Bundle 是 Executor Version 的不可变资产。脚本变化需新 Executor Version，新 Workflow Revision 固定引用新版；旧 Revision/Run 继续使用旧 Bundle。
- Automation Script Manifest 使用 `automationScript/v1`，严格声明 Node Definition、Executor Version、POSIX Shell 入口、平台、required executables、tool outputs 和 Result Adapter ID。Runtime 校验 Manifest/Registry 一致性、Bundle 摘要和产物路径。
- 每个 Bundle 只有一份 Darwin/Linux 共用的 POSIX `check.sh`，不使用 Bash 专属语法。Windows 不提供 PowerShell 变体，不支持时在运行前诊断。
- ScriptNode 以固定位置参数显式传入 Project Workspace 和 tool-output 目录。不向脚本注入 Gum 专用环境变量，也不持久化完整用户环境。
- Shell 中的 Go 使用用户 PATH、Go config、module/build cache、GOTOOLCHAIN 和网络策略。Gum 不安装或下载工具；用户已选择的 Go 自动行为仍生效。
- Node Run 只记录非敏感且与复现相关的脚本/摘要、cwd/位置参数、工具路径、launcher/final Go version、GOROOT、GOOS、GOARCH、CGO 和日志。
- stdout/stderr 永远是日志。ScriptNode 将其流式写入 Node Run 私有文件，不作为完整内存字符串处理。
- 四个内置 Result Adapter 是 ScriptNode Module 的内部 seam。它们消费进程状态、日志、tool-output、effective config 和 code ArtifactRef，返回 Quality Check Result 或 Structural Error。
- Race Script 运行全项目 Go race test 并产生 Go JSON 事件；Adapter 区分 race、普通测试/编译失败与基础设施错误。Result 只声称本次是否观察到 race。
- Static Script 运行全项目 `go vet` JSON 诊断；Adapter 报告具体 findings，不声称完成所有静态分析。
- Coverage Script 禁用 Go test cache，运行全项目测试并将 coverprofile 写入 tool-output。Adapter 验证 profile、计算 statement coverage 并应用阈值；测试失败时 metric unavailable，不伪造 0%。
- Complexity Bundle 内含仅使用 Go 标准库的 AST Analyzer 源码，Shell 通过用户 PATH 中的 Go 运行并写出结构化产物；Adapter 应用阈值和排除策略。
- Context 取消需终止 Shell 及子进程组。stdout/stderr 使用固定磁盘保护上限；超限终止并返回 Structural Error。首批不设自动时间上限。
- 实施顺序为：Local Data Root；In-place Project Workspace 与 project.code Context Binding；ScriptNode/Manifest；Quality Check Result/Result Adapters；Static；Coverage；Race；Complexity；gum-workflows dogfood Workflow。

## Testing Decisions

- 测试只断言公开行为：完整 Validation、Engine/Node Run 状态、Artifact Contract、历史、CLI adapter 和文件系统产物边界。不断言私有 Shell 解析 helper 调用顺序。
- Validation seam 覆盖 Definition/Executor/Manifest 一致性、code/Result 类型、config 默认/非法值、Context Binding、平台/required executable 诊断。沿用现有完整 Schema + Semantic Validator 模式。
- Engine.Run seam 覆盖新 code Version 促使检查重新 Ready、开发失败不触发、无隐藏串行、failed Result 是成功 Node Run 输出、Structural Error 使 Workflow Failed、取消终止在途脚本。沿用注入 fake Executor/Gateway/Recorder 的公共接缝。
- ScriptNode 的 Node.Execute 是脚本执行与 Result 合同的主要测试 seam。可注入进程 adapter 或在临时 PATH 放置可控工具，但断言停留在 Artifact、日志、错误类型和取消等公开效果。
- Result Adapter 是 ScriptNode Module 内部 seam，使用固定 Execution Record/tool-output fixture 覆盖 passed、failed、not-applicable 和 Structural Error。断言完整 Result Schema，不断言私有分支函数。
- Script Bundle 合同测试实际经 POSIX Shell 执行四份脚本，使用临时 In-place Project Workspace 和独立 tool-output，验证额外 print/echo 只进日志、不影响 result，且脚本不在项目创建 Gum 产物。
- Local Data Root/CLI adapter 测试验证 Run 不创建项目内 `.workflow`，Gum 产物进入注入根目录，Project Workspace 就是用户项目，Agent 修改实时可见。
- History Store seam 验证 Result、code ArtifactRef、effective config、Executor/Bundle 摘要、工具链与日志引用可按 Node Run/round 查询，但不声称 code Ref 可恢复源码。
- Result Schema 使用表驱动合同测试，覆盖四种 discriminator 的字段集、未知字段、metric available/unavailable 互斥、verdict 枚举、ArtifactRef/日志引用和时间。
- Coverage fixture 覆盖完整 profile、阈值下/等于/上、测试失败 metric unavailable、成功进程伴随 toolchain 诊断、缺失/损坏 profile 和无 statement。
- Race fixture 覆盖未观察/观察到 race、测试/编译失败、平台/CGO/C 编译器不支持和无 Go package。
- Static fixture 覆盖无诊断、单/多 finding、package load/编译错误、非法 JSON 产物和无 Go package。
- Complexity fixture 覆盖阈值下/等于/超过、多函数、排除测试/generated/vendor、无函数、语法错误和 Analyzer 产物损坏。
- 并发验收让至少两个质量 Node 同时 Running，验证它们共享项目目录但日志/tool-output 目录不同。使用 channel/callback，不使用 sleep。
- 资源保护测试验证超量日志导致进程组取消和 Structural Error，不产生伪完整 Result；用户取消后子进程不残留。
- 自动测试禁止网络、真实 HOME/Local Data Root 依赖和用户项目修改。PATH、Go 输出、toolchain 信息和日志由临时目录/fixture 控制。
- 实现完成后使用 gum-workflows 自身进行 Host PATH dogfood，产物写入配置的 Local Data Root。已知实证是 Vet/Race 通过、Coverage 74.4%，且成功 Coverage 进程仍可出现 covdata 诊断，因此不只依赖 exit code。

## Out of Scope

- Fuzz Node 不属于本 spec，也不在本 spec 建立后续开发承诺。
- Changed Scope、增量依赖计算、Git diff 和语言/子项目检测后置；首批只支持 Go full scope。
- 条件执行、字段比较、false/else、Skipped 传播和 Quality Gate Node 不属于本 spec。
- 用户自定义 Automation Shell Script、Result Channel/Schema、权限、分发、升级与编辑器不属于本 spec。
- Windows 原生 Shell、PowerShell、WSL 和统一 POSIX Runtime 不属于首批；首批只支持 Darwin/Linux。
- Container Execution Environment、安全沙箱、CPU/内存配额、可配置 timeout 和企业权限治理不属于本 spec。
- 任意 Shell command/arguments、用户选择底层工具、package scope 和脚本内容不是 Node Instance config。
- Gum 不在运行时安装 Go、Staticcheck、复杂度工具或其他检查工具。
- Staticcheck、govulncheck、gocyclo、Mutation Testing、Property-Based Testing、故障注入等不属于首批四个 Node。
- 每 Node 工作区副本、Code Snapshot、Runtime 内部代码 Revision、Git Requirement/worktree、Workspace Lease、自动代码回滚和恢复明确不属于本 spec。
- 内置 Workflow 库、Workflow Marketplace 和通用质量门禁工作流不属于本 spec；只交付可组合 Node 和 dogfood 验证。

## Further Notes

- “自动化脚本”指 Gum 内置、随 Executor Version 发布的不可变 POSIX Script Bundle，不指 Node Instance 中的任意命令字符串。
- “结果”指经 Result Adapter 和 `qualityCheckResult/v1` 校验的 Artifact；stdout/stderr、exit code 和 coverprofile 只是生成结果的证据。
- In-place Project Workspace 意味着 Agent 修改用户代码是预期业务效果，而非 Gum 产物污染。
- 历史 code ArtifactRef 只表达当时输入身份/触发链，不是可恢复的源码快照。
- README 的“后续待办（唯一跟踪清单）”是本 spec 之外功能的唯一产品待办索引；本 spec 不建立重复待办列表。
