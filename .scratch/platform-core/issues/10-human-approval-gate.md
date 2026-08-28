# 10: human-approval 审批节点与 approve 门控

**What to build:** 人工在环的第二半（设计文档 §6.5、§7.2）：内置 human-approval Node Definition（无 inputs、dependsOn 必须非空、输出 `approve: bool` + `advise: markdown`）与执行器；approve 门控四象限（拒绝轮：输出正常更新、已消费旧版的数据下游 dirty 重跑、未跑下游正常供给、control 下游不解禁；通过轮：输出正常更新、已消费下游**不** dirty、未跑下游正常供给、control 下游解禁）；WaitingHuman 状态进入状态机与 state.json；审批交互经 gateway 的 approval 请求（打印已产出 artifacts 摘要与历史 advise 后 A/r + 同行 advise，回车默认 Approve）；审批决策是真实人类事件重置收敛计数。coding-agent 种子契约补 optional `advise: markdown` 输入（声明式 advise 回环 `inputs.advise: {from: review.advise}` 生效）。完整验收场景：reject(advise) -> backend/frontend 重跑 -> 再审 -> approve -> 全图静止且无多余轮次。

**Blocked by:** 08（触发规则与 dirty 机制）, 09（gateway 与人类事件管道）

**Status:** done

- [x] human-approval 定义 + 执行器 + 种子；dependsOn 必填校验（fixture）
- [x] approve 门控四象限全部在主接缝断言（fake gateway 驱动）
- [x] WaitingHuman 状态流转（Ready -> WaitingHuman -> Running -> Succeeded）合法性与 state.json
- [x] 审批循环收敛：两轮拒绝两轮重跑后通过，静止且无多余轮次（设计文档 §14 场景 1）
- [x] coding-agent 契约补 advise optional 输入；advise 数据边回环生效（重跑轮 inputs 快照含 advise）
- [x] 审批人视角展示：artifacts 摘要 + 历史 advise（gateway stdin 实现单测覆盖文案）

## Comments

**2026-08-28（agent 实施记录）：** 内置 `human-approval/v1`、`WaitingHuman` 状态、stdin 审批交互与 approve 门控已落地。主接缝覆盖 Reject 后已消费下游 dirty、Approve 后已消费下游不重跑、两种决策下未运行数据下游正常首跑，以及 control 下游仅在通过后解禁。

审批循环测试以 fake gateway 驱动两轮拒绝、backend/frontend 两轮返工、第三轮通过；断言 advise 版本进入重跑输入快照、审批决策重置收敛计数、最终静止且无多余轮次。gateway 单测覆盖 artifact 摘要、历史 advise、回车默认 Approve、同行及下一行 Reject advise。

双轴代码审查：Spec 轴无发现；Standards 轴发现引擎曾通过读取 Artifact 本体恢复审批决定并吞掉读取错误。已改为以 gateway 决策保存显式运行期审批元数据，门控不再读取 Artifact 本体，并补 `StatusWaitingHuman` 导出注释。

验证：`go test ./...`、`go test -race ./internal/execution ./cmd/workflow`、`go vet ./...` 全绿。
