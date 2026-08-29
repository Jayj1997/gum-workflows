# Gum-Workflows Domain Model

本文档描述已完成的 platform-core 01–14 与 code-quality-automation，包括 Local Data Root、In-place Project Workspace、Workflow Context Binding 和首批 Go Code Quality Check。术语以根目录 `CONTEXT.md` 为权威；GUI、Workflow Revision、真实 LLM Client、Resume/Rerun/Fork 等尚未实现能力不在本文范围。

## 1. 定义、实例与运行

```text
Node Type Definition: agent | automation | human
          ↑ type
Node Definition: 契约与 requires
          ↑ node                         ↑ (node, version)
Node Instance: workflow.yaml 中的一次使用   Node Executor: Go 实现版本
          └────────────── Workflow ──────────────┘
                              │ workflow run
                              ▼
WorkflowExecution ── NodeExecution ── Node Run(round 1..n)
                                            │
                                            └─ inputs/outputs: ArtifactRef
```

- `definition.NodeTypeDefinition` 是执行主体类别，内置且固定为 agent、automation、human。
- `definition.NodeDefinition` 是平台认识的节点本体，唯一声明 inputs/outputs TypeExpr 契约和资源需求。
- `definition.NodeExecutorDefinition` 描述某 Node Definition 的可执行版本；`node.ExecutorFactory` 提供对应 Go 实现。
- `workflow.NodeSpec` 是 Workflow 内的 Node Instance：Node ID、Executor/LLM 选择、连接、展示元数据与 config。
- `execution.WorkflowExecution` 是一次独立 Run；`NodeExecution` 保存一个 Node Instance 的当前轮与历史轮；每个 `NodeRun` 有独立 UUID 和递增 round。

定义不携带运行状态，运行也不回写 YAML。Run 启动时解析并固定每个 Node Executor 与 LLM provider/model 名称；运行中注册的新 Executor 不改变该 Run。

## 2. Workflow 与连接

`workflow.Definition` 对应封闭的 `workflow/v1` YAML：一个 metadata、恰好一个 project 和 `nodes` map。

连接只有两种：

| 连接 | 声明 | 语义 |
|---|---|---|
| Data Edge | `inputs.<port>.from: <node-id>.<output>` | 传递生产者最新已完成版本的 `ArtifactRef` |
| Control Edge | `dependsOn: [<node-id>]` | 只表达执行顺序，不传数据 |

Workflow Context Binding 复用 input binding 语法，但不形成 Node Edge。当前唯一内建 Context 是 `project.code: SourceCode`：Runtime 在 Run 启动后把它解析为 `ID=project-code`、初始 `Version=1`、URI 指向规范化 In-place Project Workspace 的 ArtifactRef。它没有 Artifact Store 本体，不使用字符串模板、OS 环境变量或源码内联；`project` 因而是保留的 Context 名，不能用作 Node ID。

`workflow.BuildGraph` 保留 Data/Control 类型并用于调度分析。环不再是校验错误：含 human 的环是正常审批/意见迭代；纯机器环产生 warning，并由运行时收敛保护兜底。

全 Workflow 必须恰好一个无 inputs、无 dependsOn 的源 Node，且其 Node Type 必须是 human。当前内置入口是 `human-input`。

## 3. Artifact 与版本

`artifact.Artifact` 是数据本体，`artifact.ArtifactRef` 是运行时唯一传输形态：

```text
ArtifactRef = ID + Kind + Version + URI
```

Node 只接收与返回 `map[端口名]ArtifactRef`。需要内容时由 Node 自己调用 `artifact.Store.Get`；大型源码 Artifact 只保存 Workspace/repository 等引用信息，不在 Node 间复制本体。

同一 Node output 每次成功轮产生新版本。Coding Agent 成功轮的 `code: SourceCode` 指向共享 Workspace，新版本用于触发绑定 `backend.code` 的下游；失败轮不发布 `code`。旧 Artifact 不删除；下游新一轮的 `InputSnapshot` 同时记录 YAML `from` 与实际消费的版本，运行历史因此可以回答某一轮消费、产出了什么。历史中的 code ArtifactRef 只证明当时的消费身份和触发链，不承诺恢复当时的源码状态。

