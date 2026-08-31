# Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Provider / Model 设计计划

> 状态：14 后设计计划（2026-08-27；产品方向已确认，实施细节在各阶段开工前逐项评审）。
>
> 前置条件：`.scratch/platform-core/spec.md` 与 `.scratch/platform-core/issues/` 中 01–14 全部完成并验收。
>
> 当前进度：platform-core 与 code-quality-automation 已完成；Local Data Root、In-place Project Workspace、Workflow Context Binding、ScriptNode 和首批四个 Go Code Quality Check 已落地。当前产品化设计的第一要务是跑通完全基于 SQLite 的 Workflow 创建、配置、运行和基本结果查看；GUI、Draft/Revision、真实 LLM 与这一闭环直接需要的能力仍待实现，高级 Artifact 体验与运行恢复后置。
>
> 实施切片：最终多轮领域模型是 `human-chat -> llm-chat -> human-chat`；首个可执行切片先完成单轮 `human-chat(source) -> llm-chat`，下一切片再加入 Human Chat Entry 语义升级与 Conversation 回边。
>
> 首个产品切片边界：必须经过真实的薄 macOS Desktop UI 与通用 Application seam；只实现 OpenAI-compatible Chat Completions、非流式 text input -> text output 和用户手工声明的 Model ID，但使用正式的 ChatMessage / ContentPart、ProtocolAdapter、SQLite Workflow、Revision、Run Snapshot 与 Artifact，不得以 fake-only 或硬编码聊天页面替代。Streaming 与 Windows 支持进入明确待办；当前产品化范围不实现 `/models` 服务端发现。
>
> 范围隔离：本文只规划 14 之后的新阶段，不修改、不替代、不向前倒灌 01–14 的范围、顺序与验收标准。本文涉及的新定义字段、运行控制或持久化模型，实施前必须按新版本或显式升级设计落地，不得静默扩展现有 workflow/v1。

---

## 1. 产品定位

Gum-Workflows 从 01–14 完成后的 Workflow Runtime，继续发展为面向技术人员的本地工作流产品。这里的技术人员包括代码开发、产品经理、测试、设计、运维等能够描述自身工作过程并配置工具的人群。

产品目标：

1. **简易创作**：用户通过本地 GUI 新建 Workflow、声明 Node Instance、配置端口绑定和运行参数；无需手写 YAML，也不依赖拖拽式低代码画布。
2. **本地优先**：Workflow 定义、LLM Provider / Model、运行历史与 Artifact 默认保存在用户本地；代码工作流直接使用用户项目目录作为 In-place Project Workspace。云端与多设备同步属于后续演进，但本地数据模型须为其预留稳定身份、版本和迁移能力。
3. **可观察、可调试**：用户能理解 Workflow 为什么按当前结构运行，能查看每次 Node Run 的输入输出和错误。Rerun、Fork 和崩溃恢复不作为首个产品闭环的前置条件。
4. **Node 优先**：产品化初期先打磨 Node Definition、Node Executor、真实 Agent Node、Artifact 和调试能力，不急于建设内置 Workflow 库。
5. **结构即语义**：Workflow 的执行语义来自 Node Contract、Data Edge 与 Control Edge；画布坐标、视觉排列和 UI 操作不是执行语义。

产品不是：

- 通用无代码应用平台；
- 通过拖拽和手工拉线作为主要创作方式的 Dify 类工具；
- 云端优先或多人实时协作平台；
- 以 YAML 为主要用户界面的配置工具；
- 对生成式模型输出作完全确定性承诺的任务调度器。

首个产品闭环只承诺：一次 Run 固定 Workflow Revision、Node Executor、Resolved LLM Selection 和有效参数，并能追踪本次执行的输入、输出、错误与人工交互。重算、分叉、历史外部资源重建和崩溃恢复的产品承诺后置。

---

## 2. 已确认的阶段边界

### 2.1 本阶段纳入设计

- SQLite 中的 Workflow / Draft / immutable Revision / Run Snapshot。
- 面向 Desktop UI 和未来产品 Adapter 的 Application 模块。
- 首个闭环的 macOS 本地桌面 GUI；Windows 支持列入待办。
- 通过 UI 新建、声明、配置和优化 Workflow。
- 自动排列、只读选择的 Workflow 结构预览。
- Node Config Schema、运行资源 Requirements、运行属性和 UI Hint。
- 独立、用户级的 LLM Provider / Model 模块。
- OpenAI-compatible Chat Completions 协议 Adapter。
- 正确的单轮和多轮 text 请求组装。
- 用户手工模型配置；当前范围不实现 `/models` 服务端发现。
- 一个简单但真实的 `llm-chat` Agent Node。
- 跑通 Workflow 所需的基本 Artifact 结果查看与运行状态观测。
- 为上述能力服务的本地测试、迁移和 macOS 打包基础。

### 2.2 明确后置

- Workflow 导入/导出：YAML 只作为未来可能的导出格式；产品 v1 形态未确定前不设计导入、导出、冲突策略或 Pack 格式。
- 现有 YAML CLI 与 SQLite 产品 Workflow 的兼容、隐式导入或身份映射。
- Rerun、Fork、Manual Artifact、历史 Artifact 复用和完整崩溃恢复。
- 多类型高级 Artifact Previewer、版本比较和外部可变资源重建保证。
- Streaming、Anthropic Messages、image input 与 Windows 产品支持（明确待办，不作为首个 macOS 闭环验收条件）。
- AI 创建、修改或优化 Workflow。
- 内置 Workflow 库与 Workflow Marketplace。
- 用户自定义原生 Go 插件、动态二进制加载。
- Workflow Pack、Pack 依赖、签名和分发。
- 定时、文件变化、Webhook 等非人工 Trigger。
- 云同步、账号、多设备和多人协作。
- 完整 Secret Management、安全沙箱和企业权限治理。
- OpenAI Responses、Realtime、Anthropic 工具调用、MCP 和通用 Agent Tool Loop。

后置不表示否定这些方向；本阶段只要求领域模型不把未来实现堵死。

---

## 3. 领域语言

### 3.1 产品定义侧

**Workflow**：一个长期存在、拥有稳定 ID 的工作流身份。

**Workflow Draft**：Workflow 当前唯一可编辑的草稿。Autosave 只在规范化语义内容变化时更新同一 Draft，不创建 Revision 或历史副本；内部 lock version 只用于并发控制。既有 Revision 和 Run 不受影响，首个产品闭环没有独立 Publish 动作。

**Workflow Revision**：用户点击 Run 时，根据 Draft 的规范化语义内容创建或复用的不可变版本。每次 Run 必须绑定一个 Revision；Draft 变化不能影响已启动 Run。

**Run Snapshot**：Run 启动时固定的完整解析结果，包括 Workflow Revision、Node Executor、Resolved LLM Selection、有效生成参数和必要的项目环境信息。快照不含 API Key 明文。

