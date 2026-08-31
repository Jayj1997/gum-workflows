# gum-workflows

基于 Go 的轻量级 Workflow Runtime。使用 YAML 定义 Workflow，通过 Node 的 Input/Output Contract 形成可含环的执行图，按 Artifact 版本持续迭代。

**产品初衷**：用户的工作本就由多个工作环节和重复的人工中间处理组成；Gum-Workflows 用 Node、Artifact 和调度把这些现有中间态自动串联起来。设计顺应用户原本会执行的流程；只在明确需求或已验证风险要求时增加额外复制、隔离、版本或恢复层。

- 设计计划（只读，不要修改）：`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`
- 开发规范（必须遵守）：`docs/DEVELOPMENT.md`
- Code Quality Check automation（设计或实现内置检查节点时读取）：`.scratch/code-quality-automation/spec.md`
- README 进度同步（完成独立模块或改变产品进度时读取）：`plans/README 更新规范：模块完成后的进度同步.md`

## 当前状态

**MVP 与平台核心 01–14 已全部完成**：四层定义体系（Node Type / Node Definition / Node Executor / Node Instance）、用户级 LLM 配置解析、允许有环的迭代 Execution Engine、human-input / human-approval、advise 重试、结构性/交互性错误二分、本地 SQLite 定义与 Node Run 历史、`workflow validate|run|history` CLI，以及 `examples/fullstack` 人工在环 Demo。运行在全图静止后继续保持 Running，直至用户 Ctrl-C / SIGTERM 记为 Stopped。设计说明见 `docs/domain-model.md`。

**code-quality-automation 已完成**：Local Data Root、显式 legacy 迁移、In-place Project Workspace、`project.code` Workflow Context Binding、ScriptNode，以及真实 `go-static-analysis` / `go-coverage-check` / `go-race-check` / `go-complexity-check` 已落地。四者使用不可变 `automationScript/v1` POSIX Bundle，在 Darwin/Linux 上从用户 PATH 原地运行并产出严格的 `qualityCheckResult/v1`；详细合同见 `.scratch/code-quality-automation/spec.md`，当前实现见 `docs/domain-model.md`。

**14 后 Product Workflow 01–02 已完成**：Wails macOS 产品壳、Browser Mock 与 Desktop Adapter 共享 WorkflowClient / Product Application seam；用户可以在 UI 向 Local Data Root 的 `product.db` 创建并稳定列出带 UUID 的 Product Workflow。它使用独立 `product_workflow` schema，不读取或复用 workflow/v1 YAML identity。Draft、Node 创作、Preview、Revision、Run 与真实 LLM 仍未实施，后续从 `.scratch/product-workflow/issues/03-draft-autosave-lock-version.md` 继续。

后续版本方向（需先升级设计文档）：真实 Coding Agent Adapter（替换 MockCodingAgent）、真实 OpenAPI Generator、Skipped 传播、重试/超时等 workflow/v2 字段。

## 01–14 之后的产品规划（已确认，部分实施）

`.scratch/platform-core/spec.md` 与 `.scratch/platform-core/issues/` 中编号 01–14 的票是已完成的平台核心历史记录；以下内容只属于 14 完成后的新设计，不得倒灌或改变 01–14 的范围。

