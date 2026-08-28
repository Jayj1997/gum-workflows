# gum-workflows 开发规范

本文档是本仓库的唯一开发规范。设计计划见 `plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`（设计文档，只读）；本文档是落地到日常编码的执行标准。两者冲突时，先更新设计文档，再更新本文档。

---

## 1. 技术栈与版本

| 项 | 决定 |
|---|---|
| 语言 | Go 1.25+（`go.mod` 基线为 1.25.0） |
| Module 路径 | `github.com/Jayj1997/gum-workflows` |
| 配置格式 | YAML（gopkg.in/yaml.v3） |
| Schema 校验 | CUE（cuelang.org/go，Go API 内嵌；Schema 文件为 `schema/workflow/v1.cue`） |
| 并发 | 标准库：goroutine / channel / sync.WaitGroup |
| 日志 | 标准库 log/slog（结构化） |
| 持久化 | 文件系统（`.workflow/` 目录），不引入数据库 |

**依赖最小化原则**：新增第三方依赖前必须说明标准库无法满足的理由。运行时核心路径（loader / graph / engine）禁止引入 CLI 框架之外的重量级依赖。

## 2. 目录结构（目标态）

```text
gum-workflows/
├── CLAUDE.md
├── docs/
│   └── DEVELOPMENT.md          # 本文档
├── plans/                      # 设计文档，只读
│
├── cmd/
│   └── workflow/
│       └── main.go             # CLI 入口：run / validate
│
├── internal/
│   ├── workflow/               # Workflow 定义、加载、图结构
│   │   ├── definition.go
│   │   ├── loader.go
│   │   ├── validator.go
│   │   └── graph.go            # DAG：Data Edge + Control Edge
│   │
│   ├── node/                   # Node 抽象与 Registry
│   │   ├── node.go
│   │   ├── registry.go
│   │   └── builtins/           # 内置 Node：requirement-analysis、architecture-design、coding-agent、openapi-generator
│   │
│   ├── artifact/               # Artifact、ArtifactRef、Store
│   │   ├── artifact.go
│   │   ├── registry.go         # Artifact Kind 注册（类型匹配校验依据）
│   │   └── store.go            # FilesystemArtifactStore
│   │
│   ├── execution/              # Runtime
│   │   ├── engine.go
│   │   ├── state.go            # Node/Execution 状态机 + state.json 读写
│   │   └── scheduler.go        # Ready Queue + Dependency Counter
│   │
│   ├── project/                # ProjectContext、Workspace
│   │   ├── project.go
│   │   └── workspace.go
│   │
│   ├── agent/                  # CodingAgent Adapter
│   │   ├── agent.go            # CodingAgent 接口
│   │   ├── mock.go             # MockCodingAgent
│   │   └── adapter.go          # 真实 Agent Adapter（后期）
│   │
│   └── validation/
│       ├── cue.go              # CUE Schema 校验
│       └── semantic.go         # 语义校验
│
├── schema/
│   └── workflow/
│       ├── v1.cue              # workflow/v1 Schema
│       └── embed.go            # go:embed 将 Schema 内嵌进 Go Runtime
│
├── examples/
│   └── minimal/                 # 临时最小 human-free 示例（完整 demo 待人工在环能力落地重写）
│       ├── workflow.yaml
│       └── project/
│
└── tests/                      # 跨包集成测试（同包单测跟随源码）
    ├── workflow/
    ├── dag/
    ├── artifact/
    └── execution/
```

目录按里程碑逐步创建，不提前建空目录。

## 3. 架构分层与依赖方向

包依赖只允许自上而下，禁止反向和横向穿透：

```text
cmd/workflow
    ↓
internal/execution        （顶层编排：engine + scheduler）
    ↓
internal/workflow         （定义、加载、DAG）
    ↓
internal/node             （Node 接口 + Registry）
internal/validation       （CUE + Semantic）
    ↓
internal/artifact         （基础包：不依赖其他 internal 包）
internal/project          （基础包：不依赖其他 internal 包）
internal/agent            （依赖 artifact + project；被 node/builtins 消费）
```

