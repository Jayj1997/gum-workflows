# Workflow Run History 设计：运行数据留存与查询（run-history/v1）

> **已被《平台核心设计：组件定义体系与迭代执行引擎》吸收并取代。** 本文仅保留为历史设计记录；统一库文件名、表结构、Node Run 粒度和写入时机均以平台核心设计为准。
>
> 状态：设计文档（post-MVP 演进，实现前须按 §10 同步修订 CLAUDE.md 与 docs/DEVELOPMENT.md）。
> 范围：workflow 运行历史的**数据留存**（写入）与**查询**（查看）；不改变 workflow/v1 YAML Schema，不改变执行语义。
> 定位：MVP 计划（`plans/Workflow Engine MVP：workflow-v1 设计与实现计划.md`）是只读历史，本文档是其后的独立演进设计。

---

## 1. 目标

1. **留存**：每次 `workflow run` 的运行数据（运行级状态 + 每个 Node 的输入/输出引用 + 时间信息）落入本地 SQLite 数据库，进程崩溃前已完成的部分不丢失。
2. **查询**：提供 CLI 命令查看历史运行：运行列表、单次运行详情、单 Node 运行详情（含输入/输出）。
3. **数据结构**：两张表——`workflow_run_history`（某 workflow 的一次运行）与 `workflow_node_run_history`（该次运行下每个 Node 的一次运行记录），主键均为 UUID。
4. **可人工检查**：数据库是标准 SQLite 单文件，用户可用本地 `sqlite3` CLI 直接查询（见 §8.4 示例）。

## 2. 非目标（本次不做）

- 不改 workflow/v1 YAML Schema（无任何新 YAML 字段）。
- 不做 UI、不做远程/服务端数据库、不做多用户。
- 不用 SQLite 替换现有 `.workflow/executions/<id>/` 文件布局（state.json、artifacts、workspace 原样保留，见 §4.1）。
- 不做运行日志流（stdout/stderr 采集）、事件流水表、数据血缘表、清理/prune 命令（列入 §13 后续演进）。
- 不做 resume（从历史恢复运行）。

## 3. 现状与动机

MVP 现状（`docs/domain-model.md` §3.7、设计计划 §28）：

```text
.workflow/executions/<execution-id>/
├── workflow.yaml          # 定义快照
├── state.json             # 运行级状态（id/status/workflow/nodeCount）
├── nodes/<node-id>/state.json   # NodeExecution 快照（状态 + Outputs 引用）
├── artifacts/<n>.json     # Artifact 数据本体
└── workspace/project/     # 本次运行的 Workspace 副本
```

局限（本设计的动机）：

| 问题 | 说明 |
|---|---|
| 无法跨运行查询 | 「这个 workflow 跑过几次？哪次失败了？」需要遍历目录、逐个读 JSON。 |
| 无时间信息 | `NodeExecution` / `WorkflowExecution` 没有 started/finished 时间戳，无法回答耗时类问题。 |
| 无输入快照 | Engine 在 `runNodeExecution` 内解析输入后丢弃，`state.json` 只记 Outputs；「这个 Node 当时消费了什么」无处可查。 |
| 无结构化索引 | 运行数据散落在多目录多文件中，无统一主键、无排序、无过滤。 |

## 4. 总体设计

### 4.1 分层：文件系统为主体，SQLite 为索引

```text
┌─────────────────────────────────────────────────────┐
│ .workflow/                                          │
│ ├── history.db          ← 新增：SQLite 运行历史索引   │
│ └── executions/<id>/    ← 不变：运行数据主体          │
│     ├── workflow.yaml / state.json / nodes/...      │
│     ├── artifacts/<n>.json （Artifact 数据本体）      │
│     └── workspace/       （源码工作区副本）           │
└─────────────────────────────────────────────────────┘
```

**分工原则（对硬性约束 #3 的延伸）**：

- **DB 存引用，不存本体**：Node 输入/输出在 DB 中只记 `ArtifactRef`（id/kind/version/uri），Artifact 数据本体仍在 FS Store；workspace、workflow.yaml 快照同理，DB 只记定位信息（execution ID）。
- **state.json 仍是单次运行的第一性状态**（崩溃恢复、逐运行检查），DB 是跨运行的查询索引。两者由同一内存形态（`WorkflowExecution`）在同一次快照动作中写出（§7.2）。
- **DB 可重建**：任意时刻删掉 `history.db`，不影响历史运行的文件数据，只是丢了索引；新运行的记录会重建该文件。

