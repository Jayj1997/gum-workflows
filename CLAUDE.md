# gum-workflows

基于 Go 的轻量级 Workflow Runtime。使用 YAML 定义 Workflow，通过 Node 的 Input/Output Contract 形成可含环的执行图，按 Artifact 版本持续迭代。

- 设计计划（只读，不要修改）：`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`
- 开发规范（必须遵守）：`docs/DEVELOPMENT.md`
- README 进度同步（完成独立模块或改变产品进度时读取）：`plans/README 更新规范：模块完成后的进度同步.md`

## 当前状态

**MVP 与平台核心 01–14 已全部完成**：四层定义体系（Node Type / Node Definition / Node Executor / Node Instance）、用户级 LLM 配置解析、允许有环的迭代 Execution Engine、human-input / human-approval、advise 重试、结构性/交互性错误二分、本地 SQLite 定义与 Node Run 历史、`workflow validate|run|history` CLI，以及 `examples/fullstack` 人工在环 Demo。运行在全图静止后继续保持 Running，直至用户 Ctrl-C / SIGTERM 记为 Stopped。设计说明见 `docs/domain-model.md`。

**code-quality-automation 已完成**：Local Data Root、显式 legacy 迁移、In-place Project Workspace、`project.code` Workflow Context Binding、ScriptNode，以及真实 `go-static-analysis` / `go-coverage-check` / `go-race-check` / `go-complexity-check` 已落地。四者使用不可变 POSIX Script Bundle 在 Darwin/Linux 的用户 PATH 上原地运行并产出严格 Result；详细合同见 `.scratch/code-quality-automation/spec.md`。

**14 后 Product Workflow 01–10 已完成**：Wails macOS 产品壳、Browser Mock 与 Desktop Adapter 共享 WorkflowClient / Product Application seam；用户可以创建 SQLite Product Workflow，编辑唯一 Draft，从 Catalog 创作 `human-chat` / `llm-chat`、Input Binding / Control Dependency，并管理 SQLite `Provider -> Models`。Draft autosave 使用 expected `lock_version` CAS，Preview 在非法 Draft 下仍返回可识别图与聚合 Diagnostics。`StartRun` 校验 UI token、物化默认 Gum Model UUID、创建或复用 immutable Revision、固定不含 Secret 明文的 Run Snapshot；P10 起单轮 `human-chat(source) -> llm-chat` 经 `internal/chat` 的 OpenAI-compatible 非流式 Protocol Adapter 发起真实 Chat Completions 调用，完整响应后追加 assistant 消息并持久化 filesystem-backed Conversation Artifact，usage / finish reason / Provider request ID 进入 Node Run diagnostics；认证、限流、网络与协议错误是 Structural Error，失败零部分状态且错误不携带 API Key。相同规范化语义哈希重复运行复用 Revision，每次创建新 Run，UI 可按 Workflow → Revision → Run → Node Run/Artifact 分层浏览并在重启后查询。macOS Provider API Key 已通过注入的 Keychain Secret Adapter 保存，SQLite 只持久化引用，普通 ViewModel 不返回明文或引用；Browser Mock 与测试使用 Memory Adapter 与本地 fixture server。独立 product schema 不读取或复用 workflow/v1 YAML identity。人工输入、Interrupted 与 Resume 尚未实现；下一票是 `.scratch/product-workflow/issues/11-dangling-model-uuid-diagnostics.md`。

后续版本方向（需先升级设计文档）：真实 Coding Agent Adapter（替换 MockCodingAgent）、真实 OpenAPI Generator、Skipped 传播、重试/超时等 workflow/v2 字段。

## 硬性设计约束（任何代码变更不得违反）

这些约束来自设计计划，是架构层面的决定，不是建议：

1. **数据依赖优先**：`inputs.<name>.from: <node-id>.<output>` 隐式产生 Data Edge。`dependsOn` 仅表示 Control Edge（显式执行顺序约束），永远不是表达数据依赖的方式。
2. **Workflow 与 Node 解耦**：Workflow YAML 只声明组合；Node 通过 Registry 注册。同一个 Node Type 可被多次实例化（Node ID 与 Node Type 分离）。
3. **Artifact 是 Node 间唯一数据通道**：运行时传递 `ArtifactRef`（引用），不传递大型数据本体（如源码内容，`SourceCode` 只存 repo path / commit / workspace 引用）。
4. **Node 运行条件与迭代**：`Ready(Node) = InputsReady AND ControlDependenciesCompleted`；上游出现新 Artifact 版本或 Control 前驱出现新完成轮时可重新 Ready。环是合法迭代路径，校验只提示纯机器环，由收敛保护阻止无人工事件的死循环。全 Workflow 恰好一个无输入无依赖的 human-input 入口。
5. **CLI 不接受业务参数**：只有 `workflow run <workflow-file>`、`workflow validate <workflow-file>` 与 `workflow history [<run-id> [<node-id>]]`，不提供业务 flags。所有运行配置来自 YAML 与用户级 `llm.yaml`。
6. **Workflow 不管理 Skills**：Coding Agent 自行进入 Project Workspace 并发现 `.agents/skills/`、`.claude/skills/` 等项目约定。
7. **两层 Validation**：CUE Schema（结构）→ Go Semantic Validator（语义：Node Definition / Executor / LLM / Project 存在，端口与 Artifact 类型匹配，唯一 human 入口，环提示）。错误信息必须指明具体 Node 与字段。
8. **平台核心批准范围与排除项**：有环迭代、human 在环和本地 SQLite 统一库已经由平台核心设计批准并实现。workflow/v1 仍不接入 Product UI，也不增加 Temporal、Redis/Kafka/服务端数据库、分布式调度、多租户、Skill/Agent Marketplace、resume、复杂 Retry/Condition/Secret Management 或 `retry/timeout/parallelism/environment/hooks` 等字段。14 后 Product Workflow 只按 `.scratch/product-workflow/` 的独立设计与票据推进，不得倒灌 workflow/v1。

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
