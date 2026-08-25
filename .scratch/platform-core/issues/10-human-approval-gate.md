# 10: human-approval 审批节点与 approve 门控

**What to build:** 人工在环的第二半（设计文档 §6.5、§7.2）：内置 human-approval Node Definition（无 inputs、dependsOn 必须非空、输出 `approve: bool` + `advise: markdown`）与执行器；approve 门控四象限（拒绝轮：输出正常更新、已消费旧版的数据下游 dirty 重跑、未跑下游正常供给、control 下游不解禁；通过轮：输出正常更新、已消费下游**不** dirty、未跑下游正常供给、control 下游解禁）；WaitingHuman 状态进入状态机与 state.json；审批交互经 gateway 的 approval 请求（打印已产出 artifacts 摘要与历史 advise 后 A/r + 同行 advise，回车默认 Approve）；审批决策是真实人类事件重置收敛计数。coding-agent 种子契约补 optional `advise: markdown` 输入（声明式 advise 回环 `inputs.advise: {from: review.advise}` 生效）。完整验收场景：reject(advise) -> backend/frontend 重跑 -> 再审 -> approve -> 全图静止且无多余轮次。

**Blocked by:** 08（触发规则与 dirty 机制）, 09（gateway 与人类事件管道）

**Status:** ready-for-agent

- [ ] human-approval 定义 + 执行器 + 种子；dependsOn 必填校验（fixture）
- [ ] approve 门控四象限全部在主接缝断言（fake gateway 驱动）
- [ ] WaitingHuman 状态流转（Ready -> WaitingHuman -> Running -> Succeeded）合法性与 state.json
- [ ] 审批循环收敛：两轮拒绝两轮重跑后通过，静止且无多余轮次（设计文档 §14 场景 1）
- [ ] coding-agent 契约补 advise optional 输入；advise 数据边回环生效（重跑轮 inputs 快照含 advise）
- [ ] 审批人视角展示：artifacts 摘要 + 历史 advise（gateway stdin 实现单测覆盖文案）