### 4.2 与定义侧的关系

- 运行（run）= 一次 `Engine.Run` = 一行 `workflow_run_history`。
- 同一 Workflow（按 `metadata.name` 识别）可对应任意多行运行记录；定义在两次运行之间可能变化，因此每次运行的详情以该次 `execution_dir` 内的 `workflow.yaml` 快照为准，DB 只冗余 `workflow_name` / `workflow_version` 两个轻量字段用于列表过滤。

## 5. 数据模型

### 5.1 表：workflow_run_history

一行 = 某 Workflow 的一次运行（运行级）。

```sql
CREATE TABLE workflow_run_history (
  id                TEXT PRIMARY KEY,      -- UUID v4，运行的主键（run_id）
  workflow_name     TEXT NOT NULL,         -- metadata.name
  workflow_version  TEXT NOT NULL DEFAULT '',  -- metadata.version
  status            TEXT NOT NULL,         -- Running | Succeeded | Failed
  workflow_file     TEXT NOT NULL DEFAULT '',  -- 本次运行传入的 YAML 路径（如调用时所见）
  execution_id      TEXT NOT NULL,         -- 磁盘目录名，如 execution-000001（state/artifacts 定位）
  error             TEXT NOT NULL DEFAULT '',  -- 运行级错误摘要（如取消原因）
  started_at        TEXT NOT NULL,         -- UTC，YYYY-MM-DDTHH:MM:SS.sssZ
  finished_at       TEXT                   -- NULL = 进行中
);

CREATE INDEX idx_run_history_started_at ON workflow_run_history (started_at DESC);
CREATE INDEX idx_run_history_workflow   ON workflow_run_history (workflow_name);
```

说明：

- `execution_id` 保持现有 `execution-000001` 序号格式**不变**（目录名、`NextExecutionID`、现有测试契约都不动）。UUID 是数据主键，序号 ID 是磁盘定位符，DB 负责二者映射。见 §5.4「双 ID」。
- 耗时不落库，由查询侧计算（`julianday(finished_at) - julianday(started_at)`），避免冗余字段。
- 节点成功/失败计数不落库，列表查询用子查询聚合，避免反规范化漂移。

### 5.2 表：workflow_node_run_history

一行 = 某次运行中一个 Node 的一次运行实例（节点级，含输入/输出记录）。

```sql
CREATE TABLE workflow_node_run_history (
  id            TEXT PRIMARY KEY,      -- UUID v4，节点运行的主键
  run_id        TEXT NOT NULL REFERENCES workflow_run_history(id) ON DELETE CASCADE,
  node_id       TEXT NOT NULL,         -- Workflow 定义中的 Node ID（如 backend）
  node_type     TEXT NOT NULL,         -- 实际实例化的 Node Type（运行快照，如 coding-agent）
  status        TEXT NOT NULL,         -- Pending | Ready | Running | Succeeded | Failed | Skipped
  error         TEXT NOT NULL DEFAULT '',
  inputs_json   TEXT NOT NULL DEFAULT '{}',  -- 见 §5.3
  outputs_json  TEXT NOT NULL DEFAULT '{}',  -- 见 §5.3
  started_at    TEXT,                  -- 进入 Running 的时间；NULL = 未运行
  finished_at   TEXT,                  -- 进入终态的时间；NULL = 未结束
  UNIQUE (run_id, node_id)             -- upsert 冲突键（§7.2）
);

CREATE INDEX idx_node_history_run ON workflow_node_run_history (run_id);
```

说明：

- `status` 取值与 `execution.Status` 完全一致（拼写相同），不引入第二套状态词汇。
- 每个运行的全量 Node 在运行开始时即建行（初始 Pending），即使从未被调度（失败即停后未执行的 Node 保持 Pending 行）——保证「这次运行共 5 个节点、跑了 2 个」这类问题可回答。
- `inputs_json` 在 Node 实际解析输入后填充（§7.2 时机），未运行过的 Node 该列为 `{}`。

### 5.3 JSON 字段形态

`inputs_json`：对象，key 为输入名，值为来源绑定 + 解析后的引用：

