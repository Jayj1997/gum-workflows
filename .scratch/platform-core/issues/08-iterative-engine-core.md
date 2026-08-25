# 08: 迭代引擎核心：触发规则、Node Run 轮次、版本化与收敛保护

**What to build:** 引擎从「无环 DAG、每节点跑一次」升级为迭代模型（设计文档 §5–§6，除 human 在环外全部）：Ready = 数据输入有已完成轮产出 AND control 前驱完成过至少一轮；重新 Ready = 任一数据输入有未消费新版本（dirty）OR control 前驱有新完成轮；输入取生产者最新已完成轮；轮次合并；节点单并发（新输入排队）；每次执行 = Node Run（UUID + round，NodeExecution 变 Current+History 双层）；artifact 多版本共存（Version = 该输出第 N 次产出）；收敛保护（自上次人类事件起机器触发连续重跑超阈值 -> 节点 Failed 运行 Failed，默认 10、Option 可调）；运行级新增 Stopped（ctx 取消记 Stopped 而非 Failed）；human-free 图全图静止时以 Running 状态阻塞在等待点（本票先等 ctx 取消）返回。**本票不含 human 节点**：Gateway 接口先以「无 human 节点」的空实现满足接口位。

主接缝 Engine.Run 上用 fake factory 驱动全部行为断言（spec Testing Decisions）。

**Blocked by:** 03（新注册表/契约来源）, 05（新 NodeSpec 形态）

**Status:** ready-for-agent

- [ ] 触发规则：新输入即重跑、级联重跑全下游、轮次合并（多输入版本变化一轮消化）、单并发（重跑排队不打断当前轮）
- [ ] NodeExecution 双层形态 + state.json 扩展（当前轮+历史摘要；每轮明细 runs/<round>.json）
- [ ] from 取最新已完成轮；同输出多版本 artifact 共存不删
- [ ] 收敛保护：机器环（小阈值 Option）第 N+1 轮掐断，error=convergence-guard，运行 Failed；模拟人类事件清零（人类事件本票以可注入 hook 模拟，T09/T10 换成真实 gateway 事件）
- [ ] Stopped：ctx 取消 -> 运行 Stopped；waiting 语义：全图静止且无 Ready/Running 时阻塞等 ctx 而非退出
- [ ] 既有引擎测试迁移绿 + 设计文档 §14 场景 2/5/6/7（机器环、失败语义保留部分、多轮级联、版本固定）在无 human 前提下的等价形态
