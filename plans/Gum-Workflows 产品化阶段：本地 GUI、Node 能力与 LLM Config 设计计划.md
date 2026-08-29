# Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Config 设计计划

> 状态：14 后设计计划（2026-08-27；产品方向已确认，实施细节在各阶段开工前逐项评审）。
>
> 前置条件：`.scratch/platform-core/spec.md` 与 `.scratch/platform-core/issues/` 中 01–14 全部完成并验收。
>
> 当前进度：platform-core 与 code-quality-automation 已完成；Local Data Root、In-place Project Workspace、Workflow Context Binding、ScriptNode 和首批四个 Go Code Quality Check 已落地。本文其余 GUI、Draft/Revision、真实 LLM、Artifact 产品体验与运行恢复仍为后续规划。
>
> 范围隔离：本文只规划 14 之后的新阶段，不修改、不替代、不向前倒灌 01–14 的范围、顺序与验收标准。本文涉及的新定义字段、运行控制或持久化模型，实施前必须按新版本或显式升级设计落地，不得静默扩展现有 workflow/v1。

---

## 1. 产品定位

Gum-Workflows 从 01–14 完成后的 Workflow Runtime，继续发展为面向技术人员的本地工作流产品。这里的技术人员包括代码开发、产品经理、测试、设计、运维等能够描述自身工作过程并配置工具的人群。

产品目标：

1. **简易创作**：用户通过本地 GUI 新建 Workflow、声明 Node Instance、配置端口绑定和运行参数；无需手写 YAML，也不依赖拖拽式低代码画布。
2. **本地优先**：Workflow 定义、LLM Config、运行历史与 Artifact 默认保存在用户本地；代码工作流直接使用用户项目目录作为 In-place Project Workspace。云端与多设备同步属于后续演进，但本地数据模型须为其预留稳定身份、版本和迁移能力。
3. **可观察、可调试、可恢复**：用户能理解 Workflow 为什么按当前结构运行，能查看每次 Node Run 的输入输出和错误，并能在 Agent 出现预期外行为时通过人工、半自动或自动方式继续。
4. **Node 优先**：产品化初期先打磨 Node Definition、Node Executor、真实 Agent Node、Artifact 和调试能力，不急于建设内置 Workflow 库。
5. **结构即语义**：Workflow 的执行语义来自 Node Contract、Data Edge 与 Control Edge；画布坐标、视觉排列和 UI 操作不是执行语义。

产品不是：

- 通用无代码应用平台；
- 通过拖拽和手工拉线作为主要创作方式的 Dify 类工具；
- 云端优先或多人实时协作平台；
- 以 YAML 为主要用户界面的配置工具；
- 对生成式模型输出作完全确定性承诺的任务调度器。

Gum-Workflows 对“可重复运行”的承诺是：输入、Workflow Revision、Node Executor、LLM Selection、有效参数、Artifact、错误和人工干预均可追溯；运行可在明确位置重试、重算、分叉或恢复。

---

## 2. 已确认的阶段边界

### 2.1 本阶段纳入设计

- SQLite 中的 Workflow / Draft / immutable Revision / Run Snapshot。
- 面向 UI、CLI 和未来其他 Adapter 的 Application 模块。
- macOS 与 Windows 本地桌面 GUI。
- 通过 UI 新建、声明、配置和优化 Workflow。
- 自动排列、只读选择的 Workflow 结构预览。
- Node Config Schema、能力要求、运行属性和 UI Hint。
- 独立、用户级、可复用的 LLM Config 模块。
- OpenAI-compatible Chat Completions 与 Anthropic Messages 协议 Adapter。
- 正确的单轮、多轮和多模态请求组装。
- 服务端模型发现与手工模型配置。
- 一个简单但真实的 `llm-chat` Agent Node。
- 多类型 Artifact 预览、来源追踪和版本比较。
- 结构化 Run Event，以及 Resume / Retry / Rerun / Fork / Manual Artifact 的语义。
- 为上述能力服务的本地测试、迁移和跨平台打包基础。

### 2.2 明确后置

- Workflow 导入/导出：保留 YAML 为可移植格式的方向，但在产品接近稳定 v1 前不规划命令、冲突策略或 Pack 格式。
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

**Workflow Draft**：Workflow 当前唯一可编辑的草稿。GUI 自动保存只更新 Draft，不影响已经发布或运行的 Revision。

**Workflow Revision**：Draft 在明确时间点形成的不可变版本。每次 Run 必须绑定一个 Revision；Draft 变化不能影响已启动 Run。

**Run Snapshot**：Run 启动时固定的完整解析结果，包括 Workflow Revision、Node Executor、LLM Config、LLM Model、有效生成参数和必要的项目环境信息。快照不含 API Key 明文。

