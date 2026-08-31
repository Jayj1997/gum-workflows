# Spec: SQLite Product Workflow 与真实 LLM macOS 闭环

Status: ready-for-agent

设计权威为产品化设计计划、根领域词汇表以及已接受的 Model Slot ADR。本 spec 将已确认的 P9–P12 产品范围转为可实施规格；不得把这些语义倒灌到 platform-core workflow/v1。

## Problem Statement

用户目前只能通过 YAML 与 CLI 使用 Gum-Workflows。这个入口已经能验证迭代 Engine、Artifact、Human Gateway 和历史记录，但它不是产品化创作体验：用户无法在本地 GUI 中创建和配置 Workflow，无法自动保存一个允许非法中间态的 Draft，无法在运行时固定 immutable Revision，也无法通过真实模型完成最小对话 Workflow。

现有 LLM 配置属于 workflow/v1 的 llm.yaml 运行配置，并且真实 Agent 仍未落地。若直接把 Provider URL、API Key 或 Provider Model ID 写进 Node，Workflow 会和当前机器的连接细节强绑定；用户编辑 Provider 后，历史定义可能失效。相反，如果每次运行都动态追随 default，用户修改 default 又会悄悄改变既有 Node 的模型选择。产品需要一个简单、可追溯、不过度生产化的中间模型。

当前产品计划还混合了 GUI、双协议、多模态、Streaming、Windows、恢复和高级 Artifact 体验。若横向建设完整领域层、协议层后才接 UI，将很晚才验证真实用户闭环。首阶段需要从 macOS UI 到 SQLite、Application、Engine、真实 OpenAI-compatible 请求和 Conversation Artifact 的纵向 tracer，并明确哪些能力进入后续待办。

## Solution

构建一个只以 SQLite 为事实来源的本地产品 Workflow 模型，并通过真实薄 macOS Desktop UI 暴露通用创作能力。用户可以创建 Workflow、添加 `human-chat(source)` 与 `llm-chat` Node Instance、绑定端口、配置用户级 LLM Provider/Model、点击 Run、提交 text，并查看持久化的 Conversation Artifact 与 Run 历史。UI 必须通过 Product Application seam 工作，不能直接访问 Engine、数据库或协议 Adapter，也不能退化为硬编码聊天页面。

每个 Workflow 拥有唯一可变 Draft。Autosave 只在规范化语义内容变化时更新同一 Draft 行，并使用内部 lock version 做乐观并发控制；它不创建历史副本。StartRun 携带 UI 当前看到的 expected lock version，在 preflight 中为未选择模型的 Agent Node 按 Provider/Model 双层 default 物化稳定 Gum Model UUID，然后创建或复用 immutable Revision、固定 Resolved LLM Selection 与 Run Snapshot，并创建新 Run。相同语义内容重复运行复用 Revision，但每次创建新 Run。

用户级模型设置采用 `Provider -> Models`。Provider 保存协议、Base URL 和 API Key 引用；Model 是一个拥有稳定 Gum Model UUID 的用户配置槽，Provider Model ID 可以编辑。Node 只保存 Gum Model UUID，不保存连接或 Secret。修改配置槽影响未来 Run，历史 Run Snapshot 保留实际使用值；删除槽后不 fallback，当前 Draft/Form/Preview 报错并阻止 StartRun，直到用户重新选择。

交付按四个纵向阶段推进：P9 用真实 macOS UI、SQLite 和通用创作 seam 配合 fake executor 形成 Product Tracer；P10 接入 Keychain、OpenAI-compatible 非流式 text 和正式 Conversation Artifact，形成首个真实产品闭环；P11 加固 migration、Interrupted、历史查询、诊断、脱敏和 macOS 安装升级；P12 升级 Human Chat Entry、WaitingHuman 与显式 `human-chat -> llm-chat -> human-chat` 多轮回边。

## User Stories

