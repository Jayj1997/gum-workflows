# gum-workflows

基于 Go 的轻量级 Workflow Runtime：以 YAML 声明 Workflow，Node 之间通过 Artifact 数据依赖自动形成 DAG 并持续执行。

## Language

### 定义侧（平台认识什么）

**Node Type（节点类型）**:
节点按执行主体划分的类别，共三种：agent（需要模型推理）、automation（依据结构化配置与可观察项目状态，按预定规则运行工具并解析结果）、human（依赖人工审核或输入推进）。automation 不调用 LLM，不编写测试，也不把源码变更写回用户项目；被调用工具的结果可以具有随机性。
_Avoid_: 用 "node type" 指代具体节点（旧代码用法）--具体节点是 Node Definition。

**Node Definition（节点定义）**:
平台认识的一个节点本体（如 requirement-analysis），以 name 标识，声明其类别（Node Type）、inputs/outputs 契约与 requires。
_Avoid_: node type（旧用法）、与 Node Instance 混称。

**Node Executor（节点执行器）**:
某 Node Definition 的一个可执行版本（v1、v2……）。同一定义的多个版本并存；进行中的运行使用其启动时确定的版本。
_Avoid_: 与 Node Definition 混称。

**requires（定义层需求）**:
Node Definition 声明的能力/资源需求（如 llm、project），表达「我需要什么才能工作」。
_Avoid_: dependsOn（实例层的执行顺序声明）。

**Node Instance（节点实例）**:
Workflow 中对一个 Node Definition 的一次使用：node id（Workflow 内唯一）+ executor 版本与模型选择 + inputs 绑定 + dependsOn。以 Node Definition 的 name 引用之。
_Avoid_: 与 NodeExecution（运行侧快照）混淆。

**Workflow（工作流）**:
Node Instance 的组合声明，附项目声明。一个 Workflow 可运行任意多次，运行之间零共享。

**Project Definition（项目定义）**:
被加工的本地代码仓库声明：名称 + 仓库地址。仓库的规范化绝对路径直接成为 In-place Project Workspace，不为 Run 或 Node 复制项目。

**LLM Provider（模型提供方）**:
workflow/v1 中的一个大模型服务接入点（url + 协议类型 + apikey 引用 + 名称），下挂一组 Model。Provider 声明在用户级 llm.yaml 中，跨 Workflow 复用，是纯运行时配置：不落项目库，run 启动时把解析结果（provider/model 名）记入运行记录。14 后 SQLite 产品继续使用 Provider / Model 领域关系，但不读取或兼容 llm.yaml。

**LLM Model（模型）**:
Provider 下一个用户配置的模型槽，拥有全局稳定的 Gum Model UUID，并携带可编辑的 Provider Model ID 和生成默认值。UUID 标识配置槽而非不可变底层模型；修改 Provider Model ID 会影响未来 Run，历史 Run Snapshot 保留旧值。默认解析链：默认 Provider（显式 default，否则取未删除项按 created_at、UUID 升序的第一个）-> 默认 Model（同理）。Gum 不维护或匹配模型能力。

### 14 后产品侧（已确认、部分已实现）

**LLM Preference（模型偏好）**:
Agent Node Instance 记录的 Gum Model UUID。未选择时，StartRun preflight 按默认 Provider/Model 解析并先写回 Draft；只要 UUID 对应的 Model 仍存在，默认值变化都不改变选择。UUID 被删除时不回退，必须由用户重新选择。

**Resolved LLM Selection（已解析模型选择）**:
StartRun 根据已写入 Draft/Revision 的 Gum Model UUID，为 Agent Node 固定协议、Provider、Provider Model ID、能力和有效生成参数。它进入 Run Snapshot，不包含 API Key 明文；历史 Run 即使原 Model 后来删除仍可显示当时的解析结果。
_Avoid_: 把 Provider 的可变连接内容固化进 Workflow Revision、运行中自动切换 Provider。

**Workflow Draft（工作流草稿）**:
一个 Workflow 唯一可变的当前内容。Autosave 只在规范化语义内容变化时更新同一 Draft，不创建 Revision 或历史副本；内部 lock version 只用于并发控制。首个产品闭环没有独立 Publish 动作。

**Workflow Revision（工作流修订版）**:
用户点击 Run 时由 Draft 的规范化语义内容创建或复用的不可变版本。相同语义内容重复运行复用同一 Revision；每次执行仍创建新的 Run。
_Avoid_: 在 Run 启动后原地修改 Revision。

