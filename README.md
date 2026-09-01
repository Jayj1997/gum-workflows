# gum-workflows

## 项目概述

Gum-Workflows 是一个基于 Go 的、本地优先的轻量级 Workflow Runtime，面向需要把开发、测试、设计、审核等重复工作环节连接起来的技术人员。

Workflow 通过 Node 的 Input / Output Contract 组合工作过程，Node 之间只传递 ArtifactRef。Runtime 根据 Data Edge 与 Control Edge 调度节点，允许有环迭代和人工介入，并记录每一次 Node Run 的输入、输出、错误与 Artifact 版本。

项目遵循“现实工作流优先”：Agent 直接修改用户项目，Automation 在同一份工作状态上执行检查；Gum 负责组合、调度、结果留存和诊断，不默认复制项目、创建内部代码 Revision 或接管代码恢复。

当前 YAML、CLI 与 Mock Agent 主要服务 Runtime 开发、验证和演示。macOS 产品壳已经可以通过通用 Application seam 在 SQLite 中创作 Product Workflow、管理用户级 LLM Provider / Model Slot，并把 API Key 保存到 macOS Keychain；deterministic fake executor 可完成一次持久化 Product Run 与 Conversation 结果查看，真实 LLM 请求仍按后续票交付。

## 项目规划

基础 Runtime、平台核心和首个 14 后产品模块已经完成。后续产品化按 [`Gum-Workflows 产品化阶段设计计划`](<plans/Gum-Workflows 产品化阶段：本地 GUI、Node 能力与 LLM Config 设计计划.md>) 推进，主要方向包括：

- 使用已完成的 Keychain 凭据边界实现真实 OpenAI-compatible LLM Client 与 `llm-chat` Agent Node；
- 在已完成的只读 Revision/Run 分层历史浏览上补齐 Interrupted 标记、Resume/Rerun，并在后续支持 Windows；
- 完善 Artifact 预览、来源追踪、多版本比较和人工替换；
- 设计结构化 Run Event，以及 Resume、Retry、Rerun、Fork 和崩溃恢复；
- 在领域模型稳定后，再规划 Workflow 导入导出、Pack、AI 修改 Workflow 和云同步。

Code Quality Check 的后续增强保留为独立新模块：Changed Scope、项目语言与子项目检测、条件执行与 Skipped 传播、Container Execution Environment、Windows / WSL Script Runtime，以及用户自定义 Automation Script。当前不承诺 Fuzz Node。

任何新模块都需要先形成设计文档和开发票，再修改实现。模块完成后的 README 与进度文档同步方法见 [`README 更新规范`](<plans/README 更新规范：模块完成后的进度同步.md>)。

## 项目当前进展

### macOS Keychain Secret Adapter — 已完成

该切片让 macOS 用户在通用 Provider 设置界面中直接保存 API Key，同时把明文凭据限制在 Product Application 与注入的 Secret Adapter 边界内；SQLite、普通 ViewModel、Diagnostics、日志与 Artifact 都不保存或返回明文 Key。

主要交付：

- Desktop 通过注入的 macOS Keychain Adapter 保存、读取和删除 Provider 凭据；SQLite 只保存稳定、不可推出明文的 `keychain://` 引用；
- Provider 创建和 Key 轮换接受密码输入，普通 ViewModel 只返回 `hasApiKey`；留空更新保留原 Key，删除前由 UI 确认并同步删除凭据；
- Keychain 不可用时 Application 明确失败，不降级为 SQLite 明文；持久化失败路径会清理或恢复外部凭据；
- Browser Mock 与 Go 测试注入进程内 Memory Adapter，不访问真实用户 Keychain；macOS Adapter 通过 Security.framework 工作，并在注入的原生边界上验证错误文本不泄漏 Secret。

详细范围见 [product-workflow spec](.scratch/product-workflow/spec.md) 和 [issue 09](.scratch/product-workflow/issues/09-macos-keychain-secret-adapter.md)。

### Product Workflow Revision reuse 与 Run history UI — 已完成

该切片让用户区分“定义版本”与“执行次数”：相同语义内容重复运行复用同一 immutable Revision，但每次创建新 Run，并能在 UI 中按 Workflow → Revision → Run → Node Run/Artifact 逐层浏览历史，应用重启后历史仍可查询。

主要交付：

