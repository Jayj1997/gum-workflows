# gum-workflows

基于 Go 的轻量级 Workflow Runtime。使用 YAML 定义 Workflow，通过 Node 的 Input/Output Contract 形成可含环的执行图，按 Artifact 版本持续迭代。

**产品初衷**：用户的工作本就由多个工作环节和重复的人工中间处理组成；Gum-Workflows 用 Node、Artifact 和调度把这些现有中间态自动串联起来。设计顺应用户原本会执行的流程；只在明确需求或已验证风险要求时增加额外复制、隔离、版本或恢复层。

- 设计计划（只读，不要修改）：`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`
- 开发规范（必须遵守）：`docs/DEVELOPMENT.md`
- Code Quality Check automation（设计或实现内置检查节点时读取）：`.scratch/code-quality-automation/spec.md`

## 当前状态

**MVP 与平台核心 01–14 已全部完成**：四层定义体系（Node Type / Node Definition / Node Executor / Node Instance）、用户级 LLM 配置解析、允许有环的迭代 Execution Engine、human-input / human-approval、advise 重试、结构性/交互性错误二分、本地 SQLite 定义与 Node Run 历史、`workflow validate|run|history` CLI，以及 `examples/fullstack` 人工在环 Demo。运行在全图静止后继续保持 Running，直至用户 Ctrl-C / SIGTERM 记为 Stopped。设计说明见 `docs/domain-model.md`。

后续版本方向（需先升级设计文档）：真实 Coding Agent Adapter（替换 MockCodingAgent）、真实 OpenAPI Generator、Skipped 传播、重试/超时等 workflow/v2 字段。

## 01–14 之后的产品规划（已确认，尚未实施）

`.scratch/platform-core/spec.md` 与 `.scratch/platform-core/issues/` 中编号 01–14 的票是当前既定开发序列；以下内容只属于 14 完成后的新设计，不得倒灌或改变 01–14 的范围。

- **本地 GUI 是主要创作入口**：Workflow 的新建、节点声明、连接与配置、优化均通过 UI 完成。画布是只读的结构预览：节点按 Data/Control Edge 自动排列，可选择节点打开配置；不以拖拽节点、手工拉线或画布坐标表达执行语义。目标平台为 macOS 与 Windows。
- **本地事实来源**：14 之后以 SQLite 中的 Workflow / Draft / immutable Revision 为编辑与版本模型；每次 Run 固定 Revision、Executor、模型与配置快照。YAML 保留为调试和将来的可移植交换格式。导入/导出在产品形态接近稳定 v1 前暂不规划，AI 创建或修改 Workflow 同样后置。
- **Artifact 体验**：Artifact 是一等 UI 对象；按类型提供 Markdown、源码/diff、图片、结构化文档、测试报告及外部资源等预览，并支持查看来源节点、Node Run 轮次与多版本比较。
- **先打磨 Node 能力**：暂不规划内置 Workflow 库。14 后优先实现一个简单但真实的 AI 对话 Agent Node，用它验证真实 LLM 调用、文本/多模态输入、对话历史、输出 Artifact、配置描述、能力声明、错误、观测和 UI 展示。
- **LLM Config 后续升级**：LLM Config 是独立的用户级复用配置，包含协议、Base URL、API Key 引用、模型目录与默认模型；Agent Node 只选择 Config 和可选 Model。支持 OpenAI-compatible Chat Completions 与 Anthropic Messages，正确组装含 system/developer 指令、user/assistant 历史和多模态 content parts 的多轮请求。LLM Config 可从服务端发现模型，也允许手工声明；发现结果须标准化并保留原始响应，手工配置可补充或覆盖服务端未提供的能力信息，不得依据模型名称猜测能力。
- **运行控制待新设计**：在 14 后统一设计结构化运行事件，以及 Resume、Retry、Rerun、Fork 和人工替换 Artifact 的精确语义；这些能力服务 GUI 实时状态、调试、崩溃恢复与未来同步。
- **代码工作流原地运行**：14 后产品态中，Project Definition 指向的用户项目目录就是 Project Workspace；Agent 修改实时生效，Automation 在同一目录执行。Gum 自身的数据库、日志、tool-output 和 Result 位于 Local Data Root，不为 Run/Node 复制项目或承担代码恢复。

实现上述任一方向前，先形成 14 后的新设计文档与开发票；不要直接扩展现有 workflow/v1 或平台核心设计。

## 硬性设计约束（任何代码变更不得违反）

这些约束来自设计计划，是架构层面的决定，不是建议：

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