**Run Snapshot（运行快照）**:
Run 启动时固定的 Workflow Revision、Node Executor、Resolved LLM Selection 及有效配置；不包含 API Key 明文。

**Workflow Preview（工作流预览）**:
由 Draft 或 Revision 派生的只读结构视图，展示 Node、Data/Control Edge、循环组与诊断。自动布局坐标只服务显示，不属于 Workflow 执行语义。

**Human Chat Entry Node（人工对话入口节点）**:
14 后对话 Workflow 中唯一可在没有必需输入时自举的人工门。它把一次人工提交追加为 Conversation 中的 user message；收到 `llm-chat` 反馈的 Conversation 后只等待下一次人工事件，不会自行产出新一轮。
_Avoid_: 把它当作收到反馈便自动执行的普通 Node、让 `llm-chat` 自循环生成对话。

**Paused Run（已暂停运行）**:
仍可用同一 Run ID 继续的 Run；暂停只阻止派发新的 Node Run，不承诺冻结已经开始的外部调用。

**Interrupted Run（已中断运行）**:
因进程退出或崩溃而未正常完成、但仍可用同一 Run ID 恢复的 Run。中断时结果不确定的 Node Run 不得自动重放。

**Unknown Outcome（结果未知）**:
Node Run 已发起外部副作用但平台无法确认是否完成的结果状态，不是 Workflow Run 的独立终态。是否再次执行必须由用户明确决定。

**Local Data Root（本地数据根目录）**:
平台管理的用户级可配置存储根目录，容纳产品数据、Artifact、Node Run 日志与 tool-output。它位于用户项目之外，Workflow 不得在项目内创建 `.workflow` 等 Gum 产物。
_Avoid_: 每个 Project 各自成为产品数据事实来源。

**In-place Project Workspace（原地项目工作区）**:
14 后代码工作流中，Project Definition 指向的用户项目目录直接作为 Project Workspace；Agent 修改实时落在该目录，Automation 读取同一状态。Gum 不为 Run 或 Node 复制项目，代码版本与恢复由用户的工具负责。
_Avoid_: 把 Local Data Root 当作源码副本或内部代码 Revision 存储。

**Code Artifact（代码引用）**:
`code` 端口上的 SourceCode Artifact，指向本次 Run 共享的 Project Workspace；新 Artifact 版本表示某次成功开发 Node Run 已完成，用于触发后续检查。它不携带或持久化源码本体，Run 历史也不承诺从该引用恢复当时代码。
_Avoid_: 把 `code` 理解为修改前源码、独立快照或 Runtime 管理的代码 Revision。

**Host Execution Environment（本机执行环境）**:
在用户本机上使用 PATH 已安装工具运行受信任项目的执行后端。它记录实际工具与环境，但不提供安全隔离承诺。
_Avoid_: 把本机进程执行称为 sandbox。

**Code Quality Check（代码质量检查）**:
对 Project Workspace 按预定规则运行的 automation 验证，包括动态测试与静态度量；检查结果不代表 Node Executor 执行错误。
_Avoid_: 用“自动化测试”统称静态分析、覆盖率和圈复杂度。

**Quality Check Result（质量检查结果）**:
Code Quality Check 唯一的结构化业务输出 Artifact，同时供下游 Node 消费与 UI 渲染；以 passed、failed 或 not-applicable 表达已完整执行的检查结果。检查未完成或输出损坏时不产生结果，而是 Node Executor 的 Structural Error。
_Avoid_: 把发现缺陷与工具无法运行混为同一种失败。

**Automation Script Bundle（自动化脚本包）**:
某个 automation Node Executor Version 的不可变执行资产，包含 Manifest、单一 POSIX `check.sh` 与必要的内置辅助源码；内容摘要进入 Run Snapshot。首版仅支持 Darwin/Linux，不接受 Node Instance 覆盖脚本或底层命令。

**Result Adapter（结果适配器）**:
ScriptNode 内部的 Gum 内置解释器，从退出码、stdout/stderr 日志与正式工具产物生成并校验 Quality Check Result。Shell stdout 永远只是日志，不是 Result Channel。

### 运行侧（每次 run 独立）

**WorkflowExecution / NodeExecution**:
一次 `workflow run` 的运行快照，及其中每个节点的当前运行快照（状态、产出、错误）。运行不回写定义。有环工作流中一次 WorkflowExecution 内同一节点可有多次 Node Run。
_Avoid_: 用 Workflow / Node Instance 指代运行中的对象。

