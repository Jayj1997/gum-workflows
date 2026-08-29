# Spec: 平台核心重构——组件定义体系与迭代执行引擎

Status: completed

设计文档：`plans/平台核心设计：组件定义体系与迭代执行引擎.md`（决策已与维护者逐项确认，Q1–Q41 索引见其附录 B）。术语以根目录 `CONTEXT.md` 词汇表为准。本 spec 是该设计文档的实施规格；两者冲突时先修订设计文档（维护者要求：任何打破设计的变更必须先讨论）。

## Problem Statement

当前 MVP 把「节点」压成了一层（`nodes.<id>.type` 直接指向编译进二进制的实现），运行模型是「无环 DAG、跑完即止」：

1. **使用者无法声明节点的能力与版本**：inputs/outputs 契约只存在于 Go 代码里，YAML 作者看不见；节点迭代即行为变化，没有版本机制隔离生产中的运行。
2. **工作流表达不了真实的工作方式**：需求一次性给死、审批不存在、出错即全盘退出。真实的过程是「人给需求 -> AI 做 -> 人审 -> 带意见重做 -> ……」，一个有环、多轮、人工在环的持续收敛过程。
3. **agent 节点没有模型配置**：LLM 的 url/key/model 无处声明，跨 workflow 无法复用。
4. **运行数据无法查询**：跑完只有目录里的 state.json，「跑过几次、哪轮被拒、advise 说了什么」无从回答。

## Solution

按已批准的设计文档实施三层重构：

1. **四层定义体系**：Node Type（agent/automation/human 类别）→ Node Definition（节点本体，契约的唯一声明处，YAML）→ Node Executor（定义的可执行版本，YAML 声明 + 编译进二进制的 Go 实现）→ Node Instance（workflow 内的一次使用，按 name 引用、落库换 UUID）。契约从 Go 代码迁至 YAML；同一定义多版本并存，run 启动时解析固定。
2. **迭代执行引擎**：废除无环约束。节点只看「输入收集完毕即运行」；上游产出新版本，下游标 dirty 重新入队；节点单并发、每轮一个 Node Run（独立 UUID + round）、artifact 多版本共存；approve 门控（拒绝催更下游、通过不催更）；收敛保护只拦机器自驱死循环；错误二分法（结构性 fail-fast，交互性靠 advise 重试复活）；运行无显式终态，用户 Ctrl-C 记 Stopped。
3. **人工在环**：内置 human-input（唯一入口）与 human-approval（审批）两个 human 节点，前台阻塞 stdin 交互（HumanGateway 接口隔离）；agent 节点以节点级 optional `advise` 输入消费人类意见。
4. **用户级 llm.yaml + 本地 SQLite 统一库**：providers→models 声明在用户配置（XDG 路径），不落项目库；`.workflow/gum-workflows.db` 承载定义侧五表与 node-run 级运行历史两表，run 是唯一写入口，validate 零副作用。

## User Stories

定义侧与 Schema：

1. As a workflow 作者, I want 节点的 inputs/outputs 契约以 YAML 声明, so that 我不需要读 Go 源码就能知道一个节点吃什么、吐什么。
2. As a workflow 作者, I want 在节点实例上用 `node: <name>` 引用节点定义, so that YAML 保持简洁可读（不需要背 UUID）。
3. As a workflow 作者, I want 省略 `executor:` 时自动使用最新版本且本次运行内固定, so that 我平时不用关心版本，但行为不因运行中途的升级而漂移。
4. As a workflow 作者, I want 显式指定 `executor: v2` 固定某版本, so that 生产 workflow 不被节点迭代破坏。
5. As a 平台开发者, I want 为同一个 Node Definition 发布 v1/v2/v3 多个执行器, so that 节点可以迭代而不影响既有运行。
6. As a 平台开发者, I want 启动时做 Go 实现与 YAML 声明的一致性检查（每个 (definition, version) 双向对齐）, so that 声明与实现漂移在启动即报错而不是运行中炸。
7. As a workflow 作者, I want 端口类型支持原子类型（string/int/bool/float/markdown）、语义 Kind、union 和 list, so that 契约能精确表达「string 或 jpg/png/pdf」「多值输入」这类真实需求。
8. As a workflow 作者, I want 类型匹配无隐式子类型（要宽就显式写 union）, so that 匹配结果永远可预测。
9. As a workflow 作者, I want `requires`（定义层「我需要什么」）与 `dependsOn`（实例层「我等谁」）是两个不同的字段, so that 配置语义不再含混。

