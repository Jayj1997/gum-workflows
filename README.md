# gum-workflows

Gum-Workflows 是一个基于 Go 的、本地优先的轻量级 Workflow Runtime，并正在向面向技术人员的本地工作流产品演进。

用户通过 Node 的 Input / Output Contract 声明工作过程；Runtime 根据 Artifact 数据依赖形成执行结构，持续推进可运行的 Node，并记录每一轮输入、输出和人工干预。

```text
Human Input
    ↓
Requirement Analysis
    ↓
UI Design
    ↓ design-file: figma
Frontend
    ↓
Human Approval ── reject + advise ──┐
    ▲                               │
    └───────────────────────────────┘
```

## 产品方向

Gum-Workflows 面向代码开发、产品经理、测试、设计和运维等能够描述自身工作过程的技术人员。

它的初衷是自动化用户原本就在重复执行的中间处理，而不是重新发明用户的工作方式。例如开发者本来就会“修改代码 → 运行多项检查 → 阅读结果 → 继续开发”，Workflow 应把这些环节和 Artifact 直接串联起来。额外复制、隔离、版本或恢复机制只在有明确用户需求或已验证风险时引入。

核心方向：

- **本地优先**：Workflow、运行历史与 Artifact 保存在用户级 Local Data Root，Workspace 直接使用本地项目目录；云同步属于后续演进。
- **Node 优先**：先打磨 Node Definition、Node Executor、真实 Agent Node、Artifact 和调试能力，再建设成熟 Workflow。
- **数据依赖优先**：Node 通过 Artifact 交换数据；Data Edge 由 Input Binding 自动产生，`dependsOn` 只表示 Control Edge。
- **迭代与人工在环**：Workflow 可以表达“产出 → 审批 → 带意见重做”的多轮过程，而不只是运行一次就结束的无环 DAG。
- **可观察、可调试、可恢复**：每次 Node Run、Artifact 版本、错误和人工事件都应可追溯，并能在明确位置继续。
- **GUI 是未来的主要创作入口**：用户通过结构化配置声明 Node 和连接；预览界面自动排列执行结构，不使用拖拽节点、手工拉线或画布坐标表达执行语义。
- **现实工作流优先**：Runtime 负责组合、调度和留存结果，不默认为每个 Node 复制工作区、创建内部代码 Revision 或承担用户未要求的代码恢复职责。
- **代码修改原地生效**：14 后产品态中，Agent 与 Automation 直接使用用户项目目录；Agent 修改实时可见，项目版本和恢复交给用户已有工具。Gum 只把数据库、日志、tool-output 和 Result 写入 Local Data Root。

当前 YAML 是 Runtime 开发、测试和 CLI 调试入口。产品接近稳定 v1 后，再规划将 Workflow 导出为 YAML、通过 Git 管理以及重新导入的完整能力。

## 当前状态

项目分为三个连续阶段：

| 阶段 | 状态 | 内容 |
|---|---|---|
| workflow/v1 MVP | 已完成 | YAML Loader、CUE/语义校验、DAG、串行/并行 Engine、Artifact Store、Workspace、Mock Node、CLI 与 e2e |
| 平台核心 01–14 | 已完成 | 定义体系、迭代引擎、人工在环、LLM 配置解析、SQLite 历史与 history CLI |
| 14 后产品化 | 实施中 | Local Data Root、In-place Project Workspace、`project.code` Workflow Context Binding 与真实 Static/Coverage/Race 三个 Go 质量节点已落地；Complexity、本地 GUI、Draft/Revision、独立 LLM Config、真实 `llm-chat`、Artifact 体验与运行恢复尚未实施 |

平台核心 01–14 已完成：

- TypeExpr 端口类型语言；
- Node Type / Node Definition / Node Executor 定义与内嵌种子；
- ExecutorRegistry 与 Node 接口瘦身；
- 用户级 LLM 配置加载与默认解析链；
- 新 workflow/v1 Node Instance Schema；
- 环降为 warning 的语义校验与版本驱动迭代引擎；
- human-input、human-approval、advise retry 与错误二分；
- SQLite Node Run 历史、三级 history CLI 与 fullstack Demo。