- 独立只读 `RunHistoryRepository` 读 seam 从 `product_workflow_revision / run / node_run / artifact` 表查询，不写也不改 Workflow Run 状态；Product Application 暴露 `ListRevisions` / `ListRevisionRuns` / `GetRunHistory` 用例；
- 相同规范化语义哈希重复 StartRun 复用同一 Revision；执行语义或首次物化 Model UUID 变化产生新 Revision，而展示文案、Presentation Hint 与 UI view preference 不进入哈希；
- Run 详情复用 Run View 形态，Conversation 消息从同一 Run 的 filesystem Artifact Store 还原；Desktop 与 Browser Mock 通过同一 WorkflowClient 提供分层历史浏览；
- 历史、Run Snapshot 与 Conversation Artifact 落在 SQLite 与 Local Data Root，重启后已完成 Run 仍可查询；
- 分层历史为只读浏览，当前不包含 Interrupted 标记、Resume 或真实 LLM。

### Product Workflow fake StartRun 与 Conversation Artifact — 已完成

该切片完成 P9 Product Tracer：用户从真实 macOS UI 运行当前可见 Draft，经同一 Product Application 与 WorkflowClient 形成 immutable Revision、独立 Run、Node Run 和可查看的 Conversation Artifact。

主要交付：

- Run 动作先 flush autosave，再携带最新 expected `lock_version`；旧 token、非法 Draft 或缺少默认 Provider/Model 时不写入 Draft、Revision 或 Run；
- 空 LLM Preference 在 StartRun preflight 中按双层 default 物化 Gum Model UUID，随后按规范化执行语义创建或复用 Revision；相同语义重复运行复用 Revision，但每次生成新 Run；
- Run Snapshot 固定 Revision 与 Resolved LLM Selection，不保存 API Key；启动后不再回写 Draft 或 Revision；
- deterministic fake `human-chat(source) -> llm-chat` 产生两次成功 Node Run 和 filesystem-backed Conversation Artifact，Desktop 与 Browser Mock 共用 Run/结果 UI；
- SQLite 写链与 Artifact 发布失败会回滚或清理，不留下用户可见半状态。当前仍不包含真实 LLM、人工输入、Interrupted 或 Resume（分层历史浏览已由后续模块交付）。

详细范围见 [product-workflow spec](.scratch/product-workflow/spec.md) 和 [issue 07](.scratch/product-workflow/issues/07-fake-start-run-revision-artifact.md)。

### Product Workflow LLM Provider / Model 设置 — 已完成

该切片让用户可以在 Desktop 与 Browser Mock 共用的产品设置界面中管理 `Provider -> Models`：连接与模型槽使用稳定 Gum UUID，编辑不会破坏未来 Workflow 引用，默认解析也不依赖远端模型发现。

主要交付：

- 用户可以创建、编辑和删除多个 Provider，并在每个 Provider 下手工管理多个 Model Slot；SQLite 层只持久化通过 URI 校验的 API Key Secret 引用；
- Provider 与 Model Slot 的 UUID 在名称、Base URL、Provider Model ID 或生成默认值编辑后保持不变，Model UUID 明确表示可变配置槽；
- 每层最多一个显式 default；没有显式 default 或删除 default 后，按 `(created_at ASC, UUID ASC)` 选择有效 default；
- 没有 Provider 或有效 Provider 没有 Model 时，Application resolver 返回可定位、可理解的设置 Diagnostic；
- Browser Mock 与 Desktop Adapter 继续共享 WorkflowClient、产品壳和通用 DOM 设置表单；当前没有 `/models`、Capability、position、enable/disable 或自动 failover。

详细范围见 [product-workflow spec](.scratch/product-workflow/spec.md) 和 [issue 06](.scratch/product-workflow/issues/06-llm-provider-model-settings.md)。

### Product Workflow Input Binding 与只读 Preview — 已完成

该切片让用户可以在通用 Node 编辑器中把上游 Output 绑定到下游 Input，并在非法 Draft 仍可保存的前提下，通过只读结构 Preview 理解实际 Data Edge、Control Dependency 与循环组。

主要交付：

- `human-chat` 与 `llm-chat` Catalog 合同公开 `Conversation` 输入输出端口，UI 通过端口选择器创建 `human-chat.conversation → llm-chat.conversation` Data Edge；
- Control Dependency 使用独立复选控件与独立 Preview 样式，不承担 Artifact 数据传递；
- 缺少 required Input、未知输入/输出端口、未知来源和 Artifact 类型不兼容返回具体 Node/字段 Diagnostic，且不会遮蔽其余可识别 Node 与 Edge；
- renderer-independent Preview ViewModel 只包含 Node、Data/Control Edge、循环组与 Diagnostics；自动网格、缩放、折叠和最近选择只属于前端视图状态；
- Browser Mock 与 Desktop 继续共享 WorkflowClient 和通用 DOM view；前端合同与 Go Application seam 均覆盖绑定、循环和非法 Draft 行为。

详细范围见 [product-workflow spec](.scratch/product-workflow/spec.md) 和 [issue 05](.scratch/product-workflow/issues/05-input-binding-workflow-preview.md)。

