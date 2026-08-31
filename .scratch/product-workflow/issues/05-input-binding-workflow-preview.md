# 05: Input binding 与只读 Workflow Preview

**What to build:** 让用户为 Node Input 选择上游 Output，并在只读自动布局 Preview 中理解实际 Data/Control Edge。Preview 在 Draft 未完成时仍显示整张图和聚合 Diagnostics。

**Blocked by:** 04: Node Catalog 与通用 Config Schema 表单

**Status:** ready-for-agent

- [ ] 用户可以把 `human-chat` 的 Conversation output 绑定到 `llm-chat` input，并形成 Data Edge。
- [ ] Control Dependency 与 Data Edge 使用不同的创作控件和 Preview 表达。
- [ ] 未绑定 Input、未知端口和类型不兼容均产生具体 Node/字段 Diagnostic。
- [ ] Preview 在非法 Draft 下仍返回全部可识别 Node、Edge 和 Diagnostics。
- [ ] 自动布局、缩放、折叠和最近选择不进入 Draft 语义哈希。
- [ ] Preview ViewModel 不暴露具体渲染或布局库结构。