**LLM Provider**：用户级的一条 LLM 连接配置，包含名称、协议、Base URL、API Key 引用和手工 Model 列表。支持显式默认 Provider；未设置时取未删除 Provider 按 `(created_at ASC, UUID ASC)` 的第一个。

**LLM Model**：某个 LLM Provider 下的用户配置槽，拥有全局稳定的 Gum Model UUID，并保存可编辑的 Provider Model ID 和生成默认值。UUID 不声称底层模型不可变；修改 Provider Model ID 会影响未来 Run，历史 Snapshot 保留旧值。每个 Provider 支持显式默认 Model；未设置时取其未删除 Model 按 `(created_at ASC, UUID ASC)` 的第一个。

**LLM Preference**：Agent Node Instance 记录的 Gum Model UUID。未选择时，StartRun preflight 按默认 Provider/Model 解析并先写回 Draft；只要 UUID 仍存在，默认值改变都不改变选择。UUID 被删除时不回退，必须由用户重新选择。

**Resolved LLM Selection**：StartRun 根据已物化到 Draft/Revision 的 Gum Model UUID，为 Agent Node 固定协议、Provider、Provider Model ID 和有效参数；进入 Run Snapshot，不包含 API Key 明文。历史 Run 即使原 Model 后来删除仍展示当时结果。

**Workflow Preview**：从 Draft 或 Revision 派生的只读结构投影，包含 Node、Data Edge、Control Edge、循环组和诊断；坐标不属于 Workflow 语义。

### 3.2 对话与 Artifact

**Chat Message**：一条有 role 和一个或多个 Content Part 的对话消息。初期 role 只需 user / assistant；system/developer instruction 是 Node 配置，不混入业务对话 Artifact。

**Content Part**：Chat Message 的内容片段。P9–P12 只实现 text；image、audio 与 file 保留为后续类型方向，进入范围时二进制内容必须通过 ArtifactRef 引用，不内联进入 Node 间传递。

**Conversation**：按顺序保存的 Chat Message 集合，是多轮上下文的数据本体。对话节点自身不保存隐藏历史。

**Human Chat Entry Node**：对话 Workflow 中唯一可在没有必需输入时自举的人工门。它将每次人工提交追加为 Conversation 中的 user message；收到 `llm-chat` 反馈后只等待下一次人工事件。

### 3.3 已确认但后置的运行边界

**Paused**：暂停派发新的 Node Run；已经开始的外部调用不保证被冻结。可以保留 Run ID Resume。

**Interrupted**：进程异常退出后仍可保留 Run ID Resume；结果未知的 Node Run 不自动重放。

**Interaction Retry**：Interaction Error 不终结 Run，可在同一个 Run 内以相同输入创建新 Node Run。

**Failed / Stopped**：两者都是终态。Structural Error 导致 Failed；用户主动停止导致 Stopped。继续工作必须创建新 Run。

**UnknownOutcome**：Node Run 的结果状态，不是 Run 终态；是否再次执行必须由用户明确决定。

这些边界供未来运行恢复设计使用；Rerun、Fork、Manual Artifact 和完整恢复流程不在首个产品闭环中设计。

---

## 4. 总体架构

```text
┌──────────────────────────────────────────────────────────────┐
│ Desktop UI（Web frontend in native shell）                  │
│ Workflow / LLM Provider / Preview / Run / Artifact / History│
└───────────────────────────────┬──────────────────────────────┘
                                │ typed calls + observation signals
┌───────────────────────────────▼──────────────────────────────┐
│ Application Module                                            │
│ WorkflowAuthoring / LLMProvider / RunControl / ArtifactQuery │
└──────────────┬─────────────────────┬──────────────────────────┘
               │                     │
┌──────────────▼────────────┐  ┌────▼──────────────────────────┐
│ Product Repository        │  │ Workflow Runtime             │
│ SQLite drafts/revisions   │  │ execution/definition/history │
│ providers/run snapshots   │  │ artifact/project/node        │
└──────────────┬────────────┘  └────┬──────────────────────────┘
               │                     │
┌──────────────▼─────────────────────▼──────────────────────────┐
│ Adapters                                                     │
│ Wails / OpenAI-compatible / FS Artifact                      │
└──────────────────────────────────────────────────────────────┘
```

### 4.1 Application 模块

UI 不直接调用 Engine、SQLite 或某个协议 Adapter。Application 模块提供产品动作，隐藏事务、校验、解析和运行时编排：

```go
type WorkflowApplication interface {
    CreateWorkflow(ctx context.Context, input CreateWorkflowInput) (WorkflowView, error)
    UpdateDraft(ctx context.Context, input UpdateDraftInput) (WorkflowView, error)
    ValidateDraft(ctx context.Context, workflowID string) (WorkflowPreview, error)
    StartRun(ctx context.Context, workflowID string, expectedLockVersion uint64) (RunView, error)
}
```

接口名称为方向示例，不是本计划要求的最终 Go 签名。设计约束是：

- Desktop UI 和未来产品 Adapter 通过同一个 Application seam 使用领域能力；现有 YAML CLI 不接入产品 Workflow / Revision；
- UI 点击 Run 前先 flush 有内容变化的 autosave；StartRun 必须携带 UI 当前看到的 expected_lock_version，token 不匹配时返回最新 Draft/Diagnostics，且不得物化 UUID、创建 Revision 或创建 Run；
- StartRun 在同一 Application 用例内校验 Draft；对尚未选择模型的 Agent Node 按当前双层 default 解析 Gum Model UUID并先写回 Draft、递增 lock_version；随后按语义哈希创建或复用 Revision、固定 Run Snapshot 并创建 Run。任一步失败不得留下部分写入，不暴露独立 Publish 动作；
- Application 接收依赖，不在方法内部创建数据库、HTTP Client 或 Engine；
- 每个写操作在本地事务中完成；
- 返回产品 View/Result，不把数据库行或协议响应直接泄露给调用方；
- 长运行通过结构化暂态观测信号向 UI 推送，不用请求阻塞承担全部状态通信；这不是持久化 Run Event Log。

### 4.2 桌面技术路线

默认候选为 Wails + Web 前端。Go 继续承载本地 Runtime，React/Vue/Svelte 等前端运行在系统 WebView。正式锁定框架前先完成一个 Prototype Gate：

1. macOS 能构建启动；Windows Prototype Gate 列入后续待办；
2. 前端能调用 Application 方法；
3. 运行状态观测信号能实时推送；
4. 本地文件选择、系统路径和窗口生命周期正常；
5. 浏览器 Mock Adapter 与桌面 Adapter 能复用同一前端 Workflow Client interface。

若 Prototype Gate 不通过，只替换桌面 Adapter，不改领域模型和 Application 接口。

---

## 5. 本地持久化与版本模型

### 5.1 SQLite 是产品事实来源