类型层面的详细设计见 `docs/domain-model.md`。

具体规则：

- `internal/artifact` 与 `internal/project` 是基础包，不 import 任何其他 `internal/` 包。
- `internal/node` 定义接口，依赖基础包；`node/builtins` 可以 import `agent`、`project`、`artifact`。
- `internal/execution` 是唯一驱动 Node 执行的地方；`internal/workflow` 不感知执行。
- 接口定义在**消费方**（Go 惯例）：例如 `execution` 需要 `NodeRunner`，接口就定义在 `execution` 包中，`node` 包提供实现。
- 禁止 `init()` 中隐式注册。Registry 一律通过显式的 `RegisterXxx(registry)` 函数，在 `cmd/workflow/main.go` 中集中完成注册（Registry 模式保持可测试、可发现）。

## 4. Go 编码规范

### 4.1 基本风格

- 遵循 `gofmt` / `go vet`，注释只写「为什么」，不写「是什么」。
- 导出标识符必须有 doc comment，以标识符名开头。
- 包名：单数、全小写、无下划线。
- 文件命名：小写 + 下划线，如 `artifact_store.go`。

### 4.2 错误处理

- 错误信息**小写开头**、不以标点结尾，并用 `%w` 包装保留错误链：

```go
if err != nil {
    return fmt.Errorf("load workflow %q: %w", path, err)
}
```

- 校验类错误必须包含**定位信息**：哪个 Node（Node ID）、哪个字段、什么错误。设计计划 M3 的验收标准就是「错误 Workflow 能明确指出哪个字段、哪个 Node、什么错误」：

```go
return fmt.Errorf("node %q input %q: output %q not found in node %q", nodeID, inputName, outputName, fromNodeID)
```

- 语义校验错误聚合返回（`[]error` 或自定义 `ValidationErrors`），一次报出全部问题，不在第一个错误处短路。
- 库代码（internal/ 下）禁止 `panic`；只有 `cmd/` 顶层允许将不可恢复错误转为退出码。
- 使用哨兵错误（`var ErrXxx = errors.New(...)`）配合 `errors.Is` 表达可编程判断的类型，如 `ErrCycleDetected`、`ErrArtifactNotFound`。

### 4.3 接口与类型

- 核心接口以设计计划为准，命名与签名不得偏离：

```go
type Node interface {
    Type() string
    InputSchema() Schema
    OutputSchema() Schema
    Execute(ctx ExecutionContext, inputs map[string]ArtifactRef) ([]ArtifactRef, error)
}

type NodeFactory interface {
    Type() string
    Create(config NodeConfig) (Node, error)
}

type ArtifactStore interface {
    Put(artifact Artifact) (ArtifactRef, error)
    Get(ref ArtifactRef) (Artifact, error)
    Exists(ref ArtifactRef) bool
}

type CodingAgent interface {
    Execute(ctx context.Context, task Task, project ProjectContext, inputs []ArtifactRef) ([]ArtifactRef, error)
}
```

- Artifact 在运行时一律传 `ArtifactRef`；只有 Node 内部实际消费时才 `Get`。
- 枚举用类型化字符串常量：

```go
type EdgeType string

const (
    DataEdge    EdgeType = "data"
    ControlEdge EdgeType = "control"
)
```

- Node 状态机：`Pending -> Ready -> Running -> (Succeeded | Failed | Skipped)`。状态流转必须集中在一个文件（`execution/state.go`）中判断合法性，非法流转返回错误而不是静默接受。

### 4.4 并发

- M7 之前保持单进程串行；M7 引入并行时只用 goroutine + channel + WaitGroup。
- 所有执行路径第一个参数是 `context.Context`，Node 执行必须响应 ctx 取消。
- 状态写入集中在单一 goroutine（scheduler 事件循环）或用 mutex 保护并写明锁保护的字段。

## 5. YAML / CUE 规范