> 当前 agent 与 OpenAPI Generator 仍为 Mock；Static/Coverage/Race 三个 Go 质量节点已通过固定 POSIX Script Bundle 真实运行用户 PATH 中的 Go 工具链。`internal/llm` 当前只负责配置与解析，不包含真实网络调用。

当前 `run`、Artifact 与全局 `history` 使用用户级 Local Data Root，不再向项目写入 `.workflow`。可用 `GUM_WORKFLOWS_DATA_ROOT` 指定该目录；未指定时使用操作系统默认的用户应用数据位置。Run UUID 同时作为历史主键与 `runs/<run-id>/` 目录身份。旧项目内 `.workflow` 数据不会自动扫描；开发期需要保留时可显式调用 `history.MigrateLegacy` 一次性迁移。

## 核心概念

### 定义侧

| 概念 | 含义 |
|---|---|
| **Node Type** | 按执行主体划分的类别：agent / automation / human |
| **Node Definition** | 平台认识的节点本体，声明 inputs / outputs Contract 与 requires |
| **Node Executor** | 某个 Node Definition 的可执行版本；同一定义可以并存 v1 / v2 |
| **Node Instance** | Workflow 中对 Node Definition 的一次使用，包含节点 ID、Executor、输入绑定和配置 |
| **Workflow** | Node Instance 的组合声明；同一个 Workflow 可以运行任意多次 |
| **LLM Config** | 用户级、可复用的模型连接配置；14 后产品中由 Agent Node 从 UI 选择，不重复填写模型地址和密钥 |

### 运行侧

| 概念 | 含义 |
|---|---|
| **WorkflowExecution** | Workflow 的一次运行快照 |
| **NodeExecution** | 一个 Node Instance 在本次 WorkflowExecution 中的当前状态与历史摘要 |
| **Node Run** | Node Instance 的一次具体执行；迭代 Workflow 中同一节点可以产生多个 round |
| **Artifact / ArtifactRef** | Node 之间唯一的数据通道；Runtime 传引用，不传大型数据本体 |
| **Workspace** | Project Definition 指向的用户项目目录，Agent 与 Automation 在原地共享使用 |
| **Human Approval / Advise** | 人工审批及拒绝意见；拒绝不是运行失败，而是驱动 Agent 新一轮执行 |

术语权威见 [`CONTEXT.md`](CONTEXT.md)。

Gum 不为 Run 或 Node 创建代码副本、Snapshot 或 Revision；代码版本与恢复由用户现有工具负责。

## 数据依赖与运行条件

Data Edge 由 Input Binding 隐式产生：

```yaml
nodes:
  frontend:
    node: frontend
    inputs:
      design:
        from: ui-design.design-file
```

这表示 `frontend.design` 消费 `ui-design.design-file`。Runtime 因此知道 Frontend 必须等待 UI Design 产出，而无需再把数据依赖重复写入 `dependsOn`。

- **Data Edge**：`inputs.<name>.from: <node-id>.<output>`；表达数据依赖。
- **Workflow Context Binding**：`inputs.code.from: project.code`；把指向 In-place Project Workspace 的类型化 `SourceCode` 引用注入 Node，不形成 Node Data Edge，也不复制源码。
- **Control Edge**：`dependsOn`；只表达无数据传递的先后约束。
- **Ready**：`InputsReady AND ControlDependenciesCompleted`。
- **Artifact 不变式**：Node 间只传 `ArtifactRef`，数据本体由 Artifact Store 管理。

Node 在上游出现新 Artifact 版本时可以再次 Ready；每轮执行拥有独立 Node Run ID 和 round，历史 Artifact 版本不会被覆盖。

## 快速开始

当前开发环境要求 Go 1.25+（`go.mod` 基线为 1.25.0）。

```bash
git clone git@github.com:Jayj1997/gum-workflows.git
cd gum-workflows

go build ./...
go test ./...
go vet ./...
```

fullstack 包含 agent 节点，validate 与 run 都需要用户级 `llm.yaml`。内置 Agent 当前是 Mock，不会发起网络请求；首次体验可复制仅供本地 Demo 使用的配置：