14 之后产品创建和运行的 Workflow 只以 SQLite 为事实来源，不从 YAML CLI 隐式导入。SQLite 保存：

- Workflow 身份；
- 当前 Draft；
- immutable Revision；
- Node Instance 与绑定；
- UI 展示元数据；
- LLM Provider、Model 与默认标记（不含 API Key 明文）；
- Run Snapshot、Node Run、错误和运行索引；
- Artifact 元数据和引用。

Artifact 本体与大型文件继续存于文件系统，SQLite 保存引用、哈希和来源。代码工作流的 Project Workspace 就是用户项目目录：Agent 修改实时落在该目录，Automation 使用同一工作状态。Gum 不复制项目、不创建内部代码 Revision，也不承担代码版本恢复；这些属于用户已有项目工具。

产品需要一个由平台管理的用户级 Local Data Root，而不是把跨项目的 LLM Provider 和 Workflow Library 分散到每个项目的 `.workflow/`：

```text
<Local Data Root>/gum-workflows/
├── product.db
└── runs/<run-id>/
    ├── artifacts/
    ├── node-runs/<node-run-id>/
    │   ├── logs/
    │   └── tool-output/
    └── logs/
```

Local Data Root 由平台 Adapter 按操作系统规范解析，领域模型不保存硬编码的 macOS/Windows 根路径。该切换已在 14 后实现：当前 `run`、Artifact 与全局 `history` 只写用户级 Local Data Root，新 Run 不再写项目内 `.workflow`；需要保留的开发期 legacy 数据通过显式、幂等的一次性迁移进入新事实来源，不维持新旧位置双写。

### 5.2 Workflow / Draft / Revision

```text
Workflow 8f...
├── Draft（可编辑、自动保存）
├── Revision 1（不可变）
├── Revision 2（不可变）
└── Revision 3（不可变）── Run A / Run B
```

规则：

1. 一个 Workflow 同时至多一个 Draft。
2. Draft 持有单调递增的内部 `lock_version`，它不是 Workflow Revision。Autosave 先比较规范化语义内容：无变化时 no-op，不更新时间、不递增 token；有变化时通过 `UpdateDraft(expected_lock_version)` 在单个 SQLite 事务中更新同一 Draft 行、递增 lock_version 并更新 `updated_at`。
3. 用户点击 Run 前，UI flush 有变化的 autosave，并把当前 expected_lock_version 传给 StartRun；token 冲突时不执行任何物化、Revision 或 Run 写入。
4. StartRun preflight 为尚未选择模型的 Agent Node 按当前默认层级物化 Gum Model UUID并写回 Draft，再以该 Draft 创建或复用 immutable Revision；没有独立 Publish 动作。实际 Workflow Run 启动后不回写定义。
5. Draft 的规范化语义内容与既有 Revision 完全一致时，运行复用该 Revision，不创建重复版本；每次点击 Run 仍创建新的 Run。
6. Revision 以规范化内容哈希判断等价；哈希纳入 semantic schema version、Node Instance ID、Node Definition / Executor 选择、Node config、input bindings、dependsOn、Project binding、Node 记录的 Gum Model UUID 和其他影响验证或执行的字段。
7. Workflow / Node 展示名称与描述、Presentation Hint、时间戳、缩放、折叠、最近选择和自动布局坐标不进入语义哈希。
8. 默认 Provider/Model、Provider 的 Base URL、API Key 引用和其他可变连接内容不进入 Revision；StartRun 时的 Resolved LLM Selection 进入 Run Snapshot。API Key 不进入 Revision 或 Run Snapshot。
9. Map、set 和其他无语义顺序的集合必须先规范化再哈希；仅调整存储顺序不得创建新 Revision。
10. Run 启动后只读取 Run Snapshot；后续 Draft/Revision 变化不影响它。
11. Revision 可以比较和恢复为新 Draft，但不可原地修改。
12. 非法 Draft 允许保存，并与 Preview + Diagnostics 一起返回；首版只支持单窗口编辑，版本冲突时要求刷新，不做字段级 merge。
13. UI view preference 独立保存，不与语义 Draft 共用 lock_version。

### 5.3 UI 元数据

画布坐标不是执行语义。本产品默认自动布局，因此初期只需保存用户层面的视图偏好，如折叠组、缩放和最近选择；这些偏好：

- 不进入 Semantic Validator；
- 不影响 Revision 内容哈希；
- 不影响运行；
- 将来导出时可选择是否包含。

### 5.4 Schema Migration

- SQLite 使用顺序 `user_version` 迁移；
- 每次迁移有 upgrade 测试和旧库 fixture；
- immutable Revision 内容携带 schema version；
- 应用启动先迁移后开放写操作；
- 迁移失败保留原库并给出可恢复错误，不创建半迁移状态。

---

## 6. Workflow 创作与结构预览

### 6.1 创作入口

用户通过结构化 UI：

1. 新建 Workflow；
2. 从 Node Catalog 添加 Node Instance；
3. 在表单中配置 Node；
4. 为每个 Input 选择上游 Node Output；
5. 声明 Control Dependency；
6. 查看结构预览和诊断；
7. 点击 Run，由 Application 创建或复用 Revision 后启动运行。

首个闭环实现通用的 Node Instance 添加、端口绑定、循环配置和诊断 seam，但 Node Catalog 只需覆盖 `human-chat`、`llm-chat` 及闭环直接需要的节点。不得用硬编码聊天页面或只能运行的预置 Workflow 代替通用创作路径。

本阶段不实现 AI 修改 Workflow，也不通过画布拖入 Node、手工拉线或拖动坐标改变顺序。

### 6.2 Workflow Preview 投影

Go Application 根据 Draft/Revision 产生：

```go
type WorkflowPreview struct {
    Nodes       []PreviewNode
    Edges       []PreviewEdge
    Groups      []PreviewGroup
    Diagnostics []Diagnostic
}
```

后端负责解析：

- Data Edge 与 Control Edge；
- Input/Output TypeExpr；
- Node/Executor/LLM Preference；
- 强连通分量和循环组；
- 错误与提示定位。

前端负责：

- 自动计算坐标；
- 渲染 Node/Edge/Group；
- 选择节点或边；
- 缩放、平移、适应窗口；
- 在 Definition Mode 与 Run Mode 上覆盖不同信息。

### 6.3 自动布局

1. 对完整图运行强连通分量算法；
2. 将每个循环折叠为 Iteration Group；
3. 对压缩后的 DAG 做从左到右的 layered layout；
4. 同层 Node 使用稳定 ID 排序并执行交叉最小化；
5. 展开循环时采用复合图布局；
6. 结构未变化时保持布局稳定，配置文本修改不得导致 Node 跳动。

推荐前端以 React Flow 类库承担渲染、选择、缩放和平移，以 ELK layered 类算法承担布局；关闭 dragging 与 connecting。最终技术可替换，但 Preview ViewModel 不依赖具体渲染库。