**Node Run（节点运行）**:
一个节点在一次 Workflow 运行内的一次执行。有环工作流中同一节点可运行多次；node 的每次运行都持有独立的运行标识（node run id）。下游 `inputs.from` 总是取生产者最新已完成轮的输出；某一轮的具体输出经 node run id / round 寻址。
_Avoid_: 与 WorkflowExecution / run（整个工作流运行）混淆。

**Artifact / ArtifactRef**:
节点之间唯一的数据通道。运行时只传递引用（ArtifactRef），数据本体只存在于 Artifact Store。同一节点同一输出的多次产出以版本号共存，均不删除。
_Avoid_: 节点间直接传递数据本体（如源码内容）。

**Human Approval（人工审批）**:
type=human 的节点，无 inputs、必须声明 dependsOn 挂接被审节点，产出 approve（bool）与 advise（markdown）两个输出。agent 节点以节点级 `advise` 输入（optional 声明）消费人类意见。
_Avoid_: 把审批拒绝当作失败。

**Advise（人类意见）**:
审批者随决策给出的处理意见。agent 节点在契约中以 optional `advise: markdown` 输入显式声明后，方可接收。

**Structural Error（结构性错误）**:
节点运行的底层错误（自动化脚本无法执行、网络不可达、stdin 关闭等），人工在运行内无法修复。任一节点发生结构性错误，运行即 Failed、进程退出。automation/human 节点的错误默认结构性；执行器可显式把错误标记为结构性（如 agent 的网络错误）。
_Avoid_: 与交互性错误混称。

**Interaction Error（交互性错误）**:
agent 节点与 AI 交互的质量问题（如预期 JSON 却返回非 JSON），可通过人类 advise 重跑修复。节点置 Failed 但运行保持 Running，等待 advise 重试。

**Advise Retry（意见重试）**:
agent 节点交互性错误的恢复路径：人类给出 advise，引擎经节点声明的 advise 输入注入并重跑该节点（新 Node Run）。advise 属人类事件，重置收敛保护计数；入口节点的新一轮需求级联亦可复活失败节点。

**Iteration（迭代）**:
工作流允许有环，节点在新输入版本出现（新产出/新需求/新 advise）时重新 Ready。节点只关心输入收集完毕即可运行，无环性不是节点运行的约束。节点单并发：同一节点同时至多一个 Node Run 在跑，新输入先排队。

**Convergence Guard（收敛保护）**:
自上一次人类事件（human-input 产出一轮 / 审批做出一次决策）以来，节点连续重跑超过阈值（默认 10）仍无新人类事件则该节点转 Failed，防止机器自驱的配置错误小环死循环。人工驾驶的迭代（每轮有新需求/新决策）永不触发。静态可判的纯配置环在校验阶段提示。

**Workflow 结束**:
无显式终态条件。一次运行是持续收敛的进程：需求可能不断追加、审批可能反复拒绝；由用户手动结束（Ctrl-C / SIGTERM），运行状态记 Stopped，到达的最后一个稳定状态即结果。即便全图静止，运行也保持 Running 等待下一轮用户输入（如 Claude Code 空闲时不会自动退出）。唯一自动终态是结构性错误（Structural Error）导致运行 Failed。
_Avoid_: 把「全图静止」当作自动 Succeeded 的结束条件。

**Entry Node（入口节点）**:
全 Workflow 恰好一个源节点（无 inputs、无 dependsOn），且必须是 human-input：work（需求）必须来自人。用户主动触发开启工作流；产出一轮后可继续等待下一轮输入，直至用户 Finish。

**Data Edge / Control Edge**:
Data Edge 由 `inputs.<name>.from: <node-id>.<output>` 隐式产生，表达 Node 间数据依赖；Control Edge 由 `dependsOn` 显式声明，只表达执行顺序。数据依赖永远不以 dependsOn 表达。Workflow Context Binding 虽复用 `inputs.from` 语法，但不产生 Node Edge。

**Workflow Context Binding（工作流上下文绑定）**:
Runtime 向 Node input 提供内建、类型化 ArtifactRef 的绑定。当前唯一绑定 `project.code: SourceCode` 指向本次 Run 的 In-place Project Workspace；它没有 Artifact Store 本体，不复制或内联源码，也不是字符串模板或 OS 环境变量。`project` 是保留的 Context 名，不得作为 Node ID。
_Avoid_: 把 `project.code` 当作名为 project 的 Node output、普通字符串路径或源码快照。

**Ready（就绪）**:
节点唯一的调度关注点：inputs 收集完毕且 Control 前驱已完成即可运行。运行前校验（含环提醒）只是提示，不是节点运行的先决条件。
