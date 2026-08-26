# Domain Model 设计（Model 层 + Runtime 完整版）

> 覆盖设计计划 §44 开发顺序 ①-⑱（全部 MVP 里程碑）。本文档描述类型职责、包分层、命名映射与关键设计决策。**未实现（后续版本，需先升级设计文档）**：真实 Coding Agent Adapter、真实 OpenAPI Generator CLI、Skipped 传播、重试/超时。

## 0. 定义侧与运行侧（最重要的概念区分）

```text
定义（可以运行很多次）            运行（每次 run 独立）
─────────────────────           ─────────────────────
Workflow                        WorkflowExecution #001
  │                               ├── NodeExecution A
  ├── Node A  (id: backend,        ├── NodeExecution B
  │            type: agent)        └── NodeExecution C
  ├── Node B
  └── Node C                     WorkflowExecution #002
                                  ...（与 #001 完全独立）
```

| 概念 | 侧 | 实现 | 含义 |
|---|---|---|---|
| Workflow | 定义 | `workflow.Definition` | Node 的组合声明（YAML） |
| Node | 定义 | `workflow.NodeSpec` | 「有一个叫 backend 的节点」（id + type + inputs + dependsOn） |
| WorkflowExecution | 运行 | `execution.WorkflowExecution` | 一次 `workflow run`，如 execution-000001 |
| NodeExecution | 运行 | `execution.NodeExecution` | 一个 Node 定义在某次运行中的实例（状态、产出、错误的快照） |

规则：

1. **Node 与 NodeExecution 不得混用**。`node.Node` / `workflow.NodeSpec` 是能力与组合声明；`NodeExecution` 才有 Status/Outputs/Error。
2. 每次 `Engine.Run` 产生全新的 `WorkflowExecution`；同一 Workflow 可运行任意多次，运行对象之间零共享（含 Artifact）。
3. `NodeExecution` 快照 `NodeID` 与 `NodeType`：定义可能在两次运行之间变化，运行记录以实际实例化的类型为准。
4. 运行不回写定义：`Engine.Run` 之后 `workflow.Definition` 保持不变。
5. 这一区分是 `state.json`（`executions/<id>/nodes/<node>/state.json`，计划 §28）结构的直接依据。

## 1. 模型总览

五个核心概念（设计计划 §48）与本实现的对应：

```text
Workflow  = workflow.Definition   （YAML 静态声明：Node 的组合）
Node      = node.Node             （Input -> Execute -> Output 的执行单位）
Artifact  = artifact.Artifact     （Node 之间的数据本体）
            artifact.ArtifactRef  （运行时传递的引用）
Project   = project.Context       （Execution 的运行环境）
Execution = execution.WorkflowExecution（Workflow 的一次实际运行）
```

依赖与数据流：

```text
workflow.Definition
        │ 按 type 实例化（Registry，后续里程碑）
        ▼
     node.Node ──Execute(ExecutionContext, inputs)──> outputs
        │                                    │
        │       inputs / outputs 均为        ▼
        │       artifact.ArtifactRef   artifact.Store
        ▼
execution.WorkflowExecution（记录每个 NodeExecution 的 Status 与 Outputs）
```

## 2. 包分层

```text
internal/execution ──> internal/workflow ──> internal/node
                                              │
                                    ┌─────────┴─────────┐
                                    ▼                     ▼
                           internal/artifact        internal/project
```

- `artifact` 与 `project` 是基础包，零 internal 依赖。
- `node` 定义核心接口，依赖基础包。`node/builtins`（后续）额外依赖 `agent`。
- `workflow` 是静态声明，不感知执行；`execution` 是运行状态，不感知 YAML。
- 接口在消费处引用：`artifact.Store` 是 artifact 域的一部分故定义在 artifact 包；`node.Node` / `node.Factory` 定义在 node 包。

## 3. 类型职责

### 3.1 internal/artifact

| 类型 | 职责 |
|---|---|
| `Kind` | Artifact 类型标识（7 个 MVP 常量），Input/Output Contract 匹配的依据 |
| `Artifact` | 数据本体；`Data any`；大型数据（源码）只存引用信息 |
| `ArtifactRef` | 运行时引用（ID/Kind/Version/URI），`Validate()` 保证基本不变式 |
| `Store` | Put/Get/Exists 接口；FS 实现在 M6 后，未来可换 S3/OSS |
| `MemStore` | 纯内存实现（最小 Runtime 用）；同一 ID 多次 Put 产生不同 URI 的引用 |

