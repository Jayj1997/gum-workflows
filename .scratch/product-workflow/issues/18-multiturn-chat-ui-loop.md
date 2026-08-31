# 18: 显式多轮对话 UI 闭环

**What to build:** 完成 P12：用户在通用 UI 中创建 `human-chat -> llm-chat -> human-chat` 显式循环。assistant 输出只使 Human Entry 等待，第二次人工提交才触发下一轮模型调用，Conversation 历史始终通过 Artifact 传递。

**Blocked by:** 17: Human Chat Entry execution seam

**Status:** ready-for-agent

- [ ] UI 可以把 `llm-chat` Conversation output 回接 Human Chat Entry optional input。
- [ ] Preview 将回边显示为循环组，并保持坐标不属于执行语义。
- [ ] 第一轮人工 text、assistant text 和完整 Conversation Artifact 按顺序可见。
- [ ] assistant 输出后 Human Entry 显示 WaitingHuman，模型不会自行再次运行。
- [ ] 第二次人工提交创建新的 Human Node Run，并触发新的 `llm-chat` Node Run。
- [ ] 每轮 Conversation、Node Run round、input/output Artifact version 和 Run history 可追溯。
- [ ] 两轮 e2e 证明没有隐藏 session、non-triggering context input 或机器自循环。