**LLM Config**：用户级、可复用的 LLM 连接配置。每条 LLM Config 对应一个协议端点，包含名称、协议、Base URL、API Key 引用、模型目录、默认模型和连接状态。多个 Agent Node 可以引用同一条 LLM Config。

**LLM Model**：某条 LLM Config 下可用的模型。模型可由服务端发现、用户手工声明，或由两者合并得到。

**LLM Selection**：Agent Node Instance 对 LLM Config 和可选 LLM Model 的选择。省略 Model 时使用该 LLM Config 的默认模型。

**Model Capability**：模型支持的输入输出模态及特性，如 text/image/audio/file、streaming、structured output、tools、thinking。能力值为 supported / unsupported / unknown 三态，字段缺失不得解释为不支持。

**Workflow Preview**：从 Draft 或 Revision 派生的只读结构投影，包含 Node、Data Edge、Control Edge、循环组和诊断；坐标不属于 Workflow 语义。

### 3.2 对话与 Artifact

**Chat Message**：一条有 role 和一个或多个 Content Part 的对话消息。初期 role 只需 user / assistant；system/developer instruction 是 Node 配置，不混入业务对话 Artifact。

**Content Part**：Chat Message 的内容片段，初期实现 text 和 image；类型模型预留 audio 与 file。二进制内容通过 ArtifactRef 引用，不内联进入 Node 间传递。

**Conversation**：按顺序保存的 Chat Message 集合，是多轮上下文的数据本体。对话节点自身不保存隐藏历史。

**Manual Artifact**：用户通过 UI 人工提供或替换得到的新 Artifact 版本。Manual Artifact 不覆盖旧版本，并以人类事件记录其来源。

### 3.3 运行控制

**Resume**：继续同一个被暂停或异常中断的 Run；已成功的 Node Run 不重新执行。

**Retry**：在同一个 Run 内，以完全相同的输入快照为某个 Node Instance 创建新的 Node Run。

**Rerun**：在同一个活动 Run 内主动重算某个 Node Instance；成功产出的新 Artifact 版本按正常 dirty 规则级联下游。

**Fork**：从一个历史 Run Snapshot 或稳定事件位置创建新的 Run；原 Run 保持不变。

这些术语不得互换。UI 文案、事件名、历史查询和测试必须使用同一语义。

---

## 4. 总体架构

```text
┌──────────────────────────────────────────────────────────────┐
│ Desktop UI（Web frontend in native shell）                  │
│ Workflow / LLM Config / Preview / Run / Artifact / History  │
└───────────────────────────────┬──────────────────────────────┘
                                │ typed calls + events
┌───────────────────────────────▼──────────────────────────────┐
│ Application Module                                            │
│ WorkflowAuthoring / LLMConfig / RunControl / ArtifactQuery   │
└──────────────┬─────────────────────┬──────────────────────────┘
               │                     │
┌──────────────▼────────────┐  ┌────▼──────────────────────────┐
│ Product Repository        │  │ Workflow Runtime             │
│ SQLite drafts/revisions   │  │ execution/definition/history │
│ configs/events/snapshots  │  │ artifact/project/node        │
└──────────────┬────────────┘  └────┬──────────────────────────┘
               │                     │
┌──────────────▼─────────────────────▼──────────────────────────┐
│ Adapters                                                     │
│ Wails / CLI / OpenAI-compatible / Anthropic / FS Artifact    │
└──────────────────────────────────────────────────────────────┘
```

### 4.1 Application 模块

UI 不直接调用 Engine、SQLite、YAML Loader 或某个协议 Adapter。Application 模块提供产品动作，隐藏事务、校验、解析和运行时编排：

```go
type WorkflowApplication interface {
    CreateWorkflow(ctx context.Context, input CreateWorkflowInput) (WorkflowView, error)
    UpdateDraft(ctx context.Context, input UpdateDraftInput) (WorkflowView, error)
    ValidateDraft(ctx context.Context, workflowID string) (WorkflowPreview, error)
    CreateRevision(ctx context.Context, workflowID string) (WorkflowRevision, error)
    StartRun(ctx context.Context, revisionID string) (RunView, error)
}
```

接口名称为方向示例，不是本计划要求的最终 Go 签名。设计约束是：

- UI 和 CLI 通过同一个 Application seam 使用领域能力；
- Application 接收依赖，不在方法内部创建数据库、HTTP Client 或 Engine；
- 每个写操作在本地事务中完成；
- 返回产品 View/Result，不把数据库行或协议响应直接泄露给调用方；
- 长运行通过结构化事件向 UI 推送，不用请求阻塞承担全部状态通信。