**不变式**：Node 之间只传 `ArtifactRef`；`Artifact.Data` 只在 Node 内部消费时通过 `Store.Get` 加载。

### 3.2 internal/project

| 类型 | 职责 |
|---|---|
| `Repository` | 项目仓库（本地路径或远端地址） |
| `Context` | 一次 Execution 的项目环境：Repository / Branch / Workspace |

`Context` 由 YAML `project` 段经 Project Resolver（M8）解析生成，通过 `node.ExecutionContext` 传递给 Node。

### 3.3 internal/node

| 类型 | 职责 |
|---|---|
| `Node` | 核心接口：`Type()` + `InputSchema()`/`OutputSchema()` + `Execute()` |
| `Schema` | Contract：`map[名称]artifact.Kind`，输入与输出各一份 |
| `ExecutionContext` | 执行上下文：嵌入 `context.Context`，携带 Project / Store / Logger |
| `Config` | `map[string]any`，YAML `nodes.<id>.config` 的原始形态，由 Node 自行解码 |
| `Factory` | 按 type 创建 Node 实例，供 Registry（后续里程碑）调用 |

**关键决策：Execute 返回 `map[string]ArtifactRef`（对计划 §30 的修正）**。计划原定返回 `[]ArtifactRef`，但 Runtime 解析 `from: "<node-id>.<output-name>"` 需要「输出名 -> 引用」映射，裸列表无法建立。选定 map 方案：Node 自己调 `Store.Put`，返回值直接携带输出名，显式且类型安全。Engine 对返回值做输出契约检查（名称已声明 + Kind 一致）。

**关键决策：ExecutionContext 嵌入 context.Context**。Node 接口签名与设计计划 §30 形态一致（`Execute(ctx ExecutionContext, ...)`），同时通过嵌入获得取消/超时能力；未来扩展（retry 等字段）不需要改动接口签名。

**关键决策：Schema 用简单的 Kind 映射而非 CUE**。CUE 只负责 workflow.yaml 的结构校验（M3）；Node 层的 Contract 只需要「名称 -> Kind」级别的匹配，引入 CUE 属于过度设计。

### 3.4 internal/workflow

| 类型 | 职责 |
|---|---|
| `Definition` | workflow/v1 静态声明（apiVersion/kind/metadata/project/nodes） |
| `NodeSpec` | 一个 Node 实例的声明：Type / Inputs / DependsOn / Config |
| `InputBinding` | 数据连接，`From` 格式 `"<node-id>.<output-name>"` |
| `Metadata` / `ProjectSpec` | 元信息与项目声明 |

`Definition.Validate()` 只做结构检查（字段非空）；语义校验（Node Type 存在、Output 存在、Kind 匹配、成环）在 M3/M5 的 Validation 层。错误信息一律定位到 Node ID 与字段名。

### 3.5 internal/execution

| 类型 | 职责 |
|---|---|
| `Status` | 六状态枚举 + 集中管理的合法流转表 |
| `NodeExecution` | 一个 Node 定义在某次 WorkflowExecution 中的运行实例（见 §0） |
| `WorkflowExecution` | 一次 Workflow 运行的完整状态，state.json 的内存形态（见 §0） |

状态机（流转规则集中定义，非法流转返回错误而非静默接受）：

```text
Pending ──> Ready ──> Running ──> Succeeded
   │          │           └────> Failed
   └──> ──────┴──> Skipped
Succeeded / Failed / Skipped 为终态
```

### 3.6 Execution Engine 与 Scheduler（M6 串行 + M7 并行）

| 类型 | 职责 |
|---|---|
| `execution.Engine` | 执行：Registry 实例化 -> Ready 推进 -> 输入解析 -> Execute -> 输出契约检查 -> 依赖计数推进 |
| `execution.scheduler` | Ready Queue + Dependency Counter（计划 §26 算法形态，避免全量扫描）；非导出，仅被 Engine 驱动 |
| `execution.Option` | `WithStateDir`（持久化）、`WithParallelism`（M7 并行度）、`WithProjectContext`（注入 Project Runtime 产物） |