1. As a 本地技术用户, I want 在 macOS 应用中创建 Workflow, so that 我无需手写 YAML。
2. As a Workflow 作者, I want Workflow 保存在本地 SQLite, so that 产品定义不依赖项目目录中的配置文件。
3. As a Workflow 作者, I want 现有 YAML CLI 与产品 Workflow 相互隔离, so that 隐式导入不会制造身份或版本歧义。
4. As a Workflow 作者, I want 从 Node Catalog 添加 Node Instance, so that 创作体验是通用 Workflow 而不是硬编码聊天页面。
5. As a Workflow 作者, I want 同一 Node Definition 能被多次添加, so that Node identity 与 Definition identity 保持分离。
6. As a Workflow 作者, I want 为 Input 选择上游 Output, so that Data Edge 由真实 Artifact 绑定产生。
7. As a Workflow 作者, I want 单独配置 Control Dependency, so that 执行顺序不会冒充数据依赖。
8. As a Workflow 作者, I want 在只读结构 Preview 中看到 Node 与 Edge, so that 我能理解实际运行结构。
9. As a Workflow 作者, I want 自动布局不进入执行语义, so that 视觉变化不会创建 Revision 或改变运行。
10. As a Workflow 作者, I want 非法 Draft 仍能保存, so that 我可以逐步完成配置。
11. As a Workflow 作者, I want 非法 Draft 返回完整 Preview 与聚合 Diagnostics, so that 第一个错误不会遮蔽其他问题。
12. As a Workflow 作者, I want Diagnostic 指向具体 Node 和字段, so that 我能直接修复问题。
13. As a Workflow 作者, I want autosave 只在内容变化时写入, so that 空闲页面不会不断制造写入或伪版本。
14. As a Workflow 作者, I want autosave 更新唯一 Draft 而不创建 Revision, so that 编辑历史与可运行版本不会混淆。
15. As a Workflow 作者, I want 并发 token 不作为用户可见版本, so that 内部锁语义不会污染产品语言。
16. As a Workflow 作者, I want 冲突的 Draft 更新被拒绝并要求刷新, so that 旧页面不会覆盖较新的内容。
17. As a Workflow 作者, I want 首版只支持单窗口编辑, so that 产品不提前承担多人合并语义。
18. As a Workflow 作者, I want UI view preference 独立保存, so that 缩放、折叠和最近选择不改变语义 Draft。
19. As a Workflow 作者, I want 点击 Run 前自动 flush 已变化的 autosave, so that 运行内容与屏幕内容一致。
20. As a Workflow 作者, I want StartRun 校验 expected lock version, so that 我不会在不知情时运行其他 Draft 状态。
21. As a Workflow 作者, I want lock version 冲突时不创建 Revision 或 Run, so that 失败操作没有部分副作用。
22. As a Workflow 作者, I want 只有点击 Run 才形成 immutable Revision, so that Revision 代表真正运行过的定义边界。
23. As a Workflow 作者, I want 相同语义内容重复 Run 时复用 Revision, so that 数据库不会保存重复版本。
24. As a Workflow 作者, I want 每次点击 Run 都创建新 Run, so that 每次执行输入、输出和错误都可追溯。
25. As a Workflow 作者, I want Draft 修改不影响已经启动的 Run, so that 运行期间的行为不会漂移。
26. As a Workflow 作者, I want 从历史 Revision 恢复内容为新 Draft, so that immutable Revision 永远不被原地修改。
27. As a 模型用户, I want 在设置中创建多个 LLM Provider, so that 我可以连接不同公司网关或模型服务。
28. As a 模型用户, I want 每个 Provider 下配置多个 Model, so that 同一连接可以暴露多个可选模型。
29. As a 模型用户, I want Provider 和 Model 使用稳定 Gum UUID, so that 重命名和连接编辑不会破坏 Node 引用。
30. As a 模型用户, I want Model UUID 表示用户配置槽, so that 我能更新 Provider Model ID 而无需重新绑定所有 Workflow。
31. As a 模型用户, I want 历史 Run 显示当时实际 Provider Model ID, so that 配置槽后来变化不会改写历史。
32. As a 模型用户, I want 显式设置默认 Provider 和默认 Model, so that 新 Node 可以快速采用常用模型。
33. As a 模型用户, I want 未设置显式 default 时使用最早创建的未删除项, so that 默认解析简单且稳定。
34. As a 模型用户, I want Provider 和 Model 默认规则一致, so that 我不需要学习两套行为。
35. As a 模型用户, I want 首版不维护 enable/disable 状态, so that 模型设置不引入多余生命周期。
36. As a 模型用户, I want 删除 Provider/Model 前看到受影响 Workflow, so that 我能预见哪些 Node 会变红。
37. As a 模型用户, I want 删除 Model 后不自动 fallback, so that Workflow 不会静默改用价格或行为不同的模型。
38. As a Workflow 作者, I want 被删除的 Model UUID 在表单与 Preview 中飘红, so that 模型缺失立即可见。
39. As a Workflow 作者, I want 悬空 Model UUID 阻止 StartRun, so that 运行不会偷偷改变模型。
40. As a Workflow 作者, I want 重新选择模型后形成新的 Revision, so that 模型选择变化有明确版本边界。
41. As a Workflow 作者, I want 未选择模型时在首次 StartRun 采用双层 default, so that 我可以不逐个配置 Agent Node。
42. As a Workflow 作者, I want 首次采用 default 时把 Model UUID 写回 Draft, so that 后续 default 变化不会改变该 Node。
43. As a Workflow 作者, I want Model UUID 物化与 Revision/Run 创建原子完成, so that 失败不会留下半完成定义。
44. As a 安全敏感用户, I want API Key 保存在 macOS 安全凭据存储, so that SQLite、日志和 Artifact 不含明文密钥。
45. As a 安全敏感用户, I want Provider 只保存 Secret 引用, so that连接设置可以持久化而不泄露凭据。
46. As a 测试作者, I want 使用环境变量 Secret Adapter, so that 测试不依赖真实用户 Keychain。
47. As a 模型用户, I want 手工填写 Provider Model ID, so that 产品不依赖 `/models` discovery。
48. As a 模型用户, I want Gum 信任我的模型选择, so that 我不必维护一套模型 Capability 清单。
49. As a 模型用户, I want Provider 拒绝不支持的模态时看到真实错误, so that 我可以自行更换正确模型。
50. As a Workflow 作者, I want Node Config Schema 自动生成表单, so that Node 配置无需每个页面手写。
51. As a Workflow 作者, I want Config Contract 与 Presentation Hint 分离, so that UI 提示不会改变运行语义。
52. As a Node 作者, I want 使用小型 Gum Config Schema, so that领域接口不暴露 JSON Schema、CUE AST 或前端库结构。
53. As a 对话用户, I want `human-chat` 接收 text 并产生 Conversation, so that人工输入通过正式 Artifact 进入图。
54. As a 对话用户, I want `llm-chat` 通过真实 OpenAI-compatible 服务响应, so that Workflow 不再依赖 Mock Agent。
55. As a 对话用户, I want 首个闭环使用非流式 text, so that 最小真实路径不会被 SSE、多模态和文件处理阻塞。
56. As a 对话用户, I want 完整 Provider 响应成功后才产生 Conversation Artifact, so that 不完整输出不会成为正式数据。
57. As a 对话用户, I want Conversation 按 user/assistant 顺序持久化, so that 请求与结果可以检查。
58. As a 对话用户, I want system/developer instructions 来自 Node config, so that业务 Conversation 不混入隐藏指令消息。
59. As a 对话用户, I want Executor 不保存隐藏 session, so that多轮历史始终来自 Artifact。
60. As a 对话用户, I want 上下文超限时得到明确错误, so that Gum 不会静默删除历史。
61. As a 运维用户, I want 查看 usage、finish reason 和 Provider request ID, so that我能诊断真实模型调用。
62. As a 运维用户, I want 认证、网络、限流和协议错误终结 Run, so that结构性失败不会伪装为成功。
63. As a 运维用户, I want Provider 拒绝请求时看到 Provider 错误, so that模型选择问题由我修正。
64. As a 运维用户, I want Gum 不自动重试真实模型调用, so that费用和未知副作用不会被放大。
65. As a 运维用户, I want 应用重启后仍能查看已完成 Run, so that本地历史是真正持久化的。
66. As a 运维用户, I want 未结束 Run 在重启后标记 Interrupted, so that崩溃不会被记录成成功。
67. As a 运维用户, I want 首版 Interrupted 明确不可恢复, so that UI 不承诺尚未实现的 Resume。
68. As a 对话用户, I want P12 中 assistant 输出只让 Human Entry 等待, so that模型不会在无人输入时自行循环。
69. As a 对话用户, I want 每次人工提交创建新的 Human Node Run, so that每一轮对话都有独立身份。
70. As a 对话用户, I want 第二次人工提交才触发下一轮 `llm-chat`, so that多轮对话由人类事件驱动。
71. As a Workflow 作者, I want 显式 Conversation 回边出现在 Preview, so that对话历史路径不是 UI 隐藏状态。
72. As a Workflow 作者, I want Human Chat Entry 是唯一可自举人工入口, so that多轮图仍有明确启动点。
73. As a Workflow 作者, I want 所有 Input 继续遵守统一 dirty/Ready 规则, so that不需要 non-triggering context input 例外。
74. As a macOS 用户, I want 真实薄 Desktop UI 使用与 Browser Mock 相同的 Workflow Client, so that产品逻辑不会被桌面框架绑死。
75. As a macOS 用户, I want 安装、升级和数据库迁移可重复验证, so that本地产品不会因版本升级损坏数据。
76. As a 未来 Windows 用户, I want 当前架构不阻塞 Windows Adapter, so that后续待办无需重写领域模型。