## 4. Node、契约与 Executor

Go `node.Node` 刻意保持窄接口：

```go
type Node interface {
    Execute(ctx ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error)
}
```

Node 不再实现 `Type()`、`InputSchema()` 或 `OutputSchema()`；契约只从 `definition.Registry` 读取。`node.ExecutorRegistry` 按 `(definition, version)` 注册 `ExecutorFactory`，缺省选择数值意义上的最新 `vN`。

Run 前会交叉检查：

- Node Definition 引用的 Type 与 Artifact Kind 合法；
- 内嵌 Node Executor YAML 与已编译的 Go Factory 一一对应；
- Node Instance 引用的 Definition / Executor 存在；
- required input 已绑定，optional input 若绑定也必须类型兼容；
- agent 的 LLM 选择可以解析，human/automation 不得声明 LLM；
- `human-approval` 无 data inputs 且必须有非空 dependsOn。

## 5. 内置 Node

| Node Definition | Type | Inputs | Outputs |
|---|---|---|---|
| `human-input` | human | — | `requirement: markdown` |
| `requirement-analysis` | agent | `requirement: markdown` | `rationality: int`、`analysis-output: markdown` |
| `architecture-design` | agent | `analysis-output: markdown` | `architecture: ArchitectureSpec` |
| `coding-agent` | agent | optional `analysis-output` / `architecture` / `openapi` / `frontend-sdk` / `advise` | `code: SourceCode`、`openapi: OpenAPI` |
| `openapi-generator` | automation | `openapi: OpenAPI` | `frontend-sdk: FrontendSDK` |
| `go-static-analysis` | automation | `code: SourceCode` | `result: QualityCheckResult` |
| `go-coverage-check` | automation | `code: SourceCode` | `result: QualityCheckResult` |
| `go-race-check` | automation | `code: SourceCode` | `result: QualityCheckResult` |
| `go-complexity-check` | automation | `code: SourceCode` | `result: QualityCheckResult` |
| `human-approval` | human | —；以 dependsOn 挂接被审 Node | `approve: bool`、`advise: markdown` |

Agent 与 OpenAPI 生成目前仍是 Mock 实现；真实网络模型调用和真实 generator 属后续设计。四个 Go 质量检查是已落地的真实 automation：各自固定 v1 POSIX Script Bundle，在 Darwin/Linux 的 Host Execution Environment 中使用用户 PATH 原地运行，不接受实例级脚本、命令或 scope 覆盖。Complexity Bundle 通过用户 Go 运行仅依赖标准库的内嵌 AST Analyzer；Adapter 独立应用单函数上限以及测试/generated/vendor 排除策略。

Static Script 的 stdout/stderr 只流式进入 Node Run 日志；正式工具产物进入该轮独立的 tool-output 目录。内置 Result Adapter 从退出状态、`vet.json`、package/toolchain 产物和日志生成严格的 `qualityCheckResult/v1`：无诊断为 `passed`，vet/package 诊断为 `failed`，无 Go package 为 `not-applicable`；工具、I/O、产物或 Schema 损坏是 Structural Error，不产生 Result Artifact。业务 `failed` 仍是成功 Node Run 的普通 `result: QualityCheckResult` 输出。

Coverage Script 使用 `go test -count=1 -json -covermode=atomic -coverprofile=<Node Run tool-output> ./...` 禁用测试缓存并运行 full scope。实例只允许配置 0–100 的 statement coverage 最低阈值，默认 80；Adapter 验证并解析正式 profile，以 statement 数加权计算 `metrics.statementCoverage`。低于阈值为 `failed`，等于或高于为 `passed`，无可插桩 statement 为 `not-applicable`；测试或编译失败仍发布业务 `failed` Result，但 metric 以 unavailable + reason 表达，不伪造 0%。成功进程缺失或损坏 profile 是 Structural Error。Coverprofile 与其他临时工具产物只存在于 Node Run 的 tool-output，项目目录不写入 Gum 报告。