Engine 语义要点：

- **契约**：入参 def 已通过两层 Validator 且全部 Node Type 已注册（与 `BuildGraph` 同一契约）。
- **定义/运行**：Run 返回 `*WorkflowExecution`；内部方法命名 `runNodeExecution` 强调操作的是运行对象而非定义。
- **调度模型**：`parallelism<=1`（默认）严格串行；`WithParallelism(n>1)` 时 worker goroutine 并发消费 Ready Queue（计划 §38）。调度决策（状态迁移、依赖计数、失败即停）全部在主循环；每个 NodeExecution 只被自己的 worker 触碰，结果串行回主循环消费（race detector 验证）。
- **Ready**：源节点启动即 Ready；此后每当某 Node 完成，依赖计数归零的后继 Ready 并入队。
- **输入解析**：按 `exec.Nodes[fromNode].Outputs[fromOutput]`（生产者 NodeExecution）取引用；未产出即执行期错误。
- **输出契约**：Node 返回的输出名必须已在 `OutputSchema` 声明，且 Kind 一致（防 Mock/实现漂移）。
- **失败语义**：首个失败的 NodeExecution 记录 Error 置 Failed，停止派发新任务并等待在途 Node 结束（不强杀），WorkflowExecution 置 Failed；未执行的保持 Pending（Skipped 传播与重试属后续版本）。
- **取消**：`ctx` 取消时 WorkflowExecution 置 Failed 返回；启动前取消则全部保持 Pending。
- **ProjectContext**：`WithProjectContext` 注入 Project Runtime 产物（含 Workspace）；未注入时依据 YAML project 段构造最小 Context。

Artifact 生命周期：Node 内部 `Store.Put(数据本体)` 得到 `ArtifactRef`，经 `NodeExecution.Outputs`（输出名 -> Ref）传递给消费者，消费者按需 `Store.Get`。

### 3.7 持久化（internal/execution/persist.go + id.go + internal/artifact/fsstore.go）

- **Execution ID 单一来源**：`execution.NextExecutionID(baseDir)` 扫描磁盘目录分配 ID；CLI 侧先取 ID 建目录（workspace/artifacts/workflow.yaml 快照），再经 `WithExecutionID` 注入 Engine。Engine 未注入时退回进程内自增（库内多次 Run 场景）。**禁止在别处另行分配 ID**--曾因 CLI 磁盘扫描与 Engine 进程内自增双轨编号，导致第二次运行覆盖第一次的 state.json。
- `FilesystemStore`：`<root>/<n>.json` 自增文件，URI 为相对文件名（正则白名单防路径穿越）；重启后接续自增不覆盖。**与计划 §28 的偏差**：计划图为 `artifacts/<node>/` 按 Node 分目录；实现为扁平文件，因 Store 接口（§29）的 `Put(artifact)` 不携带产出者 Node ID，为目录图改接口得不偿失。按 Node 发现 Artifact 的需求已由 `nodes/<id>/state.json` 的 Outputs 引用满足。
- `PersistState(dir, exec)`：`state.json`（WorkflowExecution 级）+ `nodes/<id>/state.json`（NodeExecution 快照，含 NodeID/NodeType 定义身份）；`LoadNodeState` 可读回。CLI 另将 workflow 文件复制为 `<executionDir>/workflow.yaml`（§28 的定义快照）。
- Engine 经 `WithStateDir` 在每个 Node 状态变化后刷新快照；持久化失败记日志不中断运行。

### 3.8 Project Runtime（internal/project/runtime.go）

| 类型 | 职责 |
|---|---|
| `project.Runtime` | `Resolve`（project 声明 -> Context；repository 相对 workflow 文件解析）+ `CreateWorkspace`（复制项目到 `executions/<id>/workspace/project`，跳过 .git/.workflow） |

每个 Execution 拥有独立 Workspace 副本：Agent 在副本上修改代码，源仓库与其他 Execution 互不污染（计划 §17）。Agent 自行发现副本中的 `.agents/skills/`、`.claude/skills/`（§18，Workflow 不管理 Skills）。

### 3.9 Coding Agent 适配层（internal/agent）