## Implementation Decisions

- 产品 Workflow 的事实来源是本地 SQLite。现有 YAML CLI、workflow/v1 和 llm.yaml 保持历史作用域，不作为产品 Workflow 的兼容入口，也不隐式导入、物化或复用 Product Workflow/Revision。
- 建立 Product Application 模块作为 UI、Repository、Runtime 和协议之间的唯一产品编排 seam。Desktop UI 与 Browser Mock 通过相同 Workflow Client 调用它；UI 不直接调用 Engine、SQLite、Secret Store 或 Protocol Adapter。
- StartRun 接收 Workflow identity 和 expected Draft lock version。UI 必须先 flush 已变化的 autosave；token 冲突时返回最新 Draft/Diagnostics，且不得写 Draft、Revision 或 Run。
- Draft 是每个 Workflow 唯一可变当前态。Autosave 比较规范化语义内容；相同内容 no-op，不更新时间或 lock version；变化内容更新同一 Draft 行并递增内部 lock version。非法 Draft 可以持久化。
- Workflow Revision 只在 StartRun 边界创建。语义哈希相同则复用既有 Revision；每次 StartRun 成功都创建独立 Run。Workflow/Node 展示文本、Presentation Hint、时间戳和 UI view preference不进入语义哈希。
- Revision 哈希包含 schema version、Node Instance identity、Definition/Executor 选择、Node config、Input binding、dependsOn、Project binding与已物化 Gum Model UUID。无语义顺序的集合必须规范化。
- StartRun preflight 对空 Model preference 使用双层 default，先把 Gum Model UUID写回 Draft并递增 lock version，再创建/复用 Revision、固定 Run Snapshot并创建 Run。数据库写入不得留下部分 Draft/Revision/Run；真正的 Workflow Run 启动后不再回写定义。
- 用户级模型结构为 Provider -> Models。Provider 保存稳定 UUID、名称、协议、Base URL、API Key 引用、created time、可选显式 default和删除状态。
- Model 是用户配置槽，保存稳定 Gum Model UUID、所属 Provider、可编辑 Provider Model ID、展示名称、生成默认值、created time、可选显式 default和删除状态。UUID 不表示不可变底层模型；编辑 Provider Model ID 影响未来 Run，历史 Run Snapshot 保留旧值。
- 默认解析只考虑未删除数据。每层最多一个显式 default；没有显式 default 时按 created time 升序、UUID 升序取第一个。首版没有 position/reorder 和 enable/disable。
- Node 的 LLM Preference 只保存 Gum Model UUID。UUID 存在时 Provider/Model default 变化不影响 Node；UUID 被删除或缺失时不 fallback，表单和 Preview 报错，StartRun 不创建 Run。
- 删除 Provider/Model 前查询并提示受影响 Workflow，但不改写其 Draft或Revision。历史 Run 通过 Run Snapshot 继续显示当时 Provider、Provider Model ID和有效参数。
- API Key 明文只进入 macOS安全凭据存储；SQLite保存 Secret 引用。Revision、Run Snapshot、运行观测、日志、Artifact和诊断不得含明文 Secret。
- 当前范围不实现 `/models` discovery，不保存 raw discovery，不做 manual overlay、last-seen或refresh。
- 当前范围不探测、推断、声明或匹配 Model Capability。Agent Node 使用 `requires: llm` 声明资源需求；Node/Artifact Contract判断输入输出是否合法。用户选择 Model即确认适合任务，Provider拒绝请求时记录并展示 Structural/Provider Error。
- Config Schema 使用 Gum自有小型领域模型。首版字段类型为 string、markdown、integer、number、boolean、enum；Contract包含 required、default、min/max、enum values和sensitive；Presentation Hint包含 label、help和editor。Semantic Validator与UI表单消费同一 Schema。
- Workflow Preview从Draft或Revision派生，包含Node、Data/Control Edge、循环组和聚合Diagnostics。坐标由前端自动计算；非法Draft仍返回完整图。P9只需覆盖当前两节点 tracer所需结构，P12增加显式循环验证。
- 首个真实协议是OpenAI-compatible Chat Completions。Canonical Conversation、ChatMessage、ContentPart和GenerateRequest不泄漏Provider JSON字段。P10只实现非流式text；响应成功后一次性产生正式Conversation Artifact。
- Protocol Adapter通过可注入HTTP Transport工作，测试使用本地fixture server。首个非流式接口不强制StreamSink；未来Streaming使用独立可选seam。
- OpenAI-compatible instructions由Provider dialect设置决定developer或system映射。请求按顺序发送user/assistant消息，响应规范化assistant text、usage、finish reason和Provider request ID。
- `human-chat(source)`与`llm-chat`构成P10单轮Workflow。`human-chat`产生以user消息结尾的Conversation，`llm-chat`追加恰好一个assistant text消息并输出完整新Conversation。
- P12把入口升级为Human Chat Entry：允许optional Conversation feedback且无必需输入时自举；Human Executor接收上一版Conversation；feedback只创建WaitingHuman Node Run；人工提交后才产生新Conversation并触发`llm-chat`。
- P12显式升级产品Validator的入口规则，但不得改变workflow/v1的唯一无inputs human-input历史规则。
- 首闭环持久化Workflow、Draft、Revision、Run Snapshot、Node Run、正式Artifact和错误。运行中UI使用暂态observation signal，不建立append-only Run Event Log或replay。
- 应用启动时将未完成Run标为Interrupted并允许查询，但P9–P12不提供Resume。已完成Run、Node Run、Conversation Artifact与错误在重启后仍可查看。
- Structural Error包括认证、网络、限流、服务不可用、协议损坏、Provider拒绝请求和无法完成Artifact合同的底层错误。只有Provider已成功返回但业务输出违反Node Contract时才是Interaction Error。首版不增加自动Retry。
- P9为macOS Product Tracer：真实薄UI、SQLite、通用Draft/Config/Preview、Provider/Model设置、两节点创作与fake executor Run。它不是首个真实LLM产品验收。
- P10为首个真实产品闭环：macOS Keychain、OpenAI-compatible非流式text、Model UUID物化、真实Conversation Artifact和历史。
- P11加固schema migration、Interrupted、重启查询、删除模型诊断、错误展示、日志脱敏、Crash bundle以及macOS构建/安装/升级。
- P12交付Human Chat Entry、WaitingHuman、显式Conversation回边与两轮e2e。