LLM 配置：

10. As a 用户, I want 在用户级 llm.yaml 里声明 providers 及各自 models, so that 我的多个 workflow 复用同一份模型配置，不用每个 workflow 重复写。
11. As a 用户, I want 在 llm.yaml 里标记默认 provider 与默认 model（不标记则取第一个）, so that agent 节点绝大多数情况什么都不用填。
12. As a 用户, I want apikey 写成 `$ENV_VAR` 引用, so that 密钥不进任何文件明文留痕。
13. As a workflow 作者, I want 在 agent 节点上可选填 `llm:` / `target_model:`, so that 需要精调的节点可以指定模型，其余走默认链。
14. As a workflow 作者, I want 在非 agent 节点上写 llm/target_model 时得到校验错误, so that 配置错误在 validate 阶段就暴露。
15. As a 用户, I want llm.yaml 是纯运行时配置、不进项目数据库, so that 我clone 别人的项目不会把我的模型配置带走，项目库里也没有我的密钥痕迹。
16. As a workflow 作者, I want workflow 不含任何 agent 节点时不需要 llm.yaml 存在, so that 纯 automation/human 工作流零负担。

迭代执行：

17. As a workflow 作者, I want 工作流允许成环, so that 我能表达「编码 → 审查 → 带意见重做」这类真实循环。
18. As a workflow 作者, I want 环在 validate 时只得到提示（不含 human 的环警告可能死循环）而不是错误, so that 我的合法迭代配置不被拒绝。
19. As a 运行者, I want 上游产出新版本时下游自动重跑并取最新版本输入, so that 修一处、全链自动更新，不需要手工编排。
20. As a 运行者, I want 多个输入版本变化合并为一轮、当前轮不被上游新输出打断, so that 节点不会因上游并发更新而反复重启。
21. As a 运行者, I want 同一节点同时至多一个 Node Run 在跑, so that 多轮输入排队处理，不会竞态打架。
22. As a 运行者, I want 审批拒绝时下游带 advise 重跑、审批通过时已消费过旧版的下游不重跑, so that 拒绝驱动返工、通过即收敛，不会无谓空转。
23. As a 运行者, I want 机器自驱的连续重跑在第 10 轮被收敛保护掐断, so that 配置错误的小环不会让 CPU 空转到天亮。
24. As a 运行者, I want 任何人类事件（新一轮需求/审批决策/advise 重试）重置收敛计数, so that 人工驾驶的长会话永不被误伤。
25. As a 运行者, I want 每次节点执行有独立 node run id 与 round 号, so that 「第 2 轮发生了什么」可精确追溯。
26. As a 运行者, I want 同一节点同一输出的多版本 artifact 共存不删, so that 我能对比两轮产出、审计被拒的方案。

人工在环：

27. As a 用户, I want workflow 有且仅有一个 human-input 入口节点（校验强制）, so that 需求必然来自人、工作流必然由我启动。
28. As a 用户, I want 在入口节点多轮输入需求（每轮空行结束、Continue/Finish 决定是否继续）, so that 我可以边看进展边补充要求。
29. As a 审批人, I want 审批节点先展示本次运行已产出的 artifacts 摘要与历史 advise, so that 我在有依据的情况下做决定。
30. As a 审批人, I want 用单键 A/r 加同行 advise 完成审批（回车默认通过）, so that 审批操作三秒内完成。
31. As a 审批人, I want agent 节点声明 optional advise 输入即可接收我的意见, so that 节点开发者按需选择是否消费人类反馈。
32. As a CI/脚本用户, I want 非 TTY 环境运行含 human 节点的 workflow 时启动即报错, so that 我立刻知道这个 workflow 需要交互终端而不是挂死。

