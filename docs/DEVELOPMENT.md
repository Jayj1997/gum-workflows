# gum-workflows 开发规范

本文档是本仓库的唯一开发规范。MVP 历史设计见 `plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`；当前平台核心语义见 `plans/平台核心设计：组件定义体系与迭代执行引擎.md`。设计与实现冲突时，先显式修订设计，再修改本文档和代码。

---

## 1. 技术栈与版本

| 项 | 决定 |
|---|---|
| 语言 | Go 1.25+（`go.mod` 基线为 1.25.0） |
| Module | `github.com/Jayj1997/gum-workflows` |
| 配置 | YAML（`gopkg.in/yaml.v3`，严格解析） |
| Schema | CUE（`cuelang.org/go`，`schema/workflow/v1.cue` 内嵌） |
| 身份 | UUID v4（`github.com/google/uuid`） |
| 终端检测 | `github.com/mattn/go-isatty` |
| 并发 | 标准库 goroutine / channel；调度状态由 Engine 主循环串行推进 |
| 日志 | 标准库 `log/slog` |
| Artifact 与运行快照 | 用户级 Local Data Root 的 `runs/<execution-id>/` |
| Project Workspace | Project Definition 指向的用户项目规范化绝对路径（原地运行） |
| 定义与运行历史 | 用户级 Local Data Root 的 SQLite `product.db`（`modernc.org/sqlite`） |

`modernc.org/sqlite` 是平台核心唯一获批的数据库依赖：标准库不提供 SQLite 驱动，而本项目需要本地、单文件、零服务、可迁移且无 CGO 的统一定义与运行历史索引。数据库只保存定义、状态和 `ArtifactRef`；Artifact 本体位于 Local Data Root，Project Workspace 则是用户项目目录。

新增第三方依赖前必须说明标准库无法满足的理由。loader / validation / engine 等核心路径不得引入 CLI 框架或服务端基础设施依赖。

## 2. 目录结构（当前态）

```text
gum-workflows/
├── cmd/workflow/                 # validate / run / history 与 stdin HumanGateway
├── internal/
│   ├── definition/               # Node Type / Definition / Executor、TypeExpr、Registry
│   ├── llm/                      # 用户级 llm.yaml、严格加载与默认链解析
│   ├── workflow/                 # workflow/v1、严格加载、Data/Control Graph
│   ├── validation/               # CUE + 聚合语义校验与 warning
│   ├── node/                     # Node、ExecutorFactory、ExecutorRegistry
│   │   └── builtins/             # 6 个内置 Mock/Human Executor 与内嵌定义种子
│   ├── execution/                # 迭代引擎、HumanGateway、Node Run 状态与快照
│   ├── history/                  # SQLite 迁移、定义导入、Run Record 与 Query
│   ├── artifact/                 # ArtifactRef、Registry、Memory/Filesystem Store
│   ├── project/                  # Project Context 与 In-place Project Workspace 解析
│   ├── runtimepath/              # 可注入的数据库与运行产物路径布局
│   └── agent/                    # CodingAgent 接口与 Mock 实现
├── schema/workflow/              # workflow/v1 CUE 与 go:embed
├── examples/fullstack/           # human 入口、审批/advise 回环、多轮需求 Demo
└── tests/
    ├── e2e/                      # 真实 CLI：validate、非 TTY、history 查询骨架
    └── workflow/                 # 跨包 Schema / Loader / Graph 接缝
```

不提前创建没有实现的目录或空 adapter。

## 3. 架构分层与依赖方向

```text
cmd/workflow
  ├─ validation ──> definition / llm / workflow / node / artifact
  ├─ execution  ──> definition / workflow / node / artifact / project
  ├─ history    ──> execution / artifact
  ├─ runtimepath
  └─ node/builtins ──> definition / node / agent / artifact

definition ──> artifact          node ──> definition / artifact / project
workflow ──(引用校验经 validation)──> definition / llm
agent ──> artifact / project
artifact、project、runtimepath ──> 标准库
```

具体规则：

- `workflow` 只描述组合与 Graph，不感知 Executor、执行或 SQLite。平台设计所说的 workflow 对 definition/llm 引用校验依赖由 `validation` 组合完成；Go `internal/workflow` 保持字符串引用，因此没有直接 import。
- Node 契约唯一来源是 `definition.NodeDefinition` YAML；Go `node.Node` 只实现 `Execute`。`ExecutorRegistry` 按 `(definition, version)` 显式注册并在 Run 启动时固定版本。
- `execution` 是唯一驱动 Node Run 的包；它通过消费方接口 `HumanGateway` 与 `RunRecorder` 隔离终端和持久化适配器，不 import `cmd` 或 `history`。
- `history` 实现 `execution.RunRecorder`，以 DTO 接收定义导入数据，不反向 import `definition`、`workflow` 或 `llm`。
- `artifact` 与 `project` 是基础包，不 import 其他 `internal/` 包。Node 之间只传 `ArtifactRef`。
- `runtimepath` 只解析用户级 Local Data Root 并计算数据库、Run、Node Run、Artifact、日志与 tool-output 路径，不创建文件；优先级为测试注入、`GUM_WORKFLOWS_DATA_ROOT`、产品设置、操作系统默认应用数据目录，具体命令是否允许写入由 `cmd/workflow` 决定。
- Registry 禁止在 `init()` 隐式注册；内嵌定义与 Go Executor 必须由 CLI 组装层显式加载、校验并注册。

类型与运行语义详见 `docs/domain-model.md`。

## 4. Go 编码规范

### 4.1 基本风格