### 4.2 桌面技术路线

默认候选为 Wails + Web 前端。Go 继续承载本地 Runtime，React/Vue/Svelte 等前端运行在系统 WebView。正式锁定框架前先完成一个 Prototype Gate：

1. macOS 与 Windows 均能构建启动；
2. 前端能调用 Application 方法；
3. Run Event 能实时推送；
4. 本地文件选择、系统路径和窗口生命周期正常；
5. 浏览器 Mock Adapter 与桌面 Adapter 能复用同一前端 Workflow Client interface。

若 Prototype Gate 不通过，只替换桌面 Adapter，不改领域模型和 Application 接口。

---

## 5. 本地持久化与版本模型

### 5.1 SQLite 是产品事实来源

14 之后 GUI 创建的 Workflow 不以 YAML 文件作为编辑主体。SQLite 保存：

- Workflow 身份；
- 当前 Draft；
- immutable Revision；
- Node Instance 与绑定；
- UI 展示元数据；
- LLM Config 与模型目录（不含 API Key 明文）；
- Run Snapshot、Run Event 和运行索引；
- Artifact 元数据和引用。

Artifact 本体与大型文件继续存于文件系统，SQLite 保存引用、哈希和来源。代码工作流的 Project Workspace 就是用户项目目录：Agent 修改实时落在该目录，Automation 使用同一工作状态。Gum 不复制项目、不创建内部代码 Revision，也不承担代码版本恢复；这些属于用户已有项目工具。

产品需要一个由平台管理的用户级 Local Data Root，而不是把跨项目的 LLM Config 和 Workflow Library 分散到每个项目的 `.workflow/`：

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
2. 自动保存只更新 Draft 和 `updated_at`。
3. 用户显式发布，或首次以当前 Draft 运行时，创建 immutable Revision。
4. Draft 内容与最新 Revision 完全一致时，运行可以复用该 Revision，不创建重复版本。
5. Revision 以规范化内容哈希判断等价；UI 布局偏好不影响运行语义哈希。
6. Run 启动后只读取 Run Snapshot；后续 Draft/Revision 变化不影响它。
7. Revision 可以比较和恢复为新 Draft，但不可原地修改。

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
7. 发布 Revision 或运行。

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
- Node/Executor/LLM Selection；
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
- 未选择 LLM Config 的 Agent Node 保留在图上并显示缺项；
- 点击 Diagnostic 直接打开对应 Node 和字段；
- `Preview + Diagnostics` 一起返回，不因第一个错误丢失整张图。

---

## 7. Node 描述能力

### 7.1 分层

Node Definition 继续作为业务契约的唯一声明处；14 后的新版本或显式升级增加四类描述：

1. **Contract**：inputs / outputs / optional / TypeExpr；
2. **Requirements**：llm、project、网络、输入输出模态等能力要求；
3. **Config Schema**：Node Instance 可配置字段及其校验；
4. **Presentation Hint**：显示名、分类、图标、编辑器类型、文档入口等可忽略的 UI 信息。

Config Schema 负责“值是否合法”，Presentation Hint 负责“如何展示”。忽略 Presentation Hint 不得改变运行语义。

### 7.2 建议的描述维度

| 维度 | 内容 |
|---|---|
| 身份 | name、display name、description、category、version |
| Contract | port 名、TypeExpr、optional、description |
| Capability | input/output modalities、conversation、streaming、tools、structured output |
| Config | 类型、必填、默认值、范围、枚举、敏感性 |
| Requirements | llm、project、network、command、secret |
| Side Effect | Workspace 写入、外部写操作、费用、幂等性 |
| Recovery | retry 特性、是否可 resume、unknown outcome 处置 |
| Observability | usage、latency、provider request id、finish reason、logs |
| Presentation | icon、category、field editor、Artifact preview preference |

### 7.3 能力匹配

Agent Node 启动前校验：

```text
Node Capability Requirements ⊆ Resolved LLM Model Capabilities
```

规则：

- required capability 为 supported：通过；
- required capability 为 unsupported：错误；
- required capability 为 unknown：默认要求用户确认或手工补充，不能静默当作 supported；
- 实际输入出现 image/audio/file 时再次按本轮内容校验；
- 不根据模型 ID 字符串猜测能力。

---

## 8. LLM Config 模块

### 8.1 定位

LLM Config 是独立、用户级、跨 Workflow 复用的配置聚合：

```text
LLM Config: “公司 OpenAI 网关”
├── protocol: openai-chat-completions
├── base URL: https://llm.example.com/v1
├── API key: secret://...
├── default model: model-a
└── models
    ├── model-a
    └── model-b
```