错误处理：

33. As a 运行者, I want 结构性错误（automation 跑不起来、LLM 网络不可达）让运行 Failed 并退出, so that 底层坏了不再空转。
34. As a 运行者, I want agent 交互性错误（预期 JSON 得到散文）只让节点 Failed 而运行保持 Running, so that 两小时的会话不被一次模型抽风报废。
35. As a 用户, I want 在交互性错误后输入 advise 即时重试该节点, so that 我用一句话就能让 agent 修正而不是重跑整个 workflow。
36. As a 平台开发者, I want 用 Structural()/Interaction() 包装错误、缺省按结构性, so that 错误分类是显式契约而不是猜出来的。

持久化与查询：

37. As a 用户, I want 定义侧（节点类型/定义/执行器/workflow/节点实例）与运行历史都在项目本地 SQLite, so that 我能用标准工具做任意维度的检查与分析。
38. As a 用户, I want run 启动时隐式导入定义并固定解析结果（executor id、llm 名字）, so that 历史记录精确反映「当时用的是哪个版本哪个模型」。
39. As a 用户, I want `workflow validate` 纯只读零副作用, so that 我可以在任何环境随意校验而不留痕迹。
40. As a 用户, I want `workflow history` 列出运行、`history <run-id>` 看各节点各轮次、`history <run-id> <node-id>` 看节点全部轮次明细, so that 「这轮为什么被拒、上轮 advise 说了什么」一屏可查。
41. As a 用户, I want Ctrl-C 结束时运行记 Stopped 且 stopped_reason=user_interrupt, so that 「做完的」（自然收敛）与「中途停的」在历史里可区分。

Workspace：

42. As a 运行者, I want 一次运行内所有轮次复用同一份 Workspace, so that 重做是增量修正而不是从零开始，上轮痕迹也是审查依据。
43. As a workflow 作者, I want 项目声明支持相对与绝对路径（相对 = 相对 workflow 文件）, so that checked-in 的示例可移植，本机大项目可写绝对路径。

## Implementation Decisions

全部细节以设计文档为准（§ 编号引用）；此处只列规格级决定与文档未逐字写明的补充。

**分层与模块**

- 新增 `internal/definition`（四类定义侧类型 + TypeExpr 解析 + 内嵌种子 loader + CUE schema）、`internal/llm`（llm.yaml 类型与 resolver，无网络客户端）、`internal/history`（SQLite：迁移/定义导入/运行历史，实现 execution 定义的接口）。
- `internal/node` 接口瘦身：Node 接口移除 Type()/InputSchema()/OutputSchema()（契约唯一来源 = Node Definition YAML）；Registry 改为按 (definition, version) 注册的 ExecutorRegistry，Latest(definition) 供缺省解析；新增 Structural()/Interaction() 错误包装与 ErrorKindOf。
- `internal/execution`：新增 HumanGateway 与 RunRecorder 接口（均定义在消费方 execution）；NodeExecution 改为「当前轮 Current + 历史轮 History + dirty/machineRuns 内部字段」双层形态；NodeRun 含 RunID/Round/Status/Inputs/Outputs/Error/ErrorKind/时间戳。
- `internal/workflow`：NodeSpec 的 `type` 字段改名为 `node`，新增 executor/llm/target_model/metadata 可选字段；`project`（单数）改 `projects`（列表，本期恰好 1 个）；kind 值小写化。原地演进 workflow/v1，不做双版本兼容（未上线）。
- 现有 `ApprovalResult`/`TestReport` Kind 保留注册；`RequirementSpec` 不再被内置契约使用。

**执行语义（设计文档 §5–6 为权威）**