### 6.4 执行顺序表达

UI 使用“阶段/层”表达潜在先后，而非给所有 Node 标固定序号：

- Data/Control 前驱完成后，下游才可能 Ready；
- 同层无依赖 Node 可并行；
- 循环组内可能多轮执行；
- Human Node 可能等待；
- 预览不承诺实际耗时顺序。

### 6.5 不完整 Draft

Preview 必须在 Draft 非法时仍可生成：

- 未绑定 Input 显示为悬空端口；
- 类型不兼容 Edge 显示定位错误；
- LLM Preference 悬空的 Agent Node 保留在图上并显示缺项；默认 Provider/Model 缺失属于 StartRun 设置诊断；
- 点击 Diagnostic 直接打开对应 Node 和字段；
- `Preview + Diagnostics` 一起返回，不因第一个错误丢失整张图。

---

## 7. Node 描述能力

### 7.1 分层

Node Definition 继续作为业务契约的唯一声明处；14 后的新版本或显式升级增加四类描述：

1. **Contract**：inputs / outputs / optional / TypeExpr；
2. **Requirements**：llm、project、network、command、secret 等运行资源需求；
3. **Config Schema**：Node Instance 可配置字段及其校验；
4. **Presentation Hint**：显示名、分类、图标、编辑器类型、文档入口等可忽略的 UI 信息。

Config Schema 负责“值是否合法”，Presentation Hint 负责“如何展示”。忽略 Presentation Hint 不得改变运行语义。

首版 Config Schema 使用 Gum 自有的小型类型模型，不把 JSON Schema、CUE AST 或前端表单库结构暴露为领域接口：

- 字段类型：string、markdown、integer、number、boolean、enum；
- Config Contract：required、default、min/max、enum values、sensitive；
- Presentation Hint：label、help、editor；
- Go Semantic Validator 与 Desktop 表单使用同一 Schema；
- 将来接入外部 Schema 时通过 Adapter 转换。

### 7.2 建议的描述维度

| 维度 | 内容 |
|---|---|
| 身份 | name、display name、description、category、version |
| Contract | port 名、TypeExpr、optional、description |
| Config | 类型、必填、默认值、范围、枚举、敏感性 |
| Requirements | llm、project、network、command、secret |
| Side Effect | Workspace 写入、外部写操作、费用、幂等性 |
| Recovery | retry 特性、是否可 resume、unknown outcome 处置 |
| Observability | usage、latency、provider request id、finish reason、logs |
| Presentation | icon、category、field editor、Artifact preview preference |

### 7.3 模型适配责任

Gum 不维护或匹配 LLM Model Capability。Node/Artifact Contract 只判断 Workflow 数据是否合法；用户为 Agent Node 选择 Model UUID，即确认该模型适合该任务。若实际请求包含模型不支持的 image、tools 或其他特性，Protocol Adapter 仍按合同组装请求，Provider 拒绝时记录并展示原始 Provider/Structural Error，由用户更换模型解决。

---

## 8. LLM Provider / Model 模块

### 8.1 定位

LLM Provider 是用户级运行环境中的连接配置，下挂有稳定顺序的手工 Model 列表：

```text
LLM Provider: “公司 OpenAI 网关”
├── protocol: openai-chat-completions
├── base URL: https://llm.example.com/v1
├── API key: secret://...
└── models
    ├── model-a
    └── model-b
```

Workflow 中的 Agent Node Definition 只声明需要 LLM 资源：

```text
requires: [llm, network]
```

Agent Node Instance 的 LLM Preference 只记录 Gum Model UUID。UI 允许用户从 Provider 下选择 Model，或使用默认值；Node 永远不保存 Base URL、API Key 或可变 Provider Model ID。StartRun 把实际解析结果固定进 Run Snapshot。

### 8.2 数据模型

```go
type LLMProvider struct {
    ID             string
    Name           string
    Description    string
    Protocol       Protocol
    BaseURL        string
    APIKeyRef      string
    IsDefault      bool
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      *time.Time
}

type LLMModel struct {
    ID                string
    ProviderID        string
    ModelID           string
    DisplayName       string
    IsDefault         bool
    GenerationDefault GenerationConfig
    CreatedAt         time.Time
    DeletedAt         *time.Time
}

type LLMPreference struct {
    ModelUUID string // empty = resolve current defaults
}
```

`APIKeyRef` 只保存 Secret Adapter 返回的引用。桌面 UI 中用户填写的 API Key 直接写入操作系统安全凭据存储；SQLite 只保存引用。自动化测试和无桌面环境的开发 Adapter 可以使用环境变量引用。若安全凭据存储不可用，平台必须提示用户改用环境变量引用，不能静默降级为 SQLite 明文。Revision、Run Snapshot、运行记录、日志和未来导出内容均不得保存 API Key 明文。

### 8.3 UI 流程

设置页：

```text
新建 LLM Provider
→ 名称
→ 选择协议
→ Base URL
→ API Key
→ 手工添加 Provider Model ID
→ 保存 Provider
→ 可选：设为默认 Provider / 默认 Model
```

Agent Node 配置页：

```text
Provider: [使用默认值 ▼]
Model:    [使用 Provider 默认值 ▼]
```

UI 用 Provider 对 Model 分组，但持久化时只记录所选 Model 的 Gum UUID。“使用默认值”允许 Draft 暂时为空；首次 StartRun preflight 会解析当前 default 并把 UUID 写回 Draft。UI 不要求用户维护模型能力表。

### 8.4 LLM Preference 解析与 Run Snapshot

Run 启动时解析：

1. 每个 Agent Node 的 LLM Preference 结构合法；
2. Preference 已有 Gum Model UUID 时，该 UUID 必须仍存在且未删除；删除或缺失时返回 Node 字段诊断，不 fallback、不创建 Run；
3. Preference 为空时，取未删除的显式默认 Provider；没有显式默认时按 `(created_at ASC, UUID ASC)` 取第一个未删除 Provider；
4. 再取该 Provider 下未删除的显式默认 Model；没有显式默认时按 `(created_at ASC, UUID ASC)` 取第一个未删除 Model；
5. 把解析出的 Gum Model UUID 写入 Draft，并递增 Draft version；
6. 基于已物化 UUID 的 Draft 创建或复用 immutable Revision；
7. API Key 引用必须可解析；
8. 按优先级合并生成参数，把结果作为 Resolved LLM Selection 写入 Run Snapshot；
9. 创建 Run。上述写操作必须保持原子性，失败不得留下部分 Draft/Revision/Run。

任一步失败都在创建 Run 前返回具体 Node 与字段的诊断，不创建 Run。系统不自动切换 Provider，也不在已绑定 Model 被删除后改用 default。

