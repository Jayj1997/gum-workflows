# gum-workflows

基于 Go 的轻量级 Workflow Runtime。使用 YAML 定义 Workflow，通过 Node 的 Input/Output Contract 自动形成 DAG，按 Artifact 数据依赖持续执行。

- 设计计划（只读，不要修改）：`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`
- 开发规范（必须遵守）：`docs/DEVELOPMENT.md`

## 当前状态

**MVP 已全部完成**（设计计划 §44 开发顺序 ①-⑱）：Core Model、YAML Loader、CUE + 语义两层校验、Node/Artifact Registry、DAG Builder/Validator、串行与并行 Execution Engine、FilesystemArtifactStore + state.json 持久化、Project Runtime/Workspace、内置 Mock Node（requirement-analysis / architecture-design / coding-agent / openapi-generator）、`workflow validate` 与 `workflow run` CLI、fullstack Demo（`examples/fullstack`）与 e2e 测试。设计说明见 `docs/domain-model.md`。

后续版本方向（需先升级设计文档）：真实 Coding Agent Adapter（替换 MockCodingAgent）、真实 OpenAPI Generator、Skipped 传播、重试/超时等 workflow/v2 字段。

## 硬性设计约束（任何代码变更不得违反）

这些约束来自设计计划，是架构层面的决定，不是建议：

1. **数据依赖优先**：`inputs.<name>.from: <node-id>.<output>` 隐式产生 Data Edge。`dependsOn` 仅表示 Control Edge（显式执行顺序约束），永远不是表达数据依赖的方式。
2. **Workflow 与 Node 解耦**：Workflow YAML 只声明组合；Node 通过 Registry 注册。同一个 Node Type 可被多次实例化（Node ID 与 Node Type 分离）。
3. **Artifact 是 Node 间唯一数据通道**：运行时传递 `ArtifactRef`（引用），不传递大型数据本体（如源码内容，`SourceCode` 只存 repo path / commit / workspace 引用）。
4. **Node 运行条件**：`Ready(Node) = InputsReady AND ControlDependenciesCompleted`。无输入无依赖的 Node 是合法的 Trigger/Source Node。
5. **CLI 不接受业务参数**：只有 `workflow run <workflow-file>` 和 `workflow validate <workflow-file>`。所有配置必须来自 YAML。
6. **Workflow 不管理 Skills**：Coding Agent 自行进入 Project Workspace 并发现 `.agents/skills/`、`.Codex/skills/` 等项目约定。
7. **两层 Validation**：CUE Schema（结构）→ Go Semantic Validator（语义：Node Type 存在、Output 存在、Artifact 类型匹配、无环）。错误信息必须指明具体 Node 与字段。
8. **MVP 明确不做**（加入前必须先升级设计文档）：UI、Temporal、Redis/Kafka/Database、分布式调度、多租户、Skill/Agent Marketplace、复杂 Retry、Condition、Secret Management、workflow/v1 之外的 Schema 字段（retry/timeout/parallelism/environment/hooks 等）。

## 常用命令

```bash
go build ./...
go test ./...
go vet ./...
go run ./cmd/workflow validate <workflow-file>   # 已实现
go run ./cmd/workflow run <workflow-file>         # 已实现（Mock Node）
```

## Agent skills

### Issue tracker

本地 Markdown：issue 以文件形式存放在 `.scratch/<feature-slug>/` 下。见 `docs/agents/issue-tracker.md`。

### Triage labels

使用五个默认标签（needs-triage / needs-info / ready-for-agent / ready-for-human / wontfix）。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文布局（single-context）：根目录 `CONTEXT.md` + `docs/adr/`。见 `docs/agents/domain.md`。