- Ready = 数据输入有已完成轮产出 AND control 前驱完成过至少一轮；重新 Ready = 任一数据输入有未消费的新完成版本 OR control 前驱有新完成轮（审批节点限「通过轮」）。
- approve 门控四象限表（拒绝/通过 × 已消费/未消费）见设计文档 §6.5，是引擎行为的精确规范。
- 收敛保护阈值默认 10，Engine Option `WithConvergenceLimit` 调节，不进 YAML。
- 节点级 Succeeded 可重入（Succeeded -> Ready 合法）；运行级新增 Stopped（Ctrl-C/SIGTERM）；全图静止保持 Running 等待输入，唯一自动终态是结构性错误与收敛保护。
- 非阻塞：运行保持 Running 不等于引擎忙轮询--全图静止且无等待中的 human 节点时引擎阻塞在 human gateway 等待（入口未 Finish）或等 ctx 取消；这是「静止保持 Running」的实现形态。

**种子数据与一致性检查**

- 内置 6 个 Node Definition（human-input / requirement-analysis / architecture-design / coding-agent / openapi-generator / human-approval）+ 各 v1 执行器 YAML，go:embed 随二进制分发；run 启动导入。
- 内置契约按设计文档 §12 定稿表：requirement-analysis 产出 `rationality: int` + `analysis-output: markdown`；coding-agent 增 optional `advise: markdown` 输入；human-approval 产出 `approve: bool` + `advise: markdown`。
- 启动一致性检查双向：Go 注册的每个 (definition, version) 必须有 YAML 声明，反之亦然；Node Definition 的 type 合法且 Kind 已注册。任一不一致启动报错。

**LLM 配置**

- llm.yaml 查找顺序：`$XDG_CONFIG_HOME/gum-workflows/llm.yaml` → `~/.config/gum-workflows/llm.yaml`；信封 `llm/v1` + `kind: llm`。
- apikey `$VAR` 形式加载时解析（环境变量缺失 = 加载错误，指明变量名）；明文允许；解析后密钥绝不落库，DB 行只存解析后的 provider/model 名字符串。
- 解析链四象限（llm 填/空 × target_model 填/空）按设计文档 §3.4 表；只填 target_model 时在默认 provider 内找，找不到报错并提示补 llm。

**SQLite**

- 单库 `.workflow/gum-workflows.db`；驱动 modernc.org/sqlite（纯 Go）；WAL + busy_timeout + user_version 顺序迁移（沿用 run-history 设计 §9）。
- 八张表 DDL 以设计文档 §8.3 为权威（node_type_definition / node_definition / node_executor / workflow / node_instance + workflow_run_history / workflow_node_run_history，后者 UNIQUE(run_id, node_id, round)，一轮一行）。
- 运行开始即为全部节点建 Pending 行；inputs_json/outputs_json 沿用 run-history §5.3 形态，即时 advise 以特殊 from 标记 `#advise-retry`。
- state.json 扩展：运行级增 run_id/stopped_reason/时间戳/workflow_file；节点级 `nodes/<id>/state.json` = 当前轮 + 历史摘要，每轮明细 `nodes/<id>/runs/<round>.json`。记录失败不使运行失败。

**CLI**

- `workflow run|validate|history`，无 flags（UUID 前缀 ≥8 位寻址除外）；run 摘要扩展 WaitingHuman/轮次数/error_kind。
- run 唯一写入口；validate 做全部校验与解析检查但不建库不写库。
- 非 TTY 守卫：stdin 非控制台且 workflow 含 human 节点 → 启动即报错（错误信息说明需要交互终端）。

**既有代码处置**

- examples/fullstack 与现有 e2e 在 P1–P4 期间冻结（最小字段改名迁移保绿：`type:` → `node:`、`project` → `projects`），P8 整体重写为新 demo（human 入口 + 审批循环 + 多轮需求）。
- 文档同步（P8）：CLAUDE.md 约束 #4/#8 修订 + 常用命令；DEVELOPMENT.md §1/§2/§3/§5；domain-model.md 重写；run-history 设计稿顶部标注「已被本设计吸收取代」。
- 里程碑顺序 P1→P8 严格推进（P1 定义层 → P2 llm → P3 schema/校验 → P4 SQLite → P5 迭代引擎 → P6 human 在环 → P7 历史/CLI → P8 examples/文档），P(n) 未验收不开 P(n+1)。