Agent Node Instance 只保存 LLM Selection：

```text
LLM Config = “公司 OpenAI 网关”
Model      = “model-b”        # 可省略，走 Config 默认
```

Node 不保存 Base URL 和 API Key，不要求用户在每个 Node 重复配置连接。

### 8.2 数据模型

```go
type LLMConfig struct {
    ID             string
    Name           string
    Description    string
    Protocol       Protocol
    BaseURL        string
    APIKeyRef      string
    DefaultModelID string
    Enabled        bool
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type LLMModel struct {
    ID                string
    LLMConfigID       string
    ModelID           string
    DisplayName       string
    Source            ModelSource // discovered | manual | merged
    Enabled           bool
    Capabilities      ModelCapabilities
    GenerationDefault GenerationConfig
    RawDiscovery      json.RawMessage
    LastSeenAt        *time.Time
}

type LLMSelection struct {
    LLMConfigID string
    ModelID     string // empty = config default
}
```

`APIKeyRef` 只保存 Secret Adapter 返回的引用。桌面 UI 中用户填写的 API Key 直接写入操作系统安全凭据存储；SQLite 只保存引用。CLI 和自动化测试可以使用环境变量引用 Adapter。若安全凭据存储不可用，平台必须提示用户改用环境变量引用，不能静默降级为 SQLite 明文。Revision、Run Snapshot、Run Event、日志和未来导出内容均不得保存 API Key 明文。

### 8.3 UI 流程

设置页：

```text
新建 LLM Config
→ 名称
→ 选择协议
→ Base URL
→ API Key
→ 测试连接
→ 发现模型
→ 启用/补充模型
→ 设置默认模型
```

Agent Node 配置页：

```text
LLM Config [公司 OpenAI 网关 ▼]
Model      [model-b ▼]
```

模型下拉只显示该 LLM Config 中已启用的模型，并展示 text/image、context、structured output 等已知能力；unknown 必须明确显示为未知。

### 8.4 LLM Config 解析与 Run Snapshot

Run 启动时解析：

1. Node Instance 指定的 LLM Config 存在且启用；
2. 指定 Model 存在且启用，或 Config 有默认模型；
3. Node Capability 与 Model Capability 兼容；
4. API Key 引用可以解析；
5. 生成参数按优先级合并；
6. 结果写入 Run Snapshot。

生成参数优先级：

```text
Node Instance 显式参数
    > LLM Model 默认参数
    > Protocol Adapter 默认参数
```

Run Snapshot 记录：Config ID/name、Protocol、Base URL、Model ID、能力快照和有效生成参数；不记录 API Key。

---

## 9. 协议 Adapter 与多轮请求

### 9.1 模块分层

```text
Canonical Conversation Model
            ↓
Canonical GenerateRequest
            ↓
Protocol Adapter
    ├── OpenAI Chat Completions
    └── Anthropic Messages
            ↓
HTTP Transport
```

建议的深接口：

```go
type ProtocolAdapter interface {
    DiscoverModels(ctx context.Context, conn Connection) (ModelPage, error)
    Generate(ctx context.Context, conn Connection, req GenerateRequest, sink StreamSink) (GenerateResult, error)
}
```

最终签名在实施设计中确定。接口必须让两个真实 Adapter 和 fake Adapter 共用，测试只跨该 seam 断言规范化行为。

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
- instructions：根据 Config/Model capability 映射为 developer 或 system message；为兼容第三方默认可配置 system；
- history：按原顺序发送 user / assistant messages；
- multimodal：将 Content Part 映射为协议 content parts；
- response：规范化 assistant content、usage、finish reason、request id 和 stream delta。

OpenAI 官方 Chat Completions 以有序 message 列表表达对话，并支持多种 content part。OpenAI-compatible 只承诺本计划明确采用的共同子集；厂商扩展通过 Adapter 内部 dialect 处理，不泄露到 Conversation Artifact。

### 9.4 Anthropic Messages

- endpoint：相对 Base URL 的 `messages`；
- auth：`x-api-key` + `anthropic-version`；
- instructions：放入顶层 `system`，不创建 system role message；
- history：只发送 user / assistant，按协议要求规范化连续同角色 turn；
- multimodal：映射为 Anthropic content blocks；
- response：规范化 content blocks、usage、stop reason、request id 和 stream delta。

Anthropic Messages 是无状态多轮接口，每轮必须由本地 Conversation Artifact 提供所需历史。

### 9.5 对话组装不变式