Race Script 在运行前诊断当前 Go target 与宿主一致、GOOS/GOARCH 受 Race Detector 支持、`CGO_ENABLED=1` 且配置的 C 编译器可执行，再使用 `go test -race -count=1 -json ./...` 禁用测试缓存并运行 full scope。Adapter 解析正式 Go JSON 事件并生成 `metrics.racesDetected`：测试成功且本次未观察到 race 为 `passed`，观察到 race、普通测试失败、编译或 package loading 失败为业务 `failed`，无 Go package 为 `not-applicable`；Requirement、工具启动、I/O 或协议产物损坏为 Structural Error。`passed` 只陈述本次执行未观察到 race，不声称项目不存在数据竞争。

ScriptNode 在启动前诊断当前平台、POSIX Shell、Manifest required executables，并对 Go Bundle 执行有界的 `go version` / `go env` 能力探测；运行前后重新核对 Executor 身份、Result Adapter、Bundle 摘要、物化脚本和声明产物路径。Shell 运行于独立进程组，Context 取消或 stdout/stderr 合计超过固定 32 MiB 上限时终止 Shell 及其子进程；取消、日志超限、I/O、摘要、产物或 Adapter 失败均不发布 Result。Adapter 完成并校验后删除临时 tool-output，再保存 Result；Bundle、日志、Result 与其 ArtifactRef 保留用于历史查询。诊断只记录主机/Go 平台、工具路径与版本、GOROOT、CGO、cwd、固定位置参数、摘要、Adapter 和日志引用，不保存完整环境。

## 6. LLM 配置

`internal/llm` 只实现用户级 `llm.yaml` 的严格加载、校验与 provider/model 默认链解析，不实现网络 Client。查找顺序为：

1. `$XDG_CONFIG_HOME/gum-workflows/llm.yaml`
2. `~/.config/gum-workflows/llm.yaml`

Agent Node 可声明 `llm` 与 `target_model`；都省略时使用默认 provider/default model。API Key 可引用环境变量，解析后的密钥不写入 SQLite、state.json 或 Workflow 快照；运行记录只保存 provider/model 名。

## 7. 迭代执行语义

节点的 Ready 条件是：所有 required inputs 已有完成版本，且所有 Control 前驱至少完成一轮。首次运行之后，下列事件会再次触发：

- 任一 data input 出现未消费的新 Artifact 版本；
- Control 前驱出现未消费的新完成轮；
- interaction failure 收到即时 advise；
- 新一轮 human-input 级联出新版本。

同一 Node 单并发；运行中到达的多个新输入先标 dirty，当前轮完成后合并为下一轮。不同 Node 可并行。

### 7.1 HumanGateway

`execution.HumanGateway` 是 Engine 的消费方接口，支持三类请求：

- `input`：human-input 获取一轮多行需求，并选择 Continue 或 Finish；
- `approval`：展示 Artifact 摘要与历史 advise，获取 approve/reject；
- `advise-retry`：为 agent 的 interaction failure 获取即时修正意见。

CLI 用 stdin/stdout 实现该接口。含 human Node 的 Workflow 在非 TTY stdin 下于任何 Local Data Root 写入前拒绝运行；测试通过 fake Gateway 覆盖完整循环。

### 7.2 Approval 门控

`human-approval` 的 reject 轮产出 `approve=false` 与 advise 新版本，驱动声明 `advise` input 的 agent 返工，并允许审批节点在被审 Node 新一轮完成后再次运行。approve 轮不催更已经消费过旧版结果的下游，从而让图收敛而不产生空转。

### 7.3 错误与收敛

- Structural Error：运行无法由当前人工意见修复，Workflow 置 Failed；已在途 Node 等待结束，不再派发新轮。
- Interaction Error：agent 输出质量问题。若 Definition 声明 optional `advise`，该 Node Failed 而 Workflow 保持 Running，等待 advise 或新需求复活；否则按结构性错误处理。
- Convergence Guard：自上次人类事件后，同一机器 Node 连续开始超过默认 10 轮，第 11 轮以 `convergence-guard` 失败。input、approval 与 advise retry 都会重置机器计数。

Workflow 没有自动 Succeeded 终态。全图静止时仍保持 Running，等待新需求或人类交互；Ctrl-C / SIGTERM 使其 Stopped，并记录 `stopped_reason=user_interrupt`。结构性错误是唯一自动终止路径。

## 8. 运行状态与持久化