## Testing Decisions

接缝划分已与维护者确认（主接缝 = Engine.Run，人工交互与历史记录经注入的 fake 协作者驱动；不新增引擎以下接缝）。

- **好测试的标准**：只测外部可见行为（Run 的输入 def + 注入协作者 → 断言返回的 WorkflowExecution 与 Recorder 收到的快照序列、Store 中的 artifact 版本），不测 scheduler 内部结构、不测私有字段。引擎测试不依赖真实时间与真实终端。
- **主接缝 Engine.Run**（沿用 engine_test.go 的 fake factory + MemStore 模式，新增 fake HumanGateway 与 fake RunRecorder Option 注入）：覆盖触发规则（新输入重跑、级联、轮次合并、单并发）、approve 门控四象限、收敛保护（机器环第 11 轮掐断 / 人类事件清零）、错误二分法（结构性退出 vs 交互性存活 + advise 重试复活 + 未声明 advise 输入的等价结构性）、多轮需求级联、executor 版本固定、Stopped 语义。设计文档 §14 列出的 8 个关键场景是必须存在的测试清单。
- **校验 fixture 表**（沿用 internal/validation/testdata 的 valid/invalid 目录模式）：新增入口规则、llm 解析四象限、TypeExpr 语法与兼容、human dependsOn 必填、llm 字段仅 agent 合法、环降提示（warning 不再是 error）、projects 恰一个。沿用「错误聚合、定位到 Node ID 与字段」断言风格。
- **CLI e2e**（沿用 tests/e2e 编译真实二进制 + t.TempDir 模式）：Schema 加载、run 后 sqlite3 可查定义五表、validate 零副作用（无 DB 文件产生）、非 TTY 守卫（管道 stdin 下含 human 节点的 run 报错）、history 三级查询、Stopped 落库。审批循环的完整行为在主接缝用 fake gateway 覆盖（不引入 pty 依赖，pty e2e 列入后续演进）--已与维护者确认此取舍。
- **history 包公开 API**：Open/迁移幂等、Record 幂等、UUID 稳定、FK 级联、查询往返、前缀解析；直接 SQL 断言表内容（一轮一行、stopped_reason、error_kind）。
- **llm.yaml**：临时 XDG_CONFIG_HOME 注入测试路径（禁真实 $HOME）；默认链四象限、$VAR 缺失报错。
- **human gateway 的 stdin 实现单测**：管道喂字节流断言提示输出与解析结果（交互文案正确性在此层验证，不依赖 TTY）。

## Out of Scope

- UI、服务端/远程数据库、多租户、分布式调度。
- 动态加载外部 Go 代码 / 用户自定义外挂节点 / executor 的 YAML 外部导入（仅验证内嵌导入路径正确）。
- 多项目 workflow（`projects` 校验恰好 1 个）；run 级 resume 与挂起-恢复式审批（`workflow approve/resume`）。
- Skipped 传播、重试/超时/parallelism 等 workflow/v2 字段；Secret Management 基建（仅 $VAR 环境变量引用）。
- 真实 LLM 网络客户端与真实 Agent Adapter（internal/llm 无 HTTP；Mock agent 继续担任演示角色）。
- file 类 Artifact 的真实产出/消费（类型语言预留）；数据血缘表、事件流水、prune、陈旧 Running 对账、列表分页过滤；pty e2e。
- 任何未列入设计文档的 Schema 字段。

## Further Notes

- 实施期间打破设计文档的任何变更，必须先与维护者讨论并修订设计文档（文档头部已声明此规则）。
- `CONTEXT.md` 词汇表是实现期间的术语权威；发现代码与词汇表冲突时改代码不改词汇表（术语争议回到维护者）。
- 设计文档附录 A 是 P8 demo 的目标 YAML 形态（含 approve 循环的完整绑定写法），实现 demo 时以它为验收参照。
- 现有语义校验器对 OptionalInputs 的 Kind 漏检（checkSchemaKinds 未遍历）在本轮 TypeExpr 校验落地时一并修复。