```json
{
  "requirement": {
    "from": "requirement.requirement",
    "ref": { "id": "requirement", "kind": "RequirementSpec", "version": "1", "uri": "1.json" }
  }
}
```

`outputs_json`：对象，key 为输出名，值为产出的引用：

```json
{
  "openapi":     { "id": "openapi", "kind": "OpenAPI", "version": "1", "uri": "3.json" },
  "source-code": { "id": "source-code", "kind": "SourceCode", "version": "1", "uri": ".workflow/executions/execution-000001/workspace/project/.mock-agent/task.md" }
}
```

- `ref` 的字段与 `artifact.ArtifactRef` 的 JSON 形态一致（ID/Kind/Version/URI）。
- 只存引用，不存 Artifact 数据本体（§4.1 原则）。
- 采用 JSON 列而非第三张 artifact 明细表：每 Node 输入/输出数量个位数，读详情一次取全；血缘查询（「某 artifact 被谁消费」）列入 §13 后续演进，届时可迁移为范式化表。

### 5.4 主键与 ID 规则

| ID | 格式 | 生成方 | 用途 |
|---|---|---|---|
| `workflow_run_history.id` | UUID v4（36 字符小写文本） | history 层，首次 Record 时分配并回填 `exec.RunID` | 运行的数据主键、CLI 展示与寻址 |
| `workflow_node_run_history.id` | UUID v4 | history 层，INSERT 时分配（upsert 冲突时不更新此列） | 节点运行的数据主键 |
| `execution_id` | `execution-000001`（不变） | `execution.NextExecutionID`（不变） | 磁盘目录定位符 |

- **双 ID 说明**：UUID 满足「每条数据以 UUID 为主键」的要求并保证全局唯一；序号 ID 保留是为了不破坏既有目录布局、`NextExecutionID` 契约与 e2e 测试（`execution-000001` 目录断言）。二者由 DB 行映射，不引入第三个来源。
- 曾考虑「目录直接改用 UUID」：被否决——破坏 §28 布局与现有测试契约，且丢失目录名的人类可读排序，收益仅是省去映射。
- UUID 版本选 v4（`google/uuid` 已是间接依赖，转直接依赖即可）；v7 的时间有序性收益被 `started_at` 索引覆盖，不引入。

## 6. 运行对象模型扩展（internal/execution）

本设计唯一的模型层改动是给运行侧对象补齐元数据（**定义侧、YAML Schema 均不变**）：

```go
// WorkflowExecution 新增字段
type WorkflowExecution struct {
    ID            string
    RunID         string    // UUID，首次 Record 时由 history 层回填；state.json 同步携带
    Workflow      string
    WorkflowFile  string    // 本次运行的 YAML 路径（cmd 经 Option 注入）
    Status        Status
    StartedAt     time.Time
    FinishedAt    time.Time // 零值 = 未结束
    Nodes         map[string]*NodeExecution
}

// NodeExecution 新增字段
type NodeExecution struct {
    NodeID      string
    NodeType    string
    Status      Status
    Inputs      map[string]InputSnapshot // 新增：解析后的输入快照
    Outputs     map[string]artifact.ArtifactRef
    Error       string
    StartedAt   time.Time                // 进入 Running 的时间
    FinishedAt  time.Time                // 进入终态的时间
}

// 新增类型：输入快照 = 绑定声明 + 解析结果
type InputSnapshot struct {
    From string               // "requirement.requirement"
    Ref  artifact.ArtifactRef // 解析后的实际引用
}
```

- Engine 在现有流转点顺手赋值：进入 Running 记 `StartedAt`，进入 Succeeded/Failed 记 `FinishedAt`；`Run` 开始/结束记运行级时间；`runNodeExecution` 解析完输入后将 `inputs` 连同绑定写入 `ne.Inputs`（今天这段数据被丢弃）。
- 这些字段同时落入 `state.json`（JSON 序列化天然支持 `time.Time`），属**增量字段**：现有 e2e 断言的键不变，旧读方忽略新键。
- 附带收益：`printExecutionSummary` 未来可展示耗时（本次不改）。

## 7. 写入路径

### 7.1 RunRecorder 接口（定义在消费方 internal/execution）

```go
// internal/execution
// RunRecorder 接收 WorkflowExecution 的全量快照并幂等落库。
// 接口定义在消费方（execution），SQLite 实现在 internal/history，
// Engine 不感知 SQLite。
type RunRecorder interface {
    Record(exec *WorkflowExecution) error
}
```