```bash
mkdir -p "${XDG_CONFIG_HOME:-$HOME/.config}/gum-workflows"
cp -n examples/fullstack/llm.example.yaml "${XDG_CONFIG_HOME:-$HOME/.config}/gum-workflows/llm.yaml"
```

校验当前 fullstack 示例：

```bash
go run ./cmd/workflow validate examples/fullstack/workflow.yaml
```

运行当前 Mock Workflow：

```bash
go run ./cmd/workflow run examples/fullstack/workflow.yaml
```

当前 CLI 只提供：

```text
workflow validate <workflow-file>
workflow run <workflow-file>
workflow history
workflow history <run-id>
workflow history <run-id> <node-id>
```

CLI 不接受 Workflow 业务 flags；当前配置全部来自 Workflow YAML 和用户级 LLM 配置。含 human Node 的 `run` 要求 stdin 是交互式终端。

## 当前 Workflow 示例

[`examples/fullstack/workflow.yaml`](examples/fullstack/workflow.yaml) 是当前人工在环 Demo：

```yaml
apiVersion: workflow/v1
kind: workflow

metadata:
  name: fullstack-development
  version: "1.0"

projects:
  - name: order-system
    repository: ./project

nodes:
  requirement:
    node: human-input
  analysis:
    node: requirement-analysis
    inputs:
      requirement: {from: requirement.requirement}
  backend:
    node: coding-agent
    inputs:
      analysis-output: {from: analysis.analysis-output}
      advise: {from: review.advise}
  review:
    node: human-approval
    dependsOn: [backend]
```

完整文件还包含 architecture、OpenAPI/SDK 与 frontend 分支。运行时可输入多轮需求；审批 reject 产出 advise 驱动返工，approve 后图静止但 Run 继续等待，直至 Ctrl-C 记为 Stopped。YAML 仍是 Runtime/CLI 入口，不代表未来 GUI 的创作方式。

## 当前内置 Node Definition

| Node Definition | 类别 | Input | Output | 当前执行器 |
|---|---|---|---|---|
| `human-input` | human | — | `requirement: markdown` | stdin |
| `requirement-analysis` | agent | `requirement: markdown` | `rationality: int`、`analysis-output: markdown` | Mock |
| `architecture-design` | agent | `analysis-output: markdown` | `architecture: ArchitectureSpec` | Mock |
| `coding-agent` | agent | 多个可选开发 Artifact | `code: SourceCode`、`openapi: OpenAPI` | Mock |
| `openapi-generator` | automation | `openapi: OpenAPI` | `frontend-sdk: FrontendSDK` | Mock |
| `go-static-analysis` | automation | `code: SourceCode` | `result: QualityCheckResult` | 真实 `go vet` ScriptNode（Darwin/Linux） |
| `go-coverage-check` | automation | `code: SourceCode` | `result: QualityCheckResult` | 真实 statement coverage ScriptNode（Darwin/Linux） |
| `go-race-check` | automation | `code: SourceCode` | `result: QualityCheckResult` | 真实 Go Race Detector ScriptNode（Darwin/Linux） |
| `human-approval` | human | —；dependsOn 被审 Node | `approve: bool`、`advise: markdown` | stdin |

`go-static-analysis` 固定运行 full-scope `go vet -json ./...`；`go-coverage-check` 固定运行禁用缓存的 full-scope Go JSON 测试并按 statement coverage 阈值判定；`go-race-check` 在平台、CGO 和 C 编译器 Requirement 满足后固定运行 `go test -race -count=1 -json ./...`。三者都以 `passed`、`failed`、`not-applicable` 的严格 `qualityCheckResult/v1` 表达业务结果，工具或产物故障是 Structural Error。Race 的 `passed` 只表示本次执行未观察到 race，不声称项目不存在数据竞争。Complexity 与真实 `llm-chat` 仍需按各自票据实施。

## LLM Config 方向

LLM Config 是独立的用户级配置，不属于某个 Agent Node：