1. `history` 只包含本轮新 `input` 之前的消息；
2. Request Builder 产生 `history + input`；
3. 成功响应后输出 `conversation = history + input + output`；
4. 不在 Node Executor 内保存隐藏 session；
5. 发送给 Provider 的实际消息快照进入 Node Run 诊断，但密钥与敏感 Header 必须脱敏；
6. 上下文超限时不静默删除历史；初期返回明确错误，未来 Context Policy 另行设计；
7. Provider 不支持实际输入模态时在网络请求前失败；
8. stream delta 只用于 UI/日志，完整完成前不生成正式输出 Artifact。

---

## 10. 模型发现与手工模型

### 10.1 发现流程

```text
LLM Config Connection
    ↓ DiscoverModels
Protocol-specific /models
    ↓ raw response
Protocol Normalizer
    ↓ discovered catalog
Manual Overlay
    ↓ effective model catalog
```

### 10.2 标准化字段

```go
type ModelCapabilities struct {
    InputText        Capability
    InputImage       Capability
    InputAudio       Capability
    InputFile        Capability
    OutputText       Capability
    OutputAudio      Capability
    Streaming        Capability
    StructuredOutput Capability
    Tools            Capability
    Thinking         Capability
    ContextWindow    Optional[int]
    MaxOutputTokens  Optional[int]
}
```

`Capability` 为 supported / unsupported / unknown。每个标准化值保留来源：discovered、manual 或 default。

### 10.3 协议差异

- Anthropic `/v1/models` 可发现模型 ID、显示名、capabilities、max input tokens、max tokens 等已公开字段，并需处理分页。
- OpenAI 标准模型对象主要提供模型 ID 与基础元数据；不能假设所有 OpenAI-compatible `/models` 返回上下文窗口、模态或可用生成参数。
- 第三方 OpenAI-compatible 服务可能增加私有字段；Normalizer 识别已知字段，完整 `raw` 仍保存。
- 不允许按模型名称模式猜测 image/tools/context 等能力。

### 10.4 合并与刷新规则

有效字段优先级：

```text
用户手工显式值 > 服务端发现值 > unknown
```

规则：

1. `/models` 不可用不阻止用户创建手工 Model；
2. 手工 Model 标记为 manual/unverified；
3. Refresh 不删除手工 Model；
4. 已发现 Model 后续消失时标记 unavailable/last-seen，不立即删除；
5. 用户可禁用不希望出现在 Node 下拉框的模型；
6. Default Model 必须属于同一 LLM Config 且启用；
7. unknown capability 可由用户补充，但 UI 应提示这是手工声明；
8. 原始响应限制大小并脱敏后保存，不信任其中的描述文本作为指令。

### 10.5 Base URL

Base URL 表示协议 API root，例如 `https://example.com/v1`。Adapter 使用 URL parser 追加相对资源，不能用字符串拼接；需覆盖：

- 尾部有/无 `/`；
- Base URL 已含 `/v1`；
- 反向代理子路径；
- query/fragment 非法；
- 禁止因拼接产生 `/v1/v1/models`；
- localhost 与企业内网 endpoint。

---

## 11. 真实 `llm-chat` Agent Node

### 11.1 目的

`llm-chat` 是第一个简单但真实的 Agent Node，用来验证：

- LLM Config 复用与 UI 下拉选择；
- OpenAI-compatible / Anthropic 双协议；
- 单轮、多轮和多模态 Request Builder；
- Node Config Schema；
- Model Capability 校验；
- streaming Run Event；
- Chat Artifact 和 Artifact Preview；
- usage、latency、finish reason、错误分类和重试。

它不调用工具、不修改 Workspace、不承担 Coding Agent 职责。

### 11.2 Contract

```yaml
inputs:
  input:
    type: ChatMessage
  history:
    type: Conversation
    optional: true

outputs:
  output:
    type: ChatMessage
  conversation:
    type: Conversation
```

初期：

- input：text + image；
- output：text；
- history：可选；
- output role：assistant；
- conversation：完整新版本；
- usage 等运行数据记 Node Run metadata，不作为 Node 间默认数据通道。

### 11.3 Config Schema

```text
instructions       markdown，可选
temperature        float，可选
max_output_tokens  int，可选
```

LLM Config 和 Model 通过 Node Instance 的 LLM Selection 配置，不重复成为普通 config 字段。

### 11.4 运行属性

| 属性 | 值 |
|---|---|
| Node Type | agent |
| requires | llm、network |
| Workspace 写入 | 否 |
| 外部副作用 | 模型调用与费用；无业务写操作 |
| Retry | 可创建新 Node Run，但会产生额外调用/费用，输出不保证相同 |
| Resume | 已完成响应可复用；中断 HTTP/stream 不支持续传 |
| Streaming | 是；完成前只发临时事件 |
| 输出校验 | assistant ChatMessage + text content |

