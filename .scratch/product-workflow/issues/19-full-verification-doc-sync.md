# 19: P9–P12 全量验证与状态文档同步

**What to build:** 对完整 P9–P12 产品闭环执行最终集成验证、Standards/Spec 双轴审查和准确文档同步，让维护者能够区分已实现能力、明确待办与后置设计。

**Blocked by:** 18: 显式多轮对话 UI 闭环

**Status:** ready-for-agent

- [ ] Go build、完整测试、vet 和 race 验证全部通过；前端/Desktop 测试与 macOS e2e 同时通过。
- [ ] Product Application、Protocol Adapter、Repository、Engine 和 Desktop Adapter 的公共 seam 均有常绿外部行为测试。
- [ ] Standards review 确认实现符合开发规范、依赖方向、错误风格和 Secret 边界。
- [ ] Spec review 逐项核对本 feature spec、P9–P12 tickets 和 Model Slot ADR。
- [ ] README、开发规范、领域文档和 Agent 入口准确标记 P9–P12 已实现状态。
- [ ] Streaming、Anthropic、image、Windows、高级 Artifact 与恢复能力明确保留为未实现待办，不被写成当前能力。
- [ ] workflow/v1、YAML CLI、platform-core 和 code-quality-automation 的既有行为与测试没有回归。
- [ ] 未经用户明确授权不提交到 main；最终验收记录包含验证命令与结果。