- **本地 GUI 是主要创作入口**：macOS 产品壳已实现 SQLite Product Workflow 的新建与列表。后续节点声明、连接与配置、优化均通过 UI 完成。画布是只读的结构预览：节点按 Data/Control Edge 自动排列，可选择节点打开配置；不以拖拽节点、手工拉线或画布坐标表达执行语义。首个闭环平台为 macOS；Windows 是明确待办和长期目标，不属于 P9–P12 验收。
- **本地事实来源**：14 之后的产品 Workflow 只以 SQLite 中的 Workflow / Draft / immutable Revision 为编辑与运行事实来源；Draft 自动保存，不引入独立 Publish 动作，用户点击 Run 时按规范化语义内容创建或复用 immutable Revision，并固定 Executor、模型与配置快照。现有 YAML CLI 只属于已完成的平台核心历史入口，不作为产品 Workflow 的兼容入口，也不得在运行时隐式创建或复用产品 Workflow / Revision。YAML 只保留为未来可能的导出格式；产品 v1 形态未确定前不设计导入、导出或二者之间的转换。
- **Artifact 体验分阶段**：跑通 SQLite Workflow 的创建、配置、运行和基本结果查看是当前第一要务。多类型高级预览、版本比较、人工替换、历史复用以及外部可变资源的重建保证均后置，不能成为首个产品闭环的前置条件。
- **先打磨 Node 能力**：暂不规划内置 Workflow 库。14 后优先实现一个简单但真实的 AI 对话 Agent Node，用它验证真实 LLM 调用、对话历史、输出 Artifact、配置描述、错误、观测和 UI 展示。目标多轮模型使用 `human-chat -> llm-chat -> human-chat`：Human Chat Entry Node 是唯一可在没有必需输入时自举的人工门，每次人工提交追加 user message；`llm-chat` 接收并追加 assistant message 后把 Conversation 反馈给人工门，反馈只使其等待下一次人工事件，不会自动产出新一轮。P9 先用 fake executor 跑通 Product Tracer；P10 才是 `human-chat(source) -> llm-chat` 的首个真实 OpenAI text 闭环；P12 再升级入口验证、Human Executor input、WaitingHuman 与显式 Conversation 回边，不引入 triggering/context 两类 Input。
- **LLM Provider / Model**：用户级设置采用 `Provider -> Models`：Provider 保存协议、Base URL 和 API Key 引用；Model 是用户配置槽，拥有全局稳定的 Gum Model UUID 和可编辑的 Provider Model ID。UUID 标识配置槽，不声称底层模型不可变；编辑 Provider Model ID 会影响未来 Run，历史 Run Snapshot 保留旧值。Provider 与其 Model 都支持显式 default；未设置时从未删除项中按 `(created_at ASC, UUID ASC)` 取第一个，不引入 position/排序功能。Agent Node 的 LLM Preference 记录 Gum Model UUID。Node 未选择模型时，StartRun preflight 按双层 default 解析并先把 UUID 写回 Draft，再创建/复用 Revision；实际 Workflow Run 仍不回写定义。只要 UUID 仍存在，default 及连接内容变化都不改变选择。UUID 被删除时不 fallback：Draft/Preview 表单报错，StartRun 不创建 Run，用户必须重新选择模型；历史 Run 由 Run Snapshot 继续展示当时 Provider/Model。删除 Provider/Model 允许执行但应提示受影响 Workflow。当前范围不实现 `/models`、enable/disable 或自动 Provider failover。
- **首个真实产品闭环**：P10 必须经过真实的薄 Desktop UI 与通用 Application seam，允许用户从 SQLite 创建 Workflow、添加 `human-chat(source)` 和 `llm-chat`、绑定端口、配置 LLM、Run、输入文本并查看 Conversation Artifact；不得用硬编码聊天 UI 代替。P10 只实现 OpenAI-compatible Chat Completions、非流式 text input -> text output 和 macOS，但从第一天使用 ChatMessage / ContentPart 与可扩展 ProtocolAdapter。P9 可使用 fake executor，但必须经过同一真实 UI、SQLite 和通用创作 seam。Streaming、Anthropic Messages、image input 和 Windows 支持列入 P9–P12 之后的明确待办。
- **纵向交付顺序**：P9 是经过真实 macOS UI、SQLite 和通用创作 seam 的 fake Product Tracer；P10 接入 Keychain、真实 OpenAI-compatible 非流式 text 与正式 Conversation Artifact，形成首个真实闭环；P11 加固 migration、Interrupted、历史查询、悬空 Model UUID 诊断、脱敏和 macOS 安装升级；P12 才升级 Human Chat Entry、WaitingHuman 与显式多轮回边。Streaming、Anthropic Messages、image 和 Windows 分别进入待办，不再按“领域层 -> 协议层 -> UI 层”横向交付。
- **运行控制后置**：当前产品化设计先跑通 Workflow，不把 Rerun、Fork、Manual Artifact 或完整崩溃恢复作为首个闭环范围。未来设计必须保持以下已确认边界：Paused / Interrupted 可用同一 Run ID Resume；Interaction Error 不终结 Run，可在同一 Run Retry；Failed（Structural Error）与 Stopped 是终态，只能以新 Run 继续；UnknownOutcome 是 Node Run 结果且不得自动重放。具体事件、持久化和 UI 动作仍需另行设计。
- **模型能力由用户负责**：Gum 不探测、推断、维护或匹配 Model 的 image/tools/streaming 等 Capability。Agent Node 只通过 `requires: llm` 声明需要 LLM，输入输出是否合法由 Node/Artifact Contract 决定。用户为 Node 选择某个 Gum Model UUID，即确认该模型适合此任务；如果实际请求包含模型不支持的模态或特性，仍按所选协议发送，Provider 拒绝时记录并展示 Provider/Structural Error，由用户更换模型解决。
- **首个闭环的持久化与错误边界**：持久化 Workflow、Draft、Revision、Run Snapshot、Node Run、正式 Artifact 与错误；LLM Content Delta 只作为进程内临时 UI 信号。首版不做 append-only Event Log、重放或恢复；启动时发现未结束 Run，标记为可查看但当前不可恢复的 Interrupted，且不自动重放。Draft、端口、已绑定 Model UUID、默认 Provider/Model 与 Secret 引用在创建 Run 前校验；Run 创建后的认证、网络、限流、服务不可用、协议损坏或 Provider 拒绝请求均为 Structural Error 并终结 Run，只有 Provider 已成功返回但业务输出不符合 Node Contract 才是 Interaction Error，不引入自动重试。
- **Revision 语义哈希**：纳入 schema version、Node Instance identity、Definition/Executor 选择、Node config、input bindings、dependsOn、Project binding、Node 记录的 Gum Model UUID 和其他执行语义字段；排除 Workflow/Node 展示文案、Presentation Hint、时间戳、视图偏好和布局坐标。Provider 的可变连接内容、默认 Provider/Model 和 Resolved LLM Selection 不进入 Revision，而在 StartRun 时解析进 Run Snapshot；Secret 永不进入二者。无语义顺序的集合在哈希前规范化。
- **Node Config Schema 与 Draft 并发**：Config Schema 使用 Gum 自有的小型类型模型，首版字段为 string/markdown/integer/number/boolean/enum，Contract 持有 required/default/范围/敏感性，Presentation Hint 持有 label/help/editor；不暴露 JSON Schema、CUE AST 或前端表单库结构。Draft 是唯一可变当前态，autosave 不创建 Revision 或历史副本；规范化语义内容无变化时 no-op，有变化时更新同一 Draft 行并递增内部 `lock_version`。UpdateDraft 携带 expected lock_version 做 SQLite compare-and-swap。首版只支持单窗口编辑，冲突时刷新，不做字段级 merge；UI view preference 独立存储。
- **StartRun 与 Revision 去重**：StartRun 必须携带 UI 当前看到的 `expected_lock_version`；UI 点击 Run 前先 flush 有变化的 autosave。token 不匹配时不得物化 Model UUID、创建 Revision 或创建 Run。空 Model Preference 的 UUID 物化会更新 Draft lock_version；随后按规范化语义哈希创建或复用 immutable Revision。相同内容重复 Run 复用同一 Revision，但每次都创建新的 Run。
- **代码工作流原地运行**：14 后产品态中，Project Definition 指向的用户项目目录就是 Project Workspace；Agent 修改实时生效，Automation 在同一目录执行。Gum 自身的数据库、日志、tool-output 和 Result 位于 Local Data Root，不为 Run/Node 复制项目或承担代码恢复。

