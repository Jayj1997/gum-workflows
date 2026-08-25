# gum-workflows

基于 Go 的轻量级 Workflow Runtime：以 YAML 声明 Workflow，Node 之间通过 Artifact 数据依赖自动形成 DAG 并持续执行。

## Language

### 定义侧（平台认识什么）

**Node Type（节点类型）**:
节点按执行主体划分的类别，共三种：agent（AI 执行）、automation（自动化脚本执行）、human（依赖人工审核或输入推进）。
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
被加工的本地代码仓库声明：名称 + 仓库地址。每次运行把仓库复制为该运行独享的 Workspace。

**LLM Provider（模型提供方）**:
一个大模型服务接入点（url + 协议类型 + apikey 引用 + 名称），下挂一组 Model。Provider 声明在用户级 llm.yaml 中，跨 Workflow 复用，是纯运行时配置：不落项目库，run 启动时把解析结果（provider/model 名）记入运行记录。

**LLM Model（模型）**:
Provider 下可选用的具体模型（如 gpt-4o），携带可选生成参数（temperature 等）。默认解析链：默认 Provider（显式 default，缺省取第一个）-> 默认 Model（同理）。

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
Data Edge 由 `inputs.<name>.from: <node-id>.<output>` 隐式产生，表达数据依赖；Control Edge 由 `dependsOn` 显式声明，只表达执行顺序。数据依赖永远不以 dependsOn 表达。

**Ready（就绪）**:
节点唯一的调度关注点：inputs 收集完毕且 Control 前驱已完成即可运行。运行前校验（含环提醒）只是提示，不是节点运行的先决条件。