`NodeExecution` 保存 `Current NodeRun` 和已完成的 `History []NodeRun`。每轮记录 status、inputs、outputs、diagnostics、error/error_kind 和时间；ScriptNode diagnostics 包括 Bundle 摘要、主机与 Go 工具链事实、cwd、固定位置参数、launcher/工具路径、Result Adapter 和日志引用。`WorkflowExecution` 记录唯一 Run UUID、Workflow 身份、状态、停止原因和 Node 快照。Run UUID 同时是 SQLite 主键与 Local Data Root 目录名，不再维护第二套 filesystem execution ID。

用户级 Local Data Root 保存全局产品库与运行主体；路径只使用稳定的 Run / Node Run ID，不编码可变的 Project 或 Workflow 名称：

```text
<Local Data Root>/
├── product.db
└── runs/<run-uuid>/
    ├── workflow.yaml
    ├── state.json
    ├── nodes/<node-id>/state.json
    ├── artifacts/<n>.json
    └── node-runs/<node-run-id>/
        ├── bundle/
        ├── logs/{stdout,stderr}.log
        └── tool-output/  # 仅脚本与 Adapter 执行期间存在
```

CLI 可用 `GUM_WORKFLOWS_DATA_ROOT` 覆盖位置；未覆盖时使用操作系统的用户级应用数据目录。路径解析本身不创建目录；ScriptNode 执行时才创建该 Node Run 私有的 bundle、logs 与临时 tool-output，Adapter 消费正式产物后清理 tool-output。Project Definition 中的 repository 相对 Workflow 文件解析为规范化绝对路径，该目录直接作为 Agent 与 Automation 共享的 In-place Project Workspace；Runtime 不把项目复制到 Local Data Root，也不在项目内写 Gum 日志或结果。

SQLite 使用 WAL、busy_timeout、foreign keys 与 `PRAGMA user_version` 顺序迁移。定义侧保存 Node Type、Node Definition、Node Executor、Workflow 与 Node Instance；运行侧保存 Workflow Run 与逐 Node Run 历史，一行对应一个 round，inputs/outputs 只序列化 `ArtifactRef`，diagnostics 保存非敏感执行事实。Quality Check Result 本体仍由 Artifact Store 保存，历史通过输出 Ref 定位。

`history.Store` 实现 `execution.RunRecorder`。Engine 在与 state.json 相同的状态点提交完整快照；Record 使用 upsert 保持重放幂等。`validate` 只在数据库已经存在时 read-only 检查 Executor 解析，不建库、不迁移；`history` 同样 read-only 打开且无库时返回空态。新 Run 不在用户项目内创建或更新 `.workflow`，新旧位置不双写。开发期旧项目数据可通过显式调用 `history.MigrateLegacy` 一次性迁入 Local Data Root；普通 `run`、`validate` 与 `history` 不会自动扫描 legacy 目录。

## 9. 校验管线

```text
workflow.yaml
  -> CUE schema
  -> yaml.v3 KnownFields(true)
  -> workflow.Definition.Validate
  -> Semantic Validator(definition / executor / artifact / llm / project)
  -> errors + non-blocking warnings
```

结构错误与语义错误必须定位到 Node 和字段；语义问题聚合返回。纯机器环以及 agent 未声明 advise 端口等可操作风险以 warning 展示。

## 10. CLI 与公开验收接缝

```bash
workflow validate <workflow-file>
workflow run <workflow-file>
workflow history
workflow history <run-id>
workflow history <run-id> <node-id>
```

CLI 不接受业务 flags。Run ID 可用不少于 8 位的唯一 UUID 前缀。

回归测试的公共接缝是：

- `validation.Validate`：完整 schema + semantic 行为；
- `execution.Engine.Run`：注入 fake Executor、HumanGateway、RunRecorder 与 Store；
- CLI adapter：真实二进制的 fullstack validate、非 TTY 零写入守卫和种子 history 三级查询。

`examples/fullstack/workflow.yaml` 是当前公开 Demo：人工需求入口、分析/架构、backend、approval/advise 回环、OpenAPI/SDK 和 frontend。完整交互行为由 Engine 接缝测试验证，不要求 CI 提供 PTY。
