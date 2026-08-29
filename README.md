# gum-workflows

## 项目概述

Gum-Workflows 是一个基于 Go 的、本地优先的轻量级 Workflow Runtime，面向需要把开发、测试、设计、审核等重复工作环节连接起来的技术人员。

Workflow 通过 Node 的 Input / Output Contract 组合工作过程，Node 之间只传递 ArtifactRef。Runtime 根据 Data Edge 与 Control Edge 调度节点，允许有环迭代和人工介入，并记录每一次 Node Run 的输入、输出、错误与 Artifact 版本。

项目遵循“现实工作流优先”：Agent 直接修改用户项目，Automation 在同一份工作状态上执行检查；Gum 负责组合、调度、结果留存和诊断，不默认复制项目、创建内部代码 Revision 或接管代码恢复。

当前 YAML、CLI 与 Mock Agent 主要服务 Runtime 开发、验证和演示。产品目标是逐步形成以本地 GUI 为主要创作入口、以 Node 和 Artifact 为核心的本地工作流产品。

## 项目规划

基础 Runtime、平台核心和首个 14 后产品模块已经完成。后续产品化按 [`Gum-Workflows 产品化阶段设计计划`](<plans/Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Config 设计计划.md>) 推进，主要方向包括：

- 建立 SQLite 中的 Workflow、Draft、immutable Revision 与 Run Snapshot 产品模型；
- 升级独立 LLM Config，并实现真实的双协议 LLM Client 与 `llm-chat` Agent Node；
- 提供 Node Config、Workflow Preview、自动布局和 macOS / Windows 本地 GUI；
- 完善 Artifact 预览、来源追踪、多版本比较和人工替换；
- 设计结构化 Run Event，以及 Resume、Retry、Rerun、Fork 和崩溃恢复；
- 在领域模型稳定后，再规划 Workflow 导入导出、Pack、AI 修改 Workflow 和云同步。

Code Quality Check 的后续增强保留为独立新模块：Changed Scope、项目语言与子项目检测、条件执行与 Skipped 传播、Container Execution Environment、Windows / WSL Script Runtime，以及用户自定义 Automation Script。当前不承诺 Fuzz Node。

任何新模块都需要先形成设计文档和开发票，再修改实现。模块完成后的 README 与进度文档同步方法见 [`README 更新规范`](<plans/README 更新规范：模块完成后的进度同步.md>)。

## 项目当前进展

### code-quality-automation — 已完成

该模块把 Gum-Workflows 从 Mock automation 推进到可运行真实 Go 工具链的代码质量检查平台，同时完成代码工作流所需的本地数据与 Workspace 基础设施。

主要交付：

- 产品数据库、Artifact、日志、Bundle 和 tool-output 统一进入用户级 Local Data Root；新 Run 不再向项目内 `.workflow` 双写，旧数据可显式、幂等迁移；
- Project Definition 指向的目录成为 In-place Project Workspace，Agent 修改与 Automation 检查使用同一份源码状态；
- `project.code` 与普通 Node output 都可提供类型化 `SourceCode` ArtifactRef，历史保留触发链但不保存或恢复源码快照；
- ScriptNode 支持不可变 `automationScript/v1` Bundle、Manifest 与摘要校验、流式日志、正式工具产物、Result Adapter、进程组取消和 32 MiB 日志上限；
- 四个独立的 Go full-scope 检查已经落地：`go-static-analysis` 运行 `go vet`，`go-coverage-check` 计算 statement coverage，`go-race-check` 报告本次是否观察到 race，`go-complexity-check` 使用内嵌 Go AST Analyzer 计算单函数圈复杂度；
- 四个检查统一产出严格的 `qualityCheckResult/v1`。质量问题或阈值不达标是成功 Node Run 的 `failed` verdict；工具、协议或产物不完整才是 Structural Error；
- `examples/dogfood` 验证四个检查可在同一 Workspace 并发运行、各自留存 Result，且 Gum 产物只进入 Local Data Root；Darwin 与 Linux 的完整 test、vet、race 和脚本合同均已通过。

详细设计与实施记录见 [code-quality-automation spec](.scratch/code-quality-automation/spec.md) 和 [issues](.scratch/code-quality-automation/issues/)。

### platform-core — 已完成

该模块把早期 DAG Runner 重构为具有明确领域合同、可迭代执行、人工在环和可查询历史的平台核心。

主要交付：

- Node Type、Node Definition、Node Executor、Node Instance 四层定义体系，以及 TypeExpr 端口类型与 Registry；
- 用户级 `llm.yaml` 严格加载和 provider / model 默认解析链；该模块只解析配置，尚不包含真实 LLM 网络 Client；
- 允许有环的版本驱动 Execution Engine、节点单并发、dirty 合并与 convergence guard；
- `human-input`、`human-approval`、advise retry，以及 Structural Error / Interaction Error 二分；
- SQLite Node Run 历史、Artifact 多版本留存，以及 `workflow history` 的运行、节点和轮次三级查询；
- `examples/fullstack` 人工在环 Demo。Workflow 静止后仍保持 Running，直到用户 Ctrl-C / SIGTERM 后记为 Stopped。

详细设计与实施记录见 [平台核心设计](<plans/平台核心设计：组件定义体系与迭代执行引擎.md>)、[platform-core spec](.scratch/platform-core/spec.md) 和 [issues](.scratch/platform-core/issues/)。

### workflow/v1 MVP — 已完成

该阶段建立了项目最初的可运行骨架：workflow/v1 YAML、CUE 与语义校验、Data / Control Graph、Artifact Store、基础 Node 与 CLI，以及最早的 build、test 和 e2e 体系。其设计文档作为历史记录保留，不再直接承载当前产品状态。

### 当前能力边界

- `coding-agent`、需求分析、架构设计与 OpenAPI Generator 仍为 Mock；真实 Agent 与真实 OpenAPI Generator 尚未实现；
- 四个内置 Code Quality Check 当前只支持 Darwin / Linux，Windows 原生、PowerShell 与 WSL 后端尚未实现；
- Host Execution Environment 继承用户的 PATH、Go 配置、缓存、工具链与网络策略，适合受信任项目，但不是安全沙箱，也不提供容器、CPU / 内存隔离或自动 timeout；
- Static 只代表 `go vet`，Coverage 只报告本次 full-scope 测试的 statement coverage，Race 只报告本次是否观察到 race；
- 本地 GUI、Draft / Revision、产品化 LLM Config 与真实 LLM Client、Artifact 产品体验和运行恢复仍属于后续规划。

## 使用与文档

开发环境要求 Go 1.25+。常用命令：

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...

go run ./cmd/workflow validate <workflow-file>
go run ./cmd/workflow run <workflow-file>
go run ./cmd/workflow history [<run-id> [<node-id>]]
```

含 agent 节点的示例需要用户级 `llm.yaml`，但当前 Mock Agent 不会发起网络请求。可从 [`examples/fullstack/llm.example.yaml`](examples/fullstack/llm.example.yaml) 创建本地演示配置。人工在环示例位于 [`examples/fullstack`](examples/fullstack/)，真实质量检查示例位于 [`examples/dogfood`](examples/dogfood/)。

文档入口：

- [`CONTEXT.md`](CONTEXT.md)：领域词汇权威；
- [`docs/domain-model.md`](docs/domain-model.md)：当前已实现领域模型；
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)：开发、测试与文档规范；
- [`AGENTS.md`](AGENTS.md)：Agent 工作边界与项目约束；
- [`plans/`](plans/)：历史设计、产品化计划和 README 更新规范。