选择**全量快照 upsert** 而非增量事件回调：记录器无状态、与调用时机解耦（Engine 重构不漏记）、单次运行 Node 数个位数、整批在一个事务里代价可忽略。曾考虑 `RecordNode(def, exec, nodeID)` 细粒度接口：被否决——需要调用方精确逐事件通知，脆弱且无收益。

### 7.2 Engine 集成与写入时机

新增 Option：

```go
execution.WithRunRecorder(recorder RunRecorder)  // 缺省不记录（库用法/测试不受影响）
execution.WithWorkflowFile(path string)          // 记入 exec.WorkflowFile
```

时机与现有 `e.persist(exec)` **完全同点**（persist 即快照动作，一次做两件事）：

```text
Engine.Run
 ├── 构建 WorkflowExecution（RunID 空）→ snapshot   ← 运行行建立，status=Running，全部 Node 行 Pending
 ├── 每个 Node 状态变化后 → snapshot（state.json + Record）
 └── 终态（Succeeded/Failed/取消）→ snapshot         ← 运行行落终态与 finished_at
```

`Record(exec)` 的内部顺序（单事务）：

1. `exec.RunID` 为空则生成 UUID 并**回填**（此后 state.json 也携带同一 RunID）；
2. upsert 运行行（`ON CONFLICT(id) DO UPDATE`）；
3. 逐 Node upsert 节点行（`ON CONFLICT(run_id, node_id) DO UPDATE`，`id` 列不在更新集内，保持首插分配的 UUID）。

CLI 接线（`cmd/workflow/run.go`）：运行前 `history.Open(".workflow/history.db")`，`defer Close`，注入 Engine。DB 路径为固定约定值（相对 cwd），不加 CLI 参数。

### 7.3 失败语义（与 state.json 持久化一致）

- **记录失败不使运行失败**：`Record` 出错记 `slog` warning 后继续执行——历史是可观测性设施，与 `persist` 的既有原则对齐（`engine.go` 注释：「状态持久化是可观测性设施，不应使运行本身失败」）。
- `history.Open` 失败（含迁移失败）同理：CLI 打 warning，**不记录历史但正常执行运行**。
- 进程中途被杀：已 snapshot 的行都在（WAL，见 §9.4），运行行停留 `Running`。MVP 接受该显示；补偿（把陈旧 Running 标记为 Failed 的 reconcile）列入 §13。

## 8. 读取路径（CLI）

### 8.1 命令（均为只读，不改变 run/validate 语义）

```bash
workflow history                          # 运行列表（最近 20 条）
workflow history <run-id>                 # 单次运行详情（含各 Node 概要）
workflow history <run-id> <node-id>       # 单 Node 运行详情（含输入/输出引用）
```

- `<run-id>` 支持 UUID 前缀（≥8 位）；多义前缀报错并列出候选。
- DB 不存在或无记录时输出 `no runs recorded` 类空态，不报错。
- 不加任何过滤/分页参数（与「CLI 不接受业务参数」精神一致，分页过滤列入 §13）。

### 8.2 列表输出形态

```text
$ workflow history
RUN ID     WORKFLOW               STATUS     STARTED              DURATION  NODES
9f3c2a81   fullstack-development  Succeeded  2026-08-23 14:02:11  1.2s      5/5
6b1d90f4   fullstack-development  Failed     2026-08-22 09:15:44  3.4s      2/5
```

`NODES` 列为 `非Pending数/总数`（子查询聚合）。`DURATION` 由 started/finished 计算，进行中显示 `-`。

### 8.3 详情输出形态

```text
$ workflow history 9f3c2a81
Run 9f3c2a81-04c5-4f2e-9b1a-3d7e6f5a4b21
  Workflow:   fullstack-development v1.0
  Status:     Succeeded
  Started:    2026-08-23 14:02:11   Finished: 14:02:12   Duration: 1.2s
  File:       examples/fullstack/workflow.yaml
  State dir:  .workflow/executions/execution-000001

Nodes:
  NODE          TYPE                  STATUS     DURATION  INPUTS  OUTPUTS
  requirement   requirement-analysis  Succeeded  0.01s     0       1
  architecture  architecture-design   Succeeded  0.01s     1       1
  backend       coding-agent          Succeeded  0.60s     2       2
  openapi       openapi-generator     Succeeded  0.02s     1       1
  frontend      coding-agent          Succeeded  0.55s     3       2
```