用户修改 Provider 的 Base URL、API Key、Provider Model ID 或生成默认值后，Gum Model UUID 不变，Workflow Revision 不变，未来 Run 使用新连接内容；既有 Run Snapshot 保持原值。删除 Provider/Model 不改写 Workflow，但删除前提示受影响 Workflow；其 Draft 与表单随后飘红，用户重新选择 Model UUID 后才能 StartRun。历史 Run 继续从 Snapshot 展示旧 Provider/Model。

生成参数优先级：

```text
Node Instance 显式参数
    > LLM Model 默认参数
    > Protocol Adapter 默认参数
```

Run Snapshot 记录：Provider ID/name、Protocol、Base URL、Provider Model ID 和有效生成参数；不记录 API Key。

---

## 9. 协议 Adapter 与多轮请求

### 9.1 模块分层

```text
Canonical Conversation Model
            ↓
Canonical GenerateRequest
            ↓
Protocol Adapter
    └── OpenAI Chat Completions
            ↓
HTTP Transport
```

建议的深接口：

```go
type ProtocolAdapter interface {
    Generate(ctx context.Context, conn Connection, req GenerateRequest) (GenerateResult, error)
}
```

最终签名在实施设计中确定。接口必须让当前 OpenAI-compatible Adapter、future Adapter 和 fake Adapter 共用，测试只跨该 seam 断言规范化行为。Streaming 进入范围时另加可选 streaming seam，不让首个非流式接口提前承担 StreamSink。

### 9.2 Canonical Request

```go
type GenerateRequest struct {
    Model        string
    Instructions []ContentPart
    Messages     []ChatMessage
    Config       GenerationConfig
}
```

Canonical 层不保存 OpenAI/Anthropic 的原始 JSON 字段。Protocol Adapter 负责映射；原始请求与响应可在脱敏后作为诊断数据记录。

### 9.3 OpenAI-compatible Chat Completions

- endpoint：相对 Base URL 的 `chat/completions`；
- auth：`Authorization: Bearer <key>`；
- instructions：根据 Provider 的协议/dialect 设置映射为 developer 或 system message；为兼容第三方默认可配置 system；
- history：按原顺序发送 user / assistant messages；
- P10 content：只映射 text Content Part；
- response：规范化 assistant text、usage、finish reason 和 request id。

OpenAI 官方 Chat Completions 以有序 message 列表表达对话，并支持多种 content part。OpenAI-compatible 只承诺本计划明确采用的共同子集；厂商扩展通过 Adapter 内部 dialect 处理，不泄露到 Conversation Artifact。

### 9.4 Backlog：Anthropic Messages

- endpoint：相对 Base URL 的 `messages`；
- auth：`x-api-key` + `anthropic-version`；
- instructions：放入顶层 `system`，不创建 system role message；
- history：只发送 user / assistant，按协议要求规范化连续同角色 turn；
- multimodal：映射为 Anthropic content blocks；
- response：规范化 content blocks、usage、stop reason、request id 和 stream delta。

Anthropic Messages 是无状态多轮接口，每轮必须由本地 Conversation Artifact 提供所需历史。

### 9.5 对话组装不变式

1. 输入 Conversation 的最后一条消息必须是本轮新提交的 user message；
2. Request Builder 按原顺序发送输入 Conversation；
3. 成功响应后输出 `conversation = input conversation + assistant output`；
4. 不在 Node Executor 内保存隐藏 session；
5. 发送给 Provider 的实际消息快照进入 Node Run 诊断，但密钥与敏感 Header 必须脱敏；
6. 上下文超限时不静默删除历史；初期返回明确错误，未来 Context Policy 另行设计；
7. Gum 不在网络请求前判断模型是否支持实际模态；Provider 拒绝时保留并展示错误；
8. Streaming 进入范围后，delta 只用于 UI/日志，完整完成前不生成正式输出 Artifact。

---

## 10. 手工模型目录

当前产品化范围不实现 `/models` 服务端发现。模型目录只来自用户手工声明；不设计 discovered/manual merge、raw discovery、last-seen 或 disappearance 规则。

### 10.1 手工目录规则

1. 用户显式填写 Provider 使用的 Model ID；
2. 每个 Model 分配稳定 Gum UUID，Provider Model ID 可以编辑；
3. Gum 不探测、推断或要求用户维护 Model Capability；选择 Model 即表示用户确认其适合任务；
4. Default Model 必须是未删除的手工 Model；未显式设置时按 `(created_at ASC, UUID ASC)` 取第一个未删除 Model；
5. `/models` discovery 若未来重新进入范围，必须作为独立设计，不得静默改变现有手工目录或 Model UUID。

### 10.2 Base URL

Base URL 表示协议 API root，例如 `https://example.com/v1`。Adapter 使用 URL parser 追加相对资源，不能用字符串拼接；需覆盖：

- 尾部有/无 `/`；
- Base URL 已含 `/v1`；
- 反向代理子路径；
- query/fragment 非法；
- 禁止因拼接产生重复 `/v1/v1/...`；
- localhost 与企业内网 endpoint。

---

## 11. 真实 `llm-chat` Agent Node

### 11.1 目的

`llm-chat` 是第一个简单但真实的 Agent Node，用来验证：

- LLM Provider/Model 与 UI 下拉选择；
- 首切片 OpenAI-compatible，后续沿 ProtocolAdapter 增加 Anthropic；
- 首切片单轮 text Request Builder，后续增加多轮和多模态；
- Node Config Schema；
- 用户负责所选模型是否适合实际输入；
- 后续 streaming 运行观测；
- Chat Artifact 和 Artifact Preview；
- usage、latency、finish reason 和错误分类。

它不调用工具、不修改 Workspace、不承担 Coding Agent 职责。

首个产品切片只实现 OpenAI-compatible Chat Completions、非流式 text input -> text output 和 macOS。Canonical ChatMessage / ContentPart 和 ProtocolAdapter seam 必须在该切片中成立；Streaming 与 Windows 支持进入待办，Anthropic Messages 与 image input 在后续切片沿同一 seam 增加。

### 11.2 首个多轮 Workflow 拓扑

```text
human-chat ──Conversation──> llm-chat
     ▲                           │
     └──────Conversation─────────┘
```

- `human-chat` 是唯一可在没有必需输入时自举的 Human Chat Entry Node；
- 首次执行时等待用户提交，并产出以 user message 结尾的 Conversation；
- `llm-chat` 追加 assistant message 后把新 Conversation 反馈给 `human-chat`；
- 反馈只使 `human-chat` 进入 WaitingHuman，不会自动产出下一轮；
- 下一次人工提交追加新的 user message，再触发 `llm-chat`；
- 这是一项 14 后显式语义升级：唯一入口从“完全无 inputs 的 `human-input`”升级为“唯一可在没有必需输入时自举的 Human Chat Entry Node”，不得倒灌 workflow/v1。

实施不得把这项调度升级塞入第一个端到端切片：