- 遵循 `gofmt` / `go vet`；注释说明原因、不重复代码表面含义。
- 导出标识符必须有以标识符名开头的 doc comment。
- 包名使用单数、全小写、无下划线；文件名使用小写加下划线。
- 不在库代码中 `panic`；不可恢复错误只在 `cmd/` 转为进程退出码。

### 4.2 错误处理

- 错误信息小写开头、不以标点结尾，并用 `%w` 保留错误链。
- 可编程判断使用哨兵错误配合 `errors.Is`。
- 校验错误必须包含 Node ID、字段和原因，并聚合返回全部语义问题；warning 不阻断 validate。
- 结构性错误使 Workflow Failed；agent 交互性错误仅在节点声明 optional `advise` 时允许人类意见重试。

### 4.3 核心接口与状态

```go
type Node interface {
    Execute(ctx ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error)
}

type ExecutorFactory interface {
    Definition() string
    Version() string
    Create(config Config) (Node, error)
}

type HumanGateway interface {
    RequestRound(ctx context.Context, req RoundRequest) (RoundResponse, error)
}

type RunRecorder interface {
    Record(ctx context.Context, exec *WorkflowExecution) error
}
```

契约不在 `Node` 上重复声明；inputs/outputs 来自 `definition.Registry`。Node Run 的状态流转集中在 `execution/state.go`：

```text
Pending -> Ready -> Running -> Succeeded -> Ready ...
                    \-> Failed --(interaction + advise/new input)--> Ready
Ready -> WaitingHuman -> Running
Workflow: Running -> Stopped | Failed
```

`Skipped` 为预留状态，本期不实现传播。非法流转必须返回错误。

### 4.4 并发

- 所有可取消路径携带 `context.Context`，Node 必须响应取消。
- 同一 Node 同时最多一个 Node Run；新输入到达时标记 dirty，当前轮完成后合并触发下一轮。
- 不同 Node 可按 `WithParallelism` 并发；调度、状态迁移与持久化触发由 Engine 主循环串行管理。
- 人类事件重置收敛计数；无新人类事件时同一机器节点连续运行超过默认 10 轮触发 `convergence-guard`。

## 5. YAML / CUE 规范

`workflow/v1` 是封闭字段集合：

- 顶层：`apiVersion`、`kind`、`metadata`、`projects`、`nodes`。
- `metadata`：`name`，可选 `version` / `description`。
- `projects[]`：`name`、`repository`；当前语义要求恰好一个项目。
- `nodes.<id>`：`node`，可选 `executor`、`llm`、`target_model`、`metadata`、`inputs`、`dependsOn`、`config`。
- `inputs.<name>.from` 采用 `<node-id>.<output>`，产生 Data Edge；`dependsOn` 只产生 Control Edge。

新增 workflow 字段等于 Schema 变更，必须先修订设计并决定是否进入 workflow/v2。`retry/timeout/parallelism/environment/hooks` 当前禁止加入 workflow/v1。

三类定义侧信封由 `internal/node/builtins/defs/` 内嵌：

- `nodeTypeDefinition/v1`：`metadata`、`requires`。
- `nodeDefinition/v1`：`metadata`、`type`、`requires`、`inputs`、`outputs`；端口类型使用 TypeExpr。
- `nodeExecutor/v1`：`metadata`、`node`、`version`、`updates`。

用户级 `llm.yaml` 使用 `llm/v1`，只做 provider/model 解析，不落库密钥。所有 Loader 使用 `yaml.v3` `KnownFields(true)`；`schema/workflow/v1.cue` 与 `internal/workflow/definition.go` 必须同步修改。`examples/` 下 YAML 属于公开文档，必须始终通过完整 `workflow validate` 管线。

## 6. 测试规范

- 单元测试跟随源码；跨包测试放 `tests/`；fixture 使用 `testdata/`，文件系统使用 `t.TempDir()`。
- 单元与 e2e 禁止网络、禁止依赖真实 `$HOME`；LLM 配置通过临时 `XDG_CONFIG_HOME` 注入。
- 默认表驱动。测试公共接缝，不测试私有调度细节：`validation.Validate`、注入 fake Gateway/Recorder/Executor 的 `execution.Engine.Run`、CLI adapter。
- `tests/e2e` 保留真实二进制的 fullstack validate、非 TTY 零写入守卫、种子 history 三级查询，以及 macOS PTY 下最短成功 Run→Stopped→history 的 Local Data Root 生命周期；完整人工循环仍由 Engine 主接缝测试覆盖。
- 直接 SQL 断言仅用于 `internal/history` 的迁移、FK、幂等与一轮一行约束。
- 合并前运行 `go vet ./...`、`go test ./...` 与 `go test -race ./...`。

## 7. Git 规范

- 分支使用 `feat/<topic>`、`fix/<topic>`、`docs/<topic>`；Codex 创建分支时使用 `codex/` 前缀。
- Commit message 使用 Conventional Commits；一次提交只表达一个连贯变更。
- 旧 `.workflow/` 不入库；不得提交用户级 `llm.yaml`、Local Data Root 内容或密钥。
- `plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md` 是只读历史。其他设计文档只允许在明确票据授权下记录显式修订，不静默改写。

## 8. 推进纪律

MVP 与平台核心 P1–P8 已完成。14 后产品方向不得倒灌到 workflow/v1 或当前 Runtime；GUI、Revision、真实 LLM、恢复与同步等工作必须先有新设计和开发票。

每张票的验收必须进入常绿测试；实现状态、设计目标与未来计划在文档中必须分开表述。

## 9. 文档规范

- `CONTEXT.md` 是当前单上下文术语权威；重大模型决定写入 `docs/adr/`。
- `docs/domain-model.md` 描述已实现模型，不描述尚未落地的产品方向。
- 代码、Schema、examples、CLAUDE.md 与本规范必须在同一票内同步。
