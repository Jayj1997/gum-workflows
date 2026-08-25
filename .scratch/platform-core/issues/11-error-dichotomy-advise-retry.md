# 11: 错误二分法与 advise 重试

**What to build:** 错误的结构性/交互性二分与恢复路径（设计文档 §6.6）：`Structural(err)` / `Interaction(err)` 包装与 `ErrorKindOf(err)`（缺省按结构性--automation/human 的任何错误、agent 的网络错误都是结构性）；结构性错误沿用 fail-fast（节点 Failed -> 运行 Failed，在途轮完成不强杀）；交互性错误节点 Failed 但运行保持 Running（失败节点下游保持 Pending 不传播），其余分支照常推进；即时 advise 重试：CLI 提示输入 advise（空行跳过），非空输入经引擎注入该节点（优先级：即时 advise > 数据边 advise 最新版本），节点以新 Node Run 重跑复活；仅声明了 advise 输入的 agent 节点可被拯救，未声明者的交互性错误等价结构性；advise 重试是真实人类事件重置收敛计数。

**Blocked by:** 10（gateway 与 advise 数据边语义就位）

**Status:** ready-for-agent

- [ ] 错误包装/解包/ErrorKindOf + 缺省结构性语义（单测）
- [ ] 结构性：运行 Failed 进程返回，在途轮完成不强杀（设计文档 §14 场景 5）
- [ ] 交互性：节点 Failed、运行 Running、下游 Pending、其他分支推进（fake factory 注错驱动）
- [ ] advise 重试：注入优先级正确、新 Node Run 复活、inputs 快照标记 `#advise-retry`（设计文档 §14 场景 4）
- [ ] 未声明 advise 输入的 agent 节点交互性错误 -> 等价结构性（运行 Failed）
- [ ] 「agent 类 definition 未声明 advise 输入」的校验提示级 warning