### Product Workflow Node Catalog 与 Config 表单 — 已完成

该切片把 macOS 产品壳从原始 Draft JSON 编辑推进到通用 Node 创作：用户可以从注册表驱动的 Catalog 添加 `human-chat` 与 `llm-chat`，选择、重命名、配置或移除 Node Instance，而不是使用硬编码聊天页面。

主要交付：

- 14 后产品专用 Definition/Executor Registry 显式注册首批 Catalog，Node Instance UUID、Definition identity 与显示名称相互分离；
- Gum Config Schema 支持 string、markdown、integer、number、boolean、enum，以及 required/default、数值范围、枚举值和 sensitive 合同；
- Presentation Hint 只控制 label、help 与 editor，Desktop 通用表单和 Go Validator 消费同一 Schema，提示变化不改变验证语义；
- `llm-chat` 的 instructions、temperature 与 max output tokens 由 Schema 生成表单；非法值仍随 Draft 保存，并以具体 Node/字段路径返回聚合 Diagnostic；
- Browser Mock 与 Desktop Adapter 共享 Catalog/WorkflowClient 与 Draft CAS autosave 路径；端口绑定、Data/Control Edge 与只读 Preview 已由下一切片交付。

详细范围见 [product-workflow spec](.scratch/product-workflow/spec.md) 和 [issue 04](.scratch/product-workflow/issues/04-node-catalog-config-schema-form.md)。

### Product Workflow Draft autosave — 已完成

该切片让 Product Workflow 获得唯一可变 Draft，并把真实 Desktop 与 Browser Mock 的通用编辑路径推进到可靠 autosave：等价内容不会制造写入，旧页面也不能覆盖新内容。

主要交付：

- 每个 Product Workflow 在 SQLite 中恰有一行 Draft；新建与升级都保证 Draft 存在，不创建编辑历史副本；
- 语义 JSON 规范化后再比较，no-op 保留时间与 token，变化以 expected `lock_version` 做 CAS 并更新同一行；
- CAS 冲突返回数据库最新 Draft 和刷新提示，不覆盖较新内容；
- 非法中间态仍可保存，并返回 renderer-independent Preview 集合与聚合 Diagnostics；具体 Node/config、端口与图结构已由后续切片补齐；
- Desktop Adapter 与 Browser Mock 共享 `getDraft` / `updateDraft` WorkflowClient，UI 串行执行去抖 autosave，并展示保存状态、冲突和 Diagnostics；内部 token 不进入产品语言。
- Darwin 上的完整 build、test、vet、race 与前端合同测试均通过；Browser Mock 实际交互验证了创建、选择、非法 Draft autosave 与聚合 Diagnostics。

详细范围见 [product-workflow spec](.scratch/product-workflow/spec.md) 和 [issue 03](.scratch/product-workflow/issues/03-draft-autosave-lock-version.md)。

### Product Workflow SQLite 创建与列表 — 已完成

该切片把首个 macOS 产品壳从 deterministic tracer 推进到真实本地持久化：用户可以在 UI 创建带稳定 UUID 和显示名称的 Product Workflow，并在应用重启后看到相同列表。

主要交付：

- Product Workflow 使用 Local Data Root 的 `product.db`，按创建时间与 UUID 稳定展示；
- 独立 `product_workflow` schema 与 workflow/v1 定义、YAML CLI history 安全共存，不导入或复用 YAML Workflow identity；
- Browser Mock 与 Desktop Adapter 共享同一 WorkflowClient 创建/列表合同，UI 不直接访问 SQLite；
- schema migration 可重复打开，并验证升级后旧定义与 Run history 保持可读。

该切片只完成 Workflow identity、创建和列表；Draft、Catalog、Preview 以及 fake Revision/Run 已由后续模块交付，真实 LLM 仍属后续票。详细范围见 [product-workflow spec](.scratch/product-workflow/spec.md) 和 [issue 02](.scratch/product-workflow/issues/02-sqlite-workflow-list-create.md)。

### code-quality-automation — 已完成

该模块把 Gum-Workflows 从 Mock automation 推进到可运行真实 Go 工具链的代码质量检查平台，同时完成代码工作流所需的本地数据与 Workspace 基础设施。

主要交付：