1. 首个切片使用单向 `human-chat(source) -> llm-chat`，验证 SQLite Workflow、Revision、真实 LLM、Artifact 与通用 Application seam；
2. 下一切片再让 `human-chat` 接收 optional Conversation feedback，并实现 WaitingHuman、每次人工提交一个新 Node Run 与显式回边；
3. 不增加“只提供上下文但不触发 Ready”的 context input；所有 Input 继续遵守统一 dirty/Ready 规则。

### 11.3 Contract

`human-chat`：

```yaml
inputs:
  history:
    type: Conversation
    optional: true
outputs:
  conversation:
    type: Conversation
```

`llm-chat`：

```yaml
inputs:
  conversation:
    type: Conversation
outputs:
  conversation:
    type: Conversation
```

初期：

- 首个切片 human input：text Content Part；
- assistant output：text Content Part；
- `llm-chat` 输入必须以 user message 结尾，输出必须在其后追加一个 assistant message；
- Conversation 每轮产生完整新版本；
- usage 等运行数据记 Node Run metadata，不作为 Node 间默认数据通道。

### 11.4 Config Schema

```text
instructions       markdown，可选
temperature        float，可选
max_output_tokens  int，可选
```

Node Definition 通过 `requires: llm` 声明需要模型资源；Node Instance 保存可选 LLM Preference，但不保存 Provider 的 Base URL 或 API Key。StartRun 按 Preference 与默认层级解析。

### 11.5 运行属性

| 属性 | 值 |
|---|---|
| Node Type | agent |
| requires | llm、network |
| Workspace 写入 | 否 |
| 外部副作用 | 模型调用与费用；无业务写操作 |
| Interaction Retry | 可创建新 Node Run，但会产生额外调用/费用，输出不保证相同 |
| 中断 | HTTP/stream 不支持续传；UnknownOutcome 不自动重放 |
| Streaming | 首个切片不支持；列入后续待办 |
| 输出校验 | 输入 Conversation 后恰好追加一个 assistant message，且含 text content |

### 11.6 错误

- 配置缺失、认证失败、网络不可达、限流/服务不可用：Structural Error；
- Provider 拒绝所选模型不支持的模态或特性：Structural Error，由用户更换模型；
- 响应协议无法解析：Structural Error；
- 后续结构化输出不符合 Contract：Interaction Error；
- 进程崩溃或取消导致结果未知：Node Run UnknownOutcome，不自动重放；完整恢复流程后置。

---

## 12. 首个闭环的 Artifact 查看

### 12.1 Artifact 是一等 UI 对象

首个闭环只要求用户能确认 Workflow 的真实输入和输出。每个 Artifact View 最少展示：

- Kind、MIME、版本、大小、哈希；
- 生产 Node、Node Run ID、round 和时间；
- 消费 Node 与本轮绑定；
- Conversation 的 user / assistant 消息和 text Content Part；
- 通用文本或结构化数据的安全只读预览。

### 12.2 Preview Registry

首个闭环的 Preview Registry 只实现实际验收需要的类型：

- Markdown/Text；
- Chat Message / Conversation；

未知 Kind 使用通用元数据视图。Source diff、JSON/OpenAPI 专用视图、Test Report、外部资源、版本比较和人工替换均后置。

### 12.3 安全

- 不执行 Artifact 中的脚本；
- HTML 默认以文本或隔离方式预览；
- 外部 URL 打开前显示目标；
- 大文件分段读取，不一次加载进 UI；
- Secret 和敏感 Header 不得作为 Artifact 保存。

---

## 13. 首个闭环的运行观测

本阶段只设计让 Desktop UI 启动 Run、显示 Node 状态并查看最终 Artifact 所必需的观测 seam，不实现完整 append-only Run Event 或事件重放。Streaming 作为后续待办加入。

首个闭环持久化 Workflow、Draft、Revision、Run Snapshot、Node Run、正式 Artifact 和错误。已完成 Run 与 Conversation Artifact 在应用重启后仍可查看。启动时发现未结束 Run，将其标记为 Interrupted 并保留查询能力，但当前版本明确不可恢复，也不得自动重放任何 Node Run。Streaming 加入后，LLM Content Delta 仍只作为进程内临时 UI 信号，不持久化。

### 13.1 最小观测信号

- RunStarted / RunStopped / RunFailed；
- NodeReady / NodeRunStarted / NodeWaitingHuman / NodeRunSucceeded / NodeRunFailed；
- ArtifactProduced；
- HumanInputRequested / HumanInputSubmitted；
- LLMRequestStarted / LLMRequestCompleted / LLMRequestFailed。`LLMContentDelta` 随 Streaming 待办加入。

首个非流式切片只在完整响应成功后生成 Conversation Artifact。未来 Streaming 的 Content Delta 只用于临时 UI，完成前仍不得生成正式 Artifact。

### 13.2 后置边界

完整 Event Envelope、持久化重放、Pause/Resume、Interrupted 恢复、Rerun、Fork、Manual Artifact 和受影响下游预览不属于首个产品闭环。未来设计必须遵守第 3.3 节已确认的运行边界。

### 13.3 首个闭环的失败边界

- Draft 结构、端口绑定、已绑定 Model UUID、默认 Provider/Model 与 Secret 引用可解析性在创建 Run 前校验；空 Preference 在 preflight 中物化，悬空 UUID 报错；失败时不创建 Run；不做 Model Capability 校验；
- Run 创建后发生的认证失败、网络错误、限流、服务不可用或协议响应损坏均为 Structural Error，使 Run 进入终态 Failed；
- Provider 已成功返回、但业务输出不符合 Node Contract 时才是 Interaction Error，Run 保持 Running；
- 首个闭环不新增通用 Retry UI，也不因为错误可能是暂时的而自动重试；既有 advise 语义保持不变。

---

## 14. 测试策略

### 14.1 LLM Provider / Model

- Provider/Model 创建、编辑、显式默认与删除；首版不做 enable/disable。删除前提示受影响 Workflow，删除后悬空 UUID 阻止 StartRun；
- Desktop Secret Adapter 与环境变量 Secret Adapter 行为一致；
- API Key 不落库/日志/观测信号；
- Run Snapshot 固定后修改 Provider 不影响已启动 Run；
- 手工 Provider Model ID、稳定 Gum UUID 与 Provider/Model 默认解析；
- 当前范围不存在 discovery、raw response、overlay 或 refresh。

### 14.2 协议契约

全部使用 `httptest.Server` 和固定 fixture，单测禁止真实网络：

- OpenAI 首切片覆盖非流式单轮 text；后续分别增加多轮、image 和 stream；
- Backlog：Anthropic 顶层 system、user/assistant 历史、连续角色规范化、text/image、stream；
- usage、finish/stop reason、request ID；
- malformed response、认证、限流、取消；
- Base URL 所有边界；