`workflow history <run-id> <node-id>` 在此基础上打印该 Node 的输入/输出明细（名称 / from / Kind / URI / Version），形态对齐现有 `printExecutionSummary` 的 Artifacts 段。**不内联 Artifact 内容**（数据本体在 FS Store，可按 URI 直接查看；内联预览列入 §13）。

### 8.4 直接用 sqlite3 人工检查（设计目标之一）

```bash
sqlite3 .workflow/history.db

-- 最近 10 次运行
SELECT substr(id,1,8) run, workflow_name, status, started_at, finished_at
FROM workflow_run_history ORDER BY started_at DESC LIMIT 10;

-- 某 workflow 的失败历史
SELECT substr(id,1,8) run, error, started_at FROM workflow_run_history
WHERE workflow_name='fullstack-development' AND status='Failed';

-- backend 节点历史耗时趋势（ms）
SELECT r.started_at,
       CAST((julianday(n.finished_at)-julianday(n.started_at))*86400000 AS INTEGER) ms
FROM workflow_node_run_history n JOIN workflow_run_history r ON r.id=n.run_id
WHERE n.node_id='backend' ORDER BY r.started_at;
```

## 9. SQLite 工程细节

### 9.1 驱动选型

**`modernc.org/sqlite`（纯 Go，无 CGO）**，经 `database/sql` 标准接口使用。

- 拒绝 `mattn/go-sqlite3`：需要 CGO，交叉编译与单二进制分发复杂化；纯 Go 实现让「SQLite 如何到用户环境」成为非问题——驱动编译进二进制，**不依赖**用户系统 libsqlite3。
- 用户本地的 `sqlite3` CLI 只用于 §8.4 的人工检查；DB 文件是标准 SQLite 格式，二者互不依赖。
- 依赖说明（DEVELOPMENT.md §1 依赖最小化）：标准库无法提供带索引/事务/并发读的结构化本地存储；JSON 文件方案需要全量加载 + 手写过滤排序，且无完整性约束。`google/uuid` 由间接转直接依赖。

### 9.2 PRAGMA（Open 时设置）

```text
journal_mode = WAL      -- 写不阻塞读；崩溃安全
busy_timeout = 5000     -- 同项目两个 CLI 进程并发（一个 run 一个 history）时排队等待
foreign_keys = ON       -- FK + CASCADE 生效
```

单进程内并发（并行 Engine）不是问题：Record 只在调度主循环的 snapshot 点调用（与 persist 同点），天然单 goroutine。跨进程并发由 WAL + busy_timeout 覆盖。

### 9.3 迁移

- `PRAGMA user_version` 整数版本号 + 顺序迁移脚本（Go 常量数组，`internal/history/migrations.go`）。
- `Open` 时：当前 version < 最新则逐个在事务内执行并推进 user_version；幂等可重入。
- MVP 只有 v1（建两张表 + 索引）。机制第一天就位，后续加列/加表不再痛。

### 9.4 崩溃与一致性

- WAL 下已提交事务的行在进程被杀后完好；运行行停留 `Running` 是可接受的显示语义（该次运行确实没有正常结束）。
- DB 行对 `execution_dir` 的引用可能悬空（用户手删 `.workflow/executions/<id>/`）：详情查询按需检查目录存在性并标注 `state dir missing`，不报错。

## 10. 与既有约束/文档的关系（实现时必须同步的修订点）

CLAUDE.md 硬性约束逐条对照：

| # | 约束 | 影响 |
|---|---|---|
| 1 | 数据依赖优先 | 不涉及（不改执行语义） |
| 2 | Workflow 与 Node 解耦 | 不涉及 |
| 3 | Artifact 是唯一数据通道 | **延伸而非违反**：DB 只存 `ArtifactRef`，不存本体 |
| 4 | Node 运行条件 | 不涉及 |
| 5 | CLI 不接受业务参数 | `run`/`validate` 语义与参数不变；新增只读 `history` 子命令属观测性查询，非业务参数。本文档即「先升级设计文档」这一步 |
| 6 | Workflow 不管理 Skills | 不涉及 |
| 7 | 两层 Validation | 不涉及（YAML Schema 零改动） |
| 8 | MVP 不做：Database 等 | **本文档即为前置设计升级**。原排除项针对服务端基础设施（Redis/Kafka/DB 集群、分布式调度）；SQLite 是本地嵌入式单文件、零运维、不经网络，仅作运行历史索引 |