- 产品数据库、Artifact、日志、Bundle 和 tool-output 统一进入用户级 Local Data Root；新 Run 不再向项目内 `.workflow` 双写，旧数据可显式、幂等迁移；
- Project Definition 指向的目录成为 In-place Project Workspace，Agent 修改与 Automation 检查使用同一份源码状态；
- `project.code` 与普通 Node output 都可提供类型化 `SourceCode` ArtifactRef，历史保留触发链但不保存或恢复源码快照；
- ScriptNode 支持不可变 `automationScript/v1` Bundle、Manifest 与摘要校验、流式日志、正式工具产物、Result Adapter、进程组取消和 32 MiB 日志上限；
- 四个独立的 Go full-scope 检查已经落地：`go-static-analysis` 运行 `go vet`，`go-coverage-check` 计算 statement coverage，`go-race-check` 报告本次是否观察到 race，`go-complexity-check` 使用内嵌 Go AST Analyzer 计算单函数圈复杂度；
- 四个检查统一产出严格的 `qualityCheckResult/v1`。质量问题或阈值不达标是成功 Node Run 的 `failed` verdict；工具、协议或产物不完整才是 Structural Error；
- `examples/dogfood` 验证四个检查可在同一 Workspace 并发运行、各自留存 Result，且 Gum 产物只进入 Local Data Root；Darwin 与 Linux 的完整 test、vet、race 和脚本合同均已通过。

详细设计与实施记录见 [code-quality-automation spec](.scratch/code-quality-automation/spec.md) 和 [issues](.scratch/code-quality-automation/issues/)。

### platform-core — 已完成

该模块把早期 DAG Runner 重构为具有明确领域合同、可迭代执行、人工在环和可查询历史的平台核心。

主要交付：

- Node Type、Node Definition、Node Executor、Node Instance 四层定义体系，以及 TypeExpr 端口类型与 Registry；
- 用户级 `llm.yaml` 严格加载和 provider / model 默认解析链；该模块只解析配置，尚不包含真实 LLM 网络 Client；
- 允许有环的版本驱动 Execution Engine、节点单并发、dirty 合并与 convergence guard；
- `human-input`、`human-approval`、advise retry，以及 Structural Error / Interaction Error 二分；
- SQLite Node Run 历史、Artifact 多版本留存，以及 `workflow history` 的运行、节点和轮次三级查询；
- `examples/fullstack` 人工在环 Demo。Workflow 静止后仍保持 Running，直到用户 Ctrl-C / SIGTERM 后记为 Stopped。

详细设计与实施记录见 [平台核心设计](<plans/平台核心设计：组件定义体系与迭代执行引擎.md>)、[platform-core spec](.scratch/platform-core/spec.md) 和 [issues](.scratch/platform-core/issues/)。

### workflow/v1 MVP — 已完成

该阶段建立了项目最初的可运行骨架：workflow/v1 YAML、CUE 与语义校验、Data / Control Graph、Artifact Store、基础 Node 与 CLI，以及最早的 build、test 和 e2e 体系。其设计文档作为历史记录保留，不再直接承载当前产品状态。

### 当前能力边界

- `coding-agent`、需求分析、架构设计与 OpenAPI Generator 仍为 Mock；真实 Agent 与真实 OpenAPI Generator 尚未实现；
- 四个内置 Code Quality Check 当前只支持 Darwin / Linux，Windows 原生、PowerShell 与 WSL 后端尚未实现；
- Host Execution Environment 继承用户的 PATH、Go 配置、缓存、工具链与网络策略，适合受信任项目，但不是安全沙箱，也不提供容器、CPU / 内存隔离或自动 timeout；
- Static 只代表 `go vet`，Coverage 只报告本次 full-scope 测试的 statement coverage，Race 只报告本次是否观察到 race；
- macOS GUI 当前支持 Product Workflow 创建、Draft autosave、通用 Node/端口创作、只读 Preview、SQLite Provider / Model Slot 设置与 Keychain API Key 保存，deterministic fake StartRun、Revision、Run/Node Run 与本次 Conversation Artifact 查看，以及只读 Revision/Run 分层历史浏览（重启后可查询）；真实 LLM、人工输入、Interrupted 标记与运行恢复仍属于后续规划。

## 使用与文档

开发环境要求 Go 1.25+。常用命令：

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...

go run ./cmd/workflow validate <workflow-file>
go run ./cmd/workflow run <workflow-file>
go run ./cmd/workflow history [<run-id> [<node-id>]]
```

含 agent 节点的示例需要用户级 `llm.yaml`，但当前 Mock Agent 不会发起网络请求。可从 [`examples/fullstack/llm.example.yaml`](examples/fullstack/llm.example.yaml) 创建本地演示配置。人工在环示例位于 [`examples/fullstack`](examples/fullstack/)，真实质量检查示例位于 [`examples/dogfood`](examples/dogfood/)。

文档入口：

- [`CONTEXT.md`](CONTEXT.md)：领域词汇权威；
- [`docs/domain-model.md`](docs/domain-model.md)：当前已实现领域模型；
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)：开发、测试与文档规范；
- [`AGENTS.md`](AGENTS.md)：Agent 工作边界与项目约束；
- [`plans/`](plans/)：历史设计、产品化计划和 README 更新规范。