P10 请求体使用单轮 text golden fixture，准确断言 message 顺序和 text content part；P12 再增加多轮 Conversation golden。

### 14.3 `llm-chat`

- 单轮切片：`human-chat(source) -> llm-chat` 产生并持久化一次真实 Conversation；
- 单轮切片只使用 OpenAI-compatible Chat Completions 与 text Content Part；
- 多轮切片：在单轮验收后才升级入口语义；
- `human-chat` 无历史时可自举并等待首次输入；
- `human-chat -> llm-chat -> human-chat` 多轮交替；
- Conversation 的 user / assistant 追加顺序；
- P10–P12 只覆盖 text；image 进入独立待办；
- Provider 拒绝所选模型不支持的请求时返回 Structural Error；
- 首切片非流式；未来 streaming delta 不得提前产生 Artifact；
- 完整响应后产生一个完整 Conversation 新版本；
- Structural / Interaction / UnknownOutcome；
- API Key 与敏感 Header 脱敏。

### 14.4 Workflow 产品模型

- Draft 自动保存；
- immutable Revision；
- 无独立 Publish 动作，StartRun 创建或复用 Revision；
- 相同内容不重复 Revision；
- Run Snapshot 不受 Draft/Config 后续修改影响；
- Preview 在非法 Draft 下仍返回完整图和 Diagnostics；
- SCC/循环组、分支、汇合、Data/Control Edge；
- UI 坐标不影响语义哈希。

### 14.5 最小运行观测

- Desktop UI 可观察 Run、Node Run、Human Waiting 和 Artifact Produced；
- 非流式响应完成前不产生正式 Artifact；未来 Streaming Delta 同样不得提前成为 Artifact；
- 最终 Conversation Artifact 与 Node Run 成功状态一致；
- Structural Error 仍使 Run Failed；
- UnknownOutcome 不会被伪装成成功或自动重放。
- 应用重启后已完成 Run 与 Conversation 仍可查询，未完成 Run 显示为当前不可恢复的 Interrupted；
- Run 前校验失败不创建 Run；Run 后 transport/protocol failure 终结 Run 且不自动重试。

### 14.6 桌面 UI

- Browser Mock Adapter 做页面与交互测试；
- Wails/Desktop Adapter 做类型绑定和事件测试；
- 首个闭环以 macOS 为功能验收平台；Windows 支持列入待办；
- 首个产品切片必须经过真实薄 Desktop UI，走通通用 Node 添加、端口绑定、LLM Provider/Model、StartRun、人工输入与 Artifact 查看；
- 单轮 e2e：配置 LLM → 创建 `human-chat(source) -> llm-chat` → Preview → Run → 输入 → 完整响应 → Conversation Artifact；
- 后续多轮 e2e：把 Conversation 回接 `human-chat` → Run → 首轮输入 → assistant → WaitingHuman → 第二轮输入。

---

## 15. 开发阶段与验收门

### P9：macOS Product Tracer

交付：

- macOS desktop shell 与真实薄 UI；
- SQLite product schema、migration、Workflow / Draft / Revision / Run Snapshot；
- Draft lock_version + expected_lock_version CAS、无变化 autosave no-op、非法 Draft + Preview/Diagnostics；StartRun 同样校验 expected_lock_version；
- Gum Config Schema 驱动的通用 Node 表单；
- LLM Provider/Model 设置、稳定 Model UUID、双层 default 与删除提示；
- 通用 Node 添加和端口绑定，首批 Catalog 为 `human-chat(source)` 与 `llm-chat`；
- fake executor 驱动 StartRun、Revision、Node Run 和 Conversation Artifact；
- Browser Mock 与 Desktop Adapter 复用同一个 Workflow Client interface。

验收：用户在真实 macOS UI 中创建 SQLite Workflow、配置 Provider/Model、添加并连接两个 Node，通过 fake executor 完成 Run 并查看 Artifact。该阶段是 TDD tracer，不声称真实 LLM 产品闭环完成。

### P10：首个真实产品闭环

交付：

- macOS Keychain Secret Adapter 与测试用环境变量 Adapter；
- Canonical ChatMessage / ContentPart / GenerateRequest；
- OpenAI-compatible Chat Completions 非流式 text Adapter；
- StartRun preflight 物化默认 Gum Model UUID，悬空 UUID 阻止 Run；
- Resolved LLM Selection 与 Run Snapshot；
- 真实 `human-chat(source) -> llm-chat` 单轮执行；
- Conversation Artifact、usage、Provider request ID、finish reason、错误和历史持久化；
- httptest fixture、请求体 golden 和 API Key 脱敏测试。

验收：用户在 macOS UI 中手工配置 Provider/Model，创建通用 Workflow，提交 text，获得并持久化真实 Conversation；修改 default 不改变已绑定 UUID，修改 Provider 内容只影响未来 Run，删除 Model 后表单飘红且 StartRun 不创建 Run。

### P11：闭环加固

交付：

- schema migration fixture 与旧库升级测试；
- 应用重启后把未结束 Run 标为当前不可恢复的 Interrupted；
- 已完成 Run、Node Run、Conversation Artifact 与错误可重启后查询；
- Provider/Model 删除影响提示、悬空 UUID Diagnostics；
- Structural / Interaction / UnknownOutcome 展示；
- 日志脱敏、Crash report bundle；
- macOS 构建、安装和升级验证。

验收：首个真实闭环可以重复安装、升级、诊断和回归验证；崩溃或 Provider 错误不会被伪装为成功或自动重放。

### P12：多轮对话升级

交付：

- Human Chat Entry Node：optional Conversation feedback、无必需输入时自举；
- Human Executor 接收上一版 Conversation；
- WaitingHuman 状态与每次人工提交一个新 Node Run；
- `human-chat -> llm-chat -> human-chat` 显式回边；
- Validator 从唯一无 inputs human-input 升级为唯一可自举 Human Entry；
- 两轮对话、回边 dirty/Ready 和 Convergence Guard 测试。

验收：用户在 UI 中创建显式对话循环；assistant 输出只使 Human Entry 等待，第二次人工提交才触发下一轮 `llm-chat`，不存在隐藏 Conversation 或模型自循环。

### 明确待办：协议、模态与平台扩展

以下内容在 P9–P12 后分别设计和排期，不预设相互顺序：

- Streaming：SSE、Content Delta、取消和 UnknownOutcome；
- Anthropic Messages Protocol Adapter；
- image Content Part、文件选择和 ArtifactRef；
- Windows Desktop Adapter、构建、安装和 e2e 对等。

### 后置候选：高级 Artifact Experience

以下内容不属于当前 P9–P12 的验收前置，暂不编号：

- Preview Registry；
- Chat/Markdown/Image/JSON/Source diff 初始 Previewer；
- lineage、round、版本比较；
- Manual Artifact；
- 大文件与安全限制。

### 后置候选：Run Event 与恢复控制