- `workflow/v1` 的字段集合是**封闭**的（设计文档 §3.5–§3.7）：`apiVersion`、`kind`、`metadata`、`projects`（列表：`name`/`repository`）、`nodes.<id>.{node, executor, llm, target_model, metadata, inputs, dependsOn, config}`。新增字段 = 新版本（workflow/v2 或明确的 v1 扩展提案），需要先改设计文档。
- CUE Schema（`schema/workflow/v1.cue`）与 Go Struct（`internal/workflow/definition.go`）必须同步修改：改了其中一个而不改另一个的 PR 不予合并。
- Loader 解析时必须严格模式（yaml.v3 `KnownFields(true)`），未知字段报错——避免 Schema 漂移。
- examples/ 下的 YAML 是文档的一部分，必须始终可被 `workflow validate` 通过。

## 6. 测试规范

- 单元测试与源码同目录（`xxx_test.go`），跨包端到端测试放 `tests/`。
- 表驱动测试为默认风格；fixture 放 `testdata/`。
- 语义校验 fixture 按场景分目录（沿用 valid/invalid 模式；平台核心设计 §10 起环降为 warning，warning 场景独立成目录）：

```text
testdata/
├── valid/
├── invalid-node/
├── invalid-output/
├── invalid-type/
├── invalid-executor/
├── invalid-llm/
├── invalid-projects/
└── warning-cycle/
```

- 单元测试禁止网络访问、禁止依赖 `$HOME`；涉及文件系统一律用 `t.TempDir()`。
- Mock 优先于真实依赖：`MockCodingAgent`、`MockRequirementNode`、`MockArchitectureNode` 是 M1-M10 的默认实现（设计计划 §33：先跑通 DAG，再接真实 Agent）。
- 验收标准即测试用例：设计计划 §46 的 Case 1-8 每条都必须对应一个可重复执行的测试。

## 7. Git 规范

- 分支：`feat/<milestone>-<topic>`（如 `feat/m2-yaml-loader`）、`fix/<topic>`、`docs/<topic>`。
- Commit message 采用 Conventional Commits：`feat(execution): add ready-queue scheduler`、`docs: add development guidelines`。scope 用包名或里程碑。
- 一个 PR 只做一件事；里程碑内按计划 §44 的 ①-⑱ 顺序小步提交，不跳跃。
- `plans/` 目录只读：设计变更通过新文档或显式修订，不静默改写。
- `.workflow/`（运行时状态）不入库。

## 8. 里程碑与推进纪律

| 里程碑 | 内容 | 验收 |
|---|---|---|
| M1 | Core Model（Workflow/Node/Artifact/ArtifactRef/ProjectContext/Execution） | 单测通过 |
| M2 | YAML Loader | `workflow validate` 可解析 |
| M3 | CUE Validation | 错误能定位字段与 Node |
| M4 | DAG Builder（Data + Control Edge） | 单测通过 |
| M5 | DAG Validator | valid/invalid fixture 全覆盖 |
| M6 | Execution Engine（串行） | 状态机 + state.json 正确 |
| M7 | 并行 DAG | B/C 并行、D 等待 |
| M8 | Project Runtime | Workspace 正确创建 |
| M9 | OpenAPI Automation | 真实生成 FrontendSDK 文件 |
| M10 | Coding Agent Adapter | Mock 先行，真实 Agent 后接 |
| M11 | Fullstack Demo | 设计计划 §42 全流程跑通 |

推进纪律：

1. **严格顺序**：M(n) 验收未通过，不开 M(n+1)。
2. 每个里程碑完成时，其验收测试必须进入 `go test ./...` 且常绿。
3. 未列入计划的特性（见 CLAUDE.md「MVP 明确不做」）一律拒收，先改设计文档再动代码。

## 9. 文档规范

- `plans/`：设计文档，只读历史。
- `docs/DEVELOPMENT.md`：本文档，随架构决定演进。
- 重大设计决定（如引入新依赖、修改接口签名、Schema 变更）在 PR 描述中说明背景与替代方案，必要时沉淀到 `docs/decisions/`（按需创建，格式：背景 / 决定 / 后果）。
- 代码中的示例（examples/）与文档中的 YAML 必须始终与 `schema/workflow/v1.cue` 一致。