### 11.5 错误

- 配置缺失、认证失败、网络不可达、限流/服务不可用：Structural Error；
- 选择模型明确不支持实际输入模态：运行前配置错误；
- 响应协议无法解析：Structural Error；
- 后续结构化输出不符合 Contract：Interaction Error；
- 进程崩溃或取消导致结果未知：Node Run Interrupted/UnknownOutcome，由恢复策略或用户决定 Retry。

---

## 12. Artifact 体验

### 12.1 Artifact 是一等 UI 对象

每个 Artifact View 展示：

- Kind、MIME、版本、大小、哈希；
- 生产 Node、Node Run ID、round 和时间；
- 消费它的下游；
- 本体位置或外部 URI；
- 预览、下载/打开、版本比较；
- 是否由 Node、Human 或导入产生。

### 12.2 Preview Registry

前端/应用层按 Kind + MIME 选择 Previewer：

- Markdown/Text；
- Source Code 文件树与 diff；
- Image；
- JSON/OpenAPI；
- Test Report；
- Chat Message / Conversation；
- 外部资源（如 Figma URL/ID）的摘要和打开入口。

未知 Kind 使用通用元数据视图。预览器只读，不能改变 Artifact；人工替换必须创建 Manual Artifact 新版本。

### 12.3 安全

- 不执行 Artifact 中的脚本；
- HTML 默认以文本或隔离方式预览；
- 外部 URL 打开前显示目标；
- 大文件分段读取，不一次加载进 UI；
- Secret 和敏感 Header 不得作为 Artifact 保存。

---

## 13. Run Event 与运行控制

### 13.1 Event Envelope

```go
type RunEvent struct {
    EventID   string
    Sequence  uint64
    RunID     string
    NodeID    string
    NodeRunID string
    Type      EventType
    OccurredAt time.Time
    Payload   json.RawMessage
}
```

不变式：

- `Sequence` 在单个 Run 内严格递增；
- Event append 与当前状态快照在同一个 SQLite 事务；
- UI 可从最后确认 Sequence 重放并继续订阅；
- Event payload 有 schema version；
- Event 是事实，不用后续 update 修改；纠正通过新 Event 表达。

### 13.2 初始事件集

运行级：

- RunCreated / RunStarted / RunPaused / RunResumed / RunStopped / RunFailed；

节点级：

- NodeReady / NodeRunStarted / NodeWaitingHuman / NodeRunSucceeded / NodeRunFailed / NodeRunInterrupted；

数据与人工：

- ArtifactProduced / ManualArtifactProduced；
- HumanInputRequested / HumanInputSubmitted；
- ApprovalRequested / ApprovalDecided；
- RetryRequested / RerunRequested / ForkCreated；

LLM 诊断：

- LLMRequestStarted / LLMContentDelta / LLMRequestCompleted / LLMRequestFailed。

Content Delta 可设置保留策略；完整 Artifact 只在 Node Run 成功后产生。

### 13.3 Resume

- 目标是同一个 Run ID；
- 已成功 Node Run 与 Artifact 保持不变；
- WaitingHuman 恢复等待；
- 未派发 Node 继续调度；
- 崩溃时 Running Node 标为 Interrupted；
- Executor 的恢复能力决定自动 Retry、等待人工或 Fail；
- `llm-chat` 的未知请求默认等待用户确认 Retry，避免静默重复费用。

### 13.4 Retry

- 同一个 Run、同一个 Node Instance、新 Node Run ID 和 round；
- 输入 ArtifactRef 快照与目标失败轮完全相同；
- 只有成功输出后才更新下游；
- 人工 Retry 是人类事件，重置收敛保护；
- 记录发起人、原因和被重试 Node Run ID。

### 13.5 Rerun

- 用于主动重算，不要求上一轮 Failed；
- 默认解析当前最新输入并生成新 Node Run；
- 成功后新 Artifact 版本按 dirty 规则级联；
- UI 在执行前预览受影响下游；
- 若用户要求使用历史输入，应改用 Fork 或明确选择输入快照。

### 13.6 Fork

- 新 Run ID；
- 来源 Run/Sequence/Snapshot 可追溯；
- 可以复用不可变 Artifact，但不自动复制用户项目；Fork 使用哪个 Project Workspace 由用户显式选择，代码分支/恢复由用户的版本管理工具负责；
- 原 Run 不再变化；
- 适用于对历史结果做实验、从 Stopped Run 继续另一条路径。

### 13.7 Manual Artifact

- 创建新 Artifact ID/version，保留被替代引用；
- 标记 producer 为 human/manual；
- 记录用户说明；
- 消费者按新版本 dirty；
- Artifact Kind 必须符合目标 Input Contract。