实现时需同步修订（本设计获批后、动码时执行）：

1. `CLAUDE.md`：常用命令加 `workflow history`；「当前状态」与「MVP 不做」清单补充 run-history 说明。
2. `docs/DEVELOPMENT.md` §1：持久化行改为「文件系统（`.workflow/`，Artifact 与 state）+ SQLite（`.workflow/history.db`，运行历史索引）」；依赖表加 `modernc.org/sqlite`（附 §9.1 理由）。
3. `docs/DEVELOPMENT.md` §2：目录结构加 `internal/history/`。
4. `docs/domain-model.md`：补 run-history 一节（本文档的浓缩版）。

包依赖方向（新增 `internal/history`，不破坏现有分层、无环）：

```text
cmd/workflow
    ↓
internal/execution        （定义 RunRecorder 接口；不 import history）
    ↓
internal/workflow / internal/node / internal/artifact / internal/project

internal/history  ──> internal/execution（实现接口、读类型）、internal/artifact
                  ──>  被 cmd/workflow 消费
```

## 11. 测试与验收

单元/集成（`go test ./...`，含 `-race`，全部用 `t.TempDir()`，无网络）：

- `internal/history`：Open 建库与迁移幂等（重复 Open 不重放）；Record 幂等（同 exec 多次 upsert 不产生重复行，UUID 稳定）；RunID 首次分配并回填 exec；FK 级联删除；ListRuns/GetRun/GetNodeRun 往返一致；UUID 前缀解析（唯一命中/多义报错/无命中）。
- `internal/execution`：注入假 Recorder 断言 snapshot 时机（开始/每 Node 变化/终态）；**Recorder 报错时运行照常完成**（只记日志）；`Inputs`/时间戳正确写入 `NodeExecution`。
- `cmd/workflow` / `tests/e2e`：跑两次 fullstack → `history` 列出 2 条、UUID 不同、状态正确；详情含 5 个 Node 行（含 IO 计数）；失败用例的运行行 status=Failed 且未执行 Node 行为 Pending；无 DB 时 `history` 输出空态不报错。

验收问题（设计目标即验收）：

1. 「fullstack-development 最近跑过几次？哪次失败、错误是什么？」——`workflow history` 一屏回答。
2. 「backend 那次运行消费了什么、产出了什么？」——`workflow history <run-id> backend` 回答（含 from 与 ArtifactRef）。
3. 「历史运行的总耗时/各 Node 耗时？」——列表与详情的 DURATION 列。
4. 「用 SQL 自由分析历史？」——`sqlite3 .workflow/history.db` 直接可用。

## 12. 实施里程碑

| 里程碑 | 内容 | 验收 |
|---|---|---|
| H1 模型与写入 | `execution` 字段扩展（RunID/时间戳/Inputs/WorkflowFile）；`RunRecorder` 接口 + Engine Option；`internal/history`（DDL/迁移/Record upsert）；CLI 接线 `history.Open` | 单测绿；跑 fullstack 后 `sqlite3` 可查到完整两表数据；`go vet` 绿 |
| H2 查询 CLI | `cmd/workflow/history.go`（list / run 详情 / node 详情）；e2e；§10 文档同步（CLAUDE.md、DEVELOPMENT.md、domain-model.md） | e2e 绿；§11 验收问题 1-4 全部可演示 |

顺序纪律沿用 DEVELOPMENT.md §8：H1 未验收不开 H2。

## 13. 后续演进（明确不在本次范围）

- **事件流水**：`node_run_events` 表记录每次状态迁移（含 Ready），支撑过程回放。
- **数据血缘**：artifact 明细表（生产者/消费者/谱系图查询），替代 §5.3 的 JSON 列。
- **清理与保留**：`workflow history prune`（按时间/数量保留，级联删节点行；可一并清 execution 目录）。
- **陈旧 Running 对账**：启动时把无进程存活的 Running 行标记 Failed。
- **列表过滤/分页**、Artifact 内容内联预览、运行间 diff、多项目聚合 DB。
- 以上任何一项落地前，同样先升级本文档。