| 类型 | 职责 |
|---|---|
| `agent.CodingAgent` | 接口（计划 §20）：在 ProjectContext 中执行 Task，产出 Artifact 引用 |
| `agent.MockCodingAgent` | MVP 实现：在 Workspace 写 `.mock-agent/task.md` 模拟改码，产出 SourceCode/OpenAPI 引用（输入含 ArchitectureSpec 时补产 OpenAPI） |

真实 Agent Adapter 属后续版本；替换时 Node/Workflow/Engine 均不变。

### 3.10 内置 Node（internal/node/builtins）

| Node Type | 类别 | Input | Output |
|---|---|---|---|
| `requirement-analysis` | Mock | 无 | requirement: RequirementSpec |
| `architecture-design` | Mock | requirement: RequirementSpec | architecture: ArchitectureSpec |
| `coding-agent` | Agent（Mock） | 可选：requirement/architecture/openapi/frontend-sdk | source-code: SourceCode, openapi: OpenAPI |
| `openapi-generator` | Automation（Mock） | openapi: OpenAPI | frontend-sdk: FrontendSDK |

coding-agent 的输出路由：Agent 返回 `[]ArtifactRef`（按 Kind），Node 按 Kind 映射到输出名；OpenAPI 从 Workspace 文件重新写入 Store（Artifact 是唯一数据通道，§13）。`RegisterAll(registry)` 显式注册，由 `cmd/workflow` 启动时调用。

### 3.11 CLI（cmd/workflow）

- `workflow validate <file>`：CUE -> Load -> 语义校验（内置 Registry）。
- `workflow run <file>`（计划 §25）：校验 -> Project Resolve -> Workspace -> FS Artifact Store + 状态持久化 + Workspace 注入 -> Engine 执行 -> 摘要输出（计划 §42 验收形态）。

## 5. YAML Loader（internal/workflow/loader.go）

- `Load(data)` / `LoadFile(path)` 以 **yaml.v3 严格模式**（`KnownFields(true)`）解析：未知字段直接报错，防止 Schema 漂移。
- Go Struct 与 `schema/workflow/v1.cue` 必须同步修改（见 docs/DEVELOPMENT.md §5）。
- `Definition.Validate()` 只做结构层检查（不依赖 Registry）：必填字段、Node ID 约束（非空、不含 `.`，保证 `<node-id>.<output>` 引用无歧义）、`From` 格式、dependsOn 局部一致性（非空、不引用自身、不重复）。

## 6. Graph 模型（internal/workflow/graph.go）

对应设计计划 §7、§35：

| 类型 | 职责 |
|---|---|
| `EdgeType` | `DataEdge`（来自 `inputs.from`）/ `ControlEdge`（来自 `dependsOn`） |
| `Edge` | `From`/`To`（Node ID）+ `Type` |
| `Graph` | `NodeIDs` + `Edges` + 去重邻接表 |
| `ParseRef` | 解析 `"<node-id>.<output-name>"`（Node ID 不含 `.`，按第一个 `.` 切分） |

- `BuildGraph(def)`：遍历每个 Node 的 `inputs`（Data Edge）与可选的 `dependsOn`（Control Edge）构建 Execution DAG。**契约：入参是已通过两层 Validator 的 Definition**，此前提下所有引用必然存在，构建不会失败（畸形 From 仍返回错误作为防御）。
- `Roots()`：无入边的源节点。无输入也无 dependsOn 的 Node 是合法源节点；有 Control Edge 入边的 Node 不是源节点（如 approval -> deploy 中的 deploy）。
- `Cycle()`：DFS（三色标记 + 路径回溯）返回环路径（首尾相同），Data Edge 与 Control Edge 合并检测。

## 7. 两层校验（internal/validation）

对应设计计划 §21-§24：

```text
workflow.yaml
    │
    ▼
ValidateSchema(data)          CUE 结构校验（embed schema/workflow/v1.cue）
    │
    ▼
workflow.Load(data)           YAML 语法 + 严格解析
    │
    ▼
Definition.Validate()         结构检查
    │
    ▼
SemanticValidator.Validate()  语义检查（需 Node Registry + Artifact Registry）
```

`SemanticValidator` 检查（错误聚合返回，不短路）：