---

## 14. 测试策略

### 14.1 LLM Config

- Config CRUD、默认模型、启用/禁用；
- Desktop Secret Adapter 与环境变量 Secret Adapter 行为一致；
- API Key 不落库/日志/事件；
- Config 删除时被 Draft/Revision 引用的行为；
- Run Snapshot 固定后修改 Config 不影响已启动 Run；
- Model discovery 与 manual overlay 优先级；
- unavailable/last-seen 和刷新幂等。

### 14.2 协议契约

全部使用 `httptest.Server` 和固定 fixture，单测禁止真实网络：

- OpenAI 单轮、多轮、system/developer、text/image、stream；
- Anthropic 顶层 system、user/assistant 历史、连续角色规范化、text/image、stream；
- usage、finish/stop reason、request ID；
- malformed response、认证、限流、取消；
- Base URL 所有边界；
- `/models` 分页、额外字段、缺失字段、raw 保留和大小限制。

请求体使用 golden fixture，确保“多轮对话能发起”之外，还能准确断言消息顺序和 content part 映射。

### 14.3 `llm-chat`

- 无 history 单轮；
- 带 history 多轮；
- conversation 输出顺序；
- text + image；
- capability unknown/unsupported；
- streaming delta 不提前产生 Artifact；
- 完整响应后两个输出同时产生；
- Structural / Interaction / Interrupted；
- Retry 同输入但产生新 Node Run；
- API Key 与敏感 Header 脱敏。

### 14.4 Workflow 产品模型

- Draft 自动保存；
- immutable Revision；
- 相同内容不重复 Revision；
- Run Snapshot 不受 Draft/Config 后续修改影响；
- Preview 在非法 Draft 下仍返回完整图和 Diagnostics；
- SCC/循环组、分支、汇合、Data/Control Edge；
- UI 坐标不影响语义哈希。

### 14.5 Event 与恢复

- 单 Run Sequence 严格递增；
- Event + Snapshot 事务一致；
- UI 从 Sequence 重放；
- crash 后 Running -> Interrupted；
- Resume/Retry/Rerun/Fork 语义互不混淆；
- Manual Artifact 不覆盖历史；
- Recorder/Event 写入故障的错误处理策略需在实施票中明确。

### 14.6 桌面 UI

- Browser Mock Adapter 做页面与交互测试；
- Wails/Desktop Adapter 做类型绑定和事件测试；
- macOS/Windows CI 分别构建；
- 关键流程 e2e：配置 LLM → 添加 llm-chat → 绑定输入 → Preview → Run → stream → Artifact → 第二轮对话。

---

## 15. 开发阶段与验收门

### P9：产品领域模型与 Application Module

交付：

- Workflow / Draft / Revision / Run Snapshot 模型；
- SQLite migration；
- Workflow authoring Application interface；
- Revision 内容哈希和快照；
- CLI/fake Adapter 验证，不做正式 UI。

验收：Draft 可编辑，Revision 不可变；Run 固定 Revision，后续修改不漂移。

### P10：LLM Config 与 Model Catalog

交付：

- LLM Config CRUD；
- Secret Store seam：桌面安全凭据 Adapter + 环境变量 Adapter；
- Config/Model/Selection/Capability 三态；
- OpenAI-compatible 与 Anthropic DiscoverModels；
- manual overlay、raw discovery、默认模型；
- Run Snapshot 解析；
- API Key 保密测试。

验收：一个用户配置可被多个 Agent Node 引用；模型发现失败时手工模型仍可用。

### P11：协议 Client 与真实 `llm-chat`

交付：

- Canonical Conversation/Request；
- 两种 Protocol Adapter；
- text/image input、text streaming output；
- ChatMessage/Conversation Artifact；
- usage、错误和中断语义；
- 完整协议 fixture 测试。

验收：同一 Node 通过不同 LLM Config 分别完成多轮对话，请求体与历史顺序正确。

### P12：Node 描述与 Workflow Preview

交付：

- Node Config Schema、Capability Requirement、运行属性、Presentation Hint；
- Preview ViewModel 和 Diagnostics；
- SCC/Iteration Group；
- 自动 layout Prototype；
- 非法 Draft 可预览。

验收：用户无需拖拽即可从声明得到稳定结构图，点击 Node 配置，Data/Control 顺序清晰。

### P13：桌面 GUI MVP

交付：

- macOS/Windows desktop shell Prototype Gate；
- Workflow 列表、声明、配置、Preview；
- LLM Config 设置与 Agent Node 下拉选择；
- Run Mode、Human interaction、history；
- 浏览器 Mock 与 Desktop Adapter。