```text
LLM Config: 公司模型网关
├── Protocol: OpenAI-compatible
├── Base URL
├── API Key 引用
├── Models
└── Default Model

Agent Node
└── LLM Selection
    ├── Config: 公司模型网关
    └── Model: model-a
```

14 后计划包括：

- OpenAI-compatible Chat Completions 与 Anthropic Messages；
- 正确组装 system/developer instruction、user/assistant 历史和多模态 Content Part；
- 根据 Base URL 和 API Key 调用模型目录接口；
- 标准化服务端返回的模型能力，同时保留原始响应；
- 服务端未提供完整信息时允许手工声明或补充模型；
- Agent Node 从 UI 下拉选择已配置的 LLM Config 和 Model；
- API Key 存入本机安全凭据存储，Workflow、SQLite 和运行历史只保存引用。

## 结构预览与未来 GUI

未来 GUI 的 Workflow 创作流程：

```text
声明 Node
→ 配置 Input Binding / dependsOn
→ 自动生成结构预览
→ 点击 Node 配置
→ 校验并形成 Revision
→ Run
```

预览画布：

- 自动按 Data/Control Edge 分层排列；
- 支持分支、汇合和循环组；
- 可以选择 Node 或 Edge 查看配置和诊断；
- 不允许用拖动位置改变顺序；
- 不通过画布拉线建立依赖；
- 非法或未完成的 Draft 仍能显示，并定位缺失绑定和类型错误；
- Definition Mode 与 Run Mode 复用同一结构，运行时覆盖 Ready、Running、WaitingHuman、Failed 和 round 等状态。

目标平台是 macOS 和 Windows。默认桌面技术候选为 Go Runtime + Web 前端 + Wails，但需要先通过跨平台 Prototype Gate，领域模型不依赖具体桌面框架。

## 14 后产品化路线

详细计划见：

[`plans/Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Config 设计计划.md`](<plans/Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Config 设计计划.md>)

| 阶段 | 主要交付 |
|---|---|
| P9 | Workflow / Draft / immutable Revision / Run Snapshot 与 Application 模块 |
| P10 | 独立 LLM Config、Model Catalog、模型发现和 Secret Adapter |
| P11 | 双协议 Client 与真实 `llm-chat` Agent Node |
| P12 | Node Config Schema、能力描述、Workflow Preview 和自动布局 |
| P13 | macOS / Windows 桌面 GUI MVP |
| P14 | Artifact Preview、来源追踪、版本比较和 Manual Artifact |
| P15 | Run Event、Pause/Resume、Retry/Rerun/Fork 与崩溃恢复 |
| P16 | 稳定性、Schema Migration、跨平台构建与产品 v1 评审 |

### 后续待办（唯一跟踪清单）

- **项目语言检测**：首批 Code Quality Check 通过 Go 专用 Node Definition 明确语言；多语言和多子项目场景需补充自动检测、用户选择与覆盖语义。
- **Changed Scope**：首批检查只支持 full scope；后续基于用户现有 Git/项目工具定义 ChangeSet 与受影响依赖的保守边界，不默认复制项目或创建 Runtime 内部代码 Revision。
- **Container Execution Environment**：首版使用 Host Execution Environment 和客户 PATH，只运行受信任项目；后续增加可复用的容器配置，固定工具链、网络与资源策略。
- **Windows / WSL Script Runtime**：首批内置 Automation Script 只使用一份 POSIX `check.sh` 支持 Darwin/Linux，不提供 PowerShell 变体。后续单独设计 Windows 宿主上的统一 POSIX Script Runtime 或 WSL 后端，包括发行版、路径映射、PATH/工具发现、文件权限与进程取消语义。
- **条件执行与 Skipped 传播**：首批 Code Quality Check 只产出结构化结果供人类查看或下游 Node 消费，不新增自动路由语义。后续单独设计类型化 `when`、条件重新求值、false/else、Skipped 传播与迭代图中的 Artifact 版本语义。
- **用户自定义 Automation Script**：在预制质量节点的固定 Script Bundle/Result Adapter 协议稳定后，允许用户通过编辑脚本创建 automation Node Executor，而不是在 Node Instance 中填写一条任意 shell command。需单独设计脚本包身份/版本、跨平台运行时、Result Channel/Schema、权限、校验、分发和升级语义；不属于首批 Code Quality Check 实现范围。

