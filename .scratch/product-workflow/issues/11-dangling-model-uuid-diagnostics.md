# 11: 删除 Model 后的悬空 UUID 诊断

**What to build:** 让用户安全删除 Provider/Model，同时不改写任何 Workflow。删除前展示受影响 Workflow；删除后相关 Node 保留原 Gum Model UUID、表单与 Preview 飘红，并在用户重新选择前阻止 StartRun。

**Blocked by:** 10: 真实 OpenAI-compatible 单轮闭环

**Status:** ready-for-agent

- [ ] 删除 Model 前 UI 展示引用该 Gum Model UUID 的当前 Workflow/Draft 数量与身份。
- [ ] 删除 Provider 前 UI 展示其全部 Model Slot 及受影响 Workflow。
- [ ] 用户确认删除后，不修改任何 Draft、Revision 或历史 Run。
- [ ] 悬空 Model UUID 在 Node 表单和 Preview 中产生具体字段 Diagnostic。
- [ ] 悬空 UUID 不 fallback 到 default，StartRun 在创建 Run 前失败。
- [ ] 用户选择新 Model UUID 后 Draft 正常保存，并在下次 Run 形成新的 Revision。
- [ ] 历史 Run Snapshot 仍展示已删除 Slot 当时的 Provider 名称和 Provider Model ID。