验收：在两平台完成创建 Workflow 到运行 `llm-chat` 的闭环。

### P14：Artifact Experience

交付：

- Preview Registry；
- Chat/Markdown/Image/JSON/Source diff 初始 Previewer；
- lineage、round、版本比较；
- Manual Artifact；
- 大文件与安全限制。

验收：用户能从 Node Run 定位、查看和比较 Artifact，并用合法 Manual Artifact 触发下游。

### P15：Run Event 与恢复控制

交付：

- append-only Run Event；
- UI reconnect/replay；
- Pause/Resume；
- Retry/Rerun/Fork；
- Interrupted/UnknownOutcome；
- 受影响下游预览。

验收：应用重启后可恢复 WaitingHuman/静止 Run；中断 LLM 调用不会被伪装成成功或静默重复。

### P16：稳定性与跨平台发布准备

交付：

- schema migration fixtures；
- macOS/Windows 构建矩阵；
- 日志脱敏、Crash report bundle；
- 权限预览的最小模型；
- 性能基线与大图/大 Artifact 测试；
- 产品 v1 形态评审。

验收：核心闭环达到可重复安装、升级、诊断和回归验证；评审后才开始规划导入/导出、Workflow Pack 和 AI Workflow authoring。

阶段纪律：P(n) 未通过验收门，不开始依赖其领域语义的 P(n+1)；UI Prototype 可作为技术探索并行，但不得反向定义领域模型。

---

## 16. 产品 v1 评审清单

进入导入/导出规划前，至少回答：

1. Workflow / Draft / Revision 是否稳定，是否仍频繁迁移？
2. Node Config Schema 能否覆盖真实 `llm-chat` 和至少一个非 Agent Node？
3. LLM Config 能否稳定支持两个协议和第三方 OpenAI-compatible 差异？
4. Model Capability 的 unknown/manual/discovered 是否能被普通用户理解？
5. Preview 是否在分支、汇合、循环和非法 Draft 中保持清晰？
6. Artifact 是否能支撑查看、比较、人工替换和来源追踪？
7. Resume/Retry/Rerun/Fork 是否在 UI 和历史中没有语义歧义？
8. macOS 与 Windows 的安装、升级和本地文件权限是否可控？
9. Secret、Prompt、Artifact 和日志的导出边界是否明确？
10. SQLite schema 和 Revision 表达是否足以形成稳定的可移植格式？

只有这些问题稳定后，才设计 YAML 导出、导入冲突、Git 版本管理、Workflow Pack 和 AI 修改 Workflow。

---

## 17. 外部协议依据与演进原则

- OpenAI Chat Completions：有序 messages、developer/system/user/assistant role 与多种 content part。官方参考：<https://developers.openai.com/api/reference/cli/resources/chat/subresources/completions>。
- OpenAI Model 基础对象：ID、created、owner 等基础信息，不能据此假定完整 capability 元数据。官方参考：<https://developers.openai.com/api/reference/typescript/resources/models/methods/retrieve>。
- Anthropic Messages：顶层 system、无状态多轮 user/assistant messages、content blocks。官方参考：<https://platform.claude.com/docs/en/api/messages/create>。
- Anthropic Models：模型目录、capabilities、max input/output tokens 与分页。官方参考：<https://platform.claude.com/docs/en/api/models/list>。

协议会演进：实现时必须以版本化 Adapter 和 fixture 隔离变化，不能把某日的服务端私有 JSON 直接固化为 Gum-Workflows 的领域模型。

---

## 18. 决策摘要

1. 01–14 保持原计划；本文从其完成后开始。
2. GUI 是 Workflow 创作入口；画布只读、自动布局、可选择配置，不做拖拽式编排。
3. SQLite 是产品事实来源；YAML 导入/导出推迟至稳定 v1 前评审之后。
4. Workflow 使用 Draft + immutable Revision；Run 使用固定 Snapshot。
5. Artifact 是一等 UI 对象，按类型预览并支持版本比较。
6. 先打磨 Node，不规划内置 Workflow 库。
7. `llm-chat` 是第一个简单但真实的 Agent Node。
8. LLM Config 是独立用户级复用配置；Agent Node 只选择 Config 和 Model。
9. 支持 OpenAI-compatible Chat Completions 与 Anthropic Messages 的正确多轮组装。
10. 模型既可发现也可手工声明；能力三态、保留 raw、手工显式值优先，不按名称猜测。
11. 结构化 Run Event 支撑 UI、历史与恢复；Resume/Retry/Rerun/Fork 语义严格区分。
12. AI 修改 Workflow、Workflow Pack、内置工作流、云同步和高级 Trigger 全部后置。