- Node Type 已注册（Registry）
- 每个 required Input 已绑定
- `From` 引用的 Node 与 Output 存在
- 绑定的 Input 名称在消费方 InputSchema 中声明（required 或 optional）
- Artifact Kind 匹配（`producer.Outputs[name] == consumer.Inputs[name]`）
- Node Contract 引用的 Artifact Kind 已登记（artifact.Registry）
- dependsOn 引用的 Node 存在
- Data + Control Edge 合并无环

## 8. Registries

| 类型 | 职责 |
|---|---|
| `node.Registry` | Node Type -> `Factory` 映射；显式 `Register`，重复注册报错；禁止 `init()` 隐式注册 |
| `artifact.Registry` | 已登记的 Artifact Kind（构造时登记 §14 的 7 种 MVP 内置 Kind）；`Register` 供后续扩展 |

Mock 内置 Node（requirement-analysis 等）按开发顺序 ⑭ 在 Execution Engine 之后实现，届时在 `cmd/workflow` 集中注册。

## 9. validate CLI（cmd/workflow）

```bash
workflow validate <workflow-file>
```

当前执行 `YAML 语法 -> CUE Schema -> Go 解析 -> 结构校验`；语义校验已实现在 `internal/validation`（测试用假 Factory 覆盖），待内置 Node 落地后在 CLI 接入 Registry。`run` 命令属于 Runtime，后续里程碑提供。

## 10. 命名映射（计划术语 -> Go 惯例）

避免 `workflow.Workflow`、`project.ProjectContext` 一类包名口吃：

| 设计计划 | 实现 | 引用形态 |
|---|---|---|
| Workflow | `workflow.Definition` | `workflow.Definition` |
| ProjectContext | `project.Context` | `project.Context` |
| NodeFactory | `node.Factory` | `node.Factory` |
| NodeConfig | `node.Config` | `node.Config` |
| Execution | `execution.WorkflowExecution` | `execution.WorkflowExecution` |
| Node / Artifact / ArtifactRef / Edge | 同名 | `node.Node` / `artifact.Artifact` / `workflow.Edge` |

## 11. dependsOn 可选原则

**`dependsOn` 不是 Node 层的必须要求，在任何层都是可选的：**

1. `node.Node` 接口对 dependsOn **零感知**--它属于 Workflow 的组合声明（`workflow.NodeSpec.DependsOn`），不属于 Node 的能力声明。
2. `NodeSpec.DependsOn` 为 nil/空是常态：数据连接（`inputs.from`）本身就表达执行关系。
3. `BuildGraph` 仅在 dependsOn 存在时生成 Control Edge；数据依赖型 Workflow 中 Control Edge 数量为零。
4. 语义校验对 dependsOn 只做「声明了才检查」（引用存在、无环），绝不要求它存在。
5. 无输入也无 dependsOn 的 Node 是合法的源节点（Trigger/Source Node，如 requirement-analysis）。

`dependsOn` 留给「没有数据传递、但存在明确执行顺序」的场景（如 Human Approval -> CD）。

## 12. 验收

`go vet ./...` 与 `go test ./...` 全绿（含 `-race`）。测试覆盖：

- `artifact`：Validate 表驱动、Registry、MemStore、FilesystemStore（往返/并发唯一 URI/跨实例/路径穿越防护）。
- `workflow`：Loader（合法/未知字段/畸形 YAML/空文件）、`Definition.Validate()` 表驱动、Graph（Data/Control Edge、Roots、五种环场景）。
- `node`：Registry（注册/查询/重复/空 Type）；`builtins`：四个内置 Node 的 Contract 与 Execute 行为（含 Workspace 写入、OpenAPI 路由回 Store）。
- `validation`：CUE 结构校验（合法 + 9 种结构错误）、语义校验 fixture（对应设计计划 §36）+ 程序化用例。
- `execution`：最小链顺序/输入数/独立性（多次 Run 零共享）、Control Edge、失败传播、未知定义、缺输出、未声明输出、取消、状态机；并行（菱形并发峰值、串行回退、失败停止派发）与持久化布局。
- `project`：相对/绝对路径解析、Workspace 复制（.git/.workflow 排除）、Execution 间隔离、错误输入拒绝。
- `cmd/workflow`、`tests/workflow`、`tests/e2e`：CLI validate/run 集成；e2e 编译真实二进制跑 examples/minimal（临时 human-free 最小链：3 类 Artifact、§28 目录布局、多次运行独立 Execution）。
