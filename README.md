# gum-workflows

基于 Go 的轻量级 Workflow Runtime。用 YAML 声明 Workflow，通过 Node 的 Input / Output Contract 自动形成 DAG，按 Artifact 数据依赖持续执行。

```text
Requirement → Architecture → Backend Coding Agent → OpenAPI → OpenAPI Generator → Frontend Coding Agent
```

上面的软件开发流程不需要写一行胶水代码：声明每个 Node 消费什么、产出什么，Runtime 自动推导执行顺序。Workflow 增删 Node、调整组合，只改 YAML，不改 Go 代码。

## 当前状态

**workflow/v1 MVP 已完成**：Core Model、YAML Loader、CUE + 语义两层校验、DAG Builder / Validator、串行与并行执行引擎、文件系统 Artifact Store 与状态持久化、Project Runtime / Workspace、CLI（`validate` / `run`）、fullstack 端到端示例。当前所有内置 Node 均为 Mock 实现（先跑通 Runtime，再接真实 Agent——见[项目规划](#项目规划与进展)）。

`go test ./...`（含 `-race`）全绿。

## 核心概念

| 概念 | 含义 |
|---|---|
| **Workflow** | Node 的组合声明（YAML），不属于任何 Go 代码 |
| **Node** | `Input → Execute → Output` 的执行单位；同一 Node Type 可实例化多次（Node ID 与 Node Type 分离） |
| **Artifact** | Node 之间唯一的的数据通道；运行时只传 `ArtifactRef`（引用），不传数据本体 |
| **Project** | Workflow 的运行环境（repository / branch / workspace） |
| **Execution** | Workflow 的一次实际运行，状态持久化到 `.workflow/` |

DAG 有两种 Edge，数据依赖优先：

- **Data Edge（隐式）**：`inputs.<name>.from: <node-id>.<output>` 自动产生数据依赖。这是最主要的连接方式——B 消费 A 的输出，Runtime 就知道 B 必须等 A，不需要手写 `dependsOn`。
- **Control Edge（显式）**：`dependsOn` 只表达执行顺序（如 `Human Approval → CD` 这种没有数据传递、但有先后顺序的场景），永远不是表达数据依赖的方式。

Node 的运行条件：`Ready(Node) = InputsReady AND ControlDependenciesCompleted`。无输入、无依赖的 Node 是合法的源节点（Trigger / Source）。

## 快速开始

要求 Go 1.24+。

```bash
git clone git@github.com:Jayj1997/gum-workflows.git
cd gum-workflows
go build ./...

# 校验 Workflow 定义（CUE 结构校验 + 语义校验）
go run ./cmd/workflow validate examples/fullstack/workflow.yaml
# fullstack-development: valid (workflow/v1)

# 运行示例 Workflow
go run ./cmd/workflow run examples/fullstack/workflow.yaml
```

运行输出：

```text
Workflow fullstack-development Succeeded (execution-000001)
Nodes:
  architecture  Succeeded architecture-design
  backend       Succeeded coding-agent
  frontend      Succeeded coding-agent
  openapi       Succeeded openapi-generator
  requirement   Succeeded requirement-analysis
Artifacts:
  architecture  architecture    ArchitectureSpec
  backend       openapi         OpenAPI
  backend       source-code     SourceCode
  frontend      source-code     SourceCode
  openapi       frontend-sdk    FrontendSDK
  requirement   requirement     RequirementSpec
```

每次 `run` 在 `.workflow/executions/<execution-id>/` 下产生完全独立的运行记录，同一 Workflow 可运行任意多次、互不干扰：

```text
.workflow/
└── executions/
    └── execution-000001/
        ├── workflow.yaml           # 本次运行使用的定义快照
        ├── state.json              # Execution 级状态
        ├── nodes/<node-id>/state.json   # 每个 Node 的运行快照（状态、产出、错误）
        ├── artifacts/              # Artifact Store（文件系统实现）
        └── workspace/project/      # 项目副本，Coding Agent 的工作区
```

## 定义一个 Workflow

`examples/fullstack/workflow.yaml`：

```yaml
apiVersion: workflow/v1
kind: Workflow

metadata:
  name: fullstack-development
  version: "1.0"

project:
  repository: ./project
  branch: main

nodes:

  requirement:
    type: requirement-analysis

  architecture:
    type: architecture-design
    inputs:
      requirement:
        from: requirement.requirement

  backend:
    type: coding-agent
    config:
      task: 实现 order-system 后端：依据需求与架构产出源码与 OpenAPI 文档
    inputs:
      requirement:
        from: requirement.requirement
      architecture:
        from: architecture.architecture

  openapi:
    type: openapi-generator
    inputs:
      openapi:
        from: backend.openapi

  frontend:
    type: coding-agent
    config:
      task: 实现 order-system 前端：消费 OpenAPI 与生成的 Frontend SDK
    inputs:
      requirement:
        from: requirement.requirement
      openapi:
        from: backend.openapi
      frontend-sdk:
        from: openapi.frontend-sdk
```

要点：

- **CLI 不接受业务参数**：只有 `workflow run <file>` 和 `workflow validate <file>`，所有配置来自 YAML。
- **Workflow 不管理 Skills**：Coding Agent 自行进入 Project Workspace，自行发现 `.agents/skills/`、`.claude/skills/` 等项目约定。
- **每个 Execution 拥有独立的项目 Workspace 副本**：Agent 在副本上修改代码，源仓库与其他 Execution 互不污染。

## 内置 Node

当前四个内置 Node 均为 Mock 实现（设计策略：先用 Mock 跑通 Runtime，真实 Adapter 是下一阶段）：

| Node Type | 类别 | Input | Output |
|---|---|---|---|
| `requirement-analysis` | Mock | 无 | `requirement: RequirementSpec` |
| `architecture-design` | Mock | `requirement: RequirementSpec` | `architecture: ArchitectureSpec` |
| `coding-agent` | Agent（Mock） | 可选：requirement / architecture / openapi / frontend-sdk | `source-code: SourceCode`、`openapi: OpenAPI` |
| `openapi-generator` | Automation（Mock） | `openapi: OpenAPI` | `frontend-sdk: FrontendSDK` |

MVP 定义的 Artifact Kind 共 7 种：`RequirementSpec`、`ArchitectureSpec`、`OpenAPI`、`FrontendSDK`、`SourceCode`、`TestReport`、`ApprovalResult`。`SourceCode` 只引用 repo path / commit / workspace 路径，不携带源码本体。

## 架构

两层校验，通过后才允许进入 Runtime：

```text
workflow.yaml
    │
    ▼
CUE Schema 校验（结构：字段存在、类型正确）
    │
    ▼
Go Loader（yaml.v3 严格模式，未知字段报错）
    │
    ▼
语义校验（Node Type 已注册、Output 存在、Artifact Kind 匹配、DAG 无环）
    │
    ▼
Execution Engine（Ready Queue + 依赖计数，串行或并行调度）
```

包结构：

```text
cmd/workflow          CLI 入口：validate / run
internal/workflow     Workflow 定义、YAML Loader、DAG（Data Edge + Control Edge）
internal/validation   CUE Schema 校验 + 语义校验
internal/node         Node 接口与 Registry；builtins/ 内置 Node
internal/execution    执行引擎：调度、状态机、state.json 持久化
internal/artifact     Artifact / ArtifactRef / Store（文件系统实现）
internal/project      Project Runtime 与 Workspace
internal/agent        CodingAgent Adapter（当前为 Mock）
schema/workflow       workflow/v1 CUE Schema（embed 进二进制）
```

执行引擎特性：

- 默认严格串行；`WithParallelism(n)` 时由 worker goroutine 并发消费 Ready Queue（无依赖的 Node 并行执行）。
- 任一 Node 失败即停止派发新任务，Workflow 置 Failed；未执行的 Node 保持 Pending。
- Node 每次状态变化后刷新持久化快照，运行记录可事后审计。
- 错误信息一律定位到具体 Node ID 与字段。

## 项目规划与进展

设计计划：[`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`](plans/Workflow%20Engine%20MVP：workflow-v1%20设计与实现计划.md)（只读）。

### 已完成：workflow/v1 MVP（里程碑 M1–M11）

| 里程碑 | 内容 | 状态 |
|---|---|---|
| M1 | Core Model（Workflow / Node / Artifact / ArtifactRef / ProjectContext / Execution） | ✅ |
| M2 | YAML Loader（严格模式解析） | ✅ |
| M3 | CUE Validation（错误定位到字段与 Node） | ✅ |
| M4 | DAG Builder（Data Edge + Control Edge） | ✅ |
| M5 | DAG Validator（valid / invalid fixture 全覆盖） | ✅ |
| M6 | Execution Engine（串行） | ✅ |
| M7 | 并行 DAG（B / C 并行、D 等待） | ✅ |
| M8 | Project Runtime（Workspace 正确创建） | ✅ |
| M9 | OpenAPI Automation | ✅ Mock 实现 |
| M10 | Coding Agent Adapter | ✅ Mock 实现 |
| M11 | Fullstack Demo（§42 全流程跑通，e2e 测试覆盖） | ✅ |

### 下一步（需先升级设计文档）

- **真实 Coding Agent Adapter**：替换 `MockCodingAgent`，驱动真实 Agent 进入 Project Workspace 改码（接口已就位：`agent.CodingAgent`，替换时 Node / Workflow / Engine 均不变）。
- **真实 OpenAPI Generator**：替换 Mock 的 `openapi-generator`。
- **Skipped 传播**：上游失败后，下游未执行 Node 从 Pending 转为 Skipped。
- **重试 / 超时等 workflow/v2 字段**：属于 Schema 演进，须先升级设计文档。

### 明确不做（当前版本）

以下内容均不在 workflow/v1 范围内，加入前必须先升级设计文档：

UI / Web Dashboard、Temporal、Redis / Kafka / Database、分布式调度、多租户、Skill Registry、Workflow / Agent Marketplace、复杂 Retry、Workflow Condition、Workflow Version Migration、Secret Management，以及 workflow/v1 之外的 Schema 字段（retry / timeout / parallelism / environment / hooks 等）。

## 文档

- [`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`](plans/)——设计文档（只读）
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)——开发规范（技术栈、分层依赖、编码与测试规范、里程碑推进纪律）
- [`docs/domain-model.md`](docs/domain-model.md)——领域模型与类型设计（定义侧 / 运行侧、包分层、关键设计决策）

## 开发

```bash
go build ./...
go test ./...       # 含 race detector
go vet ./...
```

参与开发请先阅读 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)。核心纪律：里程碑严格按序推进；`plans/` 目录只读；CUE Schema 与 Go Struct 必须同步修改；`examples/` 下的 YAML 必须始终可被 `workflow validate` 通过。