以下内容不属于当前 P9–P12 的验收前置，暂不编号：

- append-only Run Event；
- UI reconnect/replay；
- Pause/Resume；
- Retry/Rerun/Fork；
- Interrupted Resume 与 UnknownOutcome 处置/重试；
- 受影响下游预览。

### 首个闭环稳定性与产品形态评审

交付：

- schema migration fixtures；
- macOS 构建矩阵、安装和升级验证；Windows 对等支持后置；
- 日志脱敏、Crash report bundle；
- 权限预览的最小模型；
- 性能基线与大图/大 Artifact 测试；
- 产品 v1 形态评审。

验收：核心闭环达到可重复安装、升级、诊断和回归验证。该评审只判断当前产品形态，不自动启动导入/导出、Workflow Pack 或 AI Workflow authoring 设计。

阶段纪律：P(n) 未通过验收门，不开始依赖其领域语义的 P(n+1)；UI Prototype 可作为技术探索并行，但不得反向定义领域模型。

---

## 16. 首个产品闭环评审清单

判断首个 SQLite Workflow 闭环是否跑通，至少回答：

1. Workflow / Draft / Revision 是否稳定，是否仍频繁迁移？
2. Node Config Schema 能否覆盖真实 `llm-chat` 和至少一个非 Agent Node？
3. 手工 LLM Provider 与 Model ID 能否稳定支持第三方 OpenAI-compatible 差异？
4. 用户选择 Model 即承担适配责任、Provider 拒绝时由用户换模型的反馈是否清晰？
5. Preview 是否在分支、汇合、循环和非法 Draft 中保持清晰？
6. 用户是否能查看本轮 Conversation Artifact、来源 Node Run 与错误？
7. 最小运行观测是否足以解释 Run 为什么等待、运行或失败？
8. macOS 的安装、升级和本地文件权限是否可控？Windows 支持已有独立待办且未被当前架构堵死？
9. Secret、Prompt、Artifact 和日志的本地存储边界是否明确？

产品 v1 的范围仍未确定；在其确定前，不设计 YAML 导入/导出、导入冲突、Workflow Pack 或 AI 修改 Workflow。

---

## 17. 外部协议依据与演进原则

- OpenAI Chat Completions：有序 messages、developer/system/user/assistant role 与多种 content part。官方参考：<https://developers.openai.com/api/reference/cli/resources/chat/subresources/completions>。
- Anthropic Messages：顶层 system、无状态多轮 user/assistant messages、content blocks。官方参考：<https://platform.claude.com/docs/en/api/messages/create>。

协议会演进：实现时必须以版本化 Adapter 和 fixture 隔离变化，不能把某日的服务端私有 JSON 直接固化为 Gum-Workflows 的领域模型。

---

## 18. 决策摘要

1. 01–14 保持原计划；本文从其完成后开始。
2. GUI 是 Workflow 创作入口；画布只读、自动布局、可选择配置，不做拖拽式编排。
3. SQLite 是产品 Workflow 唯一的编辑与运行事实来源；现有 YAML CLI 不兼容、不隐式导入，v1 范围确定前不设计 YAML 导入/导出。
4. Workflow 使用自动保存的 Draft + immutable Revision；没有独立 Publish，StartRun 创建或复用 Revision 并固定 Snapshot。
5. 首个闭环只实现运行所需的基本 Artifact 查看；高级预览、版本比较与人工替换后置。
6. 先打磨 Node，不规划内置 Workflow 库。
7. `llm-chat` 是第一个简单但真实的 Agent Node；首个多轮闭环固定为 `human-chat -> llm-chat -> human-chat`。
8. 用户级 LLM 设置采用 `Provider -> Models`；每个 Model 有稳定 Gum UUID。未选择模型的 Node 在首次 StartRun preflight 按双层 default 物化 UUID；UUID 存在时不受 default 变化影响，删除后不 fallback，必须由用户重新选择；Run Snapshot 保存历史实际连接与 Model。
9. 首个切片只支持 OpenAI-compatible；Anthropic Messages 后续沿同一 ProtocolAdapter seam 加入。
10. 当前范围不实现 `/models` discovery 或 Model Capability 目录；用户手工声明 Model ID，选择模型即确认适合任务，Provider 拒绝时由用户更换。
11. 首个闭环只实现 UI 必需的运行观测；完整 Run Event、Resume、Rerun、Fork、Manual Artifact 与崩溃恢复后置，但未来不得违背第 3.3 节的终态和 UnknownOutcome 边界。
12. AI 修改 Workflow、Workflow Pack、内置工作流、云同步和高级 Trigger 全部后置。
13. 首个 UI 走通通用 Node 添加、端口绑定、循环与诊断 seam，但首批 Catalog 只需支持对话闭环，不以硬编码聊天 Demo 代替 Workflow 创作。
14. 正式 Run/Node Run/Artifact/错误持久化，Content Delta 不持久化；重启后未结束 Run 显示为当前不可恢复的 Interrupted，且不自动重放。
15. Run 前结构、Model UUID、默认值和 Secret 错误不创建 Run；不做 Model Capability 校验。Run 后认证、transport、限流、协议或 Provider 拒绝错误均为 Structural Error 并终结 Run，不自动重试。
16. 最终多轮模型保持显式 Human Gate 回环；第一个可执行切片只做单轮，下一切片再升级入口与 WaitingHuman，不增加 non-triggering context input。
17. 首个产品切片只支持 OpenAI-compatible 与 text -> text；Anthropic 和 image 后续加入，但首切片必须使用正式 Canonical Message/ContentPart 与 ProtocolAdapter seam。
18. 首个产品切片必须经过真实薄 Desktop UI 和通用创作路径，不接受 fake-only 或硬编码聊天 Demo。
19. Revision 语义哈希只覆盖执行语义；展示文案、UI 状态与布局不产生新 Revision，LLM Provider 可变连接详情由 Run Snapshot 固定。
20. 首个闭环为 macOS、非流式；Streaming 与 Windows 支持均进入明确待办。
21. Config Schema 使用 Gum 自有的小型类型模型；Contract 与 Presentation Hint 分离，不暴露 JSON Schema、CUE AST 或前端库结构。
22. Draft autosave 不创建 Revision：语义内容无变化时 no-op，有变化时更新同一行并使用 lock_version + expected_lock_version CAS；StartRun 也必须校验 UI 当前 token；非法 Draft 可保存并返回 Diagnostics，首版单窗口、冲突刷新、无字段级 merge。
23. 相同语义内容重复 Run 复用同一 immutable Revision，但每次创建新的 Run；首次物化 Model UUID 导致语义变化时才创建新 Revision。
24. P9–P12 改为纵向交付：macOS fake Product Tracer -> 真实 OpenAI text 闭环 -> 持久化与诊断加固 -> Human Chat 多轮升级；Streaming、Anthropic、image 和 Windows 分别进入待办。
