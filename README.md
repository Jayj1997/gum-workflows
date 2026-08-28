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

核心方向：

- **本地优先**：Workflow、运行历史、Artifact 和 Workspace 默认保存在本地；云同步属于后续演进。
- **Node 优先**：先打磨 Node Definition、Node Executor、真实 Agent Node、Artifact 和调试能力，再建设成熟 Workflow。
- **数据依赖优先**：Node 通过 Artifact 交换数据；Data Edge 由 Input Binding 自动产生，`dependsOn` 只表示 Control Edge。
- **迭代与人工在环**：Workflow 可以表达“产出 → 审批 → 带意见重做”的多轮过程，而不只是运行一次就结束的无环 DAG。
- **可观察、可调试、可恢复**：每次 Node Run、Artifact 版本、错误和人工事件都应可追溯，并能在明确位置继续。
- **GUI 是未来的主要创作入口**：用户通过结构化配置声明 Node 和连接；预览界面自动排列执行结构，不使用拖拽节点、手工拉线或画布坐标表达执行语义。

当前 YAML 是 Runtime 开发、测试和 CLI 调试入口。产品接近稳定 v1 后，再规划将 Workflow 导出为 YAML、通过 Git 管理以及重新导入的完整能力。

## 当前状态

项目分为三个连续阶段：

| 阶段 | 状态 | 内容 |
|---|---|---|
| workflow/v1 MVP | 已完成 | YAML Loader、CUE/语义校验、DAG、串行/并行 Engine、Artifact Store、Workspace、Mock Node、CLI 与 e2e |
| 平台核心 01–14 | 推进中 | 定义体系、迭代引擎、人工在环、LLM 配置解析、SQLite 历史与 history CLI |
| 14 后产品化 | 已完成设计、尚未实施 | 本地 GUI、Draft/Revision、独立 LLM Config、真实 `llm-chat`、Artifact 体验、运行恢复 |

平台核心 01–14 当前已完成 01–05：

- TypeExpr 端口类型语言；
- Node Type / Node Definition / Node Executor 定义与内嵌种子；
- ExecutorRegistry 与 Node 接口瘦身；
- 用户级 LLM 配置加载与默认解析链；
- 新 workflow/v1 Node Instance Schema 与最小示例迁移。

06–14 按既定依赖顺序继续推进，完成前不会实施 14 后产品化能力。

> 当前内置 Node 仍为 Mock 实现；`internal/llm` 当前只负责配置与解析，不包含真实网络调用。

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
| **Workspace** | 一次 Run 独享的项目副本，Agent 在副本内工作 |
| **Human Approval / Advise** | 人工审批及拒绝意见；拒绝不是运行失败，而是驱动 Agent 新一轮执行 |

术语权威见 [`CONTEXT.md`](CONTEXT.md)。

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
- **Control Edge**：`dependsOn`；只表达无数据传递的先后约束。
- **Ready**：`InputsReady AND ControlDependenciesCompleted`。
- **Artifact 不变式**：Node 间只传 `ArtifactRef`，数据本体由 Artifact Store 管理。

01–14 完成后，Node 在上游出现新 Artifact 版本时可以再次 Ready；每轮执行拥有独立 Node Run ID 和 round，历史 Artifact 版本不会被覆盖。

## 快速开始

当前开发环境要求 Go 1.25+（`go.mod` 基线为 1.25.0）。

```bash
git clone git@github.com:Jayj1997/gum-workflows.git
cd gum-workflows

go build ./...
go test ./...
go vet ./...
```

校验当前最小示例：

```bash
go run ./cmd/workflow validate examples/minimal/workflow.yaml
```

运行当前 Mock Workflow：

```bash
go run ./cmd/workflow run examples/minimal/workflow.yaml
```

当前 CLI 只提供：

```text
workflow validate <workflow-file>
workflow run <workflow-file>
```

`workflow history` 将在平台核心 13 中实现。CLI 不接受 Workflow 业务参数；当前配置全部来自 Workflow YAML 和用户级 LLM 配置。

## 当前 Workflow 示例

[`examples/minimal/workflow.yaml`](examples/minimal/workflow.yaml) 是 01–14 开发期间的临时 human-free 示例：

```yaml
apiVersion: workflow/v1
kind: workflow

metadata:
  name: minimal-development
  version: "1.0"

projects:
  - name: order-system
    repository: ./project

nodes:
  coder:
    node: coding-agent
    config:
      task: 实现 order-system：产出源码与 OpenAPI 文档

  sdk:
    node: openapi-generator
    inputs:
      openapi:
        from: coder.openapi
```

该示例用于验证当前 Loader、Validator、ExecutorRegistry、Engine、Artifact Store 和 Workspace，不代表未来 GUI 的创作方式。完整 human-input + approval 循环示例将在平台核心 14 重写。

## 当前内置 Node Definition

| Node Definition | 类别 | Input | Output | 当前执行器 |
|---|---|---|---|---|
| `requirement-analysis` | agent | `requirement: markdown` | `rationality: int`、`analysis-output: markdown` | Mock |
| `architecture-design` | agent | `analysis-output: markdown` | `architecture: ArchitectureSpec` | Mock |
| `coding-agent` | agent | 多个可选开发 Artifact | `source-code: SourceCode`、`openapi: OpenAPI` | Mock |
| `openapi-generator` | automation | `openapi: OpenAPI` | `frontend-sdk: FrontendSDK` | Mock |

平台核心完成后还会包含 `human-input` 与 `human-approval`。14 后产品化阶段首先实现简单但真实的 `llm-chat` Agent Node，以验证真实模型调用、文本/图片输入、多轮对话、流式输出、模型能力和 Artifact 预览。

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
| 06 | 扩展语义校验与环降提示 | 按序推进 |
| 07 | SQLite 定义侧导入 | 待完成 |
| 08 | 迭代引擎核心 | 待完成 |
| 09 | human-input 与多轮输入 | 待完成 |
| 10 | human-approval 与 approve 门控 | 待完成 |
| 11 | Structural/Interaction Error 与 advise retry | 待完成 |
| 12 | Node Run 粒度历史落库 | 待完成 |
| 13 | history CLI | 待完成 |
| 14 | examples、e2e 与文档收尾 | 待完成 |

## 包结构

```text
cmd/workflow          当前 CLI：validate / run
internal/definition   Node Type / Definition / Executor、TypeExpr 与种子
internal/llm          用户级 LLM 配置加载和模型选择解析（当前无网络 Client）
internal/workflow     Workflow 定义、Loader 与 Graph
internal/validation   CUE + Semantic Validation
internal/node         ExecutorRegistry 与内置 Executor
internal/execution    Engine、Scheduler、状态与持久化
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
4. Node 的调度依据是输入与 Control Dependency 是否满足。
5. Workflow 不管理 Coding Agent 的 Skills；Agent 在 Workspace 中按项目约定自行发现。
6. CUE 结构校验与 Go 语义校验分层，错误必须定位到 Node 和字段。
7. 01–14 未完成前，不把 14 后 GUI、真实 LLM、恢复和产品模型倒灌进当前票。
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

参与开发请先阅读 [`AGENTS.md`](AGENTS.md) 与 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)。当前按 `.scratch/platform-core/issues/` 中 01–14 严格顺序推进；一个阶段未验收，不开始依赖其语义的下一阶段。