只有产品 v1 的领域模型和交互稳定后，才规划 Workflow 导入/导出、Git 版本管理、Workflow Pack、AI 修改 Workflow、内置 Workflow 库和云同步。

## 平台核心 01–14

完整规格：

- [`plans/平台核心设计：组件定义体系与迭代执行引擎.md`](<plans/平台核心设计：组件定义体系与迭代执行引擎.md>)
- [`.scratch/platform-core/spec.md`](.scratch/platform-core/spec.md)
- [`.scratch/platform-core/issues/`](.scratch/platform-core/issues/)

| Ticket | 内容 | 当前状态 |
|---|---|---|
| 01 | TypeExpr 端口类型语言 | 已完成 |
| 02 | Node Definition 类型与种子 | 已完成 |
| 03 | ExecutorRegistry 与 Node 接口瘦身 | 已完成 |
| 04 | LLM 配置与默认解析链 | 已完成 |
| 05 | 新 workflow/v1 Schema | 已完成 |
| 06 | 扩展语义校验与环降提示 | 已完成 |
| 07 | SQLite 定义侧导入 | 已完成 |
| 08 | 迭代引擎核心 | 已完成 |
| 09 | human-input 与多轮输入 | 已完成 |
| 10 | human-approval 与 approve 门控 | 已完成 |
| 11 | Structural/Interaction Error 与 advise retry | 已完成 |
| 12 | Node Run 粒度历史落库 | 已完成 |
| 13 | history CLI | 已完成 |
| 14 | examples、e2e 与文档收尾 | 已完成 |

## 包结构

```text
cmd/workflow          当前 CLI：validate / run / history
internal/definition   Node Type / Definition / Executor、TypeExpr 与种子
internal/llm          用户级 LLM 配置加载和模型选择解析（当前无网络 Client）
internal/workflow     Workflow 定义、Loader 与 Graph
internal/validation   CUE + Semantic Validation
internal/node         ExecutorRegistry 与内置 Executor
internal/execution    Engine、Scheduler、状态与持久化
internal/history      SQLite 定义导入、Node Run 历史与查询
internal/artifact     Artifact / ArtifactRef / Store
internal/project      Project Runtime 与 Workspace
internal/agent        Agent Adapter（当前为 Mock）
schema/workflow       内嵌 workflow/v1 CUE Schema
```

## 设计约束

任何实现不得绕开这些约束：

1. Data Edge 来自 Input Binding；`dependsOn` 只表达 Control Edge。
2. Workflow 与 Node Definition / Executor 解耦。
3. Artifact 是 Node 间唯一数据通道，Runtime 传递 ArtifactRef。
4. Node 的调度依据是输入与 Control Dependency 是否满足；新版本可令成功或交互失败的 Node 再次 Ready。
5. Workflow 不管理 Coding Agent 的 Skills；Agent 在 Workspace 中按项目约定自行发现。
6. CUE 结构校验与 Go 语义校验分层，错误必须定位到 Node 和字段。
7. 不把 14 后 GUI、真实 LLM、恢复和产品模型倒灌进当前 Runtime。
8. 设计发生变化时先更新对应的新设计文档，再修改实现。

## 文档导航

- [`CONTEXT.md`](CONTEXT.md)：项目领域语言。
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)：开发规范、分层、测试与推进纪律。
- [`docs/domain-model.md`](docs/domain-model.md)：当前实现的领域模型说明。
- [`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`](<plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md>)：历史 MVP 设计，只读。
- [`plans/平台核心设计：组件定义体系与迭代执行引擎.md`](<plans/平台核心设计：组件定义体系与迭代执行引擎.md>)：01–14 平台核心设计。
- [`plans/Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Config 设计计划.md`](<plans/Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Config 设计计划.md>)：14 后产品化设计。

## 开发

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
```

参与开发请先阅读 [`AGENTS.md`](AGENTS.md) 与 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)。平台核心 01–14 已完成；14 后能力须先完成对应新设计和开发票。