## Testing Decisions

- 主要验收 seam 是 Workflow Application 经同一 Workflow Client 驱动 Browser Mock 与真实 macOS Desktop Adapter。好的验收测试只观察用户可见行为：创建、autosave、Preview、StartRun、输入、结果、错误与历史，不断言内部表结构或私有调度函数。
- P9 在主要 seam 上用 fake executor完成完整 tracer。测试必须证明UI不是硬编码聊天页：Node来自Catalog，Input binding形成Data Edge，Config表单来自Config Schema，StartRun经过Product Application。
- P10 在相同 seam 上替换为真实OpenAI-compatible Adapter与本地fixture server。测试从UI操作到Conversation Artifact，断言请求message顺序、instructions映射、完整响应后才产生Artifact，以及API Key不出现在数据库、日志、诊断或测试快照。
- Product Repository保留窄契约测试，覆盖migration、旧schema fixture升级、Draft lock version CAS、相同autosave no-op、非法Draft持久化、Revision hash去重、每次Run新身份、删除Provider/Model影响查询和重启后历史。
- StartRun使用Application级事务行为测试：expected lock version冲突零写入；缺少default零写入；空preference成功物化UUID；悬空UUID阻止Run；Revision相同则复用；每次成功StartRun创建新Run；任一步失败无部分状态。
- Provider/Model resolver使用表驱动公共行为测试：显式default、created-time fallback、UUID tie-break、Provider与Model双层解析、删除default后的下一项、没有候选时诊断、Model UUID存在时不受default变化、Provider Model ID编辑影响未来Snapshot。
- Protocol Adapter使用本地HTTP fixture和golden请求测试，不访问真实网络。P10覆盖OpenAI-compatible非流式单轮text、developer/system dialect、usage、finish reason、request ID、认证、限流、malformed response、取消与Base URL边界。
- Engine.Run继续作为调度主seam，沿用注入fakeExecutor、HumanGateway和RunRecorder的既有先例。P12只通过公共Engine行为验证Human Entry自举、WaitingHuman、每次提交新Node Run、回边dirty/Ready和Convergence Guard，不测试私有queue实现。
- History/Recorder测试沿用“一轮一行、最新轮摘要、ArtifactRef可查询、Run UUID唯一身份”的既有先例，新增Revision/Run Snapshot、Resolved LLM Selection与Conversation Artifact查询行为。
- Validation测试沿用聚合Diagnostics、具体Node/字段定位、完整Preview和循环warning先例。产品Validator与workflow/v1 Validator分别测试，禁止用新入口语义改写历史fixture。
- Desktop测试使用真实macOS Adapter验证Application调用、暂态observation signal、Keychain seam、窗口关闭和本地路径生命周期。Browser Mock承担大部分交互回归；只在桌面Adapter边界保留少量集成测试。
- P11使用进程重启级测试验证未完成Run转Interrupted、已完成历史可查询、迁移失败不发布半状态和Secret/Artifact路径保持Local Data Root边界。
- 所有单元、集成和e2e禁止真实LLM网络、真实用户HOME和真实用户Keychain；使用临时数据库、临时Local Data Root、fixture server和注入Secret Adapter。
- 测试不得把Provider拒绝image/tools等请求重新实现为Gum Capability探测；只断言错误被忠实记录并归入已决定的Structural/Provider Error。
- 每个P9–P12票使用红→绿垂直切片：先写最高可见失败，再补必要窄契约测试；避免为尚未进入阶段的Streaming、Anthropic、image或Windows创建占位实现。

