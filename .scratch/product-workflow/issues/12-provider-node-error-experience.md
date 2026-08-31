# 12: Provider 与 Node 错误体验

**What to build:** 让用户从 UI 和 History 中准确理解真实模型调用为什么没有成功。Run 前定义/模型错误不创建 Run；Run 后 Provider 与底层错误是 Structural Error；只有成功响应违反 Node Contract 才是 Interaction Error。

**Blocked by:** 10: 真实 OpenAI-compatible 单轮闭环

**Status:** ready-for-agent

- [ ] Run 前 Draft、端口、Model UUID、default 和 Secret 错误定位到具体 Node/字段，且不创建 Run。
- [ ] 认证、网络、限流、服务不可用、协议损坏和 Provider 拒绝请求均记录为 Structural Error 并使 Run Failed。
- [ ] Provider 已成功返回但输出违反 Node Contract 时记录为 Interaction Error，Run 保持既有交互性语义。
- [ ] 取消或进程中断导致结果不确定时显示 UnknownOutcome，不伪装成功。
- [ ] UI 展示脱敏 Provider 错误、Node Run identity、时间和可采取的用户动作。
- [ ] 首版不自动重试、切换 Provider 或 fallback Model。