实现上述任一方向前，先形成 14 后的新设计文档与开发票；不要直接扩展现有 workflow/v1 或平台核心设计。

## platform-core workflow/v1 硬性设计约束

这些约束来自已完成的 01–14 设计计划，是 workflow/v1 与现有 YAML CLI 的历史架构决定。维护或修改 platform-core/workflow-v1 时不得违反；14 后 SQLite Product Workflow 的显式升级以本文件上一节和新的产品设计票为准，不得把新语义倒灌回 workflow/v1，也不得用本节阻止已经批准的产品模型：

1. **数据依赖优先**：`inputs.<name>.from: <node-id>.<output>` 隐式产生 Data Edge。`dependsOn` 仅表示 Control Edge（显式执行顺序约束），永远不是表达数据依赖的方式。
2. **Workflow 与 Node 解耦**：Workflow YAML 只声明组合；Node 通过 Registry 注册。同一个 Node Type 可被多次实例化（Node ID 与 Node Type 分离）。
3. **Artifact 是 Node 间唯一数据通道**：运行时传递 `ArtifactRef`（引用），不传递大型数据本体（如源码内容，`SourceCode` 只存 repo path / commit / workspace 引用）。
4. **Node 运行条件与迭代**：`Ready(Node) = InputsReady AND ControlDependenciesCompleted`；上游出现新 Artifact 版本或 Control 前驱出现新完成轮时可重新 Ready。环是合法迭代路径，校验只提示纯机器环，由收敛保护阻止无人工事件的死循环。全 Workflow 恰好一个无输入无依赖的 human-input 入口。
5. **CLI 不接受业务参数**：只有 `workflow run <workflow-file>`、`workflow validate <workflow-file>` 与 `workflow history [<run-id> [<node-id>]]`，不提供业务 flags。所有运行配置来自 YAML 与用户级 `llm.yaml`。
6. **Workflow 不管理 Skills**：Coding Agent 自行进入 Project Workspace 并发现 `.agents/skills/`、`.Codex/skills/` 等项目约定。
7. **两层 Validation**：CUE Schema（结构）→ Go Semantic Validator（语义：Node Definition / Executor / LLM / Project 存在，端口与 Artifact 类型匹配，唯一 human 入口，环提示）。错误信息必须指明具体 Node 与字段。
8. **平台核心批准范围与排除项**：有环迭代、human 在环和本地 SQLite 统一库已经由平台核心设计批准并实现。仍不做 UI、Temporal、Redis/Kafka/服务端数据库、分布式调度、多租户、Skill/Agent Marketplace、resume、复杂 Retry/Condition/Secret Management，以及 workflow/v1 之外的 `retry/timeout/parallelism/environment/hooks` 等字段；加入前必须先升级设计文档。
9. **现实工作流优先**：Runtime 负责连接用户已有的工作环节、传递 Artifact、调度和留存结果，不擅自重设用户的流程。例如“开发代码 → 在同一 Run Workspace 运行多个检查 → 读取结果”应直接表达；没有用户需求时不增加每 Node 工作区副本、内部代码 Revision 或自动恢复层。

## 常用命令

```bash
go build ./...
go test ./...
go vet ./...
go run ./cmd/workflow validate <workflow-file>   # 已实现
go run ./cmd/workflow run <workflow-file>         # 已实现（human 前台交互 + Mock Agent）
go run ./cmd/workflow history                     # 最近 20 次运行
go run ./cmd/workflow history <run-id>            # 运行与各 Node 轮次摘要
go run ./cmd/workflow history <run-id> <node-id>  # 单 Node 全部轮次明细
```

## Agent skills

### Issue tracker

本地 Markdown：issue 以文件形式存放在 `.scratch/<feature-slug>/` 下。见 `docs/agents/issue-tracker.md`。

### Triage labels

使用五个默认标签（needs-triage / needs-info / ready-for-agent / ready-for-human / wontfix）。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文布局（single-context）：根目录 `CONTEXT.md` + `docs/adr/`。见 `docs/agents/domain.md`。