## Out of Scope

- YAML导入/导出、YAML CLI与SQLite Product Workflow兼容、隐式身份映射。
- `/models` discovery、raw discovery、manual overlay、模型Capability目录、名称推断和自动Provider failover。
- Streaming、SSE、Content Delta持久化、stream取消与续传。
- Anthropic Messages Adapter、OpenAI Responses、Realtime、工具调用、MCP和通用Agent Tool Loop。
- image/audio/file Content Part、文件选择、多模态Capability校验。
- Windows构建、安装和e2e；架构不得阻塞未来Windows Adapter。
- append-only Run Event Log、event replay、Pause/Resume、Interrupted恢复、Rerun、Fork、Manual Artifact和UnknownOutcome重试策略。
- 高级Artifact Previewer、Source diff、JSON/OpenAPI专用视图、Test Report、外部资源摘要、版本比较和人工替换。
- Workflow导入/导出、Workflow Pack、内置Workflow库、Marketplace和AI创建/修改Workflow。
- 云同步、账号、多设备、多人协作、字段级merge和多窗口并发编辑。
- 定时、文件变化、Webhook等非人工Trigger。
- 完整Secret Management、企业权限、安全sandbox和远程执行。
- Coding Agent、Workspace代码修改、真实OpenAPI Generator、Skipped传播、workflow/v2 retry/timeout字段。
- Provider/Model enable/disable、用户排序position；首版只支持显式default与created-time fallback。
- 对生成式模型输出作确定性承诺、自动重试或自动恢复外部副作用。

## Further Notes

- 本spec的产品语义显式升级14后模型，不修改已完成platform-core 01–14或workflow/v1验收。
- Model UUID是配置槽身份而非不可变底层模型身份；这是有意选择，历史准确性由Run Snapshot承担。
- StartRun preflight写Draft发生在Workflow Run创建之前，因此不违反“运行中的WorkflowExecution不回写定义”。
- Product Workflow只依赖SQLite；现有llm.yaml和YAML CLI继续服务历史workflow/v1，不能成为P9–P12实现捷径。
- P9–P12完成不等于产品v1确定。进入导入/导出、Windows、Streaming或恢复设计前必须重新评审范围。
- 所有实现票必须使用ready-for-agent/ready-for-human等本地triage词汇，并遵循Repository的开发、测试和文档同步纪律。
